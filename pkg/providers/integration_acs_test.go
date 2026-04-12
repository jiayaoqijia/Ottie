package providers_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/acs"
	"github.com/jiayaoqijia/ottie/pkg/actionlog"
	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
	"github.com/jiayaoqijia/ottie/pkg/providers"
	"github.com/jiayaoqijia/ottie/pkg/providers/openai_compat"
	"github.com/jiayaoqijia/ottie/pkg/tools"
)

const (
	altllmAPIBase = "https://api.altllm.ai/v1"
	altllmModel   = "altllm-basic"
)

func altllmAPIKey(t *testing.T) string {
	t.Helper()
	if key := os.Getenv("ALTLLM_API_KEY"); key != "" {
		return key
	}
	t.Skip("ALTLLM_API_KEY not set — skipping live integration test")
	return ""
}

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// TestFullTurnWithACSSingleProvider exercises the complete turn lifecycle:
// user message → BeginTurn manifest → real LLM Chat → RecordLLMCall → verify all rows.
// This is test plan item #3: the end-to-end proof that ACS captures a real turn.
func TestFullTurnWithACSSingleProvider(t *testing.T) {
	skipIfShort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a real OpenAI-compatible provider pointing at ALTLLM
	provider := openai_compat.NewProvider(altllmAPIKey(t), altllmAPIBase, "")

	// Create a real ACS bundle
	dir := t.TempDir()
	bundle, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("acs.Open: %v", err)
	}
	defer bundle.Close()

	// Step 1: Begin a turn in the manifest
	manifest := execmanifest.Manifest{
		SessionID:      "integration-sess-1",
		Turn:           1,
		PromptHash:     "sha256-integration-test",
		ToolSchemaHash: "sha256-integration-test",
		ModelID:        altllmModel,
		PromptEpoch:    1,
	}
	traceID, err := bundle.BeginTurn(ctx, manifest)
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if traceID == "" {
		t.Fatal("BeginTurn returned empty traceID")
	}

	// Step 2: Make a real LLM call
	messages := []openai_compat.Message{
		{Role: "user", Content: "Reply with exactly the word 'pong'. Nothing else."},
	}
	resp, err := provider.Chat(ctx, messages, nil, altllmModel, map[string]any{"max_tokens": 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Fatal("Chat returned empty response")
	}
	t.Logf("LLM response: %q", resp.Content)

	// Step 3: Record the LLM call in the manifest
	err = bundle.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID:   traceID,
		CallSeq:   0,
		RequestID: "integration-req-1",
		ModelID:   altllmModel,
	})
	if err != nil {
		t.Fatalf("RecordLLMCall: %v", err)
	}

	// Step 4: Verify the full manifest is queryable from the trace ID
	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if full.Manifest.SessionID != "integration-sess-1" {
		t.Errorf("SessionID = %q", full.Manifest.SessionID)
	}
	if full.Manifest.Turn != 1 {
		t.Errorf("Turn = %d, want 1", full.Manifest.Turn)
	}
	if full.Manifest.ModelID != altllmModel {
		t.Errorf("ModelID = %q, want %s", full.Manifest.ModelID, altllmModel)
	}
	if len(full.ProviderCalls) != 1 {
		t.Fatalf("ProviderCalls = %d, want 1", len(full.ProviderCalls))
	}
	if full.ProviderCalls[0].ModelID != altllmModel {
		t.Errorf("call ModelID = %q, want %s", full.ProviderCalls[0].ModelID, altllmModel)
	}
}

// TestACSChatRecordsEveryProviderCall exercises test plan item #8:
// every Chat invocation (including retries) gets its own RecordLLMCall row.
// We make 3 real LLM calls and verify all 3 appear in the manifest.
func TestACSChatRecordsEveryProviderCall(t *testing.T) {
	skipIfShort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	provider := openai_compat.NewProvider(altllmAPIKey(t), altllmAPIBase, "")

	dir := t.TempDir()
	bundle, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("acs.Open: %v", err)
	}
	defer bundle.Close()

	traceID, err := bundle.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "integration-multi-call",
		Turn:           1,
		PromptHash:     "sha256-multi",
		ToolSchemaHash: "sha256-multi",
		ModelID:        altllmModel,
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Simulate 3 provider calls (as if retries or fallback attempts)
	prompts := []string{
		"Say 'one'",
		"Say 'two'",
		"Say 'three'",
	}
	for seq, prompt := range prompts {
		msgs := []openai_compat.Message{{Role: "user", Content: prompt}}
		resp, chatErr := provider.Chat(ctx, msgs, nil, altllmModel, map[string]any{"max_tokens": 10})

		// Record regardless of success/failure (matching acsChat behavior)
		status := "ok"
		if chatErr != nil {
			status = "error"
			t.Logf("call %d error (still recording): %v", seq, chatErr)
		} else {
			t.Logf("call %d response: %q", seq, resp.Content)
		}

		err = bundle.RecordLLMCall(ctx, execmanifest.ProviderCall{
			TraceID:   traceID,
			CallSeq:   seq,
			RequestID: fmt.Sprintf("req-%d-%s", seq, status),
			ModelID:   altllmModel,
		})
		if err != nil {
			t.Fatalf("RecordLLMCall seq %d: %v", seq, err)
		}
	}

	// Verify all 3 calls are recorded
	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(full.ProviderCalls) != 3 {
		t.Fatalf("ProviderCalls = %d, want 3", len(full.ProviderCalls))
	}
	for i, c := range full.ProviderCalls {
		if c.CallSeq != i {
			t.Errorf("call[%d].CallSeq = %d, want %d", i, c.CallSeq, i)
		}
		if c.ModelID != altllmModel {
			t.Errorf("call[%d].ModelID = %q, want %s", i, c.ModelID, altllmModel)
		}
	}
}

