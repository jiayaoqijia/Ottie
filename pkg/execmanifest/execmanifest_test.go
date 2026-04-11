package execmanifest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- test fixtures ---------------------------------------------------

func newTestStore(t *testing.T, fixedMs int64) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "manifest.db")
	s, err := Open(Options{
		DBPath:          dbPath,
		WriteQueueDepth: 8,
		NowFn:           func() time.Time { return time.UnixMilli(fixedMs) },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func sampleManifest() Manifest {
	return Manifest{
		SessionID:      "sess-happy",
		Turn:           1,
		PromptHash:     "sha256-prompt-abc",
		ToolSchemaHash: "sha256-schema-def",
		SkillHashes:    []string{"sha256-skill-1", "sha256-skill-2"},
		McpVersions:    map[string]string{"lido-mcp": "v1.2.3"},
		ModelID:        "claude-sonnet-4-6",
		PromptEpoch:    42,
	}
}

// --- Happy path -----------------------------------------------------

// TestBeginAndGet is the canonical round-trip: create a manifest
// row, retrieve it via Get, and verify every field survived exactly.
func TestBeginAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if !strings.HasPrefix(id, "trc-") {
		t.Fatalf("generated id = %q, want trc- prefix", id)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := sampleManifest()
	want.TraceID = id
	if got.Manifest.TraceID != want.TraceID {
		t.Errorf("TraceID = %q, want %q", got.Manifest.TraceID, want.TraceID)
	}
	if got.Manifest.SessionID != want.SessionID {
		t.Errorf("SessionID = %q, want %q", got.Manifest.SessionID, want.SessionID)
	}
	if got.Manifest.Turn != want.Turn {
		t.Errorf("Turn = %d, want %d", got.Manifest.Turn, want.Turn)
	}
	if got.Manifest.PromptHash != want.PromptHash {
		t.Errorf("PromptHash = %q, want %q", got.Manifest.PromptHash, want.PromptHash)
	}
	if got.Manifest.ToolSchemaHash != want.ToolSchemaHash {
		t.Errorf("ToolSchemaHash = %q, want %q", got.Manifest.ToolSchemaHash, want.ToolSchemaHash)
	}
	if len(got.Manifest.SkillHashes) != 2 ||
		got.Manifest.SkillHashes[0] != "sha256-skill-1" ||
		got.Manifest.SkillHashes[1] != "sha256-skill-2" {
		t.Errorf("SkillHashes = %v", got.Manifest.SkillHashes)
	}
	if got.Manifest.McpVersions["lido-mcp"] != "v1.2.3" {
		t.Errorf("McpVersions[lido-mcp] = %q, want v1.2.3", got.Manifest.McpVersions["lido-mcp"])
	}
	if got.Manifest.ModelID != want.ModelID {
		t.Errorf("ModelID = %q, want %q", got.Manifest.ModelID, want.ModelID)
	}
	if got.Manifest.PromptEpoch != want.PromptEpoch {
		t.Errorf("PromptEpoch = %d, want %d", got.Manifest.PromptEpoch, want.PromptEpoch)
	}
	if got.CreatedAt.UnixMilli() != 1_700_000_000_000 {
		t.Errorf("CreatedAt = %v, want 1700000000000", got.CreatedAt.UnixMilli())
	}
	if got.ProviderCalls == nil {
		t.Error("ProviderCalls should be non-nil empty slice, not nil")
	}
	if len(got.ProviderCalls) != 0 {
		t.Errorf("ProviderCalls = %v, want empty", got.ProviderCalls)
	}
}

// TestBeginWithCallerSuppliedID verifies the caller-bring-your-own-id
// path. The store should accept the supplied value verbatim.
func TestBeginWithCallerSuppliedID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	m.TraceID = "trc-caller-supplied-123"
	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if id != "trc-caller-supplied-123" {
		t.Fatalf("id = %q, want trc-caller-supplied-123", id)
	}
}

// TestRecordProviderCallAndAggregate exercises the append-only
// provider calls: after Begin, record three calls, then Get should
// return all three in call_seq order.
func TestRecordProviderCallAndAggregate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	calls := []ProviderCall{
		{TraceID: id, CallSeq: 0, RequestID: "req-abc-001", ModelID: "claude-sonnet-4-6"},
		{TraceID: id, CallSeq: 1, RequestID: "req-abc-002", ModelID: "claude-sonnet-4-6"},
		{TraceID: id, CallSeq: 2, RequestID: "req-abc-003", ModelID: "claude-opus-4-6"}, // fallback
	}
	for _, c := range calls {
		if err := s.RecordProviderCall(ctx, c); err != nil {
			t.Fatalf("RecordProviderCall %d: %v", c.CallSeq, err)
		}
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ProviderCalls) != 3 {
		t.Fatalf("ProviderCalls len = %d, want 3", len(got.ProviderCalls))
	}
	for i, c := range got.ProviderCalls {
		if c.CallSeq != i {
			t.Errorf("call %d: CallSeq = %d, want %d", i, c.CallSeq, i)
		}
		if c.RequestID != calls[i].RequestID {
			t.Errorf("call %d: RequestID = %q, want %q", i, c.RequestID, calls[i].RequestID)
		}
		if c.ModelID != calls[i].ModelID {
			t.Errorf("call %d: ModelID = %q, want %q", i, c.ModelID, calls[i].ModelID)
		}
	}
}

