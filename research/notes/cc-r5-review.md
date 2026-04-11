# CC Senior Review of Ottie ACS Design (R4) — Fourth Round

> Reviewer persona: same 30-year AI/agent veteran as R3/R4. This round consciously pushes into ten new orthogonal areas that the prior three rounds (R2, R3, R4) did not touch: observability and incident forensics, mixed-initiative HCI, confidence calibration, abstention theory, temporal fact staleness, multi-tenant trust boundaries, explainability, Goodhart corrosion of the eval harness, fail-safe modes, and tool-call reliability. If R4 is shipped as written, the system still has these ten open wounds and a crypto-user load will find every one.

## Opening: the single most dangerous under-treated issue

**R4 has no theory of when Ottie should stop acting.** Every prior round added more things Ottie can do: recall memories, draft skills, persist intentions, run shadow modes, ship trajectories, compute metareasoning budgets. Nobody has written down the dual of that: when should Ottie **refuse** or **defer**? Every production crypto agent I have seen collapses at this point. The ones that shipped in 2023-2025 either (a) refused everything interesting (useless) or (b) acted under uncertainty and drained wallets (worse). The middle path — Chow-style optimal rejection, El-Yaniv & Wiener 2010 selective prediction, Horvitz 1999 mixed-initiative — is the design work that R4 has not done.

Calling this "abstention theory" makes it sound academic. Concretely for Ottie: right now, if the user says "stake my ETH on Lido," R4's intention stack triggers, the consent gate fires, the user confirms, and the agent acts. But the user has only partial information about the current APR, the gas environment, the recent Lido governance, the validator set. The agent knows some of those and is uncertain about others. The agent's decision should be a function of both **expected reward** and **the value of more information** vs **the cost of delay**. Instead, R4 treats it as a binary: gated or not, consent or no. That is 1970s MYCIN-era design.

R5's work is to add the dimension R4 is missing: **uncertainty-aware action selection**.

The rest of this review covers the ten orthogonal areas. None of them are as important as fixing the uncertainty story, but all of them are real.

---

## A. Observability and incident forensics

**Scenario.** A user's Sepolia wallet gets drained tomorrow. The team has the trajectory sidecar, the session store, and the memory files. Can they reconstruct which decision led to the signing? Today, no. The decision-graph sidecar (R3 §13.7 #11) is per-trajectory — it records the 12 fields about the choice but it has no **causal pointers back** to the retrieved memory items, the active user facts, the active intentions, or the skill that was in context when the model made the call.

**Classical reference.** Sigelman, Barroso, Burrows, Stephenson, Plakal, Beaver, Jaspan & Shanbhag, *Dapper, a Large-Scale Distributed Systems Tracing Infrastructure*, Google Tech Report 2010. The core insight: every significant operation gets a trace ID and a span, spans have parent-span pointers, and the entire distributed operation reconstructs from the span forest after the fact. Ottie is not distributed in the microservice sense, but a single Ottie agent running a turn is a small distributed system: the LLM call, the tool invocations, the memory queries, the skill loads, and the decision recording are all parallel-ish and need to be stitched together causally.

Lamport 1978 (*Time, Clocks, and the Ordering of Events in a Distributed System*) is the other half: for a correct reconstruction you need logical clocks, not wall clocks, because concurrent events in the agent loop can't be ordered by timestamp alone.

**R5 delta #1: Dapper-style trace IDs + causal span forest.**

- Every turn opens a `trace_id` (stable hash of session_id + turn number + wallclock). Every operation within the turn — recall query, memory read, skill load, tool call, decision sidecar entry, summary render — gets a `span_id` and a `parent_span_id`.
- A new SQLite table `traces (trace_id, span_id, parent_span_id, op_type, op_metadata_json, started_at, ended_at)`. Compacted nightly (keep 90 days).
- A new CLI: `ottie trace <session_id> <turn>` reconstructs and renders the span forest as an indented tree. For the incident scenario, you run `ottie trace <session> <bad_turn>` and immediately see: "at turn 14, lido_stake was called → decision sidecar ref dec_782 → which cited memory_items [mem_3, mem_17] and user_fact jurisdiction=US and active intention int_44 → retrieved via query 'stake ETH'."
- Trace IDs propagate into the trajectory sidecar. If a trajectory gets sampled into training data later, the original causal context is recoverable.

