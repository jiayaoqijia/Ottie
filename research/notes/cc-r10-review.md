# CC R10 Code-Level Acceptance Review — pkg/execmanifest

> Reviewer persona: 30-year AI/CS veteran, same as R2-R9.
> Framing: acceptance review of pkg/execmanifest, the R6/R7 §4.4
> per-turn execution manifest. Third and final leg of the R6
> replay story (alongside pkg/principal R8 and pkg/actionlog R9).
> Coverage rule applied: happy + unhappy + corner + edge per
> subsystem. Date: 2026-04-11.

## Opening

pkg/execmanifest delivers the R6/R7 §4.4 execution-manifest
contract correctly and mirrors the pkg/actionlog pattern faithfully.
Every R9 codex fix is translated through (typed SQLite errors,
URL-escaped DBPath, RWMutex state fence, non-nil empty slice
returns, `errors.As(*mcsqlite.Error)`). The split of
`provider_request_ids` into a separate append-only
`trace_provider_requests` table is the correct choice for
incremental durability — a single JSON column on `traces` would
require post-hoc mutation that breaks the immutability invariant.

One legitimate amendment worth landing in the same commit, two
nice-to-have observations for a follow-up, and a test-coverage gap
I want codex R10 to confirm is actually a gap before we commit.

Verdict: **READY WITH AMENDMENTS** (1 load-bearing, 2 nice-to-have).

## pkg/execmanifest review

### Schema
- **Happy**: Two tables, column-for-column match with R6/R7 §4.1 for
  the `traces` table: `trace_id`, `session_id`, `turn`,
  `prompt_hash`, `tool_schema_hash`, `skill_hashes` (JSON),
  `mcp_versions` (JSON), `model_id`, `prompt_epoch`, `created_at`
  (`execmanifest.go:216-226`). The `UNIQUE(session_id, turn)`
  constraint (`execmanifest.go:226`) enforces the R6 invariant that
  a (session, turn) pair is recorded at most once.
  `trace_provider_requests` has `(trace_id, call_seq)` as composite
  PK (`execmanifest.go:238`) with a FK to `traces` for referential
  integrity.
- **Unhappy**: NOT NULL constraints backstop the Go-side validation
  on Begin. A row with a missing required field fails at the SQL
  layer even if the caller bypasses the Go validation. Good.
- **Corner**: Three indexes (`idx_traces_session`,
  `idx_traces_prompt_epoch`, `idx_traces_created_at`) cover the
  three most-likely audit query patterns (per-session, per-epoch
  replay lineage, recent-activity scans). No index on
  `trace_provider_requests.trace_id` — but the composite PK on
  `(trace_id, call_seq)` means SQLite automatically supports a
  range scan on `trace_id` since that's the leading column. Good.
- **Edge**: No CHECK constraint on `call_seq >= 0` at the schema
  level. The Go-side validation catches this
  (`execmanifest.go:385-387`), but a SQL-level CHECK would be
  belt-and-braces. **Not blocking** — the Go check is sufficient
  as long as every write goes through the public API.
- **Issue**: None load-bearing.

### Open / buildDSN / initSchema
- **Happy**: `Open` rejects empty `DBPath`, builds a pragma-bearing
  DSN via `url.URL`, caps pool at 1, inits schema, starts writer
  last (`execmanifest.go:169-197`). Matches the
  pkg/actionlog.Open flow exactly.
- **Unhappy**: Phase-specific error wrapping (`execmanifest.go:183`
  for sqlite open, `:192` for schema init). Schema init failure
  closes the DB before returning.
- **Corner**: `buildDSN` uses `url.URL{Scheme, Path, RawQuery}` so
  a path containing `?`/`#`/`&` is correctly URL-encoded
  (`execmanifest.go:203-218`). This is the R9 codex fix translated
  through to execmanifest.
- **Edge**: `initSchema` is idempotent via `IF NOT EXISTS` on every
  DDL statement (`execmanifest.go:223-244`). Confirmed by
  `TestSchemaIdempotency` at `execmanifest_test.go:611`.
- **Issue**: None.

### Writer actor (drain-on-signal)
- **Happy**: `writer()` at `execmanifest.go:272-289` is the same
  drain-on-signal pattern as pkg/actionlog. Channel never closed,
  shutdown signaled via `s.closed`. Handles unknown writeKind with
  a typed error rather than panic (`execmanifest.go:294`).
- **Unhappy**: Each op's error is returned via `op.reply`. No
  goroutine leak, no stranded producers.
- **Corner**: Inner non-blocking select drains queued ops before
  exiting (`execmanifest.go:280-287`). Producers whose ops were
  already pushed get a reply before the writer terminates.
- **Edge**: Race detector clean (`go test -race -count=1` in 2.8s).
- **Issue**: None.

### Begin
- **Happy**: Generates a random 128-bit hex ID with `trc-` prefix
  when none supplied (`execmanifest.go:322`). Validates five
  required fields (`execmanifest.go:304-320`). Canonicalizes nil
  slice/map to `[]`/`{}` via `canonicalJSONArray`/
  `canonicalJSONMap` (`execmanifest.go:327-333`). Inserts the row,
  checkpoints, returns the ID.
