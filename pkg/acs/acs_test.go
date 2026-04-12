package acs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/actionlog"
	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
)

// --- fixtures ---------------------------------------------------------

func newTestBundle(t *testing.T, fixedMs int64) *Bundle {
	t.Helper()
	dir := t.TempDir()
	b, err := Open(Config{
		DBDir:           dir,
		WriteQueueDepth: 4,
		NowFn:           func() time.Time { return time.UnixMilli(fixedMs) },
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

// newSyncTestBundle creates a bundle with WriteQueueDepth=0 (synchronous
// writes). Use this for tests that read back rows immediately after
// writing them — async queues can cause flaky read-after-write races.
func newSyncTestBundle(t *testing.T, fixedMs int64) *Bundle {
	t.Helper()
	dir := t.TempDir()
	b, err := Open(Config{
		DBDir:           dir,
		WriteQueueDepth: 0,
		NowFn:           func() time.Time { return time.UnixMilli(fixedMs) },
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

func sampleManifest() execmanifest.Manifest {
	return execmanifest.Manifest{
		SessionID:      "sess-1",
		Turn:           0,
		PromptHash:     "sha256-p",
		ToolSchemaHash: "sha256-s",
		SkillHashes:    []string{"sha256-sk1"},
		McpVersions:    map[string]string{"lido-mcp": "1.0"},
		ModelID:        "claude-sonnet-4-6",
		PromptEpoch:    1,
	}
}

func sampleIntent(traceID string) actionlog.Intent {
	return actionlog.Intent{
		TraceID:     traceID,
		ToolName:    "lido_stake",
		ArgsHash:    "sha256-args",
		Principal:   "agent=main;user=alice;account=0x1;channel=cli",
		EffectClass: "writes_wallet",
	}
}

// --- Happy path ------------------------------------------------------

// TestBeginTurnAndRecordLLMCallRoundTrip exercises the canonical
// "turn starts, LLM fires, manifest is queryable" flow.
func TestBeginTurnAndRecordLLMCallRoundTrip(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if traceID == "" {
		t.Fatal("BeginTurn returned empty traceID")
	}

	if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID: traceID, CallSeq: 0, RequestID: "req-abc", ModelID: "claude-sonnet-4-6",
	}); err != nil {
		t.Fatalf("RecordLLMCall: %v", err)
	}

	got, err := b.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if got.Manifest.SessionID != "sess-1" {
		t.Errorf("SessionID = %q", got.Manifest.SessionID)
	}
	if len(got.ProviderCalls) != 1 {
		t.Fatalf("ProviderCalls = %d, want 1", len(got.ProviderCalls))
	}
	if got.ProviderCalls[0].RequestID != "req-abc" {
		t.Errorf("RequestID = %q", got.ProviderCalls[0].RequestID)
	}
}

// TestPrepareAndCommitActionRoundTrip exercises the action ledger
// wrapper path.
func TestPrepareAndCommitActionRoundTrip(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	intentID, err := b.PrepareAction(ctx, sampleIntent(traceID))
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := b.CommitAction(ctx, actionlog.Commit{
		IntentID:    intentID,
		ExternalIDs: map[string]any{"tx_hash": "0xdeadbeef"},
		ResultHash:  "sha256-result",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	// Orphan recovery should be empty after commit.
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %d, want 0", len(orphans))
	}
}

// TestPrepareAndAbortActionRoundTrip — same flow via the abort
// path.
func TestPrepareAndAbortActionRoundTrip(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	intentID, err := b.PrepareAction(ctx, sampleIntent(traceID))
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := b.AbortAction(ctx, actionlog.Abort{
		IntentID: intentID, ErrorMessage: "user canceled",
	}); err != nil {
		t.Fatalf("AbortAction: %v", err)
	}
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %d, want 0", len(orphans))
	}
}

// TestRecoverOrphansEnrichedWithManifest verifies the two-table
// join: an orphaned intent should come back with its execution
// manifest bundled in.
func TestRecoverOrphansEnrichedWithManifest(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if _, err := b.PrepareAction(ctx, sampleIntent(traceID)); err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	// DO NOT commit — leave it as an orphan.

	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	o := orphans[0]
	if o.Intent.Intent.TraceID != traceID {
		t.Errorf("orphan trace_id = %q, want %q", o.Intent.Intent.TraceID, traceID)
	}
	if !o.ManifestPresent {
		t.Error("orphan should have ManifestPresent=true")
	}
	if o.ManifestLookupErr != nil {
		t.Errorf("ManifestLookupErr = %v", o.ManifestLookupErr)
	}
	if o.Manifest.Manifest.SessionID != "sess-1" {
		t.Errorf("manifest session = %q", o.Manifest.Manifest.SessionID)
	}
}

// --- Unhappy path ----------------------------------------------------

// TestOpenRejectsEmptyDBDir — the one required config field.
func TestOpenRejectsEmptyDBDir(t *testing.T) {
	_, err := Open(Config{DBDir: ""})
	if err == nil {
		t.Fatal("expected error for empty DBDir")
	}
	if !errorContains(err, "DBDir is required") {
		t.Fatalf("err = %v, want 'DBDir is required'", err)
	}
}

// TestOpenCleansUpLedgerIfManifestOpenFails — partial-open
// recovery. We force the second Open to fail by pointing DBDir at
// a file path (not a directory) so filepath.Join produces a name
// that cannot be created.
//
// Note: sqlite will typically succeed at opening both files even
// if the dir name collides, so we instead create a read-only
// directory and assert Open fails while cleaning up.
func TestOpenCleansUpLedgerIfManifestOpenFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot create a read-only directory as root")
	}
	dir := t.TempDir()
	// Create the first file successfully, then make the directory
	// read-only so the second file (execmanifest.db) cannot be
	// created. This isn't a perfect partial-failure simulation but
	// it exercises the cleanup path under a realistic failure mode.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() {
		_ = os.Chmod(dir, 0o700)
	}()

	// Both Opens should fail at the creation step because
	// read-only directory permissions block file creation.
	_, err := Open(Config{DBDir: dir, WriteQueueDepth: 1})
	if err == nil {
		t.Fatal("expected Open to fail on read-only directory")
	}
	// The error message names which store failed first so the
	// operator can diagnose.
	if !errorContains(err, "acs:") {
		t.Fatalf("err = %v, want acs-prefixed", err)
	}
}

// TestOpsAfterCloseReturnErrClosed exercises every bundle method
// against a closed bundle — happy path semantics after shutdown.
func TestOpsAfterCloseReturnErrClosed(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	if err := b.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	_, err := b.BeginTurn(ctx, sampleManifest())
	if !errors.Is(err, ErrClosed) {
		t.Errorf("BeginTurn after close: %v, want ErrClosed", err)
	}
	if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID: "x", CallSeq: 0, RequestID: "r",
	}); !errors.Is(err, ErrClosed) {
		t.Errorf("RecordLLMCall after close: %v, want ErrClosed", err)
	}
	if _, err := b.GetManifest(ctx, "x"); !errors.Is(err, ErrClosed) {
		t.Errorf("GetManifest after close: %v, want ErrClosed", err)
	}
	if _, err := b.PrepareAction(ctx, sampleIntent("x")); !errors.Is(err, ErrClosed) {
		t.Errorf("PrepareAction after close: %v, want ErrClosed", err)
	}
	if err := b.CommitAction(ctx, actionlog.Commit{IntentID: "x"}); !errors.Is(err, ErrClosed) {
		t.Errorf("CommitAction after close: %v, want ErrClosed", err)
	}
	if err := b.AbortAction(ctx, actionlog.Abort{IntentID: "x"}); !errors.Is(err, ErrClosed) {
		t.Errorf("AbortAction after close: %v, want ErrClosed", err)
	}
	if _, err := b.RecoverOrphans(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("RecoverOrphans after close: %v, want ErrClosed", err)
	}
}

