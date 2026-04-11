# Codex R7 Acceptance Review — Cleaned Design Doc

> Persona: 30-year AI/agent research veteran, same as R2/R3/R4/R5/R6.
> Session: background task `b6gvk7wn4`, xhigh reasoning,
> `--dangerously-bypass-approvals-and-sandbox`.
> Framing: acceptance review of the cleaned-up design doc (2221 → 909
> lines). Goal: confirm the cleanup preserved R6 decisions and the code
> changes landed.
> Verdict: **DO NOT SHIP** — two load-bearing P1 regressions found in the
> cleanup (replay contract under-specified; principal-safety compile-time
> claim contradicts its own runtime spec). Fix both, plus two reference
> typos and the stale `helpers.go` helper, before P0 starts.

---

**Opening**

The cleaned doc is mostly coherent and actionable, and the live tree does reflect the R6 cuts, reorg, tool dedupe, and default-off changes. But it does not preserve every load-bearing R3-R6 decision: the replay contract lost `provider_request_ids[]` and unambiguous `action_commits` semantics, and §4.3 promises compile-time signing exclusion while actually specifying a runtime capability check. On the three R6 bet-the-demo axes, something was lost in translation on replay and principal safety; the blast-radius/RPC-shadow story survived.

**Code spot-check**

| R6 change | Claim in §10.1 | Matches live source? | Evidence |
|-|-|-|-|
| Skill cuts (8 skills gone) | 8 skills deleted; 36 → 28 skills | Mostly | Both roots now contain 28 category-nested skills; representative live files: [workspace crypto-wallet:1](/home/coder/github/aintern/workspace/skills/crypto/crypto-wallet/SKILL.md#L1), [workspace farcaster:1](/home/coder/github/aintern/workspace/skills/research/farcaster/SKILL.md#L1), [onboard crypto-wallet:1](/home/coder/github/aintern/cmd/ottie/internal/onboard/workspace/skills/crypto/crypto-wallet/SKILL.md#L1), [onboard farcaster:1](/home/coder/github/aintern/cmd/ottie/internal/onboard/workspace/skills/research/farcaster/SKILL.md#L1). Caveat: stale old-skill names still exist in `install-builtin` helper at [helpers.go:138](/home/coder/github/aintern/cmd/ottie/internal/skills/helpers.go#L138). |
| Skill reorg (7 categories) | 28 skills moved into `crypto/`, `defi/`, `identity/`, `payments/`, `safety/`, `research/`, `meta/` | Yes | One live skill exists under each category: [crypto-wallet:1](/home/coder/github/aintern/workspace/skills/crypto/crypto-wallet/SKILL.md#L1), [defi-swap:1](/home/coder/github/aintern/workspace/skills/defi/defi-swap/SKILL.md#L1), [8004:1](/home/coder/github/aintern/workspace/skills/identity/8004/SKILL.md#L1), [mpp:1](/home/coder/github/aintern/workspace/skills/payments/mpp/SKILL.md#L1), [farcaster:1](/home/coder/github/aintern/workspace/skills/research/farcaster/SKILL.md#L1), [clawwall:1](/home/coder/github/aintern/workspace/skills/safety/clawwall/SKILL.md#L1), [self-evolve:1](/home/coder/github/aintern/workspace/skills/meta/self-evolve/SKILL.md#L1). |
| Recursive loader | `ListSkills` and `LoadSkill` use `filepath.WalkDir` for nested layouts | Yes | [loader.go:101](/home/coder/github/aintern/pkg/skills/loader.go#L101), [loader.go:105](/home/coder/github/aintern/pkg/skills/loader.go#L105), [loader.go:117](/home/coder/github/aintern/pkg/skills/loader.go#L117), [loader.go:158](/home/coder/github/aintern/pkg/skills/loader.go#L158), [loader.go:170](/home/coder/github/aintern/pkg/skills/loader.go#L170). |
| `SubagentTool` deleted | Sync duplicate removed; only backend remains | Yes | `pkg/tools/subagent.go` now defines `SubagentManager`, not a `SubagentTool`, starting at [subagent.go:24](/home/coder/github/aintern/pkg/tools/subagent.go#L24); async surface is in [spawn.go:9](/home/coder/github/aintern/pkg/tools/spawn.go#L9). |
| `spawn` / `sessions_spawn` → `delegate` | Both tool surfaces return `"delegate"` | Yes | [spawn.go:23](/home/coder/github/aintern/pkg/tools/spawn.go#L23), [swarm_spawn_tool.go:24](/home/coder/github/aintern/pkg/tools/swarm_spawn_tool.go#L24). |
| Mutually exclusive delegate registration | Orchestrator gets swarm-backed delegate; non-orchestrator gets local delegate | Yes, with stale internal naming | Exclusive branch is at [loop.go:286](/home/coder/github/aintern/pkg/agent/loop.go#L286), [loop.go:291](/home/coder/github/aintern/pkg/agent/loop.go#L291), [loop.go:292](/home/coder/github/aintern/pkg/agent/loop.go#L292), [loop.go:295](/home/coder/github/aintern/pkg/agent/loop.go#L295), [loop.go:310](/home/coder/github/aintern/pkg/agent/loop.go#L310). Comments/log strings still say `sessions_spawn` at [loop.go:365](/home/coder/github/aintern/pkg/agent/loop.go#L365) and [loop.go:377](/home/coder/github/aintern/pkg/agent/loop.go#L377). |
| `find_skills` / `install_skill` default-off | Defaults flipped from `true` to `false` | Yes | [defaults.go:503](/home/coder/github/aintern/pkg/config/defaults.go#L503), [defaults.go:509](/home/coder/github/aintern/pkg/config/defaults.go#L509), [defaults.go:512](/home/coder/github/aintern/pkg/config/defaults.go#L512). |
| CLI examples updated | `weather` → `crypto-wallet` in `install.go` / `remove.go` / `show.go` | Yes | [install.go:18](/home/coder/github/aintern/cmd/ottie/internal/skills/install.go#L18), [remove.go:15](/home/coder/github/aintern/cmd/ottie/internal/skills/remove.go#L15), [show.go:14](/home/coder/github/aintern/cmd/ottie/internal/skills/show.go#L14). |

The spot-check result is clear: the R6 cleanup landed in code. The acceptance problem is not “the code changes didn’t happen”; it is that two central P1 contracts were weakened in the cleaned prose.

**Doc-level defects**

- `action_commits` is still load-bearingly ambiguous. The cleaned doc repeatedly describes a two-table `action_intents` / `action_commits` ledger in [design:59](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L59), [design:205](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L205), and [design:214](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L214), and the archived R5/R6 settlement says the same at [history:1584](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1584) and [history:1585](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1585). But the actual P1 schema defines only `action_intents` with mutable `state` in [design:321](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L321) and [design:328](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L328), and §4.4 repeats an in-place transition at [design:401](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L401) and [design:402](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L402).

- The execution manifest lost request-scoped provider correlation. The settled history requires `provider_request_ids[]` in the manifest at [history:1557](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1557), [history:1558](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1558), [history:1580](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1580), and [history:1581](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1581). The cleaned doc weakens that to `provider_ids` in [design:315](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L315) and [design:400](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L400).

- The principal-safety delta contradicts itself. The doc promises compile-time enforcement in [design:35](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L35) and [design:105](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L105), but §4.3 actually specifies a runtime `CapabilitySet` bit-field check in [design:372](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L372), [design:387](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L387), and [design:388](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L388).

- Minor cleanup residue: `§5.3 TypedTool[T]` at [design:356](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L356) should point to §4.3, and “progressive disclosure in §7” at [design:419](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L419) should point to §6. These are not load-bearing, but they confirm the cleanup was not fully lossless.

**Does it beat hermes?**

1. Deterministic turn replay: **INSUFFICIENT**.  
Missing piece: a precise immutable replay artifact contract.  
R1-R6 had `provider_request_ids[]` plus separate `action_commits` rows in [history:1557](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1557), [history:1558](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1558), [history:1584](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1584), [history:1585](/home/coder/github/aintern/research/notes/ottie-design-history-R1-R6.md#L1585); the cleaned doc blurs that into `provider_ids` and mutable `action_intents.state` in [design:315](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L315), [design:321](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L321), [design:328](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L328), [design:400](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L400), [design:401](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L401), [design:402](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L402).

2. Typed `PrincipalContext`: **INSUFFICIENT**.  
Missing piece: capability-typed principal variants or equivalent generic constraints that make signer tools uncallable without wallet-write authority.  
As written, §4.3 is still a runtime capability check, not compile-time exclusion; Hermes is weaker because it threads string `user_id` at [run_agent.py:1179](/home/coder/github/aintern/research/hermes-agent/run_agent.py#L1179), but Ottie has not yet specified the stronger compile-time property cleanly.

3. Block-anchored memory validity + RPC shadow mode: **SUFFICIENT**.  
`environment_facts` carries `chain_id` and `block_number` in [design:504](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L504), [design:508](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L508), and [design:509](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L509); §7.4 commits to recorded per-RPC fixtures via `pkg/rpcshadow/` in [design:633](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L633), [design:641](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L641), and [design:642](/home/coder/github/aintern/research/notes/ottie-learning-loop-design.md#L642). That is enough to test a signing path against recorded fixtures before live execution.

**Verdict**

`DO NOT SHIP` — the cleaned doc is close, and the code cleanup is real, but the canonical plan still has two load-bearing P1 regressions: the replay contract is under-specified/contradictory, and the principal-safety section overclaims compile-time protection while specifying runtime checks.

Refine those two sections, then freeze the doc and start P0.