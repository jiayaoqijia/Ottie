# CC R12 Acceptance Review — Tool-Level Ledger Wiring

> Reviewer persona: 30-year AI/CS veteran, same as R2-R11.
> Framing: first slice that makes side-effecting tool dispatches
> go through the R9 action ledger. Adds `tools.EffectClassifier`
> interface + `acs.Bundle.Dispatch` helper + minimal agent-loop
> wiring.
> Coverage rule applied: happy + unhappy + corner + edge per
> subsystem.
> Date: 2026-04-12.

## Opening

The R12 slice is deliberately additive: a new optional interface
on `tools.Tool`, a new helper `acs.Bundle.Dispatch`, and one
`if al.acs != nil` branch in the agent loop's parallel tool-
execution block. Zero existing tools declare an effect class yet,
so the wiring is functionally off in production until a follow-up
slice migrates real crypto tools to declare `writes_wallet` etc.
The 18 new dispatch tests cover the full happy/unhappy/corner/
edge matrix for the helper itself. All R11 invariants (monotonic
turn counter, every-LLM-call recording) verified preserved.

Two amendments and three observations. Verdict: **READY WITH
AMENDMENTS**.

## pkg/tools.EffectClass + constants + IsSideEffecting

- **Happy**: Five constants (`EffectReadOnly`, `EffectWritesLocal`,
  `EffectWritesState`, `EffectWritesChain`, `EffectWritesWallet`)
  at `pkg/tools/base.go:34-58`. String values match
  `pkg/principal` capability markers exactly so a future slice
  can unify them without a migration.
- **Unhappy**: `EffectClass.IsSideEffecting` at `base.go:64-66`
  returns false for empty string AND `EffectReadOnly`, which is
  the right fail-closed default.
- **Corner**: A contributor adds a sixth class — the closed set
  in the `Capability` type-union of pkg/principal would catch it,
  but `pkg/tools.EffectClass` is just a `string` so there's no
  compile-time guard. **Observation**: the two type systems
  diverge here. For the current slice the mismatch is
  intentional (avoiding the pkg/principal import cycle), but a
  future unification should either pull the constants into a
  shared sub-package or use a generated type.