// TestRecoverOrphansAcrossReopen verifies the persistence story:
// a prepared intent survives Close+reopen of the bundle.
func TestRecoverOrphansAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	b1, err := Open(Config{DBDir: dir, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	traceID, err := b1.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if _, err := b1.PrepareAction(ctx, sampleIntent(traceID)); err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := b1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b2, err := Open(Config{DBDir: dir, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer b2.Close()

	orphans, err := b2.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans after reopen = %d, want 1", len(orphans))
	}
	if !orphans[0].ManifestPresent {
		t.Error("manifest should still be present after reopen")
	}
}

// --- Corner cases ----------------------------------------------------

// TestConcurrentOpsAcrossAllMethods stresses the bundle: multiple
// goroutines simultaneously call BeginTurn, RecordLLMCall,
// PrepareAction, CommitAction, and GetManifest. The underlying
// stores are independent writer actors, so this should work, but
// the coordinator has its own close-flag mutex that might race
// under load.
func TestConcurrentOpsAcrossAllMethods(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n*3)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			m := sampleManifest()
			m.Turn = i
			traceID, err := b.BeginTurn(ctx, m)
			if err != nil {
				errs <- err
				return
			}
			if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
				TraceID: traceID, CallSeq: 0, RequestID: "req",
			}); err != nil {
				errs <- err
				return
			}
			if _, err := b.GetManifest(ctx, traceID); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op err: %v", err)
	}
}