// TestGetBySessionTurn verifies the alternate-key lookup path.
func TestGetBySessionTurn(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	m.SessionID = "sess-lookup"
	m.Turn = 7
	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	got, err := s.GetBySessionTurn(ctx, "sess-lookup", 7)
	if err != nil {
		t.Fatalf("GetBySessionTurn: %v", err)
	}
	if got.Manifest.TraceID != id {
		t.Errorf("TraceID = %q, want %q", got.Manifest.TraceID, id)
	}
}

// TestListBySessionInTurnOrder verifies ListBySession returns
// every turn in ascending order.
func TestListBySessionInTurnOrder(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	// Insert turns in non-monotonic order to confirm the ORDER BY
	// is doing the work.
	for _, turn := range []int{3, 1, 5, 2, 4} {
		m := sampleManifest()
		m.SessionID = "sess-ordered"
		m.Turn = turn
		if _, err := s.Begin(ctx, m); err != nil {
			t.Fatalf("Begin turn %d: %v", turn, err)
		}
	}

	list, err := s.ListBySession(ctx, "sess-ordered")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("len = %d, want 5", len(list))
	}
	for i, want := range []int{1, 2, 3, 4, 5} {
		if list[i].Manifest.Turn != want {
			t.Errorf("list[%d].Turn = %d, want %d", i, list[i].Manifest.Turn, want)
		}
	}
}

// --- Unhappy path ---------------------------------------------------

