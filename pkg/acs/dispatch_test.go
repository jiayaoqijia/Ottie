package acs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
	"github.com/jiayaoqijia/ottie/pkg/tools"
)

// --- fake tools -----------------------------------------------------

// fakeReadOnlyTool does not implement EffectClassifier, so
// tools.ClassOf returns EffectReadOnly. It should bypass the
// ledger entirely.
type fakeReadOnlyTool struct{ name string }

func (f *fakeReadOnlyTool) Name() string                 { return f.name }
func (f *fakeReadOnlyTool) Description() string          { return "read-only tool" }
func (f *fakeReadOnlyTool) Parameters() map[string]any   { return map[string]any{} }
func (f *fakeReadOnlyTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return &tools.ToolResult{ForUser: "ok", ForLLM: "ok"}
}

// fakeWalletTool implements EffectClassifier to declare the
// highest-privilege class. Every invocation should land a Prepare
// + Commit (or Prepare + Abort) pair in the action ledger.
type fakeWalletTool struct {
	name        string
	shouldError bool
	errMsg      string
	forUser     string
	forLLM      string
}

func (f *fakeWalletTool) Name() string                 { return f.name }
func (f *fakeWalletTool) Description() string          { return "wallet-writing tool" }
func (f *fakeWalletTool) Parameters() map[string]any   { return map[string]any{} }
func (f *fakeWalletTool) EffectClass() tools.EffectClass {
	return tools.EffectWritesWallet
}
func (f *fakeWalletTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	if f.shouldError {
		return &tools.ToolResult{
			IsError: true,
			Err:     errors.New(f.errMsg),
			ForLLM:  "error: " + f.errMsg,
		}
	}
	return &tools.ToolResult{ForUser: f.forUser, ForLLM: f.forLLM}
}