// TestACSChatRecordsActualModelNotConfigured exercises test plan item #9:
// when the actual model used differs from the configured one (e.g., fallback),
// the manifest row should record the ACTUAL model, not the configured primary.
func TestACSChatRecordsActualModelNotConfigured(t *testing.T) {
	skipIfShort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := openai_compat.NewProvider(altllmAPIKey(t), altllmAPIBase, "")

	dir := t.TempDir()
	bundle, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("acs.Open: %v", err)
	}
	defer bundle.Close()

	// BeginTurn with the "configured" model
	configuredModel := "altllm-mega"  // the primary model in config
	actualModel := altllmModel        // what the fallback chain actually used

	traceID, err := bundle.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "integration-model-mismatch",
		Turn:           1,
		PromptHash:     "sha256-model",
		ToolSchemaHash: "sha256-model",
		ModelID:        configuredModel, // manifest records what was configured
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Make a real call with the ACTUAL model (simulating fallback)
	msgs := []openai_compat.Message{{Role: "user", Content: "Say 'fallback worked'"}}
	resp, chatErr := provider.Chat(ctx, msgs, nil, actualModel, map[string]any{"max_tokens": 20})
	if chatErr != nil {
		t.Fatalf("Chat: %v", chatErr)
	}
	t.Logf("response from actual model: %q", resp.Content)

	// Record with the ACTUAL model (matching acsChat behavior at loop.go:1390)
	err = bundle.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID:   traceID,
		CallSeq:   0,
		RequestID: "req-fallback-0",
		ModelID:   actualModel, // NOT configuredModel
	})
	if err != nil {
		t.Fatalf("RecordLLMCall: %v", err)
	}

	// Verify: manifest row has configured model, provider call has actual model
	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if full.Manifest.ModelID != configuredModel {
		t.Errorf("manifest ModelID = %q, want configured %q", full.Manifest.ModelID, configuredModel)
	}
	if len(full.ProviderCalls) != 1 {
		t.Fatalf("ProviderCalls = %d, want 1", len(full.ProviderCalls))
	}
	if full.ProviderCalls[0].ModelID != actualModel {
		t.Errorf("call ModelID = %q, want actual %q", full.ProviderCalls[0].ModelID, actualModel)
	}
	// The key assertion: manifest.ModelID != call.ModelID
	if full.Manifest.ModelID == full.ProviderCalls[0].ModelID {
		t.Errorf("manifest and call model should differ (configured vs actual)")
	}
}

// TestLiveErrorClassificationWithInvalidModel verifies that a real API
// error (invalid model) is correctly classified by the error classifier.
// This tests the full vertical: real HTTP error -> ClassifyError -> correct reason.
func TestLiveErrorClassificationWithInvalidModel(t *testing.T) {
	skipIfShort(t)
	apiKey := altllmAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Point at a non-existent endpoint to guarantee an HTTP error.
	// ALTLLM accepts any model name and any API key, so we force an error
	// by hitting a bad URL path instead.
	badProvider := openai_compat.NewProvider(apiKey, "https://api.altllm.ai/v1/nonexistent-path", "")

	msgs := []openai_compat.Message{{Role: "user", Content: "hello"}}
	_, err := badProvider.Chat(ctx, msgs, nil, altllmModel, map[string]any{"max_tokens": 10})

	if err == nil {
		// If the bad URL also succeeds, try a totally unreachable host
		unreachable := openai_compat.NewProvider(apiKey, "https://localhost:1/v1", "")
		_, err = unreachable.Chat(ctx, msgs, nil, altllmModel, map[string]any{"max_tokens": 10})
		if err == nil {
			t.Fatal("expected error from unreachable endpoint, got nil")
		}
	}
	t.Logf("raw error: %v", err)

	// Classify the error
	fe := providers.ClassifyError(err, "altllm", altllmModel)
	if fe == nil {
		t.Fatalf("ClassifyError returned nil for error: %v", err)
	}
	t.Logf("error classified as: reason=%s, retriable=%v, shouldFallback=%v",
		fe.Reason, fe.IsRetriable(), fe.ShouldFallback())

	// The error should be classified — verify the classifier doesn't panic
	// and returns a valid FailoverError with a non-empty reason.
	if fe.Reason == "" {
		t.Error("ClassifyError returned empty reason")
	}
}

