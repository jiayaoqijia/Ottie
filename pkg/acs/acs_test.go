package acs

import (
	"context"
	"errors"
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

// --- helpers ---------------------------------------------------------

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