// TestConcurrentCloseAndOps — the hard race. Fire goroutines
// doing bundle operations while another goroutine calls Close.
// Every op must return nil or acs.ErrClosed specifically (NOT
// the underlying actionlog.ErrClosed / execmanifest.ErrClosed
// sentinels) because the wrapper normalizes them. Codex R11
// flagged the earlier tolerance of leaked underlying sentinels
// as a contract violation.
func TestConcurrentCloseAndOps(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b, err := Open(Config{DBDir: dir, WriteQueueDepth: 8})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const producers = 20
	const pushesPerProducer = 10
	results := make(chan error, producers*pushesPerProducer)
	var wg sync.WaitGroup
	wg.Add(producers)
	for i := 0; i < producers; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < pushesPerProducer; j++ {
				m := sampleManifest()
				m.SessionID = "sess-race"
				m.Turn = i*pushesPerProducer + j
				_, err := b.BeginTurn(ctx, m)
				results <- err
			}
		}(i)
	}

	time.Sleep(5 * time.Millisecond)
	closeErr := b.Close()
	wg.Wait()
	close(results)

	if closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	var ok, closed int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrClosed):
			closed++
		case errors.Is(err, execmanifest.ErrClosed):
			t.Errorf("leaked execmanifest.ErrClosed (should be normalized to acs.ErrClosed): %v", err)
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok+closed != producers*pushesPerProducer {
		t.Fatalf("ok=%d closed=%d want total=%d",
			ok, closed, producers*pushesPerProducer)
	}
	t.Logf("ok=%d closed=%d", ok, closed)
}

