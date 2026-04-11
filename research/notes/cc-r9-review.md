# CC R9 Code-Level Acceptance Review — pkg/actionlog

> Reviewer persona: 30-year AI/CS veteran, same as R2-R8.
> Framing: acceptance review of pkg/actionlog, the R7 §4.4
> write-ahead action ledger. Third leg of the R6/R7 "bet the demo"
> replay story.
> Coverage rule applied: happy + unhappy + corner + edge per
> subsystem. Date: 2026-04-11.

## Opening

The implementation delivers on the R7 §4.4 invariant ("no signing
without a durable prepared row; exactly one finalization per
intent; orphans recoverable on startup") with two amendments worth
making before merge and one structural observation for a follow-up
commit. The two-table immutable-rows design exactly matches the R7
spec; the sqlite schema uses hermes's column naming verbatim; the
fsync durability claim is load-bearing and held via `synchronous=FULL`
plus a `wal_checkpoint(TRUNCATE)` after every write. A bug in the
initial shutdown path (send-on-closed-channel panic, caught by my
own test and fixed in the same slice) shows that the race-detector
coverage was necessary — the current implementation is clean under
`-race` and the drain-on-signal pattern is the correct Go idiom for
multi-producer shutdown.

Verdict: **READY WITH AMENDMENTS** — 2 small fixes to land in the
same commit, 1 follow-up for a later slice.

## pkg/actionlog review

### Schema (`action_intents` + `action_commits`)

- **Happy**: two tables match R7 §4.4 column-for-column. `intent_id`
  is primary key on `action_intents`
  (`pkg/actionlog/actionlog.go:208`). `action_commits` has
  `intent_id UNIQUE REFERENCES action_intents(intent_id)`
  (`pkg/actionlog/actionlog.go:220`) so the FK+UNIQUE pair enforces
  "exactly one commit per intent" at the SQL layer, which is the
  core R7 invariant. The CHECK constraint on `status IN
  ('committed','aborted')` (`pkg/actionlog/actionlog.go:222`)
  prevents bugs where a caller tries to write a bogus status value.
- **Unhappy**: missing required columns on Prepare are caught in Go
  before hitting SQL (`pkg/actionlog/actionlog.go:295-318`), so the
  NOT NULL declarations in the schema are a belt-and-braces check
  rather than the primary enforcement. Good.
- **Corner**: indexes on `trace_id` and `principal`
  (`pkg/actionlog/actionlog.go:229-230`) support the two most
  likely audit queries ("show me everything for trace X", "show me
  everything for user Y"). No index on `prepared_at` for the
  RecoverOrphans query, which does ORDER BY `prepared_at ASC`.
  **Amendment 1 (low)**: add `CREATE INDEX idx_intents_prepared_at
  ON action_intents(prepared_at)` to keep orphan recovery fast when
  the table grows. Low priority — the LEFT JOIN query will still
  work without it, just with a table scan.
- **Edge**: no `external_ids` index because it's a JSON blob;
  callers who need to query by tx_hash should extract it to
  `trace_id` or a new column. Acceptable for now, documented in
  the Commit struct comment (`pkg/actionlog/actionlog.go:91`).
- **Issue**: Amendment 1 only.

### `Open` / `Options` / DSN construction

- **Happy**: `Open` applies WAL + synchronous=FULL +
  busy_timeout(5000) + foreign_keys=ON at DSN-level
  (`pkg/actionlog/actionlog.go:174-182`). `db.SetMaxOpenConns(1)`
  (`pkg/actionlog/actionlog.go:145`) keeps the writer actor as the
  only write path to the DB. Writer goroutine is started last so
  a failed schema init doesn't leak a goroutine.
- **Unhappy**: empty `DBPath` returns a typed error early
  (`pkg/actionlog/actionlog.go:137`). A bad path (missing parent
  directory) surfaces from sqlite Open with a wrapped error.
- **Corner**: no cleanup if `initSchema` fails — actually yes,
  `_ = db.Close()` at `pkg/actionlog/actionlog.go:149`. Good.
- **Edge**: the DSN is URL-encoded (`pkg/actionlog/actionlog.go:180`)
  so a path containing a `?` or `&` wouldn't corrupt the query
  string. But wait — the path itself is NOT URL-encoded before
  being concatenated (`"file:" + dbPath + "?" + q.Encode()`). A
  path containing a `?` or `#` would be interpreted as DSN
  delimiters. **Amendment 2 (medium)**: URL-encode the path or
  validate it first. Most file paths won't contain these chars,
  but a user's `OTTIE_HOME` could be pathological.
- **Issue**: Amendment 2.

### `initSchema` idempotency

- **Happy**: all three DDL statements use `IF NOT EXISTS`
  (`pkg/actionlog/actionlog.go:200-230`). A fresh DB and an
  existing DB both succeed. Covered by
  `TestSchemaIdempotency` at `actionlog_test.go:590`.
- **Unhappy**: if a DDL statement fails, it's wrapped with the
  first line of the SQL (`pkg/actionlog/actionlog.go:232-236`) so
  the error tells the operator which statement broke. Good.
- **Corner**: running `Open` on a DB that already has
  `action_intents` but is missing `action_commits` (partial
  corruption) — the second CREATE would succeed without error
  because of `IF NOT EXISTS`. The resulting DB is half-good. Low
  risk because we never ship a half-applied migration in a single
  sql.DB invocation.
- **Edge**: no schema version table. If the R10 slice adds a
  column, there's no migration path. **Follow-up (not a blocker)**:
  add a `schema_version` table when the first migration lands.
- **Issue**: None blocking.

### Writer actor (drain-on-signal)

- **Happy**: `writer()` runs a `for-select` loop that picks up ops
  from `writeCh` and dispatches via `handleOp`
  (`pkg/actionlog/actionlog.go:262-286`). The select has two
  cases: a live op or the `closed` signal.
- **Unhappy**: if `handleOp` returns an error, it goes on
  `op.reply` so the public API gets it. No panic surface.
- **Corner**: the drain-on-shutdown path is the critical one. When
  `l.closed` fires, the writer enters an inner non-blocking select
  that drains any queued ops before exiting
  (`pkg/actionlog/actionlog.go:273-284`). Every caller whose op is
  already on the channel gets a reply before the writer
  terminates. This fixes the race where `TestOpsAfterCloseFail`
  originally panicked with "send on closed channel" — the fix was
  to NEVER close `writeCh`, only signal via `l.closed`. Verified
  clean under `go test -race -count=1`.
- **Edge**: a caller that holds an op on its local stack and never
  pushes it to the channel will not be reaped by shutdown — but
  that's the caller's bug, not the ledger's.
- **Issue**: None.

### `Prepare`

- **Happy**: generates a random 128-bit hex ID when none is
  supplied (`pkg/actionlog/actionlog.go:326-332`). Verifies all
  five required fields (`pkg/actionlog/actionlog.go:295-319`).
  Inserts the row, runs the WAL checkpoint, returns the ID.
- **Unhappy**: each missing field returns a specific error
  message. Covered exhaustively by
  `TestPrepareRejectsMissingRequiredFields`.
- **Corner**: caller supplies a colliding `IntentID` — the primary
  key constraint returns a SQLite error that gets wrapped
  (`wrapFinalizationError` handles UNIQUE constraint failed but
  currently only on action_commits; Prepare uses `handleOp` →
  `doPrepare` → raw Exec, so a colliding Prepare returns a less
  structured error). **Amendment 3 (low, nice-to-have)**: wrap
  Prepare's UNIQUE constraint error as a new
  `ErrIntentAlreadyExists` so callers can distinguish it from a
  disk-full error. Low priority because Ottie will always
  auto-generate IDs.
- **Edge**: context cancellation BEFORE the op reaches the writer
  returns `ctx.Err()` via the initial select. Context cancellation
  AFTER the writer has started the insert may leave the row
  written — `RecoverOrphans` will surface it next startup. This is
  the correct semantic and is documented in the Prepare comment
  (`pkg/actionlog/actionlog.go:523`).
- **Issue**: Amendment 3.

### `Commit`

- **Happy**: auto-generates commit_id. Marshals `ExternalIDs` map
  to JSON (nil/empty → SQL NULL via
  `marshalExternalIDs`, `pkg/actionlog/actionlog.go:408`). Inserts
  with status="committed". Tested by
  `TestPrepareCommitRoundTrip` and
  `TestCommitWithNestedExternalIDs`.
- **Unhappy**: empty `IntentID` → typed error
  (`pkg/actionlog/actionlog.go:351`). Missing intent → 
  `ErrIntentNotFound` via FOREIGN KEY wrap. Double-commit →
  `ErrAlreadyFinalized` via UNIQUE wrap. All tested.
- **Corner**: JSON marshal failure — unlikely for map[string]any
  but possible with weird types (channels, functions). Returns a
  wrapped error; no panic.
- **Edge**: `TestCommitWithEmptyExternalIDsStoresNull` confirms
  that a nil map becomes SQL NULL, not the literal `"{}"`. This
  is the sort of detail a JSON-as-blob column would silently get
  wrong; the test pins it down.
- **Issue**: None.

### `Abort`

- **Happy**: auto-generates commit_id. Inserts with status="aborted"
  and the error_message column populated (`action_commits` row is
  the same shape as a committed row, just with a different status
  and different optional columns set). Tested by
  `TestPrepareAbortRoundTrip`.
- **Unhappy**: empty `IntentID`, missing intent, double-finalize
  — same treatment as Commit. All three error kinds tested.
- **Corner**: calling Abort on an already-Committed intent
  returns `ErrAlreadyFinalized` (tested by
  `TestAbortAfterCommitReturnsAlreadyFinalized`).
- **Edge**: `error_message` is SQL TEXT with no length limit in
  the schema. A pathological caller could store megabytes. In
  practice the tool layer wraps errors into short strings.
  **Observation, not an amendment**: no clipping, acceptable.
- **Issue**: None.

### `RecoverOrphans`

- **Happy**: LEFT JOIN with NULL check on commit_id
  (`pkg/actionlog/actionlog.go:570-576`). Returns rows ordered by
  `prepared_at ASC` so the earliest orphan is surfaced first.
  Tested by `TestRecoverOrphansReturnsOnlyUnfinalized` with a
  three-way mix of committed, aborted, and orphaned intents.
- **Unhappy**: empty DB → empty slice, not nil (tested by
  `TestRecoverOrphansEmptyDatabase`). Query failure → wrapped
  error with the operation name.
- **Corner**: a caller that takes a slice of orphans and then
  closes the ledger — the slice is a copy, no dangling references.
  `defer rows.Close()` ensures the cursor is released even if the
  scan fails mid-iteration.
- **Edge**: orphans-in-prepared-at-order is a critical invariant
  because recovery tooling will typically process oldest first
  (earliest intent = most likely to have side-effected already).
  Verified by the test explicitly checking the order of B and D
  (`actionlog_test.go:470`).
- **Issue**: None.

### `Close`

- **Happy**: idempotent (tested by
  `TestOpsAfterCloseFail`'s second-Close-returns-nil check).
  Signals via `l.closed`, waits for writer drain, closes DB.
- **Unhappy**: in-flight ops get drained via the writer's
  non-blocking select. Ops arriving after Close begins return
  `ErrClosed` via the fast-fail check
  (`pkg/actionlog/actionlog.go:516` for Prepare, same pattern
  in Commit and Abort).
- **Corner**: concurrent Close calls — guarded by `l.closeMu`
  (`pkg/actionlog/actionlog.go:624`). Only one goroutine actually
  runs the close sequence.
- **Edge**: if `l.db.Close()` returns an error, Close returns it.
  If the writer goroutine panics (unreachable in current code
  because every op wraps errors), `<-l.done` would hang. No
  defensive panic recovery in the writer, which is acceptable
  given Go's "fix the panic, don't mask it" convention.
- **Issue**: None.

### Durability across process restart

- **Happy**: `TestDurabilityAcrossReopen` at
  `actionlog_test.go:493` runs the critical scenario: Prepare,
  Close, Open, RecoverOrphans. The prepared intent survives and
  is visible on reopen with all fields intact. This is the whole
  point of the ledger.
- **Unhappy**: no test for "crash instead of clean Close" because
  Go tests can't `os.Exit` mid-test without losing the other
  tests. But clean-Close is a strictly weaker requirement than
  crash — if clean-Close data survives, crash data also survives
  (the WAL is fsync'd after every write).
- **Corner**: `synchronous=FULL` + `wal_checkpoint(TRUNCATE)`
  after each write is the strongest durability SQLite offers. If
  the OS crashes before the WAL is fsync'd... but the checkpoint
  forces the fsync before returning. Good.
- **Edge**: the ledger will panic/error if the filesystem fills
  up between writes. Inserts return a "disk full" error that
  gets wrapped in `"actionlog: Prepare: insert"`. Callers should
  treat all `Prepare` errors as non-retryable unless they can
  diagnose the specific cause.
- **Issue**: None.

### Concurrency

- **Happy**: `TestConcurrentPreparesSerialize` at
  `actionlog_test.go:399` fires 50 concurrent Prepares, asserts
  all succeed with unique IDs, and that the orphan count is 50.
  This proves the writer actor serializes correctly.
- **Unhappy**: channel back-pressure. A producer attempting to
  push when the queue is full will block until the writer catches
  up — which is the correct behavior. The `WriteQueueDepth`
  option lets callers tune this.
- **Corner**: race detector clean (`go test -race -count=1`
  completes in 2.4s). This is the critical test for the
  drain-on-signal shutdown path.
- **Edge**: at very high concurrency (>100 goroutines), the write
  throughput is bounded by the WAL checkpoint after each write.
  **Observation**: this is acceptable for a crypto-signing ledger
  (write rate ~10/sec maximum) but would be a bottleneck for a
  high-frequency-trading tool. Document the throughput ceiling
  in the package comment. Low priority.
- **Issue**: None blocking.

## Cross-cutting concerns

- **R7 §4.4 match**: exact. Two immutable tables, foreign key
  plus UNIQUE enforces one-commit-per-intent, recovery returns
  orphans.
- **Clean-design §4.1 schema match**: every column name lines up
  with the design doc. No drift.
- **Fsync claim**: held. `synchronous=FULL` + `wal_checkpoint`
  after every write.
- **TOCTOU / race in Close**: previous iteration had a
  "send on closed channel" panic. Fixed by switching to
  drain-on-signal. Race detector clean.
- **Missing defenses**: no clock-skew handling (test uses a
  fixed clock; prod uses time.Now, so a back-dated system clock
  could produce out-of-order prepared_at values. Acceptable).
  No principal string validation (ledger trusts the caller;
  callers should pass `pkg/principal.Label()` output). No hash
  length validation (ArgsHash is free-form TEXT). No payload
  size limits (external_ids JSON could be arbitrarily large —
  document the expectation that callers keep it small).

## Amendments before merge

1. **Amendment 1 (low)**: add `CREATE INDEX idx_intents_prepared_at
   ON action_intents(prepared_at)` to keep RecoverOrphans
   efficient at scale. Same commit.
2. **Amendment 2 (medium)**: URL-encode the DB path before
   concatenating into the DSN, to handle paths with `?` or `#`
   characters safely.
3. **Amendment 3 (low, nice-to-have)**: wrap Prepare's UNIQUE
   constraint error so callers can distinguish
   `ErrIntentAlreadyExists` from a generic insert failure.

## Follow-ups (separate commits)

1. Schema version table + migration path for future column
   additions.
2. Integrate with pkg/principal so the `Principal` column is
   typed via `principal.Label()` at the call site, and the
   `EffectClass` is constrained to the five pkg/principal
   capability strings.
3. Wire the ledger into the agent loop so every side-effecting
   tool goes through Prepare+Commit (the thing that actually
   makes this implementation useful; currently it's a library
   nothing uses).

## Verdict

**READY WITH AMENDMENTS.**

All four category checks clean across ten subsystems. Three small
amendments (one indexing, one DSN-safety, one error-wrapping
polish) fit in the same commit. The three follow-ups are
separate-commit territory and do not block merging this slice.

## Closing

Amend and merge. The ledger is a clean, self-contained piece that
implements the R7 §4.4 invariant correctly and sets up the P1
write-ahead-journal story that the principal package (R8) and the
yet-to-be-implemented execution manifest can build on.
