# CC R8 Code-Level Acceptance Review — pkg/principal + pkg/providers errclass

> Reviewer persona: 30-year AI/CS veteran, same as R2-R7.
> Framing: acceptance review of the first R6/R7 implementation slice.
> Coverage rule applied (per user 2026-04-11): happy + unhappy + corner + edge per subsystem.
> Date: 2026-04-11.

## Opening

The implementation delivers on both R6/R7 claims with one caveat and
two small regression risks that should be fixed before merge.

**R7 §4.3 compile-time safety — REAL.** `pkg/principal/typed_tool.go`
correctly uses Go generics with per-capability marker types, and the
test suite contains a positive case (`requireWalletSigner` in
`pkg/principal/principal_test.go:58`) that compiles *only if*
`UpgradeToWritesWallet` returns a `PrincipalContext[CapWritesWallet]`.
The negative case — "compiler rejects the wrong principal" — is not
and cannot be directly tested in the same test file (it would refuse
to build). This is the correct shape for compile-time property tests
in Go, but it does mean a future refactor that weakens the types
would not be caught by `go test`; I recommend a tiny
`testdata/negative_compile/main.go` with a build tag that CI compiles
under `go build -tags negative_compile` and asserts the build fails.
Flagged in the amendments list below.

**R6 steal #1 classifier — REAL AND MOSTLY COMPLETE.** The five
hermes-derived reasons land with correct priority order (`classifyByMessage`
in `pkg/providers/error_classifier.go:253`), and the
`TestRecoveryHintsCoverEveryReason` exhaustiveness test in
`pkg/providers/error_classifier_r6_test.go:175` is exactly the right
guard against a future contributor adding a reason without updating
the hint methods. One hermes pattern family (`resource_exhausted`
variant for Google Gemini) is partially covered but could be tightened.

Verdict: **READY WITH AMENDMENTS.** Three small items before merge.

---

## pkg/principal review

### Capability type constraint + marker structs
- **Happy**: `Capability interface { CapReadOnly | CapWritesLocal | CapWritesState | CapWritesChain | CapWritesWallet }` at `pkg/principal/principal.go:46` is the Go generic type union that makes distinct markers distinct types. Each marker is a zero-sized empty struct, so `PrincipalContext[CapReadOnly]{}` and `PrincipalContext[CapWritesWallet]{}` compile as distinct types even though they share layout. Confirmed by `TestCapabilityDistinctness` at `pkg/principal/principal_test.go:22`.
- **Unhappy**: A contributor adds a new capability type (say `CapWritesCore`) without adding it to the union. Result: `PrincipalContext[CapWritesCore]` won't compile at all because `CapWritesCore` is not a member of `Capability`. This is the correct fail-closed behavior.
- **Corner**: What if someone imports the package and defines a local type `type MyCap struct{}` and tries `PrincipalContext[MyCap]`? Compiler rejects it because `MyCap` is not in the union. Confirmed by Go's generic type-parameter rules.
- **Edge**: Go's current generics do not allow methods on generic types parameterized by a specific instantiation, so I could not add `PrincipalContext[CapReadOnly].Upgrade()` as a method. The design correctly uses package-level functions (`UpgradeToWritesWallet`). No edge case loss.
- **Issue**: None.

### `PrincipalContext[C]` distinctness
- **Happy**: The struct definition at `pkg/principal/principal.go:100` carries four string fields plus the phantom type parameter `C`. The phantom is enforced at compile time even though it doesn't appear in fields — Go distinguishes `T[A]` and `T[B]` at the type level regardless.
- **Unhappy**: A caller accidentally assigns `PrincipalContext[CapReadOnly]` to a variable declared as `PrincipalContext[CapWritesWallet]`. Result: `go build` fails with "cannot use ro (variable of type PrincipalContext[CapReadOnly]) as PrincipalContext[CapWritesWallet] value".
- **Corner**: Copying a principal via field-by-field assignment across capabilities — this CAN happen. A contributor could write: `wallet := PrincipalContext[CapWritesWallet]{AgentID: ro.AgentID, UserScope: ro.UserScope, ...}`. The compiler accepts this because each field is typed independently. The design relies on the `UpgradeToWritesWallet` gate being the sole construction path for wallet-typed principals, but nothing in the package makes the struct unexported or its fields private. **AMENDMENT 1**: make the struct fields unexported and provide read accessors. A construct-by-copy bypasses the consent check.
- **Edge**: A hostile caller who imports `pkg/principal` and constructs `PrincipalContext[CapWritesWallet]{}` directly. Same fix as AMENDMENT 1.
- **Issue**: See AMENDMENT 1 below. The compile-time guarantee is real for *implicit* conversions, but the literal struct construction path is not blocked. Medium priority — a review-time catch, not a security hole.