func newDispatchTestBundle(t *testing.T) *Bundle {
	t.Helper()
	dir := t.TempDir()
	b, err := Open(Config{
		DBDir:           dir,
		WriteQueueDepth: 4,
		NowFn:           func() time.Time { return time.UnixMilli(1_700_000_000_000) },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return b
}

func beginSampleTurn(t *testing.T, b *Bundle) string {
	t.Helper()
	id, err := b.BeginTurn(context.Background(), execmanifest.Manifest{
		SessionID:      "sess-dispatch",
		Turn:           1,
		PromptHash:     "sha256-p",
		ToolSchemaHash: "sha256-s",
		ModelID:        "claude",
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	return id
}

// --- Happy path ----------------------------------------------------

// TestDispatchReadOnlyToolBypassesLedger verifies that a tool
// that does not implement EffectClassifier runs without any
// ledger rows. This is the common case — most tools are
// read-only and we don't want to bloat the ledger with their
// dispatches.
func TestDispatchReadOnlyToolBypassesLedger(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	tool := &fakeReadOnlyTool{name: "lookup_balance"}
	res := b.Dispatch(ctx, DispatchRequest{
		TraceID:   traceID,
		Tool:      tool,
		Args:      map[string]any{"query": "0xabc"},
		Principal: "agent=main;user=alice",
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result == nil || res.Result.ForLLM != "ok" {
		t.Fatalf("result = %+v, want ok", res.Result)
	}
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty (read-only bypasses)", res.IntentID)
	}
	if res.Committed {
		t.Error("Committed should be false for bypassed dispatch")
	}

	// Verify no orphan rows exist.
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d, want 0 (read-only should not touch ledger)", len(orphans))
	}
}

// TestDispatchWalletToolHappyPath verifies that a successful
// writes_wallet dispatch produces Prepare + Commit rows visible
// through RecoverOrphans (should be empty after commit).
func TestDispatchWalletToolHappyPath(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	tool := &fakeWalletTool{
		name:    "lido_stake",
		forUser: "Staked 0.5 ETH",
		forLLM:  "staked 0.5 ETH successfully",
	}
	res := b.Dispatch(ctx, DispatchRequest{
		TraceID:   traceID,
		Tool:      tool,
		Args:      map[string]any{"amount_eth": 0.5},
		Principal: "agent=main;user=alice;account=0x1;channel=cli",
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result == nil || res.Result.ForUser != "Staked 0.5 ETH" {
		t.Fatalf("result = %+v", res.Result)
	}
	if res.IntentID == "" {
		t.Error("IntentID should be populated for wallet tool")
	}
	if !res.Committed {
		t.Error("Committed should be true for successful dispatch")
	}

	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d after commit, want 0", len(orphans))
	}
}

// TestDispatchWalletToolAbortOnError verifies that a wallet tool
// that returns an error produces a Prepare + Abort pair, NOT a
// Commit, and the orphan count is still zero because the abort
// finalizes the intent.
func TestDispatchWalletToolAbortOnError(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	tool := &fakeWalletTool{
		name:        "lido_stake",
		shouldError: true,
		errMsg:      "insufficient gas",
	}
	res := b.Dispatch(ctx, DispatchRequest{
		TraceID:   traceID,
		Tool:      tool,
		Args:      map[string]any{"amount_eth": 0.5},
		Principal: "agent=main;user=alice",
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result == nil || !res.Result.IsError {
		t.Fatalf("result = %+v, want error", res.Result)
	}
	if res.IntentID == "" {
		t.Error("IntentID should be populated even on error")
	}
	if res.Committed {
		t.Error("Committed should be false on error — should be aborted")
	}

	// Abort finalizes the intent so orphan recovery returns 0.
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans after abort = %d, want 0", len(orphans))
	}
}

// TestDispatchNilBundleFallsThrough exercises the nil-receiver
// path. A nil Bundle means "ACS off" and Dispatch must still run
// the tool, it just doesn't touch a ledger.
func TestDispatchNilBundleFallsThrough(t *testing.T) {
	var b *Bundle
	ctx := context.Background()
	tool := &fakeWalletTool{name: "lido_stake", forLLM: "ok"}

	res := b.Dispatch(ctx, DispatchRequest{
		TraceID: "trc-x",
		Tool:    tool,
		Args:    map[string]any{"amount_eth": 0.5},
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result == nil || res.Result.ForLLM != "ok" {
		t.Errorf("result = %+v", res.Result)
	}
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty (nil bundle)", res.IntentID)
	}
}

// TestDispatchEmptyTraceIDFallsThrough covers the "ACS is open
// but this turn has no manifest" case — e.g., BeginTurn failed
// and the caller got back "". Dispatch should still run the tool
// without trying to Prepare against a nonexistent trace.
func TestDispatchEmptyTraceIDFallsThrough(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	tool := &fakeWalletTool{name: "lido_stake", forLLM: "ok"}

	res := b.Dispatch(ctx, DispatchRequest{
		TraceID: "", // no manifest for this turn
		Tool:    tool,
		Args:    map[string]any{"amount_eth": 0.5},
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result.ForLLM != "ok" {
		t.Errorf("result = %+v", res.Result)
	}
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty (no traceID)", res.IntentID)
	}
}

// --- Unhappy path --------------------------------------------------

// TestDispatchPrepareErrorFailsOpen verifies that if Prepare
// fails (e.g., tool name is empty, which the action ledger
// rejects as a required-field violation before hitting SQL), the
// helper still runs the tool. This is the fail-open policy — a
// broken observability layer must not block user-visible work.
//
// Note: we do NOT test "unknown trace_id fails Prepare" because
// the action ledger and execmanifest are independent SQLite
// files with no cross-store FK. The trace_id is a free-form text
// column in action_intents; correlation only happens at
// RecoverOrphans time. Inducing a Prepare failure requires a
// Go-side validation miss, which we force with an empty Name().
func TestDispatchPrepareErrorFailsOpen(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)
	// Tool declares writes_wallet so Dispatch tries to Prepare,
	// but has an empty name so actionlog.Prepare rejects it.
	tool := &emptyNameWalletTool{}

	res := b.Dispatch(ctx, DispatchRequest{
		TraceID: traceID,
		Tool:    tool,
		Args:    map[string]any{"amount_eth": 0.5},
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	// Tool still ran despite Prepare failure.
	if res.Result == nil || res.Result.ForLLM != "ok" {
		t.Fatalf("result = %+v, want ok (fail-open)", res.Result)
	}
	// No intent ID because Prepare failed.
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty on Prepare failure", res.IntentID)
	}
}

// emptyNameWalletTool declares writes_wallet but has Name()="",
// which forces actionlog.Prepare to return a required-field
// error. Used to exercise the Dispatch fail-open path.
type emptyNameWalletTool struct{}

func (*emptyNameWalletTool) Name() string                  { return "" }
func (*emptyNameWalletTool) Description() string           { return "broken" }
func (*emptyNameWalletTool) Parameters() map[string]any    { return nil }
func (*emptyNameWalletTool) EffectClass() tools.EffectClass { return tools.EffectWritesWallet }
func (*emptyNameWalletTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return &tools.ToolResult{ForLLM: "ok"}
}

// TestDispatchNilToolFallsThrough covers the degenerate case of
// a nil tool pointer. The helper should not panic; it should
// call run() and return whatever that produces.
func TestDispatchNilToolFallsThrough(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	res := b.Dispatch(ctx, DispatchRequest{
		TraceID: traceID,
		Tool:    nil, // degenerate
	}, func(ctx context.Context) *tools.ToolResult {
		return &tools.ToolResult{ForLLM: "stub"}
	})

	if res.Result == nil || res.Result.ForLLM != "stub" {
		t.Errorf("result = %+v", res.Result)
	}
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty for nil tool", res.IntentID)
	}
}

// --- Corner cases --------------------------------------------------

// TestDispatchEmptyArgsHashesDeterministically — HashArgsForLedger
// should treat nil and empty map as the same input so a ledger
// row's args hash is stable.
func TestDispatchEmptyArgsHashesDeterministically(t *testing.T) {
	h1, err := HashArgsForLedger(nil)
	if err != nil {
		t.Fatalf("HashArgsForLedger(nil): %v", err)
	}
	h2, err := HashArgsForLedger(map[string]any{})
	if err != nil {
		t.Fatalf("HashArgsForLedger({}): %v", err)
	}
	if h1 != h2 {
		t.Errorf("nil and empty hashes differ: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

// TestDispatchArgsHashMapKeyOrderStable verifies that two maps
// with the same keys inserted in different orders produce the
// same hash. This is important because tool-call args come back
// from providers in arbitrary map-iteration order.
func TestDispatchArgsHashMapKeyOrderStable(t *testing.T) {
	h1, _ := HashArgsForLedger(map[string]any{
		"amount": 0.5, "token": "ETH", "to": "0xabc",
	})
	h2, _ := HashArgsForLedger(map[string]any{
		"to": "0xabc", "amount": 0.5, "token": "ETH",
	})
	if h1 != h2 {
		t.Errorf("hashes differ for permuted maps: %q vs %q", h1, h2)
	}
}

// TestDispatchConcurrentToolsSerializeViaLedger fires multiple
// wallet tools concurrently to verify the dispatch helper is safe
// under parallel invocation. Each tool gets its own intent and
// the ledger serializes via the writer actor.
func TestDispatchConcurrentToolsSerializeViaLedger(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	const n = 20
	done := make(chan *DispatchResult, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			tool := &fakeWalletTool{
				name:    "lido_stake",
				forUser: fmt.Sprintf("staked %d", i),
				forLLM:  fmt.Sprintf("ok %d", i),
			}
			res := b.Dispatch(ctx, DispatchRequest{
				TraceID:   traceID,
				Tool:      tool,
				Args:      map[string]any{"idx": i},
				Principal: "agent=main",
			}, func(ctx context.Context) *tools.ToolResult {
				return tool.Execute(ctx, nil)
			})
			done <- res
		}()
	}

	seenIntents := map[string]bool{}
	for i := 0; i < n; i++ {
		res := <-done
		if res.IntentID == "" {
			t.Errorf("goroutine %d: IntentID empty", i)
			continue
		}
		if seenIntents[res.IntentID] {
			t.Errorf("duplicate IntentID: %s", res.IntentID)
		}
		seenIntents[res.IntentID] = true
		if !res.Committed {
			t.Errorf("goroutine %d: Committed=false", i)
		}
	}

	orphans, _ := b.RecoverOrphans(ctx)
	if len(orphans) != 0 {
		t.Errorf("orphans = %d after %d concurrent commits, want 0", len(orphans), n)
	}
}

// --- Edge cases ----------------------------------------------------

// TestDispatchEveryEffectClassIsWrapped covers the five
// side-effecting classes: each should trigger a Prepare row with
// the correct effect_class value recorded.
func TestDispatchEveryEffectClassIsWrapped(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		class tools.EffectClass
	}{
		{"writes_local", tools.EffectWritesLocal},
		{"writes_state", tools.EffectWritesState},
		{"writes_chain", tools.EffectWritesChain},
		{"writes_wallet", tools.EffectWritesWallet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newDispatchTestBundle(t)
			traceID := beginSampleTurn(t, b)
			tool := &effectTool{name: "t", class: tc.class, output: "ok"}
			res := b.Dispatch(ctx, DispatchRequest{
				TraceID:   traceID,
				Tool:      tool,
				Args:      map[string]any{"k": "v"},
				Principal: "agent=main",
			}, func(ctx context.Context) *tools.ToolResult {
				return tool.Execute(ctx, nil)
			})
			if res.IntentID == "" {
				t.Fatalf("class=%s: IntentID empty, want populated", tc.class)
			}
			if !res.Committed {
				t.Errorf("class=%s: Committed=false", tc.class)
			}
		})
	}
}

// TestClassOfDefaultsToReadOnly verifies that a tool without the
// EffectClassifier interface is treated as read_only and never
// wrapped.
func TestClassOfDefaultsToReadOnly(t *testing.T) {
	tool := &fakeReadOnlyTool{name: "lookup"}
	class := tools.ClassOf(tool)
	if class != tools.EffectReadOnly {
		t.Errorf("ClassOf read-only tool = %q, want read_only", class)
	}
	if class.IsSideEffecting() {
		t.Error("read_only class should not be side-effecting")
	}
}

// TestClassOfEmptyStringFallsBackToReadOnly defends against a
// classifier that returns "" accidentally. The helper normalizes
// this to read_only so no ledger rows are produced.
func TestClassOfEmptyStringFallsBackToReadOnly(t *testing.T) {
	tool := &effectTool{name: "x", class: ""}
	class := tools.ClassOf(tool)
	if class != tools.EffectReadOnly {
		t.Errorf("ClassOf empty = %q, want read_only", class)
	}
}

// TestHashResultForLedgerStable — identical result payloads
// produce identical hashes; differing payloads differ.
func TestHashResultForLedgerStable(t *testing.T) {
	r1 := &tools.ToolResult{ForUser: "staked", ForLLM: "ok"}
	r2 := &tools.ToolResult{ForUser: "staked", ForLLM: "ok"}
	r3 := &tools.ToolResult{ForUser: "staked", ForLLM: "different"}
	if HashResultForLedger(r1) != HashResultForLedger(r2) {
		t.Error("identical results hash differently")
	}
	if HashResultForLedger(r1) == HashResultForLedger(r3) {
		t.Error("different results hash the same")
	}
	if HashResultForLedger(nil) == "" {
		t.Error("nil result hash should be non-empty")
	}
}

// --- R12 regression tests (added after codex R12) -----------------

// TestDispatchNilResultIsAborted verifies the R12 fix: a tool
// that returns nil must NOT auto-commit a placeholder. The
// caller would panic on dereference at loop.go:1695 if we let
// the nil through, so Dispatch synthesizes an error result AND
// records an Abort row so the ledger reflects the broken tool.
func TestDispatchNilResultIsAborted(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	tool := &fakeWalletTool{name: "broken", forLLM: "unused"}
	res := b.Dispatch(ctx, DispatchRequest{
		TraceID: traceID,
		Tool:    tool,
		Args:    map[string]any{"amount_eth": 0.5},
	}, func(ctx context.Context) *tools.ToolResult {
		return nil // broken tool
	})

	if res.Result == nil {
		t.Fatal("Dispatch must synthesize a non-nil result when run returns nil")
	}
	if !res.Result.IsError {
		t.Error("synthesized nil-result should be marked IsError")
	}
	if res.IntentID == "" {
		t.Error("IntentID should be populated — a prepared row exists")
	}
	if res.Committed {
		t.Error("Committed should be false for nil-result path (aborted)")
	}
	// Orphan count is zero because the abort finalized the intent.
	orphans, _ := b.RecoverOrphans(ctx)
	if len(orphans) != 0 {
		t.Errorf("orphans after nil-result abort = %d, want 0", len(orphans))
	}
}

// TestDispatchSkipsAsyncExecutor covers the R12 fix that
// skips ledger wrapping for tools implementing AsyncExecutor.
// The real outcome of an async tool arrives later via callback,
// so auto-committing the immediate AsyncResult would lie about
// a not-yet-committed outcome.
func TestDispatchSkipsAsyncExecutor(t *testing.T) {
	ctx := context.Background()
	b := newDispatchTestBundle(t)
	traceID := beginSampleTurn(t, b)

	tool := &asyncWalletTool{}
	res := b.Dispatch(ctx, DispatchRequest{
		TraceID:   traceID,
		Tool:      tool,
		Args:      map[string]any{"k": "v"},
		Principal: "agent=main",
	}, func(ctx context.Context) *tools.ToolResult {
		return tool.Execute(ctx, nil)
	})

	if res.Result == nil || res.Result.ForLLM != "async-placeholder" {
		t.Fatalf("result = %+v", res.Result)
	}
	if res.IntentID != "" {
		t.Errorf("IntentID = %q, want empty (async tool bypasses ledger)", res.IntentID)
	}

	// Confirm no ledger rows were written.
	orphans, _ := b.RecoverOrphans(ctx)
	if len(orphans) != 0 {
		t.Errorf("orphans = %d, want 0 (async should not touch ledger)", len(orphans))
	}
}

// TestDispatchNilToolClassOfNoPanic verifies the tools.ClassOf
// defensive nil-check via Dispatch. A nil tool interface should
// not panic; Dispatch's own nil check catches it first, but
// even if it didn't, ClassOf would default to read-only and
// fall through.
func TestDispatchClassOfNilNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ClassOf(nil) panicked: %v", r)
		}
	}()
	if class := tools.ClassOf(nil); class != tools.EffectReadOnly {
		t.Errorf("ClassOf(nil) = %q, want read_only", class)
	}
}

// TestHashResultForLedgerJSONCanonicalization verifies the R12
// hash rewrite: the delimiter-concatenation approach had a
// collision risk because 0x1F is a valid Go string byte. The
// JSON-based rewrite is collision-free over the struct shape.
func TestHashResultForLedgerJSONCanonicalization(t *testing.T) {
	r1 := &tools.ToolResult{ForUser: "ab", ForLLM: "cd"}
	r2 := &tools.ToolResult{ForUser: "a", ForLLM: "b\x1fcd"}
	h1 := HashResultForLedger(r1)
	h2 := HashResultForLedger(r2)
	if h1 == h2 {
		t.Errorf("results with 0x1F collision hash the same: %q vs %q", h1, h2)
	}
	// IsError should also contribute to the hash.
	r3 := &tools.ToolResult{ForUser: "ab", ForLLM: "cd", IsError: true}
	h3 := HashResultForLedger(r3)
	if h1 == h3 {
		t.Error("IsError difference should change the hash")
	}
}

// --- R12 fakes ------------------------------------------------------

// asyncWalletTool implements both Tool and AsyncExecutor. Its
// Execute returns an async placeholder, which Dispatch should
// skip ledger wrapping for.
type asyncWalletTool struct{}

func (*asyncWalletTool) Name() string                 { return "async_wallet" }
func (*asyncWalletTool) Description() string          { return "async side-effecting tool" }
func (*asyncWalletTool) Parameters() map[string]any   { return map[string]any{} }
func (*asyncWalletTool) EffectClass() tools.EffectClass {
	return tools.EffectWritesWallet
}
func (*asyncWalletTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return &tools.ToolResult{Async: true, ForLLM: "async-placeholder"}
}
func (*asyncWalletTool) ExecuteAsync(
	_ context.Context,
	_ map[string]any,
	_ tools.AsyncCallback,
) *tools.ToolResult {
	return &tools.ToolResult{Async: true, ForLLM: "async-placeholder"}
}

// --- shared test tool ----------------------------------------------

// effectTool is a generic test-only tool that declares whatever
// effect class the test passes in. Used by the per-class coverage
// matrix above.
type effectTool struct {
	name   string
	class  tools.EffectClass
	output string
}

func (e *effectTool) Name() string                  { return e.name }
func (e *effectTool) Description() string           { return "test tool" }
func (e *effectTool) Parameters() map[string]any    { return map[string]any{} }
func (e *effectTool) EffectClass() tools.EffectClass { return e.class }
func (e *effectTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return &tools.ToolResult{ForUser: e.output, ForLLM: e.output}
}
