# CC R11 Acceptance Review — Agent-Loop ACS Wiring

> Reviewer persona: 30-year AI/CS veteran, same as R2-R10.
> Framing: acceptance review of the first agent-loop wiring for
> the R6/R7 replay triple. New package pkg/acs plus minimal hooks
> into pkg/agent/loop.go.
> Coverage rule applied: happy + unhappy + corner + edge per
> subsystem.
> Date: 2026-04-11.

## Opening

The wiring is intentionally minimal: a new coordinator package
(`pkg/acs`) opens and owns the three R6/R7 stores behind one
handle, and `pkg/agent/loop.go` acquires the bundle at
construction time behind `cfg.ACS.Enabled`. Every hook in the
loop is nil-guarded, so the ACS-off path is bit-for-bit identical
to pre-R11 behavior. All 27 existing packages pass tests with
race detector clean.

Two legitimate amendments, one structural observation, and
several follow-ups documented in the code itself. Verdict:
**READY WITH AMENDMENTS**.

## pkg/acs review

### `acs.Open` / partial-open cleanup
- **Happy**: `Open` validates `DBDir` is non-empty
  (`pkg/acs/acs.go:86`), opens both stores in sequence, returns
  an initialized Bundle. Covered by `TestBeginTurnAndRecordLLMCallRoundTrip`.
- **Unhappy**: If `actionlog.Open` fails, return with wrapped
  error and no resource leak
  (`pkg/acs/acs.go:90-97`). If `execmanifest.Open` fails, close
  the already-opened ledger before returning — the cleanup path
  at `pkg/acs/acs.go:103-106` matches the "no half-initialized
  bundle" invariant.
- **Corner**: `TestOpenCleansUpLedgerIfManifestOpenFails` at
  `pkg/acs/acs_test.go:201` exercises a read-only-directory
  failure and verifies the error message is acs-prefixed. The
  test skips when run as root because you can't chmod-out a
  directory as root.
- **Edge**: A missing DBDir (nonexistent parent path) propagates
  the sqlite open error with the acs-prefix wrapping. Tested by
  `TestDBDirMustExist`.
- **Issue**: None.

### `acs.Bundle` closed-flag + mutex
- **Happy**: Single `sync.Mutex` (`closeMu`) guards the
  `closedFlag` bool (`pkg/acs/acs.go:77-78`). Every wrapper
  method calls `isClosed()` which takes the lock briefly
  (`pkg/acs/acs.go:132-136`).
- **Unhappy**: Unlike `pkg/actionlog.Ledger` and
  `pkg/execmanifest.Store`, the acs Bundle does NOT hold the
  state lock across the wrapped write. This is correct because
  the wrapped stores already have their own RWMutex fences; the
  Bundle's lock only guards `closedFlag` consistency.
- **Corner**: Concurrent close vs wrapped ops — the wrapped
  store's own fence handles the race. The Bundle closes the
  wrapped stores in `Close` which takes `closeMu`, but the
  wrapper methods only check `isClosed()` under the same lock
  briefly, release it, then call the wrapped store. There is a
  small window where the Bundle's `closedFlag` is still false
  but the wrapped store has already begun closing. **Amendment
  1 (medium)**: this window returns a wrapped-store error
  (e.g., `actionlog.ErrClosed`) instead of `acs.ErrClosed`.
  Callers using `errors.Is(err, acs.ErrClosed)` won't match.
  Fix: normalize wrapped-store ErrClosed to `acs.ErrClosed` at
  the wrapper boundary.
- **Edge**: The test suite
  (`TestConcurrentCloseAndOps` at `pkg/acs/acs_test.go:313`)
  already accepts both `acs.ErrClosed` and
  `execmanifest.ErrClosed` as valid outcomes, which documents
  the leakage but doesn't fix it.
- **Issue**: Amendment 1.