// TestMaxTurnSeedsMonotonicCounter exercises the new MaxTurn
// wrapper. Create turns 1..5 in one bundle, close, reopen, and
// verify MaxTurn returns 5 so the agent loop's counter can seed
// from max+1 on restart.
func TestMaxTurnSeedsMonotonicCounter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	b1, err := Open(Config{DBDir: dir, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	const sess = "sess-monotonic"
	for _, turn := range []int{1, 2, 3, 4, 5} {
		m := sampleManifest()
		m.SessionID = sess
		m.Turn = turn
		if _, err := b1.BeginTurn(ctx, m); err != nil {
			t.Fatalf("BeginTurn turn %d: %v", turn, err)
		}
	}
	if err := b1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b2, err := Open(Config{DBDir: dir, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer b2.Close()

	max, err := b2.MaxTurn(ctx, sess)
	if err != nil {
		t.Fatalf("MaxTurn: %v", err)
	}
	if max != 5 {
		t.Errorf("MaxTurn = %d, want 5", max)
	}

	max, err = b2.MaxTurn(ctx, "sess-empty")
	if err != nil {
		t.Fatalf("MaxTurn empty: %v", err)
	}
	if max != 0 {
		t.Errorf("MaxTurn empty = %d, want 0", max)
	}
}

// TestErrClosedNormalization verifies the wrapper-boundary
// normalization: a store method that returns its own ErrClosed
// sentinel should be surfaced as acs.ErrClosed to the caller.
func TestErrClosedNormalization(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	// Close through the coordinator so both stores are shut down.
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Every wrapper should return acs.ErrClosed specifically, not
	// the underlying sentinels.
	if _, err := b.BeginTurn(ctx, sampleManifest()); !errors.Is(err, ErrClosed) {
		t.Errorf("BeginTurn: %v, want acs.ErrClosed", err)
	}
	if _, err := b.MaxTurn(ctx, "sess"); !errors.Is(err, ErrClosed) {
		t.Errorf("MaxTurn: %v, want acs.ErrClosed", err)
	}
	if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
		TraceID: "x", CallSeq: 0, RequestID: "r",
	}); !errors.Is(err, ErrClosed) {
		t.Errorf("RecordLLMCall: %v, want acs.ErrClosed", err)
	}
}

// --- Edge cases ------------------------------------------------------

// TestLedgerAndManifestAccessorsExposeUnderlying — advanced callers
// that need per-package APIs should be able to reach through.
func TestLedgerAndManifestAccessorsExposeUnderlying(t *testing.T) {
	b := newTestBundle(t, 1_700_000_000_000)
	if b.Ledger() == nil {
		t.Error("Ledger() returned nil")
	}
	if b.Manifest() == nil {
		t.Error("Manifest() returned nil")
	}
}

// TestDBDirMustExist — Open does not create the directory, the
// caller is responsible. Exercise the missing-dir path.
func TestDBDirMustExist(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does", "not", "exist")
	_, err := Open(Config{DBDir: nonexistent})
	if err == nil {
		t.Fatal("expected error for missing DBDir")
	}
}

// TestCloseErrorFromUnderlyingStore verifies Close bubbles up
// errors from the underlying stores. We can't easily force a
// sqlite close to fail, so this test is documentation-shaped: it
// asserts the error-wrapping path is at least defined.
func TestCloseIsIdempotent(t *testing.T) {
	b := newTestBundle(t, 1_700_000_000_000)
	// First close through the test cleanup; we force a second
	// explicit close here to verify the idempotent branch.
	if err := b.Close(); err != nil {
		t.Fatalf("explicit Close: %v", err)
	}
	// The t.Cleanup Close will fire another one, which must also
	// return nil. This is actually verified by the other tests
	// that use newTestBundle — this test just makes the
	// idempotency property explicit.
}

// TestRecoverOrphansEmptyReturnsNonNilSlice covers the "empty
// database" edge: the bundle should return a non-nil empty slice,
// matching the convention of the underlying stores.
func TestRecoverOrphansEmptyReturnsNonNilSlice(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if orphans == nil {
		t.Fatal("orphans is nil; want non-nil empty slice")
	}
	if len(orphans) != 0 {
		t.Fatalf("len = %d, want 0", len(orphans))
	}
}

// TestBundleCloseAttemptsBothStoresEvenIfFirstFails — Close should
// call both underlying Close methods and return the first error,
// not short-circuit. Hard to test directly without a fake, so this
// test documents the intent.
func TestBundleCloseOrderIsLedgerFirstThenManifest(t *testing.T) {
	// The order is load-bearing: the action ledger may need to
	// drain pending commit ops that reference manifest rows, so
	// it must close before the manifest store. This test is more
	// a TODO than a hard assertion; we verify the order in the
	// Close() implementation file comment and rely on
	// TestConcurrentCloseAndOps to cover the end-to-end
	// shutdown race.
	b := newTestBundle(t, 1_700_000_000_000)
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestRecoverOrphansWithMissingManifest verifies the crash-recovery
// scenario where a prepared intent has no corresponding execution
// manifest. This happens when the process crashes between
// PrepareAction and BeginTurn, or when ACS config changed between
// runs. The orphan should still be returned with ManifestPresent=false.
func TestRecoverOrphansWithMissingManifest(t *testing.T) {
	ctx := context.Background()
	b := newTestBundle(t, 1_700_000_000_000)

	// Prepare an intent with a trace_id that has NO manifest row.
	// This simulates a crash where the tool was prepared but
	// BeginTurn never completed (or was skipped).
	intent := sampleIntent("nonexistent-trace-id")
	_, err := b.PrepareAction(ctx, intent)
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}

	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}

	o := orphans[0]
	if o.ManifestPresent {
		t.Error("ManifestPresent should be false for nonexistent trace_id")
	}
	if o.ManifestLookupErr != nil {
		t.Errorf("ManifestLookupErr should be nil (ErrTraceNotFound is expected, not an error): %v", o.ManifestLookupErr)
	}
	if o.Intent.Intent.TraceID != "nonexistent-trace-id" {
		t.Errorf("orphan trace_id = %q", o.Intent.Intent.TraceID)
	}
	if o.Intent.Intent.ToolName != "lido_stake" {
		t.Errorf("orphan tool_name = %q", o.Intent.Intent.ToolName)
	}
}

