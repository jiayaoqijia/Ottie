# CC Senior Review of Ottie Learning-Loop Design (R2)

> Reviewer persona: 30 years in AI agent research. Symbolic AI and expert systems (CLIPS, Soar) through the 90s, BDI agents through the dot-com era, watched the RL winter and spring, shipped production RL systems, now works on LLM agents. Grounded in: Newell's *Unified Theories of Cognition*, Anderson's ACT-R, Rao & Georgeff on BDI, Brooks on subsumption, Mitchell & DeJong on explanation-based learning, Kolodner and Leake on case-based reasoning, Ross on imitation learning, Smith on contract nets, Erman on Hearsay-II, Vygotsky on the dialectic of development.

## Opening: the biggest intellectual dishonesty

**"Closed learning loop" is a misnomer.** What the design actually delivers is a *prompt cache with curated inputs*. No gradient flows. No parameter is updated. No preference pair is collected. The model does not improve; only the context window does. Calling it a "learning loop" makes it sound like SFT-over-time, but the only phase that contains anything resembling learning (Phase 4: trajectories + Atropos) is explicitly opt-in, offline, and not wired into the running agent.

This matters because the design invokes closed-loop terminology to justify architectural complexity — the nudger, MEMORY.md curation, autonomous skill creation — as if they were components of a learning system. They are not. They are components of an **input-curation** system. That is not a worthless thing; caching the right context for a powerful frozen model is a legitimate engineering target. But the design should stop pretending it is a learning loop. Calling it by its honest name — *Adaptive Context System* — changes the evaluation criteria and exposes decisions that are currently hidden.

Specifically: if the goal is input curation, then the question "is Ottie getting better?" becomes "is the context Ottie sees more useful over time?" That is measurable in ways that do not need SFT or RL. The design never asks this question, and so it does not notice it needs an answer.

## 1. Phase-by-phase mapping to classical architectures, with the failure mode each phase ignores

| Phase | Classical analog | Key paper | Failure mode this design ignores |
|-|-|-|-|
| **P1. FTS5 recall** | Case-based reasoning retrieval step | Kolodner, *Case-Based Reasoning*, 1993 | CBR's "case-base maintenance" problem (Leake & Smyth, 1998–2000): retrieval quality degrades super-linearly as stale cases accumulate. The design has *no* maintenance policy. "Last 90 days" is not a policy; it is a guillotine. |
| **P2. Memory nudger + curation** | Agent-as-curator of declarative memory | Anderson, ACT-R base-level activation, 1993 | Anderson's activation equation (`B_i = ln(Σ t_j^{-d})`) gives each memory a *decayed recency × frequency* score, and retrieval probability is proportional to activation. The design has no activation, no decay, no frequency. It has a 15 k / 30 k character budget and a text file. That is a log file, not memory. |
| **P3. Autonomous skill creation** | Explanation-based generalization + production compilation | Mitchell et al., 1986; DeJong & Mooney, 1986; Laird et al. (Soar chunking), 1986 | Every classical EBG result has the same precondition: **a domain theory strong enough to prove why the example worked**. The design has a trace (a sequence of messages) and no theory. Skills created this way will be brittle — they encode the trajectory without understanding why the trajectory succeeded. Compare to Soar's chunking, which requires a goal hierarchy and an impasse mechanism; the design has neither. |
| **P4. Trajectory capture + Atropos RL** | Imitation learning → off-policy RL | Ross, Gordon & Bagnell, *DAgger*, 2011 | DAgger's core result: naive behavior cloning has `O(T² ε)` error under a T-length episode with per-step error ε. Agent episodes are exactly where T is large and compounding errors are fatal. The design captures ShareGPT JSONL from production runs and hands it to Hermes's compressor. There is no DAgger-style correction loop, no expert query, no on-policy re-rollout. A model SFT'd on this data will, on expectation, be worse than the teacher at long-horizon tasks. |

**R3 requirement:** for each phase, name its classical analog in the design doc, state the failure mode, and state the specific mitigation. Readers should understand that Phase 1 is CBR retrieval, Phase 2 is activation-based declarative memory, Phase 3 is EBG-with-weak-theory, Phase 4 is BC-plus-RL. Calling them "P1", "P2", "P3", "P4" hides the intellectual debt.

## 2. Theory of forgetting — concrete proposal