// TestBeginRejectsMissingRequiredFields walks every required field
// on Manifest and asserts the specific error message.
func TestBeginRejectsMissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	cases := []struct {
		name    string
		mutate  func(*Manifest)
		wantMsg string
	}{
		{"missing SessionID", func(m *Manifest) { m.SessionID = "" }, "SessionID required"},
		{"missing PromptHash", func(m *Manifest) { m.PromptHash = "" }, "PromptHash required"},
		{"missing ToolSchemaHash", func(m *Manifest) { m.ToolSchemaHash = "" }, "ToolSchemaHash required"},
		{"missing ModelID", func(m *Manifest) { m.ModelID = "" }, "ModelID required"},
		{"negative Turn", func(m *Manifest) { m.Turn = -1 }, "Turn must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := sampleManifest()
			tc.mutate(&m)
			_, err := s.Begin(ctx, m)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestRecordProviderCallRejectsMissingRequiredFields mirrors the
// Begin test for the provider-call required fields.
func TestRecordProviderCallRejectsMissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	cases := []struct {
		name string
		call ProviderCall
		want string
	}{
		{"missing TraceID", ProviderCall{TraceID: "", RequestID: "r1", CallSeq: 0}, "TraceID required"},
		{"missing RequestID", ProviderCall{TraceID: id, RequestID: "", CallSeq: 0}, "RequestID required"},
		{"negative CallSeq", ProviderCall{TraceID: id, RequestID: "r1", CallSeq: -1}, "CallSeq must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.RecordProviderCall(ctx, tc.call)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestDoubleBeginOnSameTraceIDReturnsAlreadyRecorded covers the
// PRIMARY KEY / UNIQUE constraint handling.
func TestDoubleBeginOnSameTraceIDReturnsAlreadyRecorded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	m.TraceID = "trc-dup"
	if _, err := s.Begin(ctx, m); err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	_, err := s.Begin(ctx, m)
	if !errors.Is(err, ErrTraceAlreadyRecorded) {
		t.Fatalf("err = %v, want ErrTraceAlreadyRecorded", err)
	}
}

// TestDoubleBeginOnSameSessionTurnReturnsAlreadyRecorded — the
// other half of the uniqueness contract.
func TestDoubleBeginOnSameSessionTurnReturnsAlreadyRecorded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m1 := sampleManifest()
	m1.SessionID = "sess-dup"
	m1.Turn = 5
	if _, err := s.Begin(ctx, m1); err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	// Same session+turn, different (auto-generated) trace_id.
	m2 := sampleManifest()
	m2.SessionID = "sess-dup"
	m2.Turn = 5
	_, err := s.Begin(ctx, m2)
	if !errors.Is(err, ErrTraceAlreadyRecorded) {
		t.Fatalf("err = %v, want ErrTraceAlreadyRecorded", err)
	}
}

// TestRecordProviderCallOnUnknownTraceReturnsNotFound verifies the
// FK violation path.
func TestRecordProviderCallOnUnknownTraceReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	err := s.RecordProviderCall(ctx, ProviderCall{
		TraceID: "trc-nope", CallSeq: 0, RequestID: "req-1",
	})
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("err = %v, want ErrTraceNotFound", err)
	}
}

// TestDuplicateCallSeqReturnsCallAlreadyRecorded — the
// (trace_id, call_seq) PK uniqueness guard.
func TestDuplicateCallSeqReturnsCallAlreadyRecorded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.RecordProviderCall(ctx, ProviderCall{TraceID: id, CallSeq: 0, RequestID: "r1"}); err != nil {
		t.Fatalf("first RecordProviderCall: %v", err)
	}
	err = s.RecordProviderCall(ctx, ProviderCall{TraceID: id, CallSeq: 0, RequestID: "r1-repeat"})
	if !errors.Is(err, ErrCallAlreadyRecorded) {
		t.Fatalf("err = %v, want ErrCallAlreadyRecorded", err)
	}
}

// TestGetUnknownTraceReturnsNotFound — query side.
func TestGetUnknownTraceReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	_, err := s.Get(ctx, "trc-nope")
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("err = %v, want ErrTraceNotFound", err)
	}
}

// TestGetBySessionTurnUnknownReturnsNotFound
func TestGetBySessionTurnUnknownReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	_, err := s.GetBySessionTurn(ctx, "sess-nope", 0)
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("err = %v, want ErrTraceNotFound", err)
	}
}

// TestOpsAfterCloseFail verifies every public method returns
// ErrClosed after Close (no panics, no hangs). Exercises the
// shutdown fence across reads and writes.
func TestOpsAfterCloseFail(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if _, err := s.Begin(ctx, sampleManifest()); !errors.Is(err, ErrClosed) {
		t.Errorf("Begin after Close: %v, want ErrClosed", err)
	}
	if err := s.RecordProviderCall(ctx, ProviderCall{TraceID: "x", CallSeq: 0, RequestID: "r"}); !errors.Is(err, ErrClosed) {
		t.Errorf("RecordProviderCall after Close: %v, want ErrClosed", err)
	}
	if _, err := s.Get(ctx, "x"); !errors.Is(err, ErrClosed) {
		t.Errorf("Get after Close: %v, want ErrClosed", err)
	}
	if _, err := s.GetBySessionTurn(ctx, "x", 0); !errors.Is(err, ErrClosed) {
		t.Errorf("GetBySessionTurn after Close: %v, want ErrClosed", err)
	}
	if _, err := s.ListBySession(ctx, "x"); !errors.Is(err, ErrClosed) {
		t.Errorf("ListBySession after Close: %v, want ErrClosed", err)
	}
}