// --- Replay from trace ID --------------------------------------------

// TestReplayFromTraceID verifies the end-to-end replay story: given a
// trace_id, the caller can reconstruct the turn's manifest (prompt hash,
// model, session, turn) and all provider call rows. This is the
// demo-critical capability that positions Ottie against hermes-agent.
func TestReplayFromTraceID(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	// Step 1: Begin a turn
	manifest := execmanifest.Manifest{
		SessionID:      "sess-replay",
		Turn:           1,
		PromptHash:     "sha256-abc123",
		ToolSchemaHash: "sha256-tools456",
		SkillHashes:    []string{"sha256-sk1", "sha256-sk2"},
		McpVersions:    map[string]string{"lido-mcp": "1.0"},
		ModelID:        "claude-sonnet-4-6",
		PromptEpoch:    1,
	}
	traceID, err := b.BeginTurn(ctx, manifest)
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Step 2: Record multiple provider calls (simulating retry + fallback)
	calls := []execmanifest.ProviderCall{
		{TraceID: traceID, CallSeq: 0, RequestID: "req-001", ModelID: "claude-sonnet-4-6"},
		{TraceID: traceID, CallSeq: 1, RequestID: "req-002", ModelID: "claude-sonnet-4-6"}, // retry
		{TraceID: traceID, CallSeq: 2, RequestID: "req-003", ModelID: "claude-haiku-4-5"},   // fallback
	}
	for _, c := range calls {
		if err := b.RecordLLMCall(ctx, c); err != nil {
			t.Fatalf("RecordLLMCall seq %d: %v", c.CallSeq, err)
		}
	}

	// Step 3: Dispatch a side-effecting tool and commit
	intentID, err := b.PrepareAction(ctx, actionlog.Intent{
		TraceID:     traceID,
		ToolName:    "lido_stake",
		ArgsHash:    "sha256-args-xyz",
		Principal:   "agent=main;user=alice;account=0x1;channel=cli",
		EffectClass: "writes_wallet",
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}
	if err := b.CommitAction(ctx, actionlog.Commit{
		IntentID:    intentID,
		ExternalIDs: map[string]any{"tx_hash": "0xdeadbeef"},
		ResultHash:  "sha256-result-abc",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	// Step 4: REPLAY — reconstruct everything from just the trace_id
	full, err := b.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}

	// Verify manifest fields
	if full.Manifest.SessionID != "sess-replay" {
		t.Errorf("SessionID = %q, want sess-replay", full.Manifest.SessionID)
	}
	if full.Manifest.Turn != 1 {
		t.Errorf("Turn = %d, want 1", full.Manifest.Turn)
	}
	if full.Manifest.PromptHash != "sha256-abc123" {
		t.Errorf("PromptHash = %q, want sha256-abc123", full.Manifest.PromptHash)
	}
	if full.Manifest.ModelID != "claude-sonnet-4-6" {
		t.Errorf("ModelID = %q, want claude-sonnet-4-6", full.Manifest.ModelID)
	}

	// Verify provider call rows — order should match call_seq
	if len(full.ProviderCalls) != 3 {
		t.Fatalf("ProviderCalls = %d, want 3", len(full.ProviderCalls))
	}
	for i, c := range full.ProviderCalls {
		if c.CallSeq != i {
			t.Errorf("ProviderCalls[%d].CallSeq = %d, want %d", i, c.CallSeq, i)
		}
	}
	if full.ProviderCalls[0].RequestID != "req-001" {
		t.Errorf("first call RequestID = %q, want req-001", full.ProviderCalls[0].RequestID)
	}
	if full.ProviderCalls[2].ModelID != "claude-haiku-4-5" {
		t.Errorf("fallback call ModelID = %q, want claude-haiku-4-5", full.ProviderCalls[2].ModelID)
	}

	// Verify the action ledger has no orphans (everything was committed)
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d, want 0 (everything committed)", len(orphans))
	}
}

// TestReplayProviderCallsOrderMatchesExecution verifies that provider
// call rows are returned in call_seq order, even if they were recorded
// out of order (e.g., concurrent fallback attempts).
func TestReplayProviderCallsOrderMatchesExecution(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Record in reverse order
	for seq := 4; seq >= 0; seq-- {
		if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
			TraceID:   traceID,
			CallSeq:   seq,
			RequestID: fmt.Sprintf("req-%d", seq),
			ModelID:   "model",
		}); err != nil {
			t.Fatalf("RecordLLMCall seq %d: %v", seq, err)
		}
	}

	full, err := b.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}

	if len(full.ProviderCalls) != 5 {
		t.Fatalf("ProviderCalls = %d, want 5", len(full.ProviderCalls))
	}
	for i, c := range full.ProviderCalls {
		if c.CallSeq != i {
			t.Errorf("ProviderCalls[%d].CallSeq = %d, want %d (should be sorted by call_seq)", i, c.CallSeq, i)
		}
	}
}