**Priority.** Ship before P1. Not because the infrastructure is expensive — it is cheap — but because retrofitting causal instrumentation after the code is written is notoriously painful. Dapper succeeded at Google precisely because it was wired in early. For Ottie: you want to be able to answer "what happened?" before the first incident, not after.

---

## B. Mixed-initiative dialog and when to ask

**Scenario.** User types: "Can you keep an eye on Lido?" R4's intention stack would create an intention, schedule a recurring check, and act silently. But when the APR drops by 0.4% — borderline event — should Ottie surface an alert? 0.6% — probably. 1.2% — definitely. Right now R4 has consent-gated action or silent action, with no theory for the continuum.

**Classical reference.** Horvitz 1999 (*Principles of Mixed-Initiative User Interfaces*, CHI 1999). The canonical result: the cost of interrupting the user (attention tax, annoyance) and the cost of not interrupting (missed action, user frustration when they find out later) can be modeled as a decision-theoretic tradeoff. Horvitz's formula is roughly:

```
act_autonomously if: expected_value(action) > expected_cost(interrupt) + expected_value(deferred_action)
otherwise interrupt
```

This is a lightweight Bayesian decision layer, and Horvitz showed it beats both "always interrupt" and "always act" on empirical user-study metrics.

**R5 delta #2: Mixed-initiative decision layer.**

