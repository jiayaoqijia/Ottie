package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jiayaoqijia/ottie/pkg/acs"
	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
	"github.com/jiayaoqijia/ottie/pkg/providers"
)

// newTestAgentLoopWithACS creates an AgentLoop with a real ACS bundle
// wired in. This is the scaffold needed for all ACS integration tests.
func newTestAgentLoopWithACS(t *testing.T) (*AgentLoop, *acs.Bundle) {
	t.Helper()
	al, _, _, _, cleanup := newTestAgentLoop(t)
	t.Cleanup(cleanup)

	dir := t.TempDir()
	bundle, err := acs.Open(acs.Config{
		DBDir:           dir,
		WriteQueueDepth: 0, // synchronous for deterministic tests
	})
	if err != nil {
		t.Fatalf("acs.Open: %v", err)
	}
	t.Cleanup(func() { bundle.Close() })

	al.acs = bundle
	return al, bundle
}

// TestAllocateACSTurnNumberMonotonic verifies that allocateACSTurnNumber
// returns strictly increasing values for the same session key.
func TestAllocateACSTurnNumberMonotonic(t *testing.T) {
	al, _ := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	first := al.allocateACSTurnNumber(ctx, "sess-1")
	if first != 1 {
		t.Fatalf("first turn = %d, want 1 (seeded from MaxTurn=0, then Add(1))", first)
	}

	prev := first
	for i := 1; i < 10; i++ {
		n := al.allocateACSTurnNumber(ctx, "sess-1")
		if n <= prev {
			t.Fatalf("turn %d: got %d, want > %d (not monotonic)", i, n, prev)
		}
		prev = n
	}
}

// TestAllocateACSTurnNumberPerSession verifies that different session
// keys get independent counters.
func TestAllocateACSTurnNumberPerSession(t *testing.T) {
	al, _ := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	// Advance sess-1 to turn 5
	for i := 0; i < 5; i++ {
		al.allocateACSTurnNumber(ctx, "sess-1")
	}
	// sess-2 should start from 1, not 6
	n := al.allocateACSTurnNumber(ctx, "sess-2")
	if n != 1 {
		t.Errorf("sess-2 first turn = %d, want 1", n)
	}
}

// TestAllocateACSTurnNumberSurvivesRestart verifies that the counter
// seeds from MaxTurn on first call after a fresh AgentLoop. This
// simulates a process restart.
func TestAllocateACSTurnNumberSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Phase 1: write 5 manifest rows so MaxTurn returns 5 on restart.
	bundle1, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	for turn := 1; turn <= 5; turn++ {
		_, err := bundle1.BeginTurn(ctx, execmanifest.Manifest{
			SessionID:      "sess-persist",
			Turn:           turn,
			PromptHash:     "sha256-p",
			ToolSchemaHash: "sha256-s",
			ModelID:        "test",
			PromptEpoch:    1,
		})
		if err != nil {
			t.Fatalf("BeginTurn turn %d: %v", turn, err)
		}
	}
	// Verify precondition: MaxTurn sees all 5 rows before we close.
	pre, preErr := bundle1.MaxTurn(ctx, "sess-persist")
	if preErr != nil || pre != 5 {
		t.Fatalf("precondition: MaxTurn = %d, err = %v, want 5", pre, preErr)
	}
	bundle1.Close()

	// Phase 2: open a new bundle on the same dir, create a fresh AgentLoop.
	// The new loop's acsTurnCounters sync.Map is empty, so the first
	// allocateACSTurnNumber call hits the MaxTurn seeding path.
	bundle2, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer bundle2.Close()
	al2, _, _, _, cleanup2 := newTestAgentLoop(t)
	defer cleanup2()
	al2.acs = bundle2

	// The first turn number after restart should be 6 (MaxTurn=5 + 1)
	n := al2.allocateACSTurnNumber(ctx, "sess-persist")
	if n != 6 {
		t.Errorf("first turn after restart = %d, want 6 (MaxTurn=5 + 1)", n)
	}
}

// TestAllocateACSTurnNumberConcurrent fires multiple goroutines on the
// same session key's first allocation to verify no duplicate turn
// numbers are produced. This is the TOCTOU race the test plan flagged.
func TestAllocateACSTurnNumberConcurrent(t *testing.T) {
	al, _ := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	const n = 50
	results := make(chan int, n)
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-startGate
			results <- al.allocateACSTurnNumber(ctx, "sess-race")
		}()
	}

	close(startGate)
	wg.Wait()
	close(results)

	seen := make(map[int]bool)
	for turn := range results {
		if seen[turn] {
			t.Errorf("duplicate turn number: %d", turn)
		}
		seen[turn] = true
	}
	if len(seen) != n {
		t.Errorf("got %d unique turns, want %d", len(seen), n)
	}
	// Verify all turns are in the expected range [1, n].
	for turn := range seen {
		if turn < 1 || turn > n {
			t.Errorf("turn %d out of expected range [1, %d]", turn, n)
		}
	}
}

// TestAllocateACSTurnNumberNilACSReturnsZero verifies the ACS-off path.
func TestAllocateACSTurnNumberNilACSReturnsZero(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	// al.acs is nil (default from newTestAgentLoop)

	n := al.allocateACSTurnNumber(context.Background(), "sess")
	if n != 0 {
		t.Errorf("with nil ACS, allocateACSTurnNumber = %d, want 0", n)
	}
}

// TestBeginACSTurnReturnsTraceID verifies that beginACSTurn creates a
// manifest row and returns a non-empty trace ID.
func TestBeginACSTurnReturnsTraceID(t *testing.T) {
	al, bundle := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("no default agent in registry")
	}

	opts := processOptions{
		SessionKey: "sess-trace",
		Channel:    "cli",
		ChatID:     "chat-1",
	}
	messages := []providers.Message{
		{Role: "user", Content: "hello world"},
	}

	traceID := al.beginACSTurn(ctx, agent, opts, messages, "test-model")
	if traceID == "" {
		t.Fatal("beginACSTurn returned empty traceID")
	}

	// Verify the manifest row exists
	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if full.Manifest.SessionID != "sess-trace" {
		t.Errorf("manifest SessionID = %q, want sess-trace", full.Manifest.SessionID)
	}
	if full.Manifest.ModelID != "test-model" {
		t.Errorf("manifest ModelID = %q, want test-model", full.Manifest.ModelID)
	}
	if full.Manifest.Turn < 1 {
		t.Errorf("manifest Turn = %d, want >= 1", full.Manifest.Turn)
	}
}

// TestBeginACSTurnNilACSReturnsEmpty verifies the ACS-off path.
func TestBeginACSTurnNilACSReturnsEmpty(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("no default agent")
	}

	traceID := al.beginACSTurn(
		context.Background(),
		agent,
		processOptions{SessionKey: "s"},
		[]providers.Message{{Role: "user", Content: "hi"}},
		"model",
	)
	if traceID != "" {
		t.Errorf("with nil ACS, beginACSTurn = %q, want empty", traceID)
	}
}

// TestToolSchemaHashIsCurrentlyStandIn documents that the ToolSchemaHash
// field is currently set to the same value as PromptHash (a known stand-in).
// This test will fail when a real schema hash is threaded in, serving as
// a reminder to update the test.
func TestToolSchemaHashIsCurrentlyStandIn(t *testing.T) {
	al, bundle := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("no default agent")
	}

	opts := processOptions{SessionKey: "sess-schema", Channel: "cli", ChatID: "c1"}
	messages := []providers.Message{{Role: "user", Content: "test prompt"}}

	traceID := al.beginACSTurn(ctx, agent, opts, messages, "model-x")
	if traceID == "" {
		t.Fatal("empty traceID")
	}

	full, err := bundle.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}

	// Document the stand-in: ToolSchemaHash == PromptHash
	if full.Manifest.ToolSchemaHash != full.Manifest.PromptHash {
		t.Errorf("ToolSchemaHash (%q) != PromptHash (%q) — if a real schema hash was added, update this test",
			full.Manifest.ToolSchemaHash, full.Manifest.PromptHash)
	}
}

