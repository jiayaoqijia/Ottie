# Test Plan Review: ottie-test-plan.md

> Independent dual-agent review (CC strategic + Codex code-grounded) of the
> 320-item test plan. Date: 2026-04-12.
>
> **CC agent**: Strategic review — completeness, prioritization, architecture, regulatory.
> **Codex agent**: Code-grounded review — file:line citations, accuracy checks, feasibility.

---

## EXECUTIVE SUMMARY

The test plan is directionally strong: it correctly identifies the three highest-risk
gaps (agent loop ACS integration at 0 tests, end-to-end replay at 0 tests, skills
loader recursive layout at 0 dedicated tests). However, both reviewers found
**significant issues** that should be addressed before implementation begins:

1. **6 gaps the plan itself misses** — including a real bug (`ToolSchemaHash` is a
   stand-in), a production defect (`forceCompression` orphans tool-result pairs),
   and two security injection surfaces the plan doesn't cover.
2. **Priority inversions** — 3 items rated P0 should be P1/P2; 4 items not in the
   plan at all should be P0/P1.
3. **Effort underestimate** — the 6-7 day estimate for 165 tests is optimistic by
   ~2x given the test infrastructure required for ACS integration tests.
4. **Regulatory tests need a prerequisite** — 3 of 4 required Article 12/14/15
   architectural elements don't exist yet; writing tests before code is wasteful.

---

## 1. ACCURACY CHECK: What the Plan Gets Right and Wrong

### Confirmed Correct

| Plan Claim | Verification |
|-|-|
| Agent loop ACS integration — 0 tests | Confirmed. `loop_test.go` has 18 test functions, none touch `allocateACSTurnNumber`, `acsTurnCounters`, or `beginACSContext`. |
| `TestRecoverOrphansWithMissingManifest` missing | Confirmed. `acs_test.go` only tests the enriched-with-manifest path. The `ManifestPresent=false` branch at `acs.go:283` is untested. |
| `TestDispatchCommitFailureReportsCorrectly` missing | Confirmed. `dispatch_test.go` tests nil-result abort and normal commit, but no test forces `CommitAction` to error while the tool succeeds. `DispatchResult.FinalizeErr`/`Committed=false` at `dispatch.go:192-197` is uncovered. |
| `errors.Is` bug in error classifier | Confirmed real. `error_classifier.go:227-238` uses `==` not `errors.Is()`. Wrapped cancel/deadline errors leak as `FailoverUnknown`. Existing test `TestClassifyError_ContextDeadlineExceeded` only tests raw sentinels. |
| Skills loader — no category-nested tests | Partially correct. There ARE 9 loader tests, but all use flat layout. The `WalkDir` category-nested path (e.g., `workspace/skills/defi/lido-mcp/SKILL.md`) is genuinely untested. |

### Inaccurate or Overstated

| Plan Claim | Correction |
|-|-|
| "pkg/acs — 35 covered tests (CC) / 34 (Codex)" | Both overcount. Actual `func Test` count: `acs_test.go` has 13, `dispatch_test.go` has 17. Total is **30**, not 34-35. The plan counts subtests as distinct tests. |
| Item #1 "atomic counter + LoadOrStore race produces UNIQUE violations on every restart" | Mechanistically wrong. The race is a TOCTOU between `Load` returning `!ok` and `LoadOrStore` for concurrent first-calls per session. The `LoadOrStore` loser correctly uses the winner's counter, so duplicate turn numbers only happen in a narrow window — not "on every restart." |
| "TestCorruptMalformedJSONInSkillHashes — only empty-string tested" | Conflates two packages. The empty-string test is in `execmanifest_test.go:780`, not the skills loader. In the skills loader, `getSkillMetadata` at `loader.go:282` silently falls through to YAML parsing when JSON is invalid — this has no test at all. Two distinct gaps blurred into one. |
| "Skills Loader — 0 dedicated tests" (Codex summary line) | Wrong. There are 9 loader tests. The gap is specifically the category-nested `WalkDir` behavior, not "zero tests." |

---