- Every action (from intention trigger, from user message, from nudger suggestion) runs through a small classifier before execution. Inputs: `expected_value_of_action`, `expected_cost_of_interrupt`, `uncertainty_about_action`, `user_attention_state`.
- `expected_value_of_action` is scored heuristically: for crypto ops, value = estimated P&L impact; for chat responses, value = latency savings.
- `expected_cost_of_interrupt` is estimated from the user's recent interrupt tolerance (if they ignored the last 3 prompts, the tolerance is low).
- `uncertainty_about_action` comes from the calibration layer (§C below) and the `state_signature` compatibility (R4 delta #3).
- The output is a discrete action level: `silent`, `log_only`, `surface_as_notification`, `prompt_for_consent`, `block_and_wait`.
- A background-learned threshold refines these bands over time based on user accept/reject patterns. This is the "learning" that is missing from the ACS — it actually is learning, just at the dialog-management layer, not the model weights.

**Priority.** Ship before P3 (when autonomous intentions start firing at scale).

---

## C. Confidence calibration

**Scenario.** Ottie's R3/R4 user model has `confidence ∈ [0, 1]` on every user_fact. The `memory_items` table has an `activation` score which doubles as a pseudo-confidence. The drafted skills have an `expected_benefit` field. Every one of these numbers is **uncalibrated** — there is no systematic check that the number means what it claims.

If Ottie says "I am 90% sure this is a genuine Lido contract," the user expects it to be right 9 times out of 10. In reality, LLMs are systematically **overconfident** (Guo, Pleiss, Sun, Weinberger 2017, *On Calibration of Modern Neural Networks*). Without calibration, the 90% number is a vibe, not a probability.

**Classical references.**

- **Brier 1950**, *Verification of Forecasts Expressed in Terms of Probability*. The Brier score is the canonical proper scoring rule: `BS = (1/N) Σ (f_i - o_i)²` where `f_i` is the forecast probability and `o_i` is the observed outcome (0 or 1). Lower is better; 0.25 is the baseline of always guessing 0.5; < 0.1 is well-calibrated; > 0.3 is worse than random.
- **Platt 1999**, *Probabilistic Outputs for Support Vector Machines* — sigmoid rescaling to recover calibrated probabilities from uncalibrated scores.
- **Guo et al. 2017** — temperature scaling for modern deep nets; a single scalar parameter that dramatically improves calibration.

**R5 delta #3: Calibration harness + per-domain temperature scaling.**

- Add a `calibration` table: `(domain, forecast_bucket, n_forecasts, n_correct, mean_forecast, mean_outcome)`. Populated by post-action verification: every time Ottie assigns a confidence to a claim and that claim is later verified (user confirmation, tool validator success, on-chain result), the outcome goes into the appropriate bucket.
- A nightly job computes Brier score per domain. Track it over time. If `BS > 0.3` on any domain, flag the system as un-calibrated for that domain and apply a temperature-scaling correction before emitting confidence.
- Publish calibration to the user: if Ottie is poorly calibrated on "Lido contract identity," the confidence display should include a (calibration: weak) tag.
- The metric becomes a first-class signal in the eval harness (R3 §13.7 #14). Goodhart risk: Ottie may learn to be *less* confident overall to score better. Mitigation: also track **sharpness** (how close forecasts are to 0/1) as a companion metric; a system that always says 0.5 has perfect calibration and useless sharpness.

**Priority.** Ship before P1's recall goes user-facing. Calibration takes weeks of data to build up; starting early is free.

---

## D. Abstention theory

**Scenario.** User asks Ottie to analyze whether a specific new DeFi protocol is safe. Ottie has no cached information about this protocol, no memory items, no trained knowledge beyond the model's pretraining cutoff. What should happen?

Options:
1. Agent answers confidently from whatever the LLM dredges up. (Dangerous.)
2. Agent refuses: "I don't have reliable information about this protocol." (Safe but unhelpful.)
3. Agent says: "I don't have enough to give you an answer with high confidence. I can (a) research it via web_fetch and get back to you in ~30s, (b) give you my uncertain best guess tagged as speculative, or (c) decline." (This is what R4 is missing.)

**Classical references.**

- **Chow 1970**, *On Optimum Recognition Error and Reject Tradeoff* — the original optimal Bayesian rejection rule: reject when `max_y p(y|x) < threshold`, where threshold is chosen to minimize `error_cost + reject_cost`.
- **El-Yaniv & Wiener 2010**, *On the Foundations of Noise-Free Selective Classification*. Gives the modern PAC framework for "selective prediction" — a classifier that can say "don't know" and whose coverage (fraction of inputs answered) is treated as a hyperparameter.
- **Geifman & El-Yaniv 2017**, *Selective Classification for Deep Neural Networks* — operationalized for neural nets with a learned rejection head.

**R5 delta #4: Explicit abstention mechanism with crypto-domain thresholds.**

- Add a `decision_confidence` field to the decision-graph sidecar (complementing R3's `candidate_scores`).
- Define **refusal thresholds per risk class**:
  - `risk_class=read_only`: min confidence 0.4 (err on the side of answering)
  - `risk_class=advisory`: min confidence 0.6
  - `risk_class=writes_state`: min confidence 0.8
  - `risk_class=writes_wallet`: min confidence 0.95, and additionally require `state_signature` match
- When confidence is below threshold, the agent returns a structured refusal: `{"status":"insufficient_confidence", "threshold_required": 0.8, "confidence_achieved": 0.62, "reasons": [...], "alternatives": ["research first", "ask user", "dry-run only"]}`.
- The mixed-initiative layer (§B) consumes the refusal and routes to the appropriate user action.
- Track refusal rate as a first-class metric alongside success rate. A crypto agent with a 20% refusal rate and 90% action success is better than one with 0% refusals and 70% action success.

**Priority.** Ship before P2. The `memory_manage add target=user_fact` path can already benefit from abstention — low-confidence facts should not become user model rows.

---

## E. Temporal reasoning and fact staleness

**Scenario.** Ottie learns "the Lido stETH APR is 2.42%" on March 25. A user session on June 25 retrieves this memory via FTS5 recall. The retrieved fact is three months old; Lido APR has since dropped to 2.1%. The agent uses the stale fact to quote the APR and advises the user based on outdated information.

R4 has timestamps on everything but **no semantics for staleness**. Time is a datum, not a first-class reasoning concept.

**Classical references.**

- **Allen 1983**, *Maintaining Knowledge about Temporal Intervals*. Introduced the 13 interval relations (`before`, `meets`, `overlaps`, `during`, etc.) and made temporal reasoning tractable. A fact has a validity interval, not a timestamp.
- **Snodgrass 1995**, *Developing Time-Oriented Database Applications in SQL*. Temporal databases distinguish **valid time** (when the fact is true in the world) from **transaction time** (when it was recorded in the database). Both are needed.
- **Reiter 1991**, *The Frame Problem in Situation Calculus* — what does and doesn't change when an action occurs.

**R5 delta #5: Valid-time + transaction-time on every fact.**

- Extend the `memory_items` and `user_facts` tables with two new columns: `valid_from` (when the fact became true in the world, often the message timestamp) and `valid_until` (when it stops being true — often unknown, nullable). Distinct from the existing `created_at` (transaction time: when we wrote it).
- Define **fact decay policies per category**:
  - `preference`: valid_until = never (unless superseded)
  - `jurisdiction`: valid_until = never (unless superseded)
  - `holding`: valid_until = 24h from valid_from (wallet balances change)
  - `market_quote`: valid_until = 15m from valid_from
  - `identity`: valid_until = never
  - `governance_state`: valid_until = 7d
- Retrieval filters expired facts by default (`WHERE valid_until IS NULL OR valid_until > NOW()`).
- Expired facts are not deleted, they are moved to a `historical_facts` view for incident forensics (§A above — the trace might need to know what the agent thought it knew at the time).
- Surface staleness in responses: "According to my last check on March 25, the Lido APR was 2.42% — that information is 91 days old and may be stale. Should I refresh?"

**Priority.** Ship before P1. Time semantics retrofitted onto a memory store are always harder than designing them in.

---

## F. Multi-tenant trust boundaries

**Scenario.** Ottie runs as a Telegram bot serving 100 users. Alice's memory includes "I hold 0.5 ETH on a hardware wallet" and "My Lido position is 0.3 stETH." Bob asks Ottie "What's a typical Lido position?" and the retrieval layer finds Alice's memory and cites it. Alice's wallet state leaks to Bob.

R4's memory architecture is **single-agent**, but Ottie's actual deployment mode (Telegram/Slack/Discord gateway) is multi-user. The `pkg/agent/` code handles per-user routing at the loop level, but the `memory_items` and `user_facts` tables — as R4 designs them — have an `agent_id` column and a `user_id` column but no **enforced boundary**. A misconfigured recall query can cross the boundary.

**Classical references.**

- **Bell & LaPadula 1973**, *Secure Computer Systems: Mathematical Foundations*. The canonical multi-level security model: subjects and objects have classification labels; information flows down but not up. The Bell-LaPadula "simple security property" (no read up) is what we need here: agent-session subjects cannot read user_facts labeled for other users.
- **Biba 1977**, *Integrity Considerations for Secure Computer Systems*. The integrity dual: no write up. An agent session acting on Alice's behalf cannot write into Bob's memory.
- **Myers & Liskov 1997**, *A Decentralized Model for Information Flow Control*. Modernizes both with ownership labels that can be added and removed dynamically.

**R5 delta #6: Enforced multi-tenant boundary in the memory and session layers.**

- Every row in `memory_items`, `user_facts`, `goals`, `cases`, and `traces` has a **principal label**: `(agent_id, user_id, visibility)` where `visibility ∈ {private, shared_with_user_<id>, public}`. The default is `private` to the user who wrote it.
- Every query against these tables must specify a principal. `Recall(query, k, principal=...)` rejects the call if the principal is missing. This is enforced via a type-system wrapper so the compiler catches un-principaled queries.
- Session-level enforcement: `Session.Principal` is set at session open from the incoming channel (Telegram user id, Slack user id, etc.) and cannot be changed mid-session. Every query inherits from the session.
- Audit log: every query is recorded in a `memory_access_log` table with principal + SQL. Periodic audit scans for any row read where `row.principal != query.principal`.
- Emergency rollback: a `/principal-reset` operator command scrubs cached contexts when a trust boundary violation is detected.
- The **default deployment mode** remains single-user (local CLI, no gateway), but the multi-tenant primitives are present from day one so the gateway mode is safe by construction.

**Priority.** Ship before P1. Multi-tenancy bugs are impossible to retrofit — the code has to be boundary-aware from the first line. Once someone deploys Ottie on a Telegram gateway with R4's design, this is an incident waiting to happen.

---

## G. Explainability and causal attribution

**Scenario.** Ottie signs a transaction. The user asks "Why did you do that?" Ottie's current answer is either (a) a post-hoc rationalization the model generates on the spot, or (b) a dump of the decision-graph sidecar. Neither is an explanation — the first is confabulation, the second is noise.

A real explanation walks the causal chain: "You asked me to rebalance stETH when APR dropped. The APR was 1.8% this morning (retrieved from lido_mcp at 08:42). Your active intention `int_44` says 'rebalance when APR drops > 0.5 %'. Your user_fact `user_facts[jurisdiction]=Portugal` means I preferred the Coinbase routing skill over the direct Lido path. I ran `lido_stake(dry_run=true)` first, it simulated cleanly, so I proceeded."

**Classical reference.** Miller 2019, *Explanation in Artificial Intelligence: Insights from the Social Sciences*, Artificial Intelligence vol 267. Miller's thesis is important: people don't want causes, they want **contrastive** explanations — "why this action and not that one?" The explanation must name the alternatives and say why they were rejected. The decision-graph sidecar already records candidate alternatives, but the render layer throws them away.

**R5 delta #7: Contrastive explanation renderer.**

- Every action gets an `ottie explain <decision_id>` command and a corresponding tool call for the user ("why?"). Output:

```
Action: lido_stake(amount_eth=0.1, dry_run=false) at 08:43:17 turn 14
Trace: tr_abc_14_span_22

Because:
  1. Your message ("rebalance stETH on APR drop") matched intention int_44.
  2. int_44 was created on 2026-03-01 and says "rebalance when APR drops > 0.5% from baseline 2.3%".
  3. At 08:42 I retrieved lido_apr via lido_mcp → 1.78% (source span_17).
  4. 2.30 - 1.78 = 0.52% > 0.5% threshold → intention fired.
  5. Selected skill: defi-staking (confidence 0.87, rank 1/3).
  6. Alternatives considered:
     - crypto-wallet (confidence 0.41, rejected: doesn't handle staking)
     - defi-swap (confidence 0.33, rejected: this is a stake, not a swap)
  7. Your user_fact jurisdiction=Portugal influenced tool path: preferred Coinbase routing over direct Lido (state_signature match).
  8. Pre-execution dry_run: simulated 0.0983 stETH shares, no revert.
  9. Confidence: 0.89 (above 0.8 threshold for writes_wallet, per abstention policy).
  10. Mixed-initiative: did not interrupt you because your intention already authorized this.

Would not have acted if:
  - intention int_44 had been suspended (R4 delta #10)
  - confidence had been below 0.8
  - dry_run had reverted
  - state_signature had been "mainnet + no_consent"
  - eval harness had flagged a regression in defi-staking in the last 7 days
```

- The render is generated directly from the `traces` + `decision_graph` + `memory_items` + `user_facts` tables, not by the LLM. This is crucial: explanations must be **faithful** to the actual causes, which means they cannot be model-generated post-hoc.
- A smaller `ottie explain --short` version for UI display.
- Every explanation is archived so the **same explanation** can be produced later during incident forensics (§A).

**Priority.** Ship before P3. Once autonomous skill drafting and autonomous intentions exist, users will ask "why" more often, and the answer must be grounded in ground truth, not the LLM's memory of what it did.

---

## H. Goodhart corrosion of the eval harness

**Scenario.** R4 ships a nightly eval harness with `BFCL-Ottie`, `Tau-Crypto`, cross-session recall, safety regression. Six months later the score graph looks perfect — every metric trending up. Meanwhile, a user on the Telegram gateway complains that Ottie got worse at a task the harness doesn't cover. The team investigates: Ottie *did* get worse at that task, but the harness has drifted to reward behaviors that look good on paper.

**Classical references.**

- **Strathern 1997**, *"Improving Ratings": Audit in the British University System*. Short paper; canonical modern reference: "When a measure becomes a target, it ceases to be a good measure."
- **Manheim & Garrabrant 2018**, *Categorizing Variants of Goodhart's Law*. Classifies four mechanisms by which optimization corrupts measurement: regressional, extremal, causal, adversarial.
- **Campbell 1976**, *Assessing the Impact of Planned Social Change* — the original "Campbell's law" formulation.

**R5 delta #8: Eval-drift defenses.**

- **Hold-out rotation.** Ottie never sees 20% of the eval tasks. Each week, a different 20% is held out and tested **only by a separate internal team** (or in our case, manually by the maintainer). Drift is measured as the delta between the public harness score and the held-out score.
- **Adversarial eval curation.** Every month, new tasks are added to the harness based on actual production failures, not on what the system is already good at. The "adversarial task commissioner" role is explicit.
- **Metric diversity.** Track not just success rate but: refusal rate, calibration (Brier score from §C), latency, cost, user interrupt frequency, memory-fact freshness, tool-call format error rate (§J). A system that improves on success but degrades on three others is not actually improving.
- **Campbell alarms.** Any metric that improves monotonically for > 30 days triggers an automatic adversarial review: "what is the system doing to score so well? Is it the thing we care about?"
- **Public metric framing.** The eval dashboard names the Goodhart risk in plain sight: "This metric is tracked. Measurements of tracked metrics drift. Treat trends skeptically." This is a social intervention, not a technical one, but it changes how the team reads the numbers.

**Priority.** Ship before P3 (when eval signal becomes load-bearing for skill promotion). Before then, the harness isn't yet central enough to be worth gaming.

---

## I. Fail-safe vs fail-operational

**Scenario.** The SQLite backend goes down mid-session. The user is asking Ottie to sign a transaction. What happens?

- **Fail-safe:** Ottie refuses all write operations. The transaction is not signed. The user has to wait or retry.
- **Fail-operational:** Ottie proceeds with in-memory-only state (no recall, no memory writes) but continues to respond and sign. Some data loss on session close.
- **Fail-degraded:** Ottie reads from SQLite but queues writes to an append-only log, replaying when SQLite recovers. Eventual consistency.

R4 has no degradation ladder. Every component is implicitly assumed to be either up or down.

**Classical reference.** Avižienis, Laprie, Randell, Landwehr 2004, *Basic Concepts and Taxonomy of Dependable and Secure Computing* (IEEE Trans. Dependable and Secure Computing 1(1), 11-33). The canonical definition of dependability has four attributes (reliability, availability, safety, maintainability) and three threats (faults, errors, failures). The key insight: failure modes must be **designed**, not discovered in production. For each subsystem, the designer writes down what "degraded" means and what invariant is preserved.

**R5 delta #9: Explicit degradation ladder per subsystem.**

For every ACS component, a table of failure modes:

| Subsystem | Normal | Degraded | Failed | Safety invariant |
|-|-|-|-|-|
| SQLite store | full | read-only, writes queued to append log | refuse memory_manage; continue with in-memory session state | never sign with stale or missing state_signature |
| Recall | full FTS5 | keyword-only fallback | no recall block in prompt | never hallucinate missing recall as "no memory" |
| Skills hub | live install | cached skills only | no new skill loads | never activate an unverified skill |
| Memory writer | normal | write-through with checksum | reject writes, return error | never lose a consent-relevant write silently |
| Nudger | running | suspended | disabled | never block main turn path |
| Eval harness | nightly | daily smoke only | disabled | never auto-promote a skill when harness is dark |
| Trajectory capture | append | buffered in memory | disabled | never block main turn path |

- Explicit `/status` command shows the current degradation state per subsystem.
- Every sign-bearing action checks the current degradation state and refuses if any upstream guarantee is unmet.
- Degradation is observable: users see "ACS: degraded (SQLite read-only)" in the status bar.

**Priority.** Ship before P2. Once the ACS is in production, the question of what to do when a component fails becomes urgent. Answering it in advance is cheaper than answering it during a wallet-drain incident.

---

## J. Tool-call reliability / the Moravec paradox for LLMs

**Scenario.** The #1 failure mode in production LLM agents is not reasoning errors — it is **tool call formatting errors**. The model emits `{"tool": "lido_stake", "args": {"amount": "0.1 ETH"}}` instead of `{"name": "lido_stake", "arguments": {"amount_eth": "0.1"}}`. The agent loop catches the format error, retries, the retry has a different error, three retries later the agent gives up and hallucinates a response.

R4 has zero telemetry for this. We have no idea what Ottie's tool-call success rate is on any given day.

**Classical reference.** Moravec 1988, *Mind Children: The Future of Robot and Human Intelligence*. Moravec's paradox: tasks that are hard for humans (complex reasoning) are easy for machines; tasks that are easy for humans (sensorimotor coordination, reliable output formatting) are hard for machines. Transferred to LLMs: high-level reasoning can be surprisingly good while low-level output adherence is surprisingly bad.

**R5 delta #10: Tool-call telemetry layer.**

- Every tool call attempt is logged with `{tool_name, schema_valid, params_valid, retry_count, final_status}`. `final_status ∈ {success, schema_error, param_error, handler_error, timeout, retry_exhausted}`.
- A nightly report surfaces the top 10 failing tool names, failure reasons, and retry counts.
- **Format validator before execution**: every tool call goes through a JSON Schema validator against the tool's declared schema **before** reaching the handler. On schema failure, the agent gets a **structured error** telling it exactly which field is wrong, and retries once. If the retry also fails, the tool is marked `unavailable_this_turn` and the agent must pick a different approach.
- **Canary suite**: 20 fixed tool-call patterns run nightly against the current model. If any canary fails (a tool call that was working yesterday is now failing today), the eval harness flags it.
- **Retry-exhaustion escape hatch**: if any tool is retry-exhausted more than once in a session, the session enters degraded mode and all subsequent tool calls require user confirmation.

**Priority.** Ship before P1 becomes user-facing. Tool-call format errors are the invisible tax on every LLM agent; they will start biting on day one.

---

## Closing: the 10 R5 deltas ranked by crypto-user 90-day risk

1. **Multi-tenant boundary enforcement (§F).** Impossible to retrofit; the day Ottie is deployed on a Telegram gateway, a single wrong query can leak Alice's wallet state to Bob. Ship before any gateway deploy.
2. **Abstention theory + refusal thresholds (§D).** Crypto agents that don't know when to refuse are the ones that drain wallets. Second-highest priority.
3. **Temporal fact staleness + valid_until (§E).** Three-month-old "Lido APR 2.4%" retrievals are an accuracy failure waiting to happen.
4. **Confidence calibration (§C).** Overconfidence in crypto is deadly. Start collecting calibration data from day 1 of P1.
5. **Observability + trace IDs (§A).** The incident forensics gap. You only notice this exists when you need it — and then it is too late to retrofit.
6. **Tool-call telemetry (§J).** The Moravec gap. #1 production failure mode, zero visibility in R4.
7. **Fail-safe degradation ladder (§I).** Required before any production load.
8. **Mixed-initiative dialog layer (§B).** Required before autonomous intentions fire at real users.
9. **Explainability / contrastive renderer (§G).** Required before first user asks "why did you sign that?"
10. **Goodhart eval-drift defenses (§H).** Lowest priority because the eval harness isn't central yet — but will be critical in 6 months.

Every one of these is orthogonal to R2/R3/R4. R5 is not about rethinking what R4 said; it is about finding what R4 didn't look at.

**Single-sentence R5 summary:** R4 gave Ottie the right architecture; R5 gives Ottie the right **uncertainty story** (abstention + calibration + mixed-initiative), the right **forensics story** (traces + explanations + telemetry), and the right **deployment story** (multi-tenancy + fail-safes + Goodhart defenses) — all of which R4 assumed away.