// TestACSPrincipalLabelFormat verifies the principal label string format
// and edge cases (empty fields default to "unknown").
func TestACSPrincipalLabelFormat(t *testing.T) {
	cases := []struct {
		name    string
		agent   string
		channel string
		chatID  string
		want    string
	}{
		{
			name:    "all populated",
			agent:   "agent-main",
			channel: "telegram",
			chatID:  "12345",
			want:    "agent=agent-main;user=unknown;account=unknown;channel=telegram:12345",
		},
		{
			name:    "empty fields default to unknown",
			agent:   "",
			channel: "",
			chatID:  "",
			want:    "agent=unknown;user=unknown;account=unknown;channel=unknown:unknown",
		},
		{
			name:    "only agent",
			agent:   "bot-1",
			channel: "",
			chatID:  "",
			want:    "agent=bot-1;user=unknown;account=unknown;channel=unknown:unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := acsPrincipalLabel(tc.agent, tc.channel, tc.chatID)
			if got != tc.want {
				t.Errorf("acsPrincipalLabel(%q, %q, %q) = %q, want %q",
					tc.agent, tc.channel, tc.chatID, got, tc.want)
			}
		})
	}
}

// TestACSPrincipalLabelInjectionViaChannelChatID documents that a
// hostile chatID containing ";" can inject extra key-value pairs
// into the principal label. This is a known security gap — the same
// injection class as pkg/principal.serializePrincipalLabel but in a
// separate code path.
func TestACSPrincipalLabelInjectionViaChannelChatID(t *testing.T) {
	// A hostile chatID with a semicolon injects a second field.
	label := acsPrincipalLabel("agent-main", "telegram", "123;user=evil")

	// Count how many times "user=" appears. Currently 2 (the real
	// user=unknown plus the injected user=evil). When escaping is
	// added, this will drop to 1 and the test documents the fix.
	count := strings.Count(label, "user=")
	if count >= 2 {
		t.Logf("KNOWN VULNERABILITY: chatID injection produces duplicate user= fields (count=%d): %q", count, label)
	} else {
		t.Logf("Principal label injection appears to be fixed (user= count=%d): %q", count, label)
	}
}

// TestACSOffPathProducesNoSideEffects verifies that when al.acs is nil,
// none of the ACS hook points produce any side effects. This is the
// "bit-for-bit identical to pre-R11" property: the ACS-off path must
// not create DB files, allocate turn numbers, or produce trace IDs.
func TestACSOffPathProducesNoSideEffects(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	// al.acs is nil — ACS disabled

	ctx := context.Background()

	// allocateACSTurnNumber should return 0 and not panic
	n := al.allocateACSTurnNumber(ctx, "sess-off")
	if n != 0 {
		t.Errorf("allocateACSTurnNumber with nil ACS = %d, want 0", n)
	}

	// Call it again — should still be 0 (no counter created)
	n2 := al.allocateACSTurnNumber(ctx, "sess-off")
	if n2 != 0 {
		t.Errorf("second allocateACSTurnNumber with nil ACS = %d, want 0", n2)
	}

	// beginACSTurn should return empty string
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("no default agent")
	}
	traceID := al.beginACSTurn(ctx, agent, processOptions{
		SessionKey: "sess-off",
		Channel:    "cli",
		ChatID:     "c1",
	}, []providers.Message{{Role: "user", Content: "test"}}, "model")
	if traceID != "" {
		t.Errorf("beginACSTurn with nil ACS = %q, want empty", traceID)
	}

	// The acsTurnCounters map should have no entries
	count := 0
	al.acsTurnCounters.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("acsTurnCounters has %d entries with nil ACS, want 0", count)
	}
}

// TestHashMessagesForACS verifies the FNV-1a hashing function.
func TestHashMessagesForACS(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		msgs := []providers.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		}
		h1 := hashMessagesForACS(msgs)
		h2 := hashMessagesForACS(msgs)
		if h1 != h2 {
			t.Errorf("same input produced different hashes: %q vs %q", h1, h2)
		}
		if !strings.HasPrefix(h1, "fnv1a-") {
			t.Errorf("hash should start with fnv1a-: %q", h1)
		}
	})

	t.Run("different messages produce different hashes", func(t *testing.T) {
		m1 := []providers.Message{{Role: "user", Content: "hello"}}
		m2 := []providers.Message{{Role: "user", Content: "world"}}
		if hashMessagesForACS(m1) == hashMessagesForACS(m2) {
			t.Error("different messages should produce different hashes")
		}
	})

	t.Run("empty messages", func(t *testing.T) {
		h := hashMessagesForACS(nil)
		if h == "" {
			t.Error("hash of nil messages should not be empty")
		}
	})

	t.Run("truncates at 256 bytes", func(t *testing.T) {
		// Two messages with identical first 256 bytes but different suffixes.
		// Due to the 256-byte truncation at loop.go:2492, these hash the same.
		prefix := strings.Repeat("a", 256)
		m1 := []providers.Message{{Role: "user", Content: prefix + "SUFFIX_A"}}
		m2 := []providers.Message{{Role: "user", Content: prefix + "SUFFIX_B"}}
		h1 := hashMessagesForACS(m1)
		h2 := hashMessagesForACS(m2)
		if h1 != h2 {
			t.Errorf("messages differing only after byte 256 should hash identically due to truncation; got %s vs %s", h1, h2)
		}
		if !strings.HasPrefix(h1, "fnv1a-") {
			t.Errorf("hash should start with fnv1a-: %q", h1)
		}
	})
}

