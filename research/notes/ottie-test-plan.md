# Ottie Comprehensive Test Plan — R6 through R14

> Produced by two independent 30-year test experts (CC strategic +
> Codex code-grounded) reading all source files across 10 feature
> areas. Date: 2026-04-12.
>
> **CC agent**: 311 items (147 covered, 164 missing) across 10 feature areas + regulatory compliance.
> **Codex agent**: 285 items (140 covered, 145 missing) across 7 feature areas with file:line citations.
> **Synthesized**: ~320 unique test items after dedup.

## Coverage Summary (CC)

| Feature Area | Covered | Missing | Total |
|-|-|-|-|
| pkg/principal (PrincipalContext) | 4 | 12 | 16 |
| pkg/principal (TypedTool/Adapters) | 7 | 10 | 17 |
| pkg/actionlog | 27 | 13 | 40 |
| pkg/execmanifest | 29 | 12 | 41 |
| pkg/acs (Bundle + Dispatch) | 35 | 14 | 49 |
| pkg/tools EffectClassifier | 2 | 12 | 14 |
| pkg/providers Error Classifier | 23 | 18 | 41 |
| pkg/skills Loader | 16 | 19 | 35 |
| Agent Loop ACS Integration | 0 | 23 | 23 |
| End-to-End Replay Story | 0 | 12 | 12 |
| Regulatory Compliance (EU AI Act) | 4 | 19 | 23 |
| **TOTAL** | **147** | **164** | **311** |

## Coverage Summary (Codex)

| Area | Covered | Missing | Total |
|-|-|-|-|
| pkg/principal | 17 | 22 | 39 |
| pkg/actionlog | 27 | 19 | 46 |
| pkg/execmanifest | 32 | 17 | 49 |
| pkg/acs + dispatch | 34 | 16 | 50 |
| error classifier | 30 | 20 | 50 |
| skills loader | 0 | 28 | 28 |
| agent loop ACS | 0 | 23 | 23 |
| **Totals** | **140** | **145** | **285** |

## TOP 20 PRIORITY TESTS (Both Agents Converged)

These are the tests both CC and Codex independently identified as the highest-risk gaps. Implement these first.

1. **TestAllocateACSTurnNumberConcurrent** — atomic counter + LoadOrStore race is the highest-risk untested path; a bug produces UNIQUE constraint violations on every restart [P0]
2. **TestRecoverOrphansWithMissingManifest** — the crash-recovery scenario the entire ACS was designed for [P0]
3. **TestFullTurnWithACSSingleProvider** — end-to-end: user message → BeginTurn → Chat → tool dispatch → Commit; all rows present [P0]
4. **TestMonotonicTurnCounterAfterHistorySummarization** — history summarization shrinks len(history); turn counter must NOT reset [P0]
5. **TestClassifyErrorWrappedContextCanceled/DeadlineExceeded** — current code uses `==` not `errors.Is()`; wrapped cancel/deadline errors leak as FailoverUnknown [P0]
6. **TestDispatchCommitFailureReportsCorrectly** — silent commit failure hides ledger/reality drift; Committed=false + FinalizeErr untested [P0]
7. **TestReplayFromTraceID** — given trace_id, reconstruct prompt hash, model, tools, provider calls [P0]
8. **TestACSChatRecordsEveryProviderCall** — every Chat invocation including retries gets a RecordLLMCall [P0]
9. **TestACSChatRecordsActualModelNotConfigured** — fallback-chain uses model-B; row says model-B [P0]
10. **TestListSkillsCategoryNestedLayout** — skills loader has zero test coverage for the category layout [P0]
11. **TestFallbackChainShouldFallbackWiring** — ShouldFallback drives the retry engine but its integration is untested [P0]
12. **TestCorruptMalformedJSONInSkillHashes** — real-world corruption recovery (only empty-string tested, not malformed JSON) [P0]
13. **TestArticle12_EveryToolDispatchHasAuditTrail** — EU AI Act record-keeping compliance [P0]
14. **TestSerializePrincipalLabelWithSpecialCharacters** — "=" or ";" in labels would enable principal impersonation [P0]
15. **TestConcurrentDispatchAndCloseOnSameBundle** — hardest shutdown race across the entire ACS [P1]
16. **TestBuildSkillsSummaryXMLEscaping** — skills with `<script>` in names inject markup into system prompt [P1]
17. **TestEffectClassDeclarationsMatchDocumentation** — 7 tool declarations have no verification [P0]
18. **TestZeroValuedWritesWalletCannotPassGuardDispatch** — forged zero-valued wallet principal must be blocked [P0]
19. **TestSQLInjectionInToolName/PrincipalLabel** — parameterized query safety for the ledger [P0]
20. **TestACSOffPathBitForBitIdenticalToPreR11** — verify zero ACS overhead when disabled [P0]

## CRITICAL GAPS BY AREA

### Agent Loop ACS Integration — 0 tests exist, 23 missing

Both agents flagged this as the #1 gap. The wiring between loop.go and pkg/acs that makes the whole replay story work has ZERO dedicated tests. Key items:

- TestBeginACSTurnReturnsTraceID / ReturnsEmptyWhenACSNil
- TestMonotonicTurnCounterSeededFromMaxTurn / SurvivesRestart / Increments / PerSession
- TestACSChatRecordsEveryProviderCall / CallSeqIncrements / RecordsActualModel
- TestToolDispatchThroughACSForSideEffecting / BypassesACSForReadOnly
- TestACSOffPathBitForBitIdenticalToPreR11
- TestFullTurnWithACSSingleProvider / FallbackChain / RetryLoop
- TestMonotonicTurnCounterAfterHistorySummarization
- TestACSTurnAllocRaceCondition

### End-to-End Replay Story — 0 tests exist, 12 missing

The demo-critical capability that positions Ottie against hermes-agent is untested:

- TestReplayFromTraceID — reconstruct everything from a trace_id
- TestReplayFindsMatchingActionIntent — cross-table join
- TestReplayProviderCallsOrderMatchesExecution
- TestReplayPromptHashMatchesActualPrompt
- TestReplayArgsHashMatchesActualArgs / ResultHashMatchesActualResult
- TestReplayOrphanedIntentShowsMissingOutcome
- TestReplayMultipleToolDispatchesInOneTurn
- TestReplayFallbackChainShowsAllAttempts

### Skills Loader — 0 dedicated tests for new recursive layout

Codex confirmed: the loader has no dedicated tests for the category-nested `WalkDir` behavior added in R6. Key items:

- TestListSkillsFlatLayout / CategoryNestedLayout
- TestLoadSkillNestedCategoryLayout / FlatPathFirst / RecursiveFallback
- TestSkillInfoValidateNameRegex boundary cases
- TestPathTraversalInSkillName (security)
- TestBuildSkillsSummaryXMLEscaping (security)

### Regulatory Compliance — 19 missing

EU AI Act enforcement August 2, 2026. No tests validate the record-keeping (Article 12), human oversight (Article 14), or accuracy/robustness (Article 15) properties that Ottie's positioning depends on.

## IMPLEMENTATION ESTIMATE

| Priority | Count | Effort |
|-|-|-|
| P0 (blocks ship/demo/compliance) | ~80 tests | 3-4 days |
| P1 (should ship with) | ~50 tests | 2 days |
| P2 (nice-to-have) | ~35 tests | 1 day |
| **Total** | **~165 tests** | **~6-7 days** |

## RECOMMENDED ORDER

1. Agent loop ACS integration tests (P0, ~23 items)
2. End-to-end replay story tests (P0, ~12 items)
3. Skills loader nested layout + security (P0, ~10 items)
4. Principal upgrade gate edge cases + security (P0, ~12 items)
5. Error classifier recovery hints + priority + fallback integration (P0, ~18 items)
6. EffectClassifier integration verification (P0, ~12 items)
7. Regulatory compliance tests (P0, ~19 items)
8. Remaining P1/P2 items