- **Edge**: A tool that implements `EffectClass()` returning
  `EffectClass("writes_mars")` — not in the known set — would be
  treated as side-effecting (because it isn't read_only) and
  would produce an `effect_class` column value of "writes_mars"
  in the ledger. The schema's CHECK constraint on
  `trace_provider_requests.status` is an allowlist but
  `action_intents.effect_class` is free-form TEXT, so there is
  no storage-layer rejection of bogus values. **Amendment 1
  (low)**: add a CHECK constraint on `effect_class` at the
  schema level to enforce the known set. Defensive hardening,
  not blocking.
- **Issue**: Amendment 1.

## pkg/tools.EffectClassifier + ClassOf

- **Happy**: The interface at `base.go:70-72` is a single method
  `EffectClass() EffectClass`. `ClassOf(t)` at `base.go:78-85`
  uses a type assertion, returning `EffectReadOnly` when the
  tool does not implement the interface or returns `""`. Tested
  by `TestClassOfDefaultsToReadOnly` and
  `TestClassOfEmptyStringFallsBackToReadOnly`.
- **Unhappy**: A nil tool passed to `ClassOf` would panic on the
  type assertion. `ClassOf` does not nil-check its input.
  **Amendment 2 (low)**: add `if t == nil { return EffectReadOnly }`
  at the top of ClassOf. Defensive. The agent loop already
  nil-checks before calling `Dispatch`, so this is a library-
  hardening fix, not a live bug.
- **Corner**: A tool that implements `EffectClassifier` but
  returns different values on successive calls (stateful) —
  `ClassOf` calls it once and caches nothing, so the ledger sees
  whatever the first call returned. Acceptable because the
  classification is meant to be static per-tool.
- **Edge**: Interfaces with `Tool` + `EffectClassifier` via
  embedding work correctly because Go's type assertion on `Tool`
  can be promoted to `EffectClassifier` if the concrete type
  implements both.
- **Issue**: Amendment 2.

## acs.Bundle.Dispatch

- **Happy**: The full lifecycle is at `pkg/acs/dispatch.go:68-167`.
  Read-only tools bypass (tested by
  `TestDispatchReadOnlyToolBypassesLedger`). Wallet-class tools
  trigger Prepare+Commit (tested by
  `TestDispatchWalletToolHappyPath`). Error paths trigger
  Prepare+Abort (tested by `TestDispatchWalletToolAbortOnError`).
- **Unhappy**: Nil bundle → falls through to `run(ctx)` directly,
  no panic (`TestDispatchNilBundleFallsThrough`). Nil tool →
  same (`TestDispatchNilToolFallsThrough`). Empty trace ID →
  same (`TestDispatchEmptyTraceIDFallsThrough`). Prepare failure
  → fail-open, tool still runs
  (`TestDispatchPrepareErrorFailsOpen`).
- **Corner**: 20 concurrent dispatches on one trace_id — all
  succeed, all intents unique, all committed, zero orphans
  (`TestDispatchConcurrentToolsSerializeViaLedger`).
- **Edge**: Every side-effecting class is tested via the
  `effectTool` table test
  (`TestDispatchEveryEffectClassIsWrapped`) — local, state,
  chain, wallet all produce ledger rows.
- **Issue**: One observation — `Dispatch` ignores the return
  value from `CommitAction` and `AbortAction` (line 148, 167).
  If the ledger write fails after the tool succeeded, the user
  still sees the tool result but there's no audit trail of the
  commit failure. Current code logs nothing. **Amendment 3
  (medium)**: log a warning when `CommitAction` or `AbortAction`
  returns a non-nil error so operators can see when the ledger
  is falling behind reality.

## HashArgsForLedger / HashResultForLedger

- **Happy**: Deterministic sha256 over JSON-marshaled inputs.
  Tested by `TestDispatchEmptyArgsHashesDeterministically`
  (nil and empty map produce same hash) and
  `TestDispatchArgsHashMapKeyOrderStable` (permuted maps
  produce same hash).
- **Unhappy**: `json.Marshal` failure (impossible for
  `map[string]any` with primitive values, but possible if a
  caller sneaks in a `chan` or a function) returns a wrapped
  error. The `Dispatch` caller falls through to `run(ctx)` on
  hash failure.
- **Corner**: `HashResultForLedger(nil)` returns
  `"sha256-empty"` — non-empty sentinel so the ledger row has
  something to store (`TestHashResultForLedgerStable`).
- **Edge**: The result-hash canonicalization uses ASCII unit
  separator (0x1F) between `ForUser` and `ForLLM`. A tool that
  somehow embeds 0x1F in its output would produce a collision
  with a different split. The chance is near-zero in practice
  (0x1F doesn't appear in typical tool text output).
- **Issue**: None.

## Agent loop wiring

- **Happy**: The parallel goroutine at `pkg/agent/loop.go:1646-1691`
  now calls `al.acs.Dispatch` when `al.acs != nil && acsTraceID
  != ""`. The `runTool` closure captures the same
  `ExecuteWithContext` call that existed pre-R12, so the
  dispatch path is identical for read-only tools.
- **Unhappy**: Tool not registered → `agent.Tools.Get` returns
  `(nil, false)`, and the wiring falls through to `runTool(ctx)`
  directly (the "unknown tool" error comes from the registry).
  No ledger rows.
- **Corner**: `runTool` is called inside a goroutine so the
  dispatch is per-tool-call. Each call gets its own intent ID.
  Verified by the concurrent dispatch test in pkg/acs.
- **Edge**: ACS-off path is a single `if al.acs != nil` check
  against the pre-R12 code. Zero additional allocations on the
  off path because `runTool` is defined lazily as a closure.
- **Issue**: None.

## acsPrincipalLabel stand-in

- **Happy**: Produces
  `"agent=main;user=unknown;account=unknown;channel=<ch>:<cid>"`
  which parses with the same regex as
  `pkg/principal.Label()` output.
- **Unhappy**: Empty `channel` or `chatID` → fallback to
  `"unknown"` so the column is never empty.
- **Corner**: The label will be wrong once real multi-user
  gateway mode exists — `user=unknown` drops the actual user
  identity that the channel layer knows. **Observation**: the
  stand-in is a temporary bridge until pkg/principal is
  threaded through the tool registry. The helper is clearly
  labeled as such in the doc comment.
- **Edge**: None.
- **Issue**: None blocking. Follow-up: kill this helper when
  PrincipalContext lands.

## R11 invariants preservation

- **acsCallSeq + per-turn LLM-call recording**: not touched by
  R12. Still inside `callLLM` at `loop.go:1321-1378`.
- **Monotonic per-session turn counter**: not touched by R12.
  Still in `allocateACSTurnNumber` at
  `loop.go:2289-2328`.
- **Bundle.ErrClosed normalization**: not touched.
- **Bundle.Close ordering**: not touched.

All four R11 fixes preserved.

## Amendments before merge

1. **Amendment 1 (low)**: add a SQL CHECK constraint on
   `action_intents.effect_class` to enforce the known set.
2. **Amendment 2 (low)**: nil-check in `tools.ClassOf` so a
   nil tool returns `EffectReadOnly` instead of panicking.
3. **Amendment 3 (medium)**: log warnings when `CommitAction`
   or `AbortAction` fail inside `Bundle.Dispatch` so operators
   see ledger-reality drift.

## Follow-ups (separate slices)

- Migrate real crypto tools (lido_stake, erc20_transfer,
  etc.) to declare `EffectWritesWallet` / `EffectWritesChain`.
- Thread `pkg/principal.Label()` through the tool dispatch to
  replace `acsPrincipalLabel`.
- Strict-mode config that blocks dispatch when Prepare fails
  (currently fail-open).
- `ottie inspect trace` / `ottie replay` CLI surfaces.

## Verdict

**READY WITH AMENDMENTS**.

Three small amendments, all additive and local. The R12 slice
is the smallest wiring slice yet (the dispatch path is a single
new branch in the parallel loop) and the test coverage is
solid. Every side-effecting class is exercised, every error
path is tested, concurrent dispatch is verified.

## Closing

Amend and merge. Next slice should migrate the real
`workspace/skills/defi/lido-mcp` and related crypto tools to
declare their effect classes so the R6/R7 replay story has
actual production data flowing through it.