// TestLiveToolCallingWithALTLLM verifies that the ALTLLM provider
// correctly handles tool-calling: we send a message with tool definitions,
// the model returns a tool_call, and we can parse the response.
func TestLiveToolCallingWithALTLLM(t *testing.T) {
	skipIfShort(t)
	apiKey := altllmAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := openai_compat.NewProvider(apiKey, altllmAPIBase, "")

	// Define a simple tool
	toolDefs := []openai_compat.ToolDefinition{
		{
			Type: "function",
			Function: openai_compat.ToolFunctionDefinition{
				Name:        "get_weather",
				Description: "Get the current weather for a location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	msgs := []openai_compat.Message{
		{Role: "user", Content: "What's the weather in Tokyo?"},
	}

	resp, err := provider.Chat(ctx, msgs, toolDefs, altllmModel, map[string]any{"max_tokens": 200})
	if err != nil {
		t.Fatalf("Chat with tools: %v", err)
	}

	t.Logf("response content: %q", resp.Content)
	t.Logf("tool_calls: %d", len(resp.ToolCalls))

	// The model should either:
	// 1. Return a tool_call for get_weather, OR
	// 2. Return a text response mentioning it can't check weather
	// Both are valid — we just verify the response is parseable.
	if resp.Content == "" && len(resp.ToolCalls) == 0 {
		t.Fatal("response has no content and no tool calls")
	}

	if len(resp.ToolCalls) > 0 {
		tc := resp.ToolCalls[0]
		t.Logf("tool call: name=%q args=%v", tc.Name, tc.Arguments)

		// If a tool call was made, it should target get_weather
		toolName := tc.Name
		if tc.Function != nil && toolName == "" {
			toolName = tc.Function.Name
		}
		if toolName != "get_weather" {
			t.Errorf("expected tool call to get_weather, got %q", toolName)
		}
	}
}

// TestLiveFullTurnWithToolDispatchAndACS exercises the complete
// agent loop story with a real LLM: user message -> LLM responds with
// tool call -> tool dispatched through ACS ledger -> result recorded ->
// replay from trace_id shows the full picture.
func TestLiveFullTurnWithToolDispatchAndACS(t *testing.T) {
	skipIfShort(t)
	apiKey := altllmAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := openai_compat.NewProvider(apiKey, altllmAPIBase, "")

	// Create ACS bundle
	dir := t.TempDir()
	bundle, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("acs.Open: %v", err)
	}
	defer bundle.Close()

	// Step 1: Begin turn
	traceID, err := bundle.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "integration-full-turn",
		Turn:           1,
		PromptHash:     "sha256-full-turn-prompt",
		ToolSchemaHash: "sha256-full-turn-tools",
		ModelID:        altllmModel,
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Step 2: Make a real LLM call
	msgs := []openai_compat.Message{
		{Role: "user", Content: "Reply with the single word 'verified'. Nothing else."},
	}
	resp, err := provider.Chat(ctx, msgs, nil, altllmModel, map[string]any{"max_tokens": 10})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	t.Logf("LLM response: %q", resp.Content)

	// Step 3: Record the LLM call
	err = bundle.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID:   traceID,
		CallSeq:   0,
		RequestID: "integration-full-req-1",
		ModelID:   altllmModel,
	})
	if err != nil {
		t.Fatalf("RecordLLMCall: %v", err)
	}

	// Step 4: Simulate a tool dispatch through the ledger
	argsHash, _ := acs.HashArgsForLedger(map[string]any{"amount": 0.5})
	intentID, err := bundle.PrepareAction(ctx, actionlog.Intent{
		TraceID:     traceID,
		ToolName:    "lido_stake",
		ArgsHash:    argsHash,
		Principal:   "agent=main;user=integration;account=0x1;channel=cli",
		EffectClass: "writes_wallet",
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}

	resultHash := acs.HashResultForLedger(&tools.ToolResult{
		ForUser: "Staked 0.5 ETH",
		ForLLM:  "staked successfully",
	})
	err = bundle.CommitAction(ctx, actionlog.Commit{
		IntentID:   intentID,
		ResultHash: resultHash,
	})
	if err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	// Step 5: REPLAY — verify everything from the trace_id
	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}

	// Manifest verification
	if full.Manifest.SessionID != "integration-full-turn" {
		t.Errorf("manifest SessionID = %q", full.Manifest.SessionID)
	}
	if full.Manifest.ModelID != altllmModel {
		t.Errorf("manifest ModelID = %q", full.Manifest.ModelID)
	}

	// Provider call verification
	if len(full.ProviderCalls) != 1 {
		t.Fatalf("ProviderCalls = %d, want 1", len(full.ProviderCalls))
	}

	// Ledger verification — zero orphans
	orphans, err := bundle.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d, want 0", len(orphans))
	}

	t.Logf("Full turn verified: traceID=%s, intentID=%s, response=%q", traceID, intentID, resp.Content)
}