- **Unhappy**: Each missing field returns a specific error message
  (`execmanifest.go:305-320`). All six failure modes covered by
  `TestBeginRejectsMissingRequiredFields`
  (`execmanifest_test.go:256-292`).
- **Corner**: Double-Begin on same `trace_id` → `ErrTraceAlreadyRecorded`
  via PK constraint (tested by
  `TestDoubleBeginOnSameTraceIDReturnsAlreadyRecorded`,
  `execmanifest_test.go:329`). Double-Begin on same
  `(session_id, turn)` → same error via UNIQUE constraint
  (`TestDoubleBeginOnSameSessionTurnReturnsAlreadyRecorded`,
  `execmanifest_test.go:345`).
- **Edge**: `wrapBeginError` uses `errors.As(*mcsqlite.Error)` with
  typed code comparison against both `SQLITE_CONSTRAINT_UNIQUE`
  and `SQLITE_CONSTRAINT_PRIMARYKEY` — the latter is necessary
  because the `trace_id` PK violation uses a different extended
  code than the `(session_id, turn)` UNIQUE violation
  (`execmanifest.go:498-510`). The substring fallback is kept as
  last-resort. This is tighter than pkg/actionlog's wrapping.
- **Issue**: None.

### RecordProviderCall
- **Happy**: Validates `TraceID`, `RequestID`, `CallSeq >= 0`
  (`execmanifest.go:382-396`). `ModelID` is nullable — empty
  string becomes `sql.NullString{}` (`execmanifest.go:394-397`).
  Tested by `TestProviderCallModelIDNullable`
  (`execmanifest_test.go:700`).
- **Unhappy**: FK violation → `ErrTraceNotFound`. PK violation →
  `ErrCallAlreadyRecorded`. Both tested by
  `TestRecordProviderCallOnUnknownTraceReturnsNotFound`
  (`execmanifest_test.go:376`) and
  `TestDuplicateCallSeqReturnsCallAlreadyRecorded`
  (`execmanifest_test.go:391`).
- **Corner**: 50 concurrent Begins with unique `(session, turn)`
  succeed with distinct IDs
  (`TestConcurrentBeginsSerializeAndUnique`). No equivalent
  concurrency test for RecordProviderCall on a shared trace —
  **Amendment 1 (low)**: add a test that fires N concurrent
  RecordProviderCall calls on the same trace_id with different
  call_seq values, asserts all succeed and appear in order on
  Get. The writer actor serializes, so this should work, but no
  explicit coverage.
- **Edge**: `wrapRecordCallError` covers both FK and
  UNIQUE/PRIMARY_KEY (`execmanifest.go:513-527`).
- **Issue**: Amendment 1.

### Get / GetBySessionTurn
- **Happy**: `scanTrace` decodes the traces row into a
  `FullManifest`, then `loadProviderCalls` fills in the
  `ProviderCalls` slice (`execmanifest.go:565-585`). Two-step
  reads work because `SetMaxOpenConns(1)` serializes at the
  connection level — a concurrent writer cannot interleave
  between the two queries.
- **Unhappy**: `sql.ErrNoRows` is mapped to `ErrTraceNotFound`
  (`execmanifest.go:697-698`). Missing `traceID` arg → typed
  error before hitting SQL (`execmanifest.go:569`).
- **Corner**: JSON round-trip for `skill_hashes` and
  `mcp_versions` handled by `decodeJSONArray`/`decodeJSONMap`
  (`execmanifest.go:747-766`). Empty-string column is treated as
  an empty collection, not a nil. Tested by
  `TestNilSkillHashesAndMcpVersionsStoreAsEmpty`
  (`execmanifest_test.go:521`).
- **Edge**: `TestLargeSkillHashesRoundTrip` with 500 hashes
  exercises the JSON column against a plausible high-end payload
  (`execmanifest_test.go:619`). Columns are TEXT; SQLite allows
  arbitrarily large TEXT, so no truncation concerns.
- **Issue**: None.

### ListBySession
- **Happy**: Query orders by `turn ASC` (`execmanifest.go:635-640`).
  Non-monotonic-turn input is re-ordered on read. Tested by
  `TestListBySessionInTurnOrder` (`execmanifest_test.go:238`).
- **Unhappy**: Empty `sessionID` → typed error
  (`execmanifest.go:628`). Empty result → non-nil empty slice
  (`execmanifest.go:648`). Tested by
  `TestEmptyListBySessionReturnsNonNilSlice`
  (`execmanifest_test.go:497`).
- **Corner**: **N+1 query pattern**
  (`execmanifest.go:659-666`). ListBySession runs one query for
  the traces rows, then N queries to `loadProviderCalls` — one
  per trace. For a session with 100 turns this is 101 queries.
  **Amendment 2 (medium, nice-to-have)**: replace with a single
  JOIN query that returns `traces` LEFT JOIN
  `trace_provider_requests` and groups in Go. For the expected
  use case (audit queries on a single session with at most a few
  dozen turns), the N+1 pattern is fine. For large sessions (100+
  turns), it becomes a real cost. Low priority because typical
  Ottie sessions are short.