### `SignedConsent` struct + field contract
- **Happy**: Fields `PrincipalLabel`, `EffectClass`, `ArgsHash`, `ExpiresAt`, `Signature` at `pkg/principal/principal.go:140`. Each upgrade gate validates all five before producing a typed output.
- **Unhappy**: Missing signature → `ErrConsentRequired`. Mismatched label → `ErrConsentInvalid` wrapped with "principal label". Mismatched effect → `ErrConsentInvalid` wrapped with "effect class". Verifier error → `ErrConsentInvalid` wrapping the verifier's error. All four covered by `TestUpgradeGatesRejectMissingConsent` at `principal_test.go:76`.
- **Corner**: `ExpiresAt` is in the struct but **never checked by the gate**. A replayed consent with a past expiry would still be accepted as long as the signature verifies. **AMENDMENT 2**: `validateUpgrade` at `pkg/principal/principal.go:238` must also check `consent.ExpiresAt > time.Now().Unix()`. High priority — this is a replay-attack surface. The test suite does not cover it.
- **Edge**: `ArgsHash` is in the struct but not used by the gate either. The design comment at `principal.go:158` says the hash "makes an attacker unable to swap out the arguments after consent," but the gate doesn't take args as input so it can't compare. The hash becomes a verifier responsibility. Document this more clearly in the struct doc comment or check it at a higher layer.
- **Issue**: AMENDMENT 2 (ExpiresAt check) is load-bearing for R7's "consent envelopes hashable + replayable" claim.

### `UpgradeTo*` gates (four functions)
- **Happy**: Each gate constructs the correct typed output and returns it. Covered by `TestUpgradeGatesCoverAllFourCapabilities` at `principal_test.go:137`.
- **Unhappy**: Each gate returns a wrapped `ErrConsentInvalid` on any of the four documented failure modes.
- **Corner**: The four gates are structurally identical. A contributor adds a fifth capability; they must also add a fifth gate, a fifth pattern set in `untypedFrom`, and a fifth row in `TestRecoveryHintsCoverEveryReason`. No single list enforces this — the contributor must remember three places. **Observation (not a blocker)**: consider adding a single `Upgrade[C Capability]` generic function that dispatches internally, reducing the fan-out.
- **Edge**: Calling `UpgradeToWritesWallet` twice with the same consent returns the upgraded principal twice — consent is not consumed. For the replay-attack defense to be complete, the verifier should maintain a used-consent cache or the consent should include a nonce. Document in the `SignedConsent` comment that nonce tracking is the verifier's responsibility.
- **Issue**: Minor. The gates' runtime path is correct.

### `TypedTool[C, T]` interface + compile-time dispatch
- **Happy**: `pkg/principal/typed_tool.go:39` defines the generic interface. Tool implementations declare their required cap at the type level; calls with the wrong principal fail at build. Proven by `TestTypedToolCompilesWithCorrectPrincipal` at `typed_tool_test.go:60`.
- **Unhappy**: Any direct `tool.Execute(ctx, wrongPrincipal, args)` call fails `go build` — this is the whole point. Cannot be tested in-place (compile failure would break the test package), but the CI build itself is the test.
- **Corner**: `TypedTool[C, T]` must declare `Execute` returning `(Result, error)`. If a contributor tries to return a bigger result type, the generic constraint rejects it. No surprise there.
- **Edge**: The `Result` struct in `typed_tool.go:56` is duplicated from `pkg/tools.ToolResult`. This is intentional to avoid a circular import, but it means the typed-side `Result` and the runtime `ToolResult` must be kept in sync. Document this in a comment on `Result`.
- **Issue**: None blocking. Document the `Result` duplication.

### `Runnable` adapter + `RunUntyped` dispatch (five adapters)
- **Happy**: Each adapter projects the runtime `UntypedPrincipal` into the typed `PrincipalContext[C]` and calls `tool.Execute`. Happy-path success for read-only and wallet tools covered by `TestAdaptersEnforceCapabilityAtRuntime` at `typed_tool_test.go:92`.
- **Unhappy**: Runtime cap check refuses dispatch if `untyped.HasCap("writes_wallet")` is false; wraps `ErrInsufficientCap`. Covered.
- **Corner**: Decoder failure wraps the tool name in the error message so the retry layer can surface it. Covered by `TestAdapterDecodeErrorSurfacesCleanly` at `typed_tool_test.go:140`.
- **Edge**: All five adapters share almost identical body (lines 220-290). This is DRY-fail but acceptable given Go 1.26 doesn't support per-instantiation methods. A future refactor could use a single generic adapter if Go gains field-access-via-type-parameter. Not a blocker.
- **Issue**: None.