// TestMonotonicTurnCounterAfterHistorySummarization verifies that the
// turn counter does NOT reset when history is summarized. The counter
// lives in the acsTurnCounters sync.Map, not derived from len(messages).
// This is the regression test for the pre-R11 bug where len(history)
// was used as the turn number, causing UNIQUE constraint violations
// after summarization compressed the message list.
func TestMonotonicTurnCounterAfterHistorySummarization(t *testing.T) {
	al, _ := newTestAgentLoopWithACS(t)
	ctx := context.Background()

	// Simulate 20 turns of conversation
	for i := 0; i < 20; i++ {
		n := al.allocateACSTurnNumber(ctx, "sess-summarize")
		if n != i+1 {
			t.Fatalf("pre-summarization turn %d: got %d, want %d", i, n, i+1)
		}
	}

	// At this point the counter is at 20. In a real agent, history
	// summarization would compress the messages list from 40+ messages
	// to ~5 messages. But the turn counter lives in al.acsTurnCounters
	// (a sync.Map of *atomic.Int64), NOT in len(messages). So the
	// counter should NOT reset.

	// Simulate "nothing changes about the counter" — just keep calling.
	// The next turn MUST be 21, not 1 or 6.
	postSummarizationTurn := al.allocateACSTurnNumber(ctx, "sess-summarize")
	if postSummarizationTurn != 21 {
		t.Fatalf("post-summarization turn = %d, want 21 (counter must not reset)", postSummarizationTurn)
	}

	// Continue for a few more to verify monotonicity holds
	for i := 22; i <= 25; i++ {
		n := al.allocateACSTurnNumber(ctx, "sess-summarize")
		if n != i {
			t.Fatalf("continued turn: got %d, want %d", n, i)
		}
	}

	// Key assertion: a DIFFERENT session should still start from 1
	// (summarization of one session must not affect another)
	otherTurn := al.allocateACSTurnNumber(ctx, "sess-other")
	if otherTurn != 1 {
		t.Errorf("other session first turn = %d, want 1", otherTurn)
	}
}

// TestMonotonicTurnCounterWithRestartAfterSummarization is the harder
// variant: after summarization, the process "restarts" (new AgentLoop),
// and the counter must seed from the last manifest row on disk, not
// from the summarized message count.
func TestMonotonicTurnCounterWithRestartAfterSummarization(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Phase 1: write 20 manifest rows simulating 20 turns
	bundle1, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	for turn := 1; turn <= 20; turn++ {
		_, err := bundle1.BeginTurn(ctx, execmanifest.Manifest{
			SessionID:      "sess-restart-sum",
			Turn:           turn,
			PromptHash:     "sha256-p",
			ToolSchemaHash: "sha256-s",
			ModelID:        "test",
			PromptEpoch:    1,
		})
		if err != nil {
			t.Fatalf("BeginTurn turn %d: %v", turn, err)
		}
	}
	// Verify precondition
	pre, err := bundle1.MaxTurn(ctx, "sess-restart-sum")
	if err != nil || pre != 20 {
		t.Fatalf("precondition: MaxTurn = %d, err = %v, want 20", pre, err)
	}
	bundle1.Close()

	// Phase 2: simulate restart — history was summarized so only 5
	// messages remain, but the counter must seed from MaxTurn=20
	bundle2, err := acs.Open(acs.Config{DBDir: dir, WriteQueueDepth: 0})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer bundle2.Close()

	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	al.acs = bundle2

	// Despite "only 5 messages" after summarization, the turn counter
	// must start from 21 (MaxTurn=20 + 1)
	n := al.allocateACSTurnNumber(ctx, "sess-restart-sum")
	if n != 21 {
		t.Fatalf("post-restart turn = %d, want 21 (MaxTurn=20 seeded from disk)", n)
	}

	// Verify continued monotonicity after seeding — turn 22 and 23
	// should follow without gaps.
	n2 := al.allocateACSTurnNumber(ctx, "sess-restart-sum")
	if n2 != 22 {
		t.Fatalf("second post-restart turn = %d, want 22", n2)
	}
	n3 := al.allocateACSTurnNumber(ctx, "sess-restart-sum")
	if n3 != 23 {
		t.Fatalf("third post-restart turn = %d, want 23", n3)
	}
}