**Current state:** MEMORY.md and USER.md are bounded by character count. When full, the design says nothing. When stale, it says nothing. When contradicted, it says nothing. This will produce an increasingly incoherent memory file over months of use.

**Classical answer:** ACT-R's base-level activation (Anderson, 1993; Anderson & Lebiere, 1998). Each memory entry has an activation score:

```
B_i(t) = ln( Σ_j (t - t_j)^(-d) )
```

- `t_j` = timestamps at which the entry was used or re-confirmed
- `d` = decay parameter (ACT-R uses 0.5; this is remarkably robust across domains)
- `B_i` is log-probability of retrieval; entries below a threshold are evicted

**Concrete port for Ottie (Go):**

```go
type MemoryEntry struct {
    ID        string    // stable hash of content
    Content   string
    Created   time.Time
    UseTimes  []time.Time // every time the entry was "touched" — read into prompt, re-confirmed by the model, referenced by the user
    Confidence float64  // 0..1, managed by the agent via memory_manage
    Tags      []string  // crypto-domain tags: "jurisdiction", "preference", "wallet", "risk"
}

func (e *MemoryEntry) Activation(now time.Time, d float64) float64 {
    if len(e.UseTimes) == 0 {
        return 0
    }
    var sum float64
    for _, t := range e.UseTimes {
        dt := now.Sub(t).Hours() / 24.0  // days
        if dt <= 0 { dt = 0.01 }
        sum += math.Pow(dt, -d)
    }
    return math.Log(sum)
}
```

- On every session open, touch the entries that were actually retrieved into the prompt → boosts their activation.
- On eviction (when the budget is hit), drop the lowest-activation entries.
- Never drop entries with `Confidence == 1.0` unless the user explicitly does so; those are "pinned" facts.
- On contradiction (detected by the agent at review time), mark both entries with a `contradicted_with` pointer and prompt the user at next opportunity. Do not silently overwrite.

**Contradiction handling** is not solved by ACT-R alone — it is a belief-revision problem (AGM: Alchourrón, Gärdenfors, Makinson, 1985). The minimal thing Ottie can ship: detect contradiction via an LLM pass at memory-write time, store both versions with a `conflicts_with` pointer, and surface a `/memory reconcile` slash command.

**R3 requirement:** replace the "character budget" memory model with an activation-based model. Ship the above formula behind a test that asserts: given two entries, one touched every day for 30 days and one touched once 6 months ago, the former survives eviction and the latter does not.

## 3. Multi-agent coordination — pick a model

**Current state:** the design spawns review subagents (Phase 2.6), refers to Mode A sub-agents, and mentions Mode B multi-bot swarms. **There is no coordination protocol.** What happens if the main agent and the nudger subagent simultaneously produce conflicting memory writes? What happens if two Mode-A workers want to write to the same file? What happens if a Mode-B bot observes another bot's message and needs to react?

**Classical options — pick one:**

| Option | Paper | Fit for Ottie |
|-|-|-|
| **Blackboard architecture** | Erman, Hayes-Roth, Lesser & Reddy, *Hearsay-II*, 1980 | Good for multi-bot swarm (Mode B). Each agent writes hypotheses to a shared blackboard, knowledge sources react. The existing `pkg/swarm/board/` is literally this — but the design never claims the label or commits to its semantics. |
| **Contract Net Protocol** | Smith, *Contract Net*, 1980 | Good for task delegation. Manager announces task, workers bid, manager awards. Clean for "orchestrator + workers" (Mode A) but requires a bid function that LLMs do not naturally produce. |
| **Joint intentions** | Cohen & Levesque, 1991; Tambe, *STEAM*, 1997 | Heavyweight. Agents commit to shared goals and must notify each other on goal satisfaction / abandonment. Too much for Ottie's scale. |
| **Actor model + supervisor tree** | Hewitt et al., 1973; Armstrong, Erlang, 1996 | Good for the nudger + memory writer. Let-it-crash semantics, supervisor restarts, clean shutdown. This is what Go's errgroup approximates but doesn't enforce. |

**Recommendation for R3:** commit to a two-tier coordination model.

- **Tier 1 (intra-process: nudger, memoryWriter, review subagents): actor model with supervisor.** Every background worker is an actor with a mailbox and an explicit shutdown message. There is one supervisor per `AgentLoop` that tracks all children and is joined in `Close()`. Errgroup is the transport; the *semantics* are actor-model.
- **Tier 2 (multi-bot swarm, Mode B): blackboard via `pkg/swarm/board/`.** Commit to Hearsay-II semantics: no agent may read another's working notes; only explicitly-posted hypotheses are visible; hypothesis levels (speech-level, phrase-level, etc.) map to skill categories. Document this in the design.