### `UntypedPrincipal.HasCap` + capability ladder monotonicity
- **Happy**: The ladder is `["read_only", "writes_local", "writes_state", "writes_chain", "writes_wallet"]` at `typed_tool.go:108`. Each projection fills the set up to the highest cap it holds. Covered by `TestFromWritesWalletLadder` at `typed_tool_test.go:190`.
- **Unhappy**: A cap string not in the ladder returns false from `HasCap`. A principal constructed by hand with an empty `HeldCaps` returns false for everything. Both paths are safe.
- **Corner**: The fix I made earlier today (`untypedFrom` originally included every cap unconditionally) — covered by test and fixed. Good.
- **Edge**: What about a principal that somehow holds `writes_wallet` but not `read_only`? The `untypedFrom` builder always walks the ladder from `read_only` upward, so it can't produce such a shape. But `UntypedPrincipal` is a public struct — a caller could construct one by hand with `HeldCaps: map[string]bool{"writes_wallet": true}`. Then `HasCap("read_only")` returns false even though monotonicity implies it should be true. **AMENDMENT 3**: either (a) make `HeldCaps` unexported and force construction through `FromXxx`, or (b) add a `Normalize()` method that fills in lower caps. Low priority — the FromXxx helpers are the sanctioned path.
- **Issue**: AMENDMENT 3.

---

## pkg/providers classifier review

### `FailoverReason` closed set + string stability
- **Happy**: Fourteen string constants at `pkg/providers/types.go:47`. String values are snake_case, stable across serialization. Covered.
- **Unhappy**: No unhappy path — constants don't fail.
- **Corner**: A future contributor adds a fifteenth reason and forgets to update `TestRecoveryHintsCoverEveryReason`. The test has a length-mismatch check at `error_classifier_r6_test.go:224` that panics the test with a clear message. Correct fail-closed.
- **Edge**: What if two reasons have the same string value? Go treats them as different constants but the serialized form would collide. No guard in code. Low risk because the file is small and review would catch it.
- **Issue**: None.

### New patterns (five reason families)
- **Happy**: Each of `contextOverflowPatterns`, `payloadTooLargePatterns`, `thinkingSignaturePatterns`, `longContextTierPatterns`, `authPermanentPatterns` has 4-5 patterns covering the hermes reference plus a regex fallback. Covered by dedicated per-reason tests at `error_classifier_r6_test.go:18, 47, 73, 99, 124`.
- **Unhappy**: What if a message matches *both* a new-reason pattern and an old-reason pattern? E.g., a context_overflow error that also mentions "rate limit" somehow. The order in `classifyByMessage` at `pkg/providers/error_classifier.go:253` puts context_overflow first, so context wins. **Tested implicitly** by `TestClassifyError_PermanentAuthBeatsTransientAuth` at `error_classifier_r6_test.go:170`, but not explicitly for the other precedence pairs. Low-priority gap.
- **Corner**: An error message with only a single matching pattern — handled correctly. An empty error string — `matchesAny` returns false for every pattern, falls through to `FailoverUnknown`. Safe.
- **Edge**: Case sensitivity. `ClassifyError` lowercases the message at `error_classifier.go:149` before matching, so `"RATE LIMIT"` and `"Rate Limit"` both match. Good.
- **Issue**: None.

### Classification priority order in `classifyByMessage`
- **Happy**: Order is compression hints → anthropic-specific → rate limit → overloaded → billing → timeout → model not found → session expired → auth_permanent → auth → format. This is **different from hermes's own order** (hermes checks billing before rate_limit because billing errors often *contain* rate-limit phrasing). Let me cross-check against Ottie's pattern set: `billingPatterns` includes "insufficient credits" and "credit balance"; `rateLimitPatterns` includes "exceeded your current quota". The Ottie patterns do not overlap, so the order doesn't matter in practice. But a future contributor adding "quota exceeded" to both lists would silently misclassify. **AMENDMENT 4 (observation, not blocker)**: add a `TestPatternsAreDisjoint` that cross-checks every billingPatterns entry against every rateLimitPatterns entry and fails if a single test input matches both.
- **Unhappy**: A message matching nothing → returns empty string, `ClassifyError` returns `nil`. The retry layer treats nil as "don't failover", which is the safe default.
- **Corner**: Permanent-auth before auth is explicitly tested. Thinking-signature before format is correct because thinking blocks are narrower. Good.
- **Edge**: What if hermes adds a new pattern in a release? The Ottie classifier will fall through to unknown. That's the safe default, and the R2-R6 review cycle is designed to catch new patterns by re-reading hermes's source. Acceptable.
- **Issue**: AMENDMENT 4 is nice-to-have, not blocker.