- **Edge**: Ordering is stable within a turn because the traces
  PK provides a deterministic row identity. No need for a
  secondary ORDER BY tiebreaker (unlike RecoverOrphans in
  pkg/actionlog, where millisecond ties were a concern).
- **Issue**: Amendment 2 is polish, not blocking.

### Close (idempotency + fence)
- **Happy**: Same pattern as pkg/actionlog.Close
  (`execmanifest.go:812-832`). `state.Lock()` blocks until
  in-flight enqueues release, sets `closedFlag=true`, closes
  `s.closed`, drops the lock, waits on `s.done`, closes the DB.
- **Unhappy**: Idempotent — second Close returns nil
  (`execmanifest.go:819-821`). DB close error propagates
  (`execmanifest.go:831`).
- **Corner**: Concurrent Close calls serialized by `closeMu`
  (`execmanifest.go:815`). Race detector clean.
- **Edge**: `TestConcurrentCloseAndProducers`
  (`execmanifest_test.go:546`) fires 20 producers × 10 pushes
  each while Close races in at 5ms. Every result is nil or
  ErrClosed; no panics.
- **Issue**: None.

### Durability across reopen
- **Happy**: `TestDurabilityAcrossReopen`
  (`execmanifest_test.go:618`) runs Begin, RecordProviderCall,
  Close, Open, Get — every field including the provider call
  survives reopen.
- **Unhappy**: No crash-without-Close test (same gap as
  pkg/actionlog R9). This is a documented follow-up, not a gap
  in the current slice.
- **Corner**: `TestPragmaStateIsActive`
  (`execmanifest_test.go:656`) directly queries `journal_mode`,
  `synchronous`, `foreign_keys` and asserts they're set. This
  pins the durability claim against a silent pragma failure.
- **Edge**: `synchronous=FULL` + `wal_checkpoint(TRUNCATE)` after
  every write is the same strongest-durability stance as
  pkg/actionlog. Good.
- **Issue**: None.

### Concurrency
- **Happy**: `TestConcurrentBeginsSerializeAndUnique`
  (`execmanifest_test.go:467`) exercises 50 concurrent Begins.
  All succeed with unique IDs.
- **Unhappy**: ErrClosed steady-state covered by
  `TestOpsAfterCloseFail` (`execmanifest_test.go:421`).
- **Corner**: `TestConcurrentCloseAndProducers` covers the
  active-race case. Amendment 1 above adds concurrent
  RecordProviderCall coverage.
- **Edge**: Reads (Get/GetBySessionTurn/ListBySession) do NOT go
  through the writer actor — they hit `s.db.QueryRowContext`
  directly. Under `SetMaxOpenConns(1)`, `database/sql` serializes
  reads and writes on a shared mutex so a read cannot interleave
  with a write mid-statement. The `isClosedLocked` check
  (`execmanifest.go:806`) uses the RLock for shutdown
  consistency. **Worth verifying with codex R10** that this is
  the right design vs routing reads through the writer too.
- **Issue**: Question for codex R10 — is the read-path-
  bypasses-writer-actor pattern safe in the general case?

### Pattern fidelity vs pkg/actionlog
Every R9 codex fix I can identify is present in execmanifest:
- ✓ Typed `*mcsqlite.Error` + `errors.As` (not substring-first)
  at `execmanifest.go:498-527`.
- ✓ URL-escaped DBPath via `url.URL{Scheme,Path,RawQuery}` at
  `execmanifest.go:208-217`.
- ✓ RWMutex state fence at `execmanifest.go:151-152`, 531-543,
  812-832.
- ✓ `make([]X, 0)` instead of nil slice returns at
  `execmanifest.go:648`, 667, 684, 730, 742.
- ✓ Race-detector clean.
- ✓ Drain-on-signal shutdown without closing writeCh at
  `execmanifest.go:272-289`.

## Amendments before merge

1. **Amendment 1 (low)**: add a concurrent-RecordProviderCall
   regression test on a shared trace_id with distinct call_seqs.
   The writer actor should serialize, but no explicit coverage.
2. **Amendment 2 (medium-low, polish)**: consider replacing
   ListBySession's N+1 query pattern with a single JOIN. For
   typical session lengths (10-20 turns) the N+1 is fine; for
   long-lived gateway sessions it becomes a real cost. Not
   blocking merge.

## Follow-ups

- N/A for this slice. The three follow-ups from pkg/actionlog R9
  (schema version, crash-without-Close harness, principal
  integration) apply equally here but should be addressed for
  the whole subsystem, not just this package.

## Verdict

**READY WITH AMENDMENTS.**

Amendment 1 is worth adding in the same commit (~15 lines of test
code). Amendment 2 is observation-only unless codex R10 flags it
as load-bearing. All ten subsystems pass happy/unhappy/corner/edge
coverage. The R9 codex fixes are faithfully translated to
execmanifest with no regressions I can see.

## Closing

Amend and merge. Completes the R6 bet-the-demo replay triple
(pkg/principal + pkg/actionlog + pkg/execmanifest). The next
slice should be the registry/agent-loop wiring that actually
makes this infrastructure reachable from the live turn path.