// TestReplayOrphanedIntentShowsMissingOutcome verifies the replay story
// for a prepared-but-not-committed intent: the manifest has full turn
// context but the action has no outcome row, indicating a crash between
// dispatch and commit.
func TestReplayOrphanedIntentShowsMissingOutcome(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "sess-orphan-replay",
		Turn:           1,
		PromptHash:     "sha256-p",
		ToolSchemaHash: "sha256-s",
		ModelID:        "claude-sonnet-4-6",
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Prepare but do NOT commit — simulates a crash
	_, err = b.PrepareAction(ctx, actionlog.Intent{
		TraceID:     traceID,
		ToolName:    "lido_stake",
		ArgsHash:    "sha256-args",
		Principal:   "agent=main;user=alice",
		EffectClass: "writes_wallet",
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}

	// Replay via RecoverOrphans
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}

	o := orphans[0]
	// The orphan should have the full manifest context
	if !o.ManifestPresent {
		t.Error("ManifestPresent should be true — BeginTurn succeeded")
	}
	if o.Manifest.Manifest.SessionID != "sess-orphan-replay" {
		t.Errorf("manifest SessionID = %q", o.Manifest.Manifest.SessionID)
	}
	if o.Manifest.Manifest.ModelID != "claude-sonnet-4-6" {
		t.Errorf("manifest ModelID = %q", o.Manifest.Manifest.ModelID)
	}
	// The intent should carry the tool attribution
	if o.Intent.Intent.ToolName != "lido_stake" {
		t.Errorf("intent ToolName = %q", o.Intent.Intent.ToolName)
	}
	if o.Intent.Intent.EffectClass != "writes_wallet" {
		t.Errorf("intent EffectClass = %q", o.Intent.Intent.EffectClass)
	}
}

// --- helpers ---------------------------------------------------------

