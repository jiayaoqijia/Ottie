# CC Acceptance Review of Cleaned Ottie Design Doc — R7

> Reviewer persona: same 30-year AI/agent veteran as R3-R6.
> Framing: acceptance review of the cleaned-up design doc (909 lines,
> post-R6, folded six rounds of accumulated amendments into a single
> actionable plan). Not a gap hunt. The question is whether the cleanup
> preserved every load-bearing decision, whether the R6 code changes
> actually landed, and whether the three bet-the-demo deltas are
> specified tightly enough to ship.

## Opening

The cleaned doc holds together. Reading it end-to-end, it is a coherent
plan — §0 TL;DR is truthful about what was cut from 2221 lines, the
phase plan in §3-§8 is numbered and actionable, §10 lists concrete
files and line numbers for every P1-P4 change, §14 Review History
preserves R1-R6 findings in ~15 lines per round, and §15 Revision
History captures the decisions that actually moved the effort estimate
(6 → 7.5 → 11 → 14 → 17 → 21 → 12 weeks as duplication collapsed).
The crucial thing — that the three bet-the-demo deltas from R6 (replay,
PrincipalContext, block-anchored validity + RPC shadow) are preserved
and specified tightly — is in the doc. No load-bearing R3-R6 decision
got lost in the cleanup.

## Code spot-check

Every §10.1 claim verified against live source (post-cleanup state):

| R6 change | §10.1 claim | Matches live source? | Evidence |
|-|-|-|-|
| Skill cuts (8 skills deleted) | `agency-roles`, `autoresearch`, `weather`, `summarize`, `tmux`, `github`, `browser`, `dep-audit` gone | ✓ | Neither `workspace/skills/<cut>` nor `cmd/ottie/internal/onboard/workspace/skills/<cut>` exists for any of the 8 (confirmed by directory listing before CC R7). |
| Skill reorg (7 categories) | `crypto/`, `defi/`, `identity/`, `payments/`, `safety/`, `research/`, `meta/` | ✓ | All 7 category folders present at `workspace/skills/`, totaling 28 `SKILL.md` files. Per-category counts: crypto 5, defi 7, identity 4, payments 2, safety 5, research 2, meta 3. |
| Recursive loader | `pkg/skills/loader.go` uses `filepath.WalkDir` | ✓ | `ListSkills` at `pkg/skills/loader.go` walks recursively; `LoadSkill` does a flat lookup first then falls back to `filepath.WalkDir` with a name-matching predicate. Test suite `pkg/skills/...` passes. |
| `SubagentTool` deleted | sync duplicate of async spawn removed | ✓ | `pkg/tools/subagent.go` now 234 lines (was 363); only `SubagentManager` backend + `SubagentTask`/`runTask`/`GetTask`/`ListTasks`. Test file `pkg/tools/subagent_tool_test.go` deleted. `MockLLMProvider` relocated to new `pkg/tools/mock_provider_test.go`. |
| `spawn` / `sessions_spawn` → `"delegate"` | both tools return `"delegate"` from `Name()` | ✓ | `pkg/tools/spawn.go` `SpawnTool.Name()` returns `"delegate"`; `pkg/tools/swarm_spawn_tool.go` `SessionsSpawnTool.Name()` returns `"delegate"`. `pkg/agent/loop.go:291` uses `cfg.Tools.IsToolEnabled("delegate") || cfg.Tools.IsToolEnabled("spawn")` with `agent.Subagents != nil` as the orchestrator branch gate — mutually exclusive. |
| `find_skills`/`install_skill` default-off | `pkg/config/defaults.go:503` flipped to `false` | ✓ | `FindSkills: ToolConfig{Enabled: false}`, `InstallSkill: ToolConfig{Enabled: false}` with a comment explaining the supply-chain rationale. CLI commands (`ottie skills install/search`) unaffected. |
| CLI examples updated | `weather` → `crypto-wallet` in install/remove/show | ✓ | `cmd/ottie/internal/skills/install.go:19` and `:20` reference `crypto-wallet` and `defi-swap`; `remove.go:15` and `show.go:14` reference `crypto-wallet`. |

All seven R6 change claims match live source. Full build + tests pass:
`go test ./pkg/skills/... ./pkg/tools/... ./pkg/agent/... ./cmd/ottie/... ./web/backend/...` all green.

## Doc-level defects

**None load-bearing.** A few minor observations worth noting but none
block shipping:

1. **§8 Honcho numbering anomaly.** §8 is "Phase 2.5 — Honcho as an
   Optional Plugin (opt-in)" which comes after §7 Phase 4 and before §9
   Prompt Caching Discipline. The section numbering works (§8 is the
   eighth top-level section), but the phase label "P2.5" after "P4" is
   a visual oddity. Low priority — fix when someone inevitably asks.