Without these commitments, the swarm modes are decorative and the nudger is a loose goroutine.

## 4. Imitation learning's T²ε problem (Phase 4)

**Ross et al., *A Reduction of Imitation Learning and Structured Prediction to No-Regret Online Learning*, AISTATS 2011** is the canonical result: if you behavior-clone an expert with per-step error `ε`, a rollout of length `T` compounds to `O(T² ε)` expected total loss. For agent episodes (20+ turns) this is catastrophic.

**The DAgger fix:** iteratively, (a) train a policy on the teacher, (b) roll out the student, (c) query the teacher on the *student's* state distribution, (d) aggregate, (e) retrain. This keeps the training distribution aligned with the rollout distribution, giving `O(T ε)` instead of `O(T² ε)`.

**Minimum viable DAgger for Ottie:**

- At inference time, log not just the action the student (production Ottie) takes but the *candidate actions* from the tool registry at each decision point. This gives us the set of choices without executing them.
- Periodically, send a sample of production trajectories back through a *stronger* teacher model (e.g., the current altllm-basic replaced by a premium tier for the sampling pass). The teacher produces an action at each state. The pair (production state, teacher action) becomes a DAgger training example.
- Aggregate these into the trajectory JSONL under a new `supervision_source` field: `"production"` for normal runs, `"dagger_teacher"` for teacher-corrected runs.
- Hermes's compressor will ignore the `supervision_source` field; our RL shim filters on it.

**R3 requirement:** document the DAgger protocol in the trajectory capture section and ship the `supervision_source` field in the ShareGPT-clean `.jsonl` (as a top-level key next to `completed` — which we verify Hermes's parser tolerates in the round-trip conformance test). Without this, Phase 4 is a foot-gun: training on Ottie's own traces will make Ottie worse.

## 5. Case-base maintenance (Phase 1 recall)

**Leake & Smyth, *Remembering to Forget*, 1998; Smyth & Keane, 1995**, established the case-base maintenance result empirically: unbounded case bases don't just grow linearly in storage — retrieval quality degrades because of *interference* from similar-but-irrelevant cases. The remedy is principled eviction by *coverage* and *reachability*:

- **Coverage** of a case = the set of future problems it would solve alone.
- **Reachability** of a case = the set of past problems it could have solved.
- Cases that have both low coverage and low reachability are safe to drop; cases with high coverage are essential.

**For Ottie:** FTS5 cannot compute coverage or reachability directly, but we can approximate:

- Track, per session message, how often it was *selected* by the `Recall` function (i.e., appeared in the top-k for some future query). Call this `selection_count`.
- Track, per session, the *outcome* of the conversation that cited it. If the conversation ended in `completed=true` with no user retry, boost the message's `success_count`.
- At eviction time, drop messages with `selection_count == 0 AND success_count == 0 AND age > 30 days`. Keep everything else.
- Eviction is **explicit**, not time-based. A 6-month-old message that is still actively retrieved survives; a 14-day-old message nobody ever retrieved does not.

**R3 requirement:** replace the "90-day filter" in §3.6 with a coverage/reachability-inspired eviction policy. Ship the `selection_count` and `success_count` columns on the messages table. Add a `memory gc` subcommand that runs the eviction. Cite Leake & Smyth in the design doc so future maintainers know where the policy came from.

## 6. Decision structure is thrown away in trajectories

**Current state:** trajectories are flat message sequences. You see what the agent did, not what it considered.

**Classical observation:** rational-agent literature from Russell & Norvig onward models agent decisions as a *DAG*: at each state, the agent has a set of actions, each with a utility estimate, and picks (argmax, softmax, epsilon-greedy) one. The choice *rule* and the *alternatives* are first-class. Throwing them away and keeping only the chosen action gives you imitation data at best and misleading imitation data at worst.

**Minimum-viable decision metadata for R3:**

```json
{
  "from": "gpt",
  "value": "<think>...</think>I'll use lido_stake.<tool_call>...</tool_call>",
  "decision": {
    "candidates": [
      {"tool": "lido_stake", "score": 0.82, "reason": "matches intent"},
      {"tool": "lido_wrap",  "score": 0.21, "reason": "wrong step"},
      {"tool": "web_fetch",  "score": 0.14, "reason": "no network info needed"}
    ],
    "rule": "argmax",
    "selected": "lido_stake"
  }
}
```

Where do candidates + scores come from? Two options:

- **Cheap:** ask the model, in its system prompt, to emit a `<candidates>` block before the tool call. It's ~50 tokens per decision and is logged.
- **Expensive (preferred for training data):** sample multiple completions with temperature > 0, record all tool calls observed, treat frequency as score. This is essentially a Monte Carlo rollout of the policy at each decision.

The **cheap** version ships immediately and lets us pool Ottie trajectories with Hermes's (since the field is optional and Hermes's compressor ignores it — verified in the round-trip test).

**R3 requirement:** add a `decision` field to the per-turn schema in §6.2. Make it optional so un-annotated trajectories remain Hermes-compatible. Cite Russell & Norvig's treatment of agent decisions and Ross's DAgger paper in the justification.

## 7. Goal stack gap (BDI)

**Current state:** Ottie has no goal stack. Every turn is reactive. The cron scheduler handles "run this at 9 AM" but not "continue the Lido analysis I was in the middle of."

**Classical answer:** BDI (Rao & Georgeff, *AAAI-91*). An agent has:

- **Beliefs** (what it thinks is true — MEMORY.md + USER.md + retrieved FTS5 snippets)
- **Desires** (long-lived preferences — user stated "I want to maximize stETH yield")
- **Intentions** (committed plans — "execute the weekly rebalance if APR drops > 0.5 %")

A full BDI reasoner is overkill. A **minimal intention stack** is not:

```go
type Intention struct {
    ID         string
    Goal       string          // "rebalance stETH positions when APR < 2.0%"
    Trigger    TriggerSpec     // cron expr, on-message, on-threshold
    Preconditions []string    // "user has stETH balance > 1.0"
    Plan       []PlanStep      // sketched steps; each step is a tool call intent
    Status     string          // "pending" | "active" | "suspended" | "done" | "abandoned"
    Owner      string          // user_id or "system"
    CreatedAt  time.Time
    LastFired  time.Time
}
```

- Store in SQLite (reuse Phase 1's store).
- On every turn-start, the loop consults the intention stack: are any intentions triggered by the current state? If so, inject them into the system prompt (at session start, to preserve cache) or as a tool-result message (mid-session, cache-safe).
- Expose `intention_manage` tool analogous to `memory_manage` for the agent to create / update / abandon intentions.
- Cron becomes a special case of an intention whose trigger is a cron expression.

This is ~300 LoC of Go plus a schema addition and gives Ottie a usable goal layer without building a full BDI reasoner.

**R3 requirement:** add Phase 2.5 "Intention Stack" between Phase 2 and Phase 3. Reuse the SQLite store. Cite Rao & Georgeff in the design doc.

## 8. User model is under-determined

**Current state:** USER.md is an append-only text file. Honcho is mentioned but vaguely. There is no statement of what properties a user model must have.

**Classical answer (drawn from the user-modeling literature, Kobsa & Wahlster, 1989; Rich, 1979 "User Models in Dialog Systems"):** a useful user model has four properties:

1. **Consistency.** Contradictions are flagged, not silently overwritten.
2. **Temporal decay.** Preferences change over time; old preferences are downweighted unless re-confirmed.
3. **Confidence.** Some facts are certain ("user holds a hardware wallet — I saw them use it"), others are inferred ("user seems cautious — based on 5 instances of asking for dry_run first"). The model should know the difference.
4. **Coverage.** The model should know what it does *not* know ("I have never observed the user's risk tolerance for leverage"). Gaps are a first-class datum.

**None of these are in the current design.** USER.md is a monolithic text blob.

**Minimal port:** extend the Phase 1 SQLite schema with a `user_facts` table:

```sql
CREATE TABLE user_facts (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT,
    user_id     TEXT,
    category    TEXT,            -- "preference" | "jurisdiction" | "holding" | "risk" | "identity"
    content     TEXT,
    confidence  REAL,            -- 0..1
    source      TEXT,            -- "stated" | "inferred" | "observed"
    activation  REAL,            -- ACT-R style, updated on touch
    contradicts_id TEXT,         -- FK if this fact contradicts another
    created_at  REAL,
    last_used   REAL
);
```

The LLM-facing interface is the same — the agent writes to USER.md via `memory_manage` — but the write path parses the content into structured facts via a small LLM pass, and retrieval merges structured facts + raw text.

**R3 requirement:** state the four user-model properties in §4.2 of the design. Ship the `user_facts` table as part of Phase 1 SQLite. Cite Rich 1979 and Kobsa & Wahlster 1989.

## 9. No offline evaluation harness

**Current state:** Phase 4 captures trajectories and trains via Atropos. There is no way to know if the trained model is better than the previous one without deploying it. This is the most commercially dangerous gap in the design.

**Classical answer:** build an eval set *before* you train. For agent systems, the current standard is:

- **BFCL v3** (Berkeley Function Calling Leaderboard, 2024) — tool-selection accuracy, parameter correctness, parallel calls.
- **TauBench** (Sierra, 2024) — multi-turn tool-use with user simulation. Measures end-to-end task success.
- **HELM Agent** (Stanford CRFM, 2024) — broad agent benchmarks including multi-hop web tasks.
- **AgentBench** (Liu et al., 2023) — 8 distinct environments from code to web to DB.

**For a crypto agent** none of these are perfect. We should ship a custom `ottie-eval` harness that includes:

1. A frozen 50-task Ottie eval set derived from the existing `demos/test-onchain.sh`. Each task has (a) a prompt, (b) an expected set of tool calls, (c) an expected outcome, (d) a grading rubric (LLM-as-judge or exact match).
2. A `ottie eval run` subcommand that loads a given model into the agent loop and measures: task success rate, tool-selection accuracy, mean tool calls per task, hallucinated tool rate, refusal rate.
3. CI integration: the eval harness runs against every nightly build. If success rate drops > 5 % the build fails.

This is ~1 week of work and is the single highest-ROI addition to the design. Without it, Phase 4 is untestable.

**R3 requirement:** add Phase 0.5 "Evaluation Harness" between P0 and P1. It blocks all subsequent phases and is a precondition for declaring anything "done". Cite BFCL, TauBench, AgentBench.

## 10. Structural patching on Markdown (Phase 3)

Even the R2-revised fuzzy matcher still patches *text*. A 30-year veteran would say: parse the Markdown, patch *nodes*, serialize.

**Concrete:** use `goldmark` (pure-Go CommonMark + extensions, already in the Go ecosystem) to parse the SKILL.md into an AST. `skill_manage patch` operates on node paths, not byte offsets. A patch request looks like:

```json
{
  "action": "patch",
  "target_node": "skill.sections['Usage'].codeBlock[0]",
  "operation": "replace",
  "content": "..."
}
```

Advantages:

- Code fences, list structures, front-matter, and tables are never crossed by accident.
- Unicode is handled by the parser.
- The patch is round-trippable: serialize the modified AST back to Markdown with the same writer and idempotent on unchanged nodes.
- Parity with Hermes's regex-based matcher is no longer a goal; we can do better.

The cost is ~300 LoC of Go (goldmark adapter + node-path resolver) plus a dependency. Since we already vendor goreleaser and other moderate-size deps, adding goldmark is acceptable.

**R3 requirement:** replace the rune-aware line-matcher in §5.4 with a goldmark AST-patch approach. Keep the fallback text-match for emergencies (behind a config flag).

## Closing: the 5 concrete deliverables that must enter R3

1. **Rename the concept.** Stop saying "closed learning loop." Say "Adaptive Context System" or "Memory-and-Skill Curation Loop." Update §0 TL;DR, §2 Architecture, and all user-facing copy. The rest of R3 follows from this honesty.
2. **ACT-R activation memory.** Ship the `MemoryEntry.Activation` Go code in §4.3. Replace character budgets with activation thresholds. Ship contradiction detection + `/memory reconcile`.
3. **Coverage/reachability eviction on FTS5.** Add `selection_count` and `success_count` columns. Ship `ottie memory gc`. Cite Leake & Smyth.
4. **DAgger supervision field + intention stack.** Ship `supervision_source` on trajectories. Add a minimal intention-stack Phase 2.5 with `intention_manage` tool. Cite Ross 2011 and Rao & Georgeff 1991.
5. **Evaluation harness Phase 0.5.** Ship `ottie-eval` with 50 frozen tasks, run on every nightly, fail build on > 5 % regression. Cite BFCL, TauBench, AgentBench.

Without these five, R3 is still decorative. With them, Ottie becomes the first agent framework that is honestly named and that knows whether it is getting better.