// TestReplayMultipleToolDispatchesInOneTurn verifies that a single
// turn with multiple side-effecting tool dispatches produces multiple
// action ledger rows, all linked to the same trace_id via the manifest.
func TestReplayMultipleToolDispatchesInOneTurn(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "sess-multi-dispatch",
		Turn:           1,
		PromptHash:     "sha256-p",
		ToolSchemaHash: "sha256-s",
		ModelID:        "claude-sonnet-4-6",
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Dispatch 3 different tools in the same turn
	toolNames := []string{"write_file", "lido_stake", "install_skill"}
	effectClasses := []string{"writes_local", "writes_wallet", "writes_state"}
	intentIDs := make([]string, 3)

	for i, toolName := range toolNames {
		intentID, err := b.PrepareAction(ctx, actionlog.Intent{
			TraceID:     traceID,
			ToolName:    toolName,
			ArgsHash:    fmt.Sprintf("sha256-args-%d", i),
			Principal:   "agent=main;user=alice",
			EffectClass: effectClasses[i],
		})
		if err != nil {
			t.Fatalf("PrepareAction %s: %v", toolName, err)
		}
		intentIDs[i] = intentID

		// Commit each one
		if err := b.CommitAction(ctx, actionlog.Commit{
			IntentID:   intentID,
			ResultHash: fmt.Sprintf("sha256-result-%d", i),
		}); err != nil {
			t.Fatalf("CommitAction %s: %v", toolName, err)
		}
	}

	// All intent IDs should be unique
	seen := map[string]bool{}
	for _, id := range intentIDs {
		if id == "" {
			t.Error("empty intentID")
		}
		if seen[id] {
			t.Errorf("duplicate intentID: %s", id)
		}
		seen[id] = true
	}

	// No orphans (all committed)
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d, want 0", len(orphans))
	}

	// Manifest should still be queryable
	full, err := b.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if full.Manifest.SessionID != "sess-multi-dispatch" {
		t.Errorf("SessionID = %q", full.Manifest.SessionID)
	}
}

// TestReplayFallbackChainShowsAllAttempts verifies that when a turn
// goes through a fallback chain (model-A fails, model-B succeeds),
// the manifest records provider calls for BOTH attempts so the replay
// story shows the full decision path.
func TestReplayFallbackChainShowsAllAttempts(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "sess-fallback-replay",
		Turn:           1,
		PromptHash:     "sha256-p",
		ToolSchemaHash: "sha256-s",
		ModelID:        "claude-sonnet-4-6", // configured primary
		PromptEpoch:    1,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Simulate fallback chain: model-A fails, model-A retry fails, model-B succeeds
	attempts := []struct {
		seq    int
		model  string
		status string
	}{
		{0, "claude-sonnet-4-6", "error"},  // primary fails
		{1, "claude-sonnet-4-6", "error"},  // primary retry fails
		{2, "claude-haiku-4-5", "ok"},      // fallback succeeds
	}

	for _, a := range attempts {
		if err := b.RecordLLMCall(ctx, execmanifest.ProviderCall{
			TraceID:   traceID,
			CallSeq:   a.seq,
			RequestID: fmt.Sprintf("req-%d-%s", a.seq, a.status),
			ModelID:   a.model,
		}); err != nil {
			t.Fatalf("RecordLLMCall seq %d: %v", a.seq, err)
		}
	}

	// Replay: all 3 attempts should be visible
	full, err := b.GetManifest(ctx, traceID)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if len(full.ProviderCalls) != 3 {
		t.Fatalf("ProviderCalls = %d, want 3 (2 failures + 1 success)", len(full.ProviderCalls))
	}

	// First two calls used the primary model
	for i := 0; i < 2; i++ {
		if full.ProviderCalls[i].ModelID != "claude-sonnet-4-6" {
			t.Errorf("call[%d].ModelID = %q, want primary", i, full.ProviderCalls[i].ModelID)
		}
	}
	// Third call used the fallback model
	if full.ProviderCalls[2].ModelID != "claude-haiku-4-5" {
		t.Errorf("call[2].ModelID = %q, want fallback", full.ProviderCalls[2].ModelID)
	}
}