// --- Corner cases ---------------------------------------------------

// TestConcurrentBeginsSerializeAndUnique exercises the writer actor:
// 50 goroutines, each with a distinct (session_id, turn), all
// should succeed with distinct trace_ids.
func TestConcurrentBeginsSerializeAndUnique(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	const n = 50
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			m := sampleManifest()
			m.SessionID = fmt.Sprintf("sess-%d", i)
			m.Turn = i
			id, err := s.Begin(ctx, m)
			ids[i] = id
			errs[i] = err
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
			continue
		}
		if seen[ids[i]] {
			t.Errorf("duplicate trace_id: %s", ids[i])
		}
		seen[ids[i]] = true
	}
	if len(seen) != n {
		t.Errorf("unique ids = %d, want %d", len(seen), n)
	}
}

// TestEmptyListBySessionReturnsNonNilSlice verifies the empty-
// result case does not return nil — JSON encoders differentiate
// null from [].
func TestEmptyListBySessionReturnsNonNilSlice(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	list, err := s.ListBySession(ctx, "sess-empty")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if list == nil {
		t.Fatal("list is nil; want non-nil empty slice")
	}
	if len(list) != 0 {
		t.Fatalf("len = %d, want 0", len(list))
	}
}

// TestEmptyProviderCallsReturnNonNilSlice exercises the per-trace
// empty case: a Begin with no RecordProviderCall should yield a
// FullManifest whose ProviderCalls is non-nil empty.
func TestEmptyProviderCallsReturnNonNilSlice(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProviderCalls == nil {
		t.Fatal("ProviderCalls is nil; want non-nil empty slice")
	}
}

// TestNilSkillHashesAndMcpVersionsStoreAsEmpty exercises the JSON
// canonicalization: nil slice → `[]`, nil map → `{}`. The decoder
// reconstructs them as non-nil empty collections.
func TestNilSkillHashesAndMcpVersionsStoreAsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	m.SkillHashes = nil
	m.McpVersions = nil
	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Manifest.SkillHashes == nil {
		t.Error("SkillHashes should be non-nil empty slice")
	}
	if len(got.Manifest.SkillHashes) != 0 {
		t.Errorf("SkillHashes = %v, want empty", got.Manifest.SkillHashes)
	}
	if got.Manifest.McpVersions == nil {
		t.Error("McpVersions should be non-nil empty map")
	}
	if len(got.Manifest.McpVersions) != 0 {
		t.Errorf("McpVersions = %v, want empty", got.Manifest.McpVersions)
	}
}

// TestConcurrentCloseAndProducers stresses the shutdown fence: a
// set of producers calls Begin in a loop while another goroutine
// calls Close. Every result must be nil (accepted before Close) or
// ErrClosed (rejected after Close). No panics.
func TestConcurrentCloseAndProducers(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifest.db")
	s, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 16})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const producers = 20
	const pushesPerProducer = 10
	results := make(chan error, producers*pushesPerProducer)
	var wg sync.WaitGroup
	wg.Add(producers)

	// Each producer pushes with a unique (session_id, turn) so we
	// don't collide on the UNIQUE constraint.
	for i := 0; i < producers; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < pushesPerProducer; j++ {
				m := sampleManifest()
				m.SessionID = fmt.Sprintf("sess-race-%d", i)
				m.Turn = j
				_, err := s.Begin(ctx, m)
				results <- err
			}
		}(i)
	}

	time.Sleep(5 * time.Millisecond)
	closeErr := s.Close()
	wg.Wait()
	close(results)

	if closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	var accepted, rejected int
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrClosed):
			rejected++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if accepted+rejected != producers*pushesPerProducer {
		t.Fatalf("accepted=%d rejected=%d want total=%d",
			accepted, rejected, producers*pushesPerProducer)
	}
	t.Logf("accepted=%d rejected=%d", accepted, rejected)
}

// --- Edge cases -----------------------------------------------------