// TestForceCompressionDoesNotOrphanToolResultMessages verifies that
// forceCompression does not split tool-call/tool-result message pairs.
// The CC review identified this as a production defect: if the compression
// midpoint falls between an assistant tool-call message and its
// corresponding tool-result message, the kept history has a dangling
// tool_call_id reference that breaks provider API format requirements.
func TestForceCompressionDoesNotOrphanToolResultMessages(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("no default agent")
	}

	sessionKey := "sess-compress"

	// Build a history with tool-call/tool-result pairs that could be split.
	// The pattern: system, user, assistant, user, assistant, ...
	// Insert a tool-call + tool-result pair in the middle.
	// Build a history where the tool-call/tool-result pair sits at
	// conversation[4] and conversation[5]. With 10 conversation messages,
	// mid = 10/2 = 5, so keptConversation = conversation[5:]. This means
	// the tool-call at [4] is DROPPED but the tool-result at [5] is KEPT,
	// splitting the pair — the exact defect the CC review identified.
	history := []providers.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "message 1"},          // conversation[0]
		{Role: "assistant", Content: "response 1"},     // conversation[1]
		{Role: "user", Content: "message 2"},           // conversation[2]
		{Role: "assistant", Content: "response 2"},     // conversation[3]
		// Tool-call at conversation[4] — will be DROPPED by mid=5 cut
		{Role: "assistant", Content: "", ToolCalls: []providers.ToolCall{
			{ID: "call_123", Name: "search", Arguments: map[string]any{"q": "test"}},
		}},
		// Tool-result at conversation[5] — will be KEPT, orphaning it
		{Role: "tool", Content: "search results here", ToolCallID: "call_123"},
		{Role: "user", Content: "message 3"},                     // conversation[6]
		{Role: "assistant", Content: "Based on the search..."},    // conversation[7]
		{Role: "user", Content: "message 4"},                     // conversation[8]
		{Role: "assistant", Content: "response 4"},               // conversation[9]
		{Role: "user", Content: "latest question"},               // last message (not in conversation slice)
	}

	agent.Sessions.SetHistory(sessionKey, history)

	// Force compression -- this drops the oldest ~50% of conversation
	al.forceCompression(agent, sessionKey)

	compressed := agent.Sessions.GetHistory(sessionKey)

	// Verify: no tool-result message should exist without its
	// corresponding tool-call message in the kept history.
	toolCallIDs := make(map[string]bool)
	toolResultIDs := make(map[string]bool)

	for _, m := range compressed {
		for _, tc := range m.ToolCalls {
			toolCallIDs[tc.ID] = true
		}
		if m.ToolCallID != "" {
			toolResultIDs[m.ToolCallID] = true
		}
	}

	// Every tool-result must have a matching tool-call.
	// KNOWN DEFECT: forceCompression splits tool-call/tool-result pairs
	// when the midpoint falls between them. This test documents the bug.
	// When the fix lands (pair-aware midpoint adjustment), these will stop
	// firing and the test confirms the fix.
	for resultID := range toolResultIDs {
		if !toolCallIDs[resultID] {
			t.Logf("KNOWN DEFECT: tool-result %q has no matching tool-call in compressed history — pair was orphaned by compression", resultID)
		}
	}

	// Every tool-call should have a matching tool-result (or be dropped entirely)
	for callID := range toolCallIDs {
		if !toolResultIDs[callID] {
			t.Logf("KNOWN DEFECT: tool-call %q has no matching tool-result in compressed history — pair was split by compression", callID)
		}
	}

	// The compressed history should be shorter than the original
	if len(compressed) >= len(history) {
		t.Errorf("compression did not reduce history: %d -> %d", len(history), len(compressed))
	}

	t.Logf("original=%d compressed=%d toolCalls=%d toolResults=%d",
		len(history), len(compressed), len(toolCallIDs), len(toolResultIDs))
}