## 2. GAPS THE PLAN ITSELF MISSES

Both reviewers independently discovered gaps not in the 320-item plan:

### Gap A — `ToolSchemaHash` Is a Stand-In (Both reviewers)

`loop.go:2388`: `ToolSchemaHash: promptHash` — the tool schema hash is set to the
same value as the prompt hash. Every replay query comparing tool schema hashes
produces false positives. The plan mentions replay tests but never flags that the
schema hash field is architecturally broken. **Impact**: The replay story — Ottie's
core differentiator — silently produces wrong data in every manifest row.

**Recommended test**: `TestToolSchemaHashIsCurrentlyStandIn` — assert
`ToolSchemaHash == PromptHash` in all current rows (documenting the known-wrong
state) so a future real-hash implementation will break this test intentionally.

### Gap B — `acsPrincipalLabel` Injection via `channel`/`chatID` (Both reviewers)

`loop.go:2417-2428`: The principal label uses string concatenation
(`"channel=" + channel + ":" + chatID`) with no escaping. A `chatID` of
`"123;user=alice"` injects a second key-value pair. This is the **same injection
class** as plan item #14 (`TestSerializePrincipalLabelWithSpecialCharacters`) but
in a completely separate code path. The plan misses the `loop.go` variant.

**Recommended test**: `TestACSPrincipalLabelInjectionViaChannelChatID` [P0]

### Gap C — `forceCompression` Orphans Tool-Result Pairs (CC only)

`loop.go:1993`: `summarizeSession` filters out `Role: "tool"` messages, but
`forceCompression` at line 1843 works on `history[1:]` which includes tool messages.
Forced compression can split a tool-call/tool-result pair across the boundary,
producing history where an assistant message references a `tool_call_id` but the
corresponding `tool` result was dropped. This breaks OpenAI/Anthropic API message
format requirements. **This is a production defect, not just a test gap.**

**Recommended test**: `TestForceCompressionDoesNotOrphanToolResultMessages` [P0]

### Gap D — `Close()` Does Not Join `activeRequests` (CC only)

`loop.go:513-550`: `AgentLoop.Close()` does not call `al.activeRequests.Wait()`.
If `Close()` is called while LLM requests are in flight, the ACS bundle is closed
before those requests finish, producing `acs.ErrClosed` on `RecordLLMCall`.

**Recommended test**: `TestCloseRacesWithInFlightLLMCall` [P1]

### Gap E — `escapeXML` Missing `"` and `'` Escaping (Codex only)

`loader.go:412-416`: `escapeXML` escapes `&`, `<`, `>` but not `"` or `'`. If the
XML output is ever embedded in an HTML attribute context, unescaped quotes allow
attribute injection. Additionally, `s.Source` at `loader.go:241` is NOT escaped
(currently low risk since source is hardcoded to "workspace"/"global"/"builtin").

### Gap F — `LoadSkill` Path Traversal Wider Than Plan States (Codex only)

`loader.go:165`: `filepath.Join(root, name, "SKILL.md")` — the plan scopes
`TestPathTraversalInSkillName` to `ListSkills` only. The real exposure is in
`LoadSkill`, which directly `os.ReadFile`s the resolved path. A caller of
`LoadSkill("../../sensitive")` bypasses all validation. **The security test must
target `LoadSkill` directly.**

### Gap G — FNV-1a Hash Over 256-Byte Prefixes Breaks Replay (CC only)

`loop.go:2482-2501`: The prompt hash is FNV-1a over the first 256 bytes of each
message. Two 4000-token prompts sharing the same 256-byte prefix produce identical
`prompt_hash` values, making the replay story unreliable. The plan mentions replay
tests but never flags this weak-hash design limitation.

**Recommended test**: `TestHashMessagesForACSDistinguishesLongPrompts` [P1]

### Gap H — `acsCallSeq` Data Race Under Fallback Chain (CC only)

`loop.go:1352-1376`: `acsCallSeq` is a plain `int` captured by the `acsChat`
closure. Under fallback chain execution with multiple concurrent provider attempts,
multiple goroutines modify `acsCallSeq` without synchronization. The plan's
`TestACSChatCallSeqIncrements` doesn't identify this as a `sync/atomic` requirement.

**Recommended test**: `TestACSCallSeqAtomicUnderFallbackChain` (use `-race`) [P0]

---

## 3. PRIORITY CORRECTIONS

### Should Be P0 (Not in plan or under-prioritized)

| Test | Why |
|-|-|
| `TestACSPrincipalLabelInjectionViaChannelChatID` | Security: principal injection via chatID in production dispatch path |
| `TestForceCompressionDoesNotOrphanToolResultMessages` | Production defect: breaks API message format |
| `TestACSCallSeqAtomicUnderFallbackChain` | Data race detectable by `-race` |
| `TestToolSchemaHashIsCurrentlyStandIn` | Documents known-wrong replay data |

### Should Be Downgraded

| Test | Current | Recommended | Reason |
|-|-|-|-|
| `TestSQLInjectionInToolName/PrincipalLabel` (#19) | P0 | P2 | Parameterized `?` queries throughout `actionlog.go` and `execmanifest.go` make SQL injection impossible. Keep for documentation value only. |
| `TestEffectClassDeclarationsMatchDocumentation` (#17) | P0 | P1 | Declarations exist; worst failure is a side-effecting tool skipping ledger wrap. Doesn't block demo. |
| `TestZeroValuedWritesWalletCannotPassGuardDispatch` (#18) | P0 | P1 | The compile-time guarantee is already enforced by the type system. Runtime check is belt-and-suspenders. |

### Priority Inversion: Regulatory Tests

The plan orders regulatory compliance tests as step 7 (last infrastructure step).
With the EU AI Act deadline 112 days away, and 3 of 4 required architectural
elements not yet implemented, regulatory work should run **in parallel** with
infrastructure work, not after it. Current order creates a critical path with no
remediation buffer.

---

## 4. EFFORT ESTIMATE CORRECTION

The plan estimates 165 tests in 6-7 days. Both reviewers agree this is optimistic.

| Category | Plan Estimate | Realistic Estimate | Why |
|-|-|-|-|
| Agent loop ACS integration (23 tests) | ~1 day | 3-4 days | Requires full in-process agent harness (mock bus, mock provider, real ACS bundle). Each test ~30-60 min. |
| End-to-end replay (12 tests) | ~0.5 day | 2-3 days | Requires full turn execution, trace ID capture, cross-table SQLite queries. 2-4 hours each. |
| Regulatory compliance (19 tests) | ~1 day | Blocked | 3 of 4 required elements (user identity, retention enforcement, human oversight wiring) need code changes first. |
| P0 total (~80 tests) | 3-4 days | **8-12 days** | |
| All 165 tests | 6-7 days | **12-16 days** | |

### Tests Requiring Significant Infrastructure

- `TestFullTurnWithACSSingleProvider` — High effort. `newTestAgentLoop` helper doesn't initialize `acs` field; that scaffolding is non-trivial.
- `TestReplayFromTraceID` — High effort. Replay query logic joining `traces`, `trace_provider_requests`, and `action_intents` doesn't exist as a function yet.
- `TestFallbackChainShouldFallbackWiring` — High effort. Multi-provider setup with mock failure + ACS recording verification.
- `TestArticle12_EveryToolDispatchHasAuditTrail` — Blocked on ACS-wired loop scaffold.

### Tests That Are Straightforward

- `TestClassifyErrorWrappedContextCanceled` — Trivial. Add `fmt.Errorf("wrap: %w", context.DeadlineExceeded)` to existing test file. Also fix the source (`errors.Is` instead of `==`).
- `TestEffectClassDeclarationsMatchDocumentation` — Low effort. Iterate 7 declared tools, call `ClassOf`, assert match.
- `TestBuildSkillsSummaryXMLEscaping` — Low effort. Uses existing `createSkillDir` helper.
- `TestSQLInjectionInToolName` — Low effort. Round-trip parameterized insert with adversarial input.

---

## 5. EXISTING TEST QUALITY ISSUES

Both reviewers identified weak existing tests the plan should mention upgrading:

| Test | Issue |
|-|-|
| `TestDispatchEveryEffectClassIsWrapped` (`dispatch_test.go:438`) | Verifies `IntentID != ""` and `Committed == true` but never reads back the stored `effect_class` column to verify the correct string was persisted. A tool returning `EffectWritesChain` that stores `"writes_wallet"` would pass. |
| `TestBundleCloseOrderIsLedgerFirstThenManifest` (`acs_test.go:568`) | Self-documented as "more a TODO than a hard assertion" — test body does nothing except open and close a bundle. Should be upgraded or removed. |
| `TestConcurrentBeginsSerializeAndUnique` (`execmanifest_test.go:420`) | Fires concurrent `Begin` calls but doesn't verify all successful rows have distinct `(session_id, turn)` pairs. Relies on UNIQUE constraint implicitly. |
| `TestConcurrentCloseAndOps` (`acs_test.go:376`) | Uses `time.Sleep(5ms)` for synchronization — timing-dependent and flaky on fast/slow CI. Should use `sync.WaitGroup` + ready channel. |

---

## 6. TEST DESCRIPTIONS NEEDING CLARIFICATION

| Item | Problem | Suggested Clarification |
|-|-|-|
| `TestACSOffPathBitForBitIdenticalToPreR11` (#20) | "Bit-for-bit identical" is undefined. Cannot compare LLM responses without deterministic mock. | "Given the same mock provider returning the same response, session history and tool results are identical regardless of whether `al.acs` is non-nil." |
| `TestFallbackChainShouldFallbackWiring` (#11) | Implies testing `ShouldFallback` directly, but the retry engine calls `al.fallback.Execute` which internally uses it. | "Given a fallback chain with provider A returning rate-limit error and provider B configured, assert provider B is called and ACS manifest records both attempts." |
| `TestArticle12_EveryToolDispatchHasAuditTrail` (#13) | Completely unspecified. Article 12 has specific field requirements. | Must assert: operator identity, timestamp, input/output content, automated decision markers. Also: read-only tools are currently bypassed at `dispatch.go:106-108` — clarify if Article 12 requires ALL dispatches or only side-effecting ones. |
| `TestReplayFromTraceID` (#7) | Mentions "tools" reconstruction, but manifest stores `ToolSchemaHash` (a hash), not tool list. | Scope to existing fields: trace_id -> manifest row -> provider call rows. Defer "tools" until `ToolSchemaHash` is a real hash. |
| `TestMonotonicTurnCounterAfterHistorySummarization` (#4) | Needs to specify the counter is in-memory (`acsTurnCounters`), not derived from `len(history)`. | "After `summarizeSession` truncates history, the next `allocateACSTurnNumber` returns a value higher than any existing turn on disk." |
| `TestSerializePrincipalLabelWithSpecialCharacters` (#14) | Does not define expected behavior: reject, escape, or document? | Specify whether the fix is input validation (error on `=`/`;`) or encoding (percent-encoding). Without this, the test cannot be implemented. |
| `TestRecoverOrphansWithMissingManifest` (#2) | Ambiguous scope: verify `ManifestPresent=false` return (trivial) or simulate crash mid-Prepare (complex)? | Specify both: (a) unit test asserting `ManifestPresent=false` when trace missing, (b) integration test with injected crash between Prepare and Commit. |

---

## 7. ARCHITECTURAL GAPS NOT IN THE PLAN

### Schema Migration — Zero Coverage

Both `actionlog.go` and `execmanifest.go` use `CREATE TABLE IF NOT EXISTS`. No
migration system exists. A future column addition would silently leave existing
databases at the old schema. No tests for: schema version detection, forward
migration, backward compatibility, or SQLite `ALTER TABLE` behavior in WAL mode.

### `acsTurnCounters` Memory Leak

The `sync.Map` grows indefinitely as new sessions are created. A long-running
deployment with many unique sessions accumulates memory without bound. Needs a
TTL eviction strategy before production scale.

### ACS Not Re-Initialized on Hot Reload

`ReloadProviderAndConfig` (`loop.go:567-676`) swaps registry, config, fallback
chain — but does NOT close/reopen the ACS bundle. If config changes `ACS.DBDir`
or `ACS.WriteQueueDepth`, the bundle keeps old config.

### ACS `Open` Silently Fails When Subdir Missing

`loop.go:155-173`: ACS defaults `dbDir` to `filepath.Join(workspace, "acs")` but
no code creates this subdirectory. `Open` fails, error is only logged with
`WarnCF`, and ACS is silently disabled. Operators enabling ACS get a silent no-op.

---

## 8. REGULATORY COMPLIANCE ASSESSMENT

With 112 days until EU AI Act enforcement (August 2, 2026), the regulatory section
needs a **prerequisite compliance gap analysis** before test writing:

| Article | Current State | Test-Ready? |
|-|-|-|
| **Article 12 (Record-keeping)** | Action ledger + exec manifest are plausible, BUT: `user=unknown` in principal labels means operator can't identify authorizing human; no retention-period enforcement exists. | No — needs code changes first |
| **Article 14 (Human oversight)** | `UpgradeToWritesWallet` consent gate exists in type system, but no code in agent loop calls it. `acsPrincipalLabel` bypasses typed principal entirely. | No — oversight wiring not connected |
| **Article 15 (Accuracy/robustness)** | No accuracy measurement exists. Tests would need external benchmarks or golden datasets not in the repo. | No — no measurement infrastructure |
| **Article 12 read-only gap** | `dispatch.go:106-108` explicitly skips ledger recording for `EffectReadOnly` tools. If Article 12 requires ALL tool audit trails, architecture doesn't satisfy it. | Needs legal clarification |

**Recommendation**: The 19 regulatory test items should be preceded by a compliance
gap analysis deliverable that identifies which requirements need code changes vs.
which can be verified with existing infrastructure. Without this, the tests are
testing infrastructure that doesn't exist.

---

## 9. FLAKY TEST RISKS

| Proposed Test | Risk | Mitigation |
|-|-|-|
| `TestAllocateACSTurnNumberConcurrent` | Race window requires concurrent first-call goroutines; non-deterministic | Use `sync.WaitGroup` start gate, not naked `go func()` |
| `TestConcurrentDispatchAndCloseOnSameBundle` | Timing-dependent interleave of Prepare-Commit with Close | Use barrier between Prepare and Commit phases |
| `TestFullTurnWithACSSingleProvider` | I/O-bound SQLite writes may not flush before assertions | Use synchronous mode (`WriteQueueDepth=0`) in tests |
| `TestFallbackChainShouldFallbackWiring` | Retry timing sensitive to goroutine scheduling | Mock provider with deterministic delays |
| Existing `TestConcurrentCloseAndOps` | Uses `time.Sleep(5ms)` — flaky on fast/slow CI | Replace with `sync.WaitGroup` + ready channel |

---

## 10. RECOMMENDED ACTIONS BEFORE IMPLEMENTATION

1. **Correct the plan** — fix the 4 inaccurate claims and 7 vague test descriptions identified above.
2. **Add 8 missing tests** — the gaps both reviewers found (especially `TestForceCompressionDoesNotOrphanToolResultMessages` which is a production defect).
3. **Re-prioritize** — downgrade 3 items from P0, promote 4 new items to P0/P1.
4. **Revise effort estimate** — P0 alone is 8-12 days, not 3-4. Total is 12-16 days.
5. **Build ACS test scaffold first** — the `newTestAgentLoop` helper needs ACS field initialization before ~35 tests can be written.
6. **Regulatory gap analysis** — before writing any of the 19 compliance tests, produce a deliverable identifying code changes needed for Articles 12, 14, 15.
7. **Run regulatory work in parallel** — don't sequence it after all infrastructure work.
8. **Upgrade 4 weak existing tests** — before adding new ones, fix the assertions that give false confidence.