// TestDurabilityAcrossReopen proves the write-ahead property: a
// Begin'd manifest survives a clean Close and reopen of the DB.
func TestDurabilityAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifest.db")

	s1, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	id, err := s1.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s1.RecordProviderCall(ctx, ProviderCall{
		TraceID: id, CallSeq: 0, RequestID: "req-survive",
	}); err != nil {
		t.Fatalf("RecordProviderCall: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Manifest.PromptHash != "sha256-prompt-abc" {
		t.Errorf("PromptHash = %q, want sha256-prompt-abc", got.Manifest.PromptHash)
	}
	if len(got.ProviderCalls) != 1 {
		t.Fatalf("ProviderCalls len = %d, want 1", len(got.ProviderCalls))
	}
	if got.ProviderCalls[0].RequestID != "req-survive" {
		t.Errorf("RequestID = %q, want req-survive", got.ProviderCalls[0].RequestID)
	}
}

// TestPragmaStateIsActive asserts WAL + FULL + foreign_keys are
// actually applied at the connection level. If any silently fails
// the durability story is a lie, so this is load-bearing.
func TestPragmaStateIsActive(t *testing.T) {
	s := newTestStore(t, 1_700_000_000_000)

	var journal string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var sync int
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatalf("PRAGMA synchronous: %v", err)
	}
	if sync != 2 {
		t.Errorf("synchronous = %d, want 2 (FULL)", sync)
	}

	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

// TestLargeSkillHashesRoundTrip exercises the JSON column against a
// large payload — 500 hashes per turn, plausible for an agent with
// a sizeable skill set.
func TestLargeSkillHashesRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	big := make([]string, 500)
	for i := range big {
		big[i] = fmt.Sprintf("sha256-skill-%04d", i)
	}
	m.SkillHashes = big

	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Manifest.SkillHashes) != 500 {
		t.Fatalf("SkillHashes len = %d, want 500", len(got.Manifest.SkillHashes))
	}
	for i, h := range got.Manifest.SkillHashes {
		if h != big[i] {
			t.Fatalf("SkillHashes[%d] = %q, want %q", i, h, big[i])
		}
	}
}

// TestSchemaIdempotency — open, write, close, open again — the
// existing data must survive.
func TestSchemaIdempotency(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifest.db")

	s1, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	id, err := s1.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	_ = s1.Close()

	s2, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 1})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	got, err := s2.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Manifest.TraceID != id {
		t.Errorf("TraceID = %q, want %q", got.Manifest.TraceID, id)
	}
}

// TestErrorMessageIncludesPackageName — UX guard: every returned
// error should be greppable via "execmanifest:".
func TestErrorMessageIncludesPackageName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	_, err := s.Begin(ctx, Manifest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "execmanifest:") {
		t.Fatalf("err = %q, want 'execmanifest:' prefix", err.Error())
	}
}

// --- R10 regression tests (added in response to codex R10) ---------

// TestScanErrorAttributionByOp verifies codex R10 finding #2: the
// scanner helpers hardcoded "Get:" in error messages so a
// GetBySessionTurn miss was labeled as "execmanifest: Get: trace
// not found". Now the op is threaded through and the error prefix
// matches the actual caller.
func TestScanErrorAttributionByOp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	_, err := s.Get(ctx, "trc-does-not-exist")
	if err == nil {
		t.Fatal("Get: expected error")
	}
	if !strings.Contains(err.Error(), "execmanifest: Get:") {
		t.Errorf("Get error = %q, want 'execmanifest: Get:' prefix", err.Error())
	}

	_, err = s.GetBySessionTurn(ctx, "sess-nope", 0)
	if err == nil {
		t.Fatal("GetBySessionTurn: expected error")
	}
	if !strings.Contains(err.Error(), "execmanifest: GetBySessionTurn:") {
		t.Errorf("GetBySessionTurn error = %q, want 'execmanifest: GetBySessionTurn:' prefix", err.Error())
	}
	if strings.Contains(err.Error(), "execmanifest: Get:") {
		t.Errorf("GetBySessionTurn error leaked Get: prefix: %q", err.Error())
	}
}