### BeginTurn / RecordLLMCall / GetManifest
- **Happy**: Thin wrappers that check `isClosed()` then
  delegate. Manifest round-trip tested by
  `TestBeginTurnAndRecordLLMCallRoundTrip` at
  `pkg/acs/acs_test.go:67`.
- **Unhappy**: Closed bundle → `acs.ErrClosed`. Tested by
  `TestOpsAfterCloseReturnErrClosed`.
- **Corner**: Concurrent ops across all methods — 30
  goroutines racing tested by `TestConcurrentOpsAcrossAllMethods`
  at `pkg/acs/acs_test.go:278`, all green with race detector.
- **Edge**: None missing for the wrapper layer. Validation
  stays in the underlying store, as it should.
- **Issue**: None.

### PrepareAction / CommitAction / AbortAction
- **Happy**: Thin wrappers, action-ledger round-trip tested by
  `TestPrepareAndCommitActionRoundTrip` and `TestPrepareAndAbortActionRoundTrip`.
- **Unhappy**: Closed bundle → `acs.ErrClosed`. Same treatment
  as the manifest wrappers.
- **Corner**: Error types from the underlying ledger
  (`ErrAlreadyFinalized`, `ErrIntentNotFound`) flow through
  unchanged, which is the right call — callers should
  `errors.Is` against the typed sentinels, not the acs layer.
- **Edge**: The wrapper does not re-document the underlying
  error contract. **Observation, not blocker**: the GoDoc on
  each wrapper should at least link to the underlying method's
  error set.
- **Issue**: Minor doc polish.

### RecoverOrphans with manifest join
- **Happy**: Returns a non-nil slice. `ManifestPresent=true`
  when the execmanifest trace exists. Tested by
  `TestRecoverOrphansEnrichedWithManifest`.
- **Unhappy**: Missing manifest is surfaced via
  `ManifestPresent=false` + `ManifestLookupErr=nil`. A real
  lookup failure (not `ErrTraceNotFound`) populates
  `ManifestLookupErr`. This distinguishes "manifest missing"
  from "lookup failed" cleanly.
- **Corner**: `RecoverOrphans` runs N+1 queries (one for the
  orphan list, one per orphan for the manifest lookup). Same
  analysis as `ListBySession` in execmanifest: acceptable
  because orphan count is bounded, under `SetMaxOpenConns(1)`
  the queries are serialized anyway.
- **Edge**: Empty orphan list returns a non-nil empty slice
  (`TestRecoverOrphansEmptyReturnsNonNilSlice`).
- **Issue**: None.

### Close ordering
- **Happy**: Idempotent — second Close returns nil
  (`pkg/acs/acs.go:284`). Actions close first, manifest second
  (`pkg/acs/acs.go:290-291`).
- **Unhappy**: If `ledger.Close()` returns an error, the
  manifest is still closed (`pkg/acs/acs.go:290`). The Bundle
  returns the first error via wrapping.
- **Corner**: Concurrent Close + ops race covered by
  `TestConcurrentCloseAndOps`.
- **Edge**: The order (`ledger.Close` before `manifest.Close`)
  matters because a reconciliation hook might need to read
  manifest rows while finalizing ledger rows; reversing the
  order would break that guarantee. The test
  `TestBundleCloseOrderIsLedgerFirstThenManifest` documents the
  intent but doesn't actually enforce the order — **Amendment
  2 (low)**: make the test a real order check by using a stub
  pair that records close-call timestamps. Not blocking.
- **Issue**: Amendment 2 is polish.

## Agent loop wiring review

### NewAgentLoop bundle opener
- **Happy**: ACS disabled → `al.acs = nil`, all hooks skip
  (`pkg/agent/loop.go:135`). ACS enabled with a valid DBDir →
  bundle opens and logs success. Tested implicitly by every
  existing agent test that doesn't set `cfg.ACS.Enabled`.