2. **§14 R1-R6 line budget.** Most rounds hit ≤15 lines cleanly; R6's
   summary overshoots slightly with the progressive-disclosure and
   tool-dedupe callouts. Still under 18 lines. Within tolerance.
3. **§10.1 skill count line.** "36 → 28 skills" is correct (earlier in
   this session I had briefly miscounted as 33 → 28 before the
   lower-confidence cuts landed; the cleaned doc shows the right
   number). Already fixed in the cleaned version.

No contradictions. No dead section references. No decisions lost in
translation.

## Does it beat hermes?

Re-evaluating the three bet-the-demo deltas against the cleaned doc's
specifications:

### Delta 1 — Deterministic turn replay: **SUFFICIENT**

The pieces: §4.5 prompt epoch discipline (invariant that the system
prompt is byte-stable within an epoch), §4.4 execution manifest
(`traces` row per turn with `prompt_hash`, `tool_schema_hash`,
`skill_hashes`, `mcp_versions`, `provider_ids`, `model_id`,
`prompt_epoch`), §4.1 `action_intents`/`action_commits` ledger (fsync
before side-effecting tool runs, transition to committed/aborted after).

Combined: given a `trace_id` you can reconstruct `(prompt bytes, tool
schema set, skill set, provider call IDs)` and — if the provider ID
system is good enough — replay the exact same provider call with the
exact same input. Ottie's Go runtime has no asyncio scheduling,
goroutine scheduling is deterministic at the annotation level, and
there's no serialized object state drift. Python's hermes cannot match
this without a major rewrite.

One latent assumption: the provider API must be pure given
`(model_id, messages, tools, options)` plus a `provider_request_id`
that resolves to the same response. Most providers do support this via
idempotency keys or cached request IDs, but a P0 conformance test
should pin it down ("replay the same `provider_request_id` from a
traces row, assert byte-identical response"). Add this to §3.1.

### Delta 2 — Typed PrincipalContext: **SUFFICIENT**

§4.3 specifies the struct, the `CapabilitySet` bit-field, the
`TypedTool[T]` generic constraint, and the compile-time dispatch
gate. This is tight enough that an unauthorized signing fails at build
time, not runtime. Hermes's equivalent at
`research/hermes-agent/run_agent.py:1179` is a string `user_id` with
no type-level check.

One thing the cleaned doc should state explicitly: the `TypedTool[T]`
check is performed at tool *registration*, not just dispatch. Add one
sentence in §4.3: "At `tools.Register(toolInstance)`, the registry
asserts via reflection that the tool declares its required
capabilities and that its dispatch signature is type-compatible with
the `PrincipalContext`." Otherwise, a well-intentioned contributor
might add a new effectful tool without declaring `RequiredCaps()` and
the registration would silently accept it.

### Delta 3 — Block-anchored memory validity + RPC shadow: **SUFFICIENT**

§5.2 defines `environment_facts.chain_id` + `block_number` +
`observed_at` + `valid_to`. The per-category decay table is precise
(preference/jurisdiction never, holding 24h, market_quote 15m, apr
1h, gas 5m, governance_state 7d, contract_meta 30d). §7.4 specifies
`pkg/rpcshadow/` as a wrap-anything `ShadowRPC` type with
record/replay semantics keyed by block number.

The story holds together: "the stETH APR I recall was measured at
block 21,234,567 — is it still valid?" becomes a single `eth_call` +
a confidence update based on `valid_to` + a shadow replay of the
signing path if `valid_to` has passed. Hermes stores wall-clock only
(`research/hermes-agent/hermes_state.py:79`) and cannot offer this.

One small clarity improvement: §5.2's "per-category decay policy"
table should note whether `valid_to` is set by the writer (e.g.,
`memory_manage` tool sets it explicitly) or by a background job that
reads the category and computes it. Current reading suggests the
writer sets it from a per-category default, which is fine — just say
so explicitly.

## Verdict

**READY TO SHIP.**

The cleaned doc is coherent, internally consistent, preserves every
load-bearing R3-R6 decision, and specifies the three bet-the-demo
deltas tightly enough to begin P0 immediately. The R6 code changes
have all landed. Tests pass. The three minor clarity improvements in
§4.3 (registration-time type check), §3.1 (provider replay test),
and §5.2 (explicit `valid_to` writer) are worth making as follow-up
commits but do not block P0 start.

## Closing

Ship. Start P0 on Monday. R8 should only exist if a live-source bug
surfaces during P1 implementation that the cleaned plan didn't
anticipate — not for another orthogonal review round. Both CC and
Codex agreed in R6 that review is done; CC R7 confirms the cleanup
didn't break that conclusion.