// TestCorruptEmptyJSONColumnRejected verifies codex R10 finding #3:
// the previous implementation silently coerced empty-string JSON
// columns to empty collections, masking a real corruption class.
// Now an empty-string column returns ErrCorruptJSON on decode. We
// simulate corruption by updating the row directly via the DB
// handle, bypassing the canonical writer.
func TestCorruptEmptyJSONColumnRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Corrupt skill_hashes directly.
	if _, err := s.db.Exec(`UPDATE traces SET skill_hashes = '' WHERE trace_id = ?`, id); err != nil {
		t.Fatalf("UPDATE traces: %v", err)
	}
	_, err = s.Get(ctx, id)
	if !errors.Is(err, ErrCorruptJSON) {
		t.Fatalf("Get after skill_hashes corruption err = %v, want ErrCorruptJSON", err)
	}

	// Restore skill_hashes to []; corrupt mcp_versions.
	if _, err := s.db.Exec(`UPDATE traces SET skill_hashes = '[]', mcp_versions = '' WHERE trace_id = ?`, id); err != nil {
		t.Fatalf("UPDATE traces: %v", err)
	}
	_, err = s.Get(ctx, id)
	if !errors.Is(err, ErrCorruptJSON) {
		t.Fatalf("Get after mcp_versions corruption err = %v, want ErrCorruptJSON", err)
	}
}

// TestConcurrentReadersAndClose verifies codex R10 finding #1: the
// read-vs-Close TOCTOU. Without the guardedRead fence, a reader
// that got past isClosedLocked() could race Close and surface raw
// "sql: database is closed" errors. The fix holds state.RLock
// across the entire read path so Close blocks until all in-flight
// readers finish.
func TestConcurrentReadersAndClose(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "manifest.db")
	s, err := Open(Options{DBPath: dbPath, WriteQueueDepth: 4})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Seed a manifest so readers have something to read.
	m := sampleManifest()
	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	const readers = 30
	const readsPerGoroutine = 20
	results := make(chan error, readers*readsPerGoroutine)
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				_, err := s.Get(ctx, id)
				results <- err
			}
		}()
	}

	time.Sleep(3 * time.Millisecond)
	closeErr := s.Close()
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
		default:
			t.Errorf("unexpected read error (not nil and not ErrClosed): %v", err)
		}
	}
	if ok+closed != readers*readsPerGoroutine {
		t.Fatalf("ok=%d closed=%d want total=%d", ok, closed, readers*readsPerGoroutine)
	}
	t.Logf("ok=%d closed=%d", ok, closed)
}