- **Unhappy**: ACS enabled but Open fails → log warning and
  continue with `al.acs = nil`. This is the "fail open" choice.
  Defensible: an observability layer that takes the agent down
  when its storage breaks is worse than one that logs the
  error and continues.
- **Corner**: Empty DBDir + ACS enabled → derives
  `<workspace>/acs/` from the default agent, or skips if no
  workspace (`pkg/agent/loop.go:147-154`). The directory must
  already exist; the opener does not create it. This matches
  actionlog/execmanifest semantics.
- **Edge**: No test for "ACS enabled but workspace is nil" —
  the default-agent path guards against this but the explicit
  `dbDir == ""` fallthrough is not exercised. **Amendment 3
  (low)**: add a minimal integration test that constructs an
  AgentLoop with ACS enabled and asserts the bundle is
  reachable via `al.acs`. Not blocking because existing tests
  prove the nil-bundle path is preserved.
- **Issue**: Amendment 3 (test coverage, not code defect).

### runAgentLoop BeginTurn hook
- **Happy**: Hook at step 2.5 after the user message is saved
  and before `runLLMIteration` is called
  (`pkg/agent/loop.go:1076-1080`). The returned `acsTraceID` is
  threaded into runLLMIteration.
- **Unhappy**: If `BeginTurn` fails (disk full, closed bundle,
  etc.), the helper logs a warning and returns "". The turn
  continues normally with ACS disabled for this turn.
- **Corner**: Empty `acsTraceID` propagates correctly — every
  downstream `recordACSLLMCall` call is a nil-check no-op.
- **Edge**: The hook places manifest recording AFTER the user
  message is persisted. If the process crashes between the
  user-message persist and the BeginTurn, the message is still
  in the session store but has no manifest row. This is
  acceptable because the ACS story covers side effects, not
  input provenance — the session store is the input ledger.
- **Issue**: None.

### runLLMIteration RecordLLMCall hook
- **Happy**: Fires once per LLM response, right after the
  `response` is received but before tool dispatch
  (`pkg/agent/loop.go:1385-1391`). `callSeq` is `iteration-1`
  so the sequence starts at 0.
- **Unhappy**: Fires on success path only — if the LLM call
  returned an error, the hook is skipped. **Is that the right
  choice?** I'd argue yes: a failed LLM call shouldn't record a
  provider-call row because there's no real provider request
  to correlate with. A future slice can add a separate
  `failed_llm_calls` table if the observability story needs it.
- **Corner**: Concurrent LLM calls across sessions — each has
  its own traceID, so no collision on
  `(trace_id, call_seq)`.
- **Edge**: The synthetic `request_id` is
  `<trace_id>-call-<seq>`. Because the PK on
  `trace_provider_requests` is `(trace_id, call_seq)`, the
  synthetic form is collision-free per trace. A future slice
  can replace it with the real provider request ID from the
  LLM response header.
- **Issue**: None.

### AgentLoop.Close bundle hook
- **Happy**: Closes the bundle (if opened) before the other
  subsystems (`pkg/agent/loop.go:517-523`). Errors are logged,
  not returned — matches Close's non-error signature.
- **Unhappy**: Double Close is idempotent via the Bundle's
  own idempotency check.
- **Corner**: `al.acs = nil` after close prevents subsequent
  hooks from re-using a closed bundle.
- **Edge**: `Close` is called after `Stop`. If the loop is
  mid-turn when Close fires, the BeginTurn hook may already
  have written a manifest row that never gets a matching
  LLM-call row. This is the same "orphan manifest" case as
  `beginACSTurn` errors — acceptable because the manifest is a
  record of what we tried, not only what completed.
- **Issue**: None.

### Stand-in helpers: hashMessagesForACS, acsTurnNumber
- **Happy**: FNV-1a hash produces a deterministic 16-char
  string. History-length turn counter is monotonic within a
  session.
- **Unhappy**: Very large messages cap at 256 bytes per field,
  which is a lossy hash. A clearer label might help operators
  distinguish "FNV stand-in" from "real prompt epoch hash".