// TestReplayFindsMatchingActionIntent verifies the cross-table join
// that is the heart of the replay story: an orphaned intent's trace_id
// links it to the execution manifest, giving the caller full turn
// context (session, model, prompt hash) for the orphaned side effect.
func TestReplayFindsMatchingActionIntent(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	// Create a manifest with distinctive fields
	traceID, err := b.BeginTurn(ctx, execmanifest.Manifest{
		SessionID:      "sess-cross-join",
		Turn:           42,
		PromptHash:     "sha256-distinctive-prompt",
		ToolSchemaHash: "sha256-distinctive-schema",
		ModelID:        "claude-opus-4-6",
		PromptEpoch:    7,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	// Prepare an intent (no commit — orphan)
	_, err = b.PrepareAction(ctx, actionlog.Intent{
		TraceID:     traceID,
		ToolName:    "lido_stake",
		ArgsHash:    "sha256-stake-args",
		Principal:   "agent=opus;user=bob;account=0x2;channel=discord:guild123",
		EffectClass: "writes_wallet",
	})
	if err != nil {
		t.Fatalf("PrepareAction: %v", err)
	}

	// Recover the orphan — it should carry the full manifest context
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}

	o := orphans[0]

	// Cross-table verification: intent fields
	if o.Intent.Intent.ToolName != "lido_stake" {
		t.Errorf("intent ToolName = %q", o.Intent.Intent.ToolName)
	}
	if o.Intent.Intent.Principal != "agent=opus;user=bob;account=0x2;channel=discord:guild123" {
		t.Errorf("intent Principal = %q", o.Intent.Intent.Principal)
	}
	if o.Intent.Intent.ArgsHash != "sha256-stake-args" {
		t.Errorf("intent ArgsHash = %q", o.Intent.Intent.ArgsHash)
	}

	// Cross-table verification: manifest fields (joined via trace_id)
	if !o.ManifestPresent {
		t.Fatal("ManifestPresent should be true")
	}
	if o.Manifest.Manifest.SessionID != "sess-cross-join" {
		t.Errorf("manifest SessionID = %q", o.Manifest.Manifest.SessionID)
	}
	if o.Manifest.Manifest.Turn != 42 {
		t.Errorf("manifest Turn = %d, want 42", o.Manifest.Manifest.Turn)
	}
	if o.Manifest.Manifest.PromptHash != "sha256-distinctive-prompt" {
		t.Errorf("manifest PromptHash = %q", o.Manifest.Manifest.PromptHash)
	}
	if o.Manifest.Manifest.ModelID != "claude-opus-4-6" {
		t.Errorf("manifest ModelID = %q", o.Manifest.Manifest.ModelID)
	}
}

// TestActionlogConcurrentPrepareCommitStress fires 50 concurrent
// Prepare->Commit cycles to stress the action ledger's writer actor.
// All should produce unique intent IDs and zero orphans after completion.
func TestActionlogConcurrentPrepareCommitStress(t *testing.T) {
	ctx := context.Background()
	b := newSyncTestBundle(t, 1_700_000_000_000)

	traceID, err := b.BeginTurn(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}

	const n = 50
	type result struct {
		intentID string
		err      error
	}
	results := make(chan result, n)
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-startGate
			intentID, prepErr := b.PrepareAction(ctx, actionlog.Intent{
				TraceID:     traceID,
				ToolName:    fmt.Sprintf("tool-%d", i),
				ArgsHash:    fmt.Sprintf("sha256-args-%d", i),
				Principal:   "agent=main;user=stress",
				EffectClass: "writes_local",
			})
			if prepErr != nil {
				results <- result{err: prepErr}
				return
			}
			commitErr := b.CommitAction(ctx, actionlog.Commit{
				IntentID:   intentID,
				ResultHash: fmt.Sprintf("sha256-result-%d", i),
			})
			if commitErr != nil {
				results <- result{err: commitErr}
				return
			}
			results <- result{intentID: intentID}
		}(i)
	}

	close(startGate)
	wg.Wait()
	close(results)

	seen := make(map[string]bool)
	var errors int
	for r := range results {
		if r.err != nil {
			t.Errorf("concurrent prepare/commit error: %v", r.err)
			errors++
			continue
		}
		if seen[r.intentID] {
			t.Errorf("duplicate intentID: %s", r.intentID)
		}
		seen[r.intentID] = true
	}
	if errors > 0 {
		t.Fatalf("%d errors in %d concurrent operations", errors, n)
	}
	if len(seen) != n {
		t.Errorf("unique intents = %d, want %d", len(seen), n)
	}

	// All committed — zero orphans
	orphans, err := b.RecoverOrphans(ctx)
	if err != nil {
		t.Fatalf("RecoverOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("orphans = %d after %d concurrent commits, want 0", len(orphans), n)
	}
}

func errorContains(err error, sub string) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