### `classifyByStatus` additions (413 → payload_too_large)
- **Happy**: `pkg/providers/error_classifier.go:249` adds the 413 case. Covered by `TestClassifyError_Status413MapsToPayloadTooLarge` at `error_classifier_r6_test.go:181`.
- **Unhappy**: A 413 without "413" in the message — caught by the status code switch.
- **Corner**: A 413 that also has "rate limit" in the body — status code check runs before message check, so 413 wins. Correct because transport-layer errors are more reliable than body content.
- **Edge**: None.
- **Issue**: None.

### `FailoverError` recovery-hint methods
- **Happy**: All four methods return the expected value per reason. Covered exhaustively by `TestRecoveryHintsCoverEveryReason` at `error_classifier_r6_test.go:175`.
- **Unhappy**: `FailoverError{Reason: ""}` (unconstructed). All four methods return false. Safe default.
- **Corner**: `FailoverError{Reason: "nonexistent_reason"}`. All four methods return false via the default switch arm. Correct fail-closed.
- **Edge**: The test matrix at `error_classifier_r6_test.go:200` has one row per reason and fails to compile if a new reason is added without updating the matrix (length mismatch check). Correct fail-fast design.
- **Issue**: None.

### Interaction with existing OpenClaw patterns
Ran `go test ./pkg/providers/ -v` — all existing R2-era tests pass unchanged. No regressions. The existing `TestClassifyError_RateLimitPatterns`, `TestClassifyError_OverloadedPatterns`, etc. still green. Confirmed.

---

## Cross-package concerns

- **pkg/tools/registry.go does not yet adopt Runnable.** The typed dispatch path exists but is not wired into the tool registry that the agent loop uses. This is intentional for the current slice — wiring it would require changing every existing tool's registration. Flag as a follow-up: P1 will need to migrate the registry to accept both `Tool` and `principal.Runnable` values. Without this wiring, the compile-time principal guarantee does not actually reach production code. High-priority follow-up, not a blocker for merging the current slice.
- **No dependency on pkg/principal from pkg/providers.** Intentional — errclass is a lower-level concern. Clean.
- **Model-dispatch runtime safety.** Confirmed by reading `pkg/agent/loop.go`: the current model dispatch path calls `tool.Execute(ctx, args map[string]any)` on the untyped `pkg/tools.Tool` interface. Until the registry migration happens, the principal check doesn't fire. This is why the follow-up matters.
- **Missed hermes patterns.** Checked hermes `agent/error_classifier.py` for patterns Ottie still doesn't have. Ottie is missing: `_USAGE_LIMIT_PATTERNS` needs disambiguation between billing and rate_limit. Ottie's current `rateLimitPatterns` include "usage limit" but that pattern can also mean billing. Hermes disambiguates via a second pass. Low priority — add a follow-up TODO if needed.

---

## Amendments before merge

1. **AMENDMENT 1 (medium)**: Make `PrincipalContext[C]` struct fields unexported and expose them via read accessors. Prevents literal construction of a wallet-typed principal without going through the upgrade gate. `pkg/principal/principal.go:100`.

2. **AMENDMENT 2 (high)**: Check `SignedConsent.ExpiresAt` in `validateUpgrade`. Without this, a replayed consent with a past expiry is still accepted. `pkg/principal/principal.go:238`.

3. **AMENDMENT 3 (low)**: Either make `UntypedPrincipal.HeldCaps` unexported or add a `Normalize()` method that enforces monotonicity after hand construction. `pkg/principal/typed_tool.go:70`.

4. **AMENDMENT 4 (nice-to-have)**: Add a `TestPatternsAreDisjoint` that cross-checks pattern sets across FailoverReason classes. `pkg/providers/error_classifier_r6_test.go`.

5. **FOLLOW-UP (high, separate commit)**: Wire `pkg/principal.Runnable` into `pkg/tools/registry.go` so the compile-time principal guarantee reaches the model-dispatch path.

## Verdict

**READY WITH AMENDMENTS.**

Amendments 1 and 2 are load-bearing for the R7 compile-time safety
guarantee to hold in practice (the `ExpiresAt` replay gap is the more
urgent one). Amendment 3 is low-priority polish. Amendment 4 is a
test-only nice-to-have. Fix 1 and 2 in the same commit; land the
implementation; follow up with the registry wiring and amendments 3
and 4 as separate commits.

## Closing

Amend and merge. The compile-time-safety claim is structurally sound;
the two amendments close the remaining construction-path and
replay-window gaps. Ship after those are in.