// TestListBySessionHydratesProviderCalls verifies codex R10
// finding #4: the existing list test only checked ordering, not
// that each returned FullManifest had its ProviderCalls filled.
// Set up a session with three turns that have varying numbers of
// provider calls (0, 2, 3), then verify ListBySession returns
// the right hydrated shape for each.
func TestListBySessionHydratesProviderCalls(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	const sess = "sess-hydrate"

	// Turn 1: zero provider calls.
	m1 := sampleManifest()
	m1.SessionID = sess
	m1.Turn = 1
	id1, err := s.Begin(ctx, m1)
	if err != nil {
		t.Fatalf("Begin turn 1: %v", err)
	}

	// Turn 2: two provider calls.
	m2 := sampleManifest()
	m2.SessionID = sess
	m2.Turn = 2
	id2, err := s.Begin(ctx, m2)
	if err != nil {
		t.Fatalf("Begin turn 2: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.RecordProviderCall(ctx, ProviderCall{
			TraceID: id2, CallSeq: i, RequestID: fmt.Sprintf("req-t2-%d", i),
		}); err != nil {
			t.Fatalf("RecordProviderCall turn 2 call %d: %v", i, err)
		}
	}

	// Turn 3: three provider calls.
	m3 := sampleManifest()
	m3.SessionID = sess
	m3.Turn = 3
	id3, err := s.Begin(ctx, m3)
	if err != nil {
		t.Fatalf("Begin turn 3: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.RecordProviderCall(ctx, ProviderCall{
			TraceID: id3, CallSeq: i, RequestID: fmt.Sprintf("req-t3-%d", i),
		}); err != nil {
			t.Fatalf("RecordProviderCall turn 3 call %d: %v", i, err)
		}
	}

	list, err := s.ListBySession(ctx, sess)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3", len(list))
	}

	// Turn 1 — zero calls, non-nil empty slice.
	if list[0].Manifest.Turn != 1 {
		t.Errorf("list[0].Turn = %d, want 1", list[0].Manifest.Turn)
	}
	if list[0].Manifest.TraceID != id1 {
		t.Errorf("list[0].TraceID = %q, want %q", list[0].Manifest.TraceID, id1)
	}
	if list[0].ProviderCalls == nil {
		t.Error("list[0].ProviderCalls should be non-nil empty slice")
	}
	if len(list[0].ProviderCalls) != 0 {
		t.Errorf("list[0].ProviderCalls len = %d, want 0", len(list[0].ProviderCalls))
	}

	// Turn 2 — two calls, ordered.
	if list[1].Manifest.Turn != 2 {
		t.Errorf("list[1].Turn = %d, want 2", list[1].Manifest.Turn)
	}
	if len(list[1].ProviderCalls) != 2 {
		t.Fatalf("list[1].ProviderCalls len = %d, want 2", len(list[1].ProviderCalls))
	}
	if list[1].ProviderCalls[0].RequestID != "req-t2-0" {
		t.Errorf("list[1][0].RequestID = %q, want req-t2-0", list[1].ProviderCalls[0].RequestID)
	}
	if list[1].ProviderCalls[1].RequestID != "req-t2-1" {
		t.Errorf("list[1][1].RequestID = %q, want req-t2-1", list[1].ProviderCalls[1].RequestID)
	}

	// Turn 3 — three calls, ordered.
	if list[2].Manifest.Turn != 3 {
		t.Errorf("list[2].Turn = %d, want 3", list[2].Manifest.Turn)
	}
	if len(list[2].ProviderCalls) != 3 {
		t.Fatalf("list[2].ProviderCalls len = %d, want 3", len(list[2].ProviderCalls))
	}
	for i := 0; i < 3; i++ {
		want := fmt.Sprintf("req-t3-%d", i)
		if list[2].ProviderCalls[i].RequestID != want {
			t.Errorf("list[2][%d].RequestID = %q, want %q", i, list[2].ProviderCalls[i].RequestID, want)
		}
	}
}

// TestRawStorageShapeOfCanonicalJSON verifies that nil slice/map
// inputs produce on-disk `[]`/`{}` JSON literals, not nil or empty
// string. This is the load-bearing invariant behind
// canonicalJSONArray/canonicalJSONMap — if the writer emits `""`
// or NULL, the R10 ErrCorruptJSON check would fire on every read.
func TestRawStorageShapeOfCanonicalJSON(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	m := sampleManifest()
	m.SkillHashes = nil
	m.McpVersions = nil
	id, err := s.Begin(ctx, m)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	var skillRaw, mcpRaw string
	err = s.db.QueryRow(
		`SELECT skill_hashes, mcp_versions FROM traces WHERE trace_id = ?`,
		id,
	).Scan(&skillRaw, &mcpRaw)
	if err != nil {
		t.Fatalf("direct SELECT: %v", err)
	}
	if skillRaw != "[]" {
		t.Errorf("skill_hashes on disk = %q, want '[]'", skillRaw)
	}
	if mcpRaw != "{}" {
		t.Errorf("mcp_versions on disk = %q, want '{}'", mcpRaw)
	}
}

// --- (original tests continue below) -------------------------------

// TestProviderCallModelIDNullable — empty ModelID on a provider
// call should store as SQL NULL, not the empty string, and come
// back as empty-string on read via COALESCE.
func TestProviderCallModelIDNullable(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, 1_700_000_000_000)

	id, err := s.Begin(ctx, sampleManifest())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.RecordProviderCall(ctx, ProviderCall{
		TraceID: id, CallSeq: 0, RequestID: "r1", // ModelID left empty
	}); err != nil {
		t.Fatalf("RecordProviderCall: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ProviderCalls) != 1 {
		t.Fatalf("calls len = %d, want 1", len(got.ProviderCalls))
	}
	if got.ProviderCalls[0].ModelID != "" {
		t.Errorf("ModelID = %q, want empty", got.ProviderCalls[0].ModelID)
	}
}