- **Corner**: Two different messages that only differ beyond
  the 256-byte cutoff will produce the same hash. The comment
  at `pkg/agent/loop.go:2321` documents this as an
  observability stand-in, not a cryptographic integrity check.
- **Edge**: `acsTurnNumber` uses history length as a proxy.
  Concurrent turns on the same session could produce
  duplicate `(session_id, turn)` pairs, which would fail the
  UNIQUE constraint on execmanifest. **Amendment 4 (medium)**:
  either gate the turn recording through an atomic per-session
  counter OR catch `ErrTraceAlreadyRecorded` in `beginACSTurn`
  and retry with `turn+1`. Low risk in current Ottie because
  per-session turns are serialized by the agent loop, but
  worth documenting.
- **Issue**: Amendment 4.

## Cross-cutting concerns

- **ACS-off is bit-for-bit identical to pre-R11?** Almost.
  The only delta is a single `if al.acs == nil { return "" }`
  on `beginACSTurn` and another inside `recordACSLLMCall`. The
  function-call overhead is negligible and the nil-check is
  essentially free. No new allocations on the off path. Every
  existing test passes unchanged.
- **acsTraceID plumbing**: the signature change to
  `runLLMIteration` is the only observable difference in the
  loop structure. Empty string propagates through the hooks
  correctly.
- **Fail-open vs fail-closed on Open**: fail-open is the right
  call for an observability layer. An ACS failure must not
  take the agent down.
- **Synthetic request_id collisions**: impossible per trace
  because of the PK, impossible across traces because the
  trace_id is 128-bit random.
- **Concurrent turns on same session**: a real race if gateway
  mode launches multiple turns concurrently. See Amendment 4.
- **Fallback-chain model vs primary model**: `beginACSTurn`
  records `agent.Model` (primary), but
  `runLLMIteration` may actually hit `activeModel` (fallback).
  The manifest says "we intended to use X" while the provider
  call row records "we actually hit Y". This is fine — the
  two fields serve different questions.

## Amendments before merge

1. **Amendment 1 (medium)**: at the acs wrapper boundary,
   normalize `actionlog.ErrClosed` / `execmanifest.ErrClosed`
   to `acs.ErrClosed` so callers using
   `errors.Is(err, acs.ErrClosed)` match consistently.
2. **Amendment 2 (low)**: make `TestBundleCloseOrderIsLedgerFirstThenManifest`
   actually assert the order with a stub pair (not just a
   documentation comment).
3. **Amendment 3 (low)**: add a minimal "ACS on" integration
   test that constructs an AgentLoop with the flag enabled and
   verifies the bundle is reachable.
4. **Amendment 4 (medium)**: document that concurrent turns on
   the same session would collide on `UNIQUE(session_id, turn)`
   and add a retry/turn-bump path in `beginACSTurn`. Low risk
   in current Ottie because turns are serialized per-session,
   but worth the defense.

## Follow-ups (separate slices)

- Tool-level action ledger wiring (Prepare/Commit around
  side-effecting tool dispatches).
- Real prompt_hash / tool_schema_hash from the R4 §14 prompt
  epoch API.
- PrincipalContext plumbing into the tool dispatch path.
- `ottie inspect trace` / `ottie replay` CLI surfaces.

## Verdict

**READY WITH AMENDMENTS.**

The core wiring is correct: ACS-off preserves existing
behavior, ACS-on records a manifest row per turn and a
provider-call row per LLM call. The four amendments are small
(two are test-only polish, two are defensive hardening) and can
all land in the same commit. All 27 existing packages pass with
race detector clean.

## Closing

Amend and merge. This slice makes the R6/R7 replay triple
reachable from the live agent loop for the first time. The next
slice should wire the action ledger into the tool dispatch path
so side-effecting tools actually go through Prepare/Commit.
