# Codex Senior-Expert Review of Ottie Learning Loop Design (R2)

> Persona: 30-year AI/agent research veteran grounded in Soar, ACT-R, BDI, CBR, EBL, Hearsay-II, DAgger, BFCL, TauBench, HELM.
> Session: `019d7d2b-cba8-7093-898a-cf881af875bc` (session 2, independent from the first codex review)
> Tokens: 154,848 (xhigh reasoning)
> Date: 2026-03-25
> Input: inlined R2 design doc + R1→R2 codex-review.md for context
> Verification: codex ran under `-s workspace-write` this run (previous run was sandbox-blocked)

---

The biggest intellectual dishonesty is the phrase **“closed learning loop.”** In the classical sense, that phrase does not mean “the agent saves traces of experience.” It means experience is evaluated against goals, converted into control knowledge, selectively retained or forgotten, and then shown to improve later behavior under some measurable criterion. Newell’s demand in *Unified Theories of Cognition* was exactly about this causal closure: memory, control, goals, and action must form one architecture, not a pile of side files. Ottie R2 does not yet do that. It stores episodes, lets a model rewrite two markdown files, drafts skills, and exports ShareGPT logs, but it never specifies what counts as improvement, how retained knowledge is judged useful, how contradictory lessons are resolved, how a drafted skill earns promotion, or how later action is causally attributed to prior experience.

That matters because the design repeatedly treats persistence as learning. Classical AI spent decades learning that these are not the same. CBR degraded when cases accumulated without maintenance. EBL hit operationality and utility problems. Soar chunking needed impasses and compilation discipline. Blackboard systems needed explicit control. Imitation learning broke under covariate shift. Ottie R2 recreates all of those failure modes in modern LLM form. There is no competence model, no forgetting model, no explicit intention structure, no counterfactual correction loop, and no evaluation harness that can say Ottie_t+1 is better than Ottie_t. So the loop is not closed. It is **persistent autobiographical state plus self-editing prompts plus future training exhaust**.

So: “closed learning loop” is not yet a fair label. “Cross-session recall, reflective memory writing, skill drafting, and trajectory export” is fair. This is not merely a prompt-cache with extra steps, because the artifacts do persist outside the prompt. But architecturally it is still much closer to a **self-editing prompt/cache regime** than to a learning architecture. To earn the stronger label, R3 must add performance-coupled retention, forgetting, correction, commitment, and evaluation.

1. **Biggest dishonesty.**  
Newell (1990/1992, *Unified Theories of Cognition*) and Minton (1990, *Quantitative Results Concerning the Utility of Explanation-Based Learning*) are the right standards here. A system has not “learned” because it wrote something down; it has learned when acquired knowledge improves future control enough to justify its acquisition and application cost. Ottie has no such utility test. It logs, curates, drafts, and exports, but it never defines a promotion rule from experience to better policy.  
**Required change before P1:** add a “learning contract” section to the design: for each artifact type (`case`, `memory fact`, `skill`, `trajectory example`), define objective, acquisition trigger, promotion criterion, rollback criterion, and evaluation slice. Without that, stop using “closed learning loop.”

2. **Phase mapping to classical architectures and missing failure modes.**  
**P1** is indeed CBR, but only the `retrieve` step. Kolodner (1992, *An Introduction to Case-Based Reasoning*) framed CBR as retrieve, reuse, revise, retain. Ottie R2 has text retrieval with no adaptation model, no revision stage, and no competence model. Classical failure mode: superficial similarity wins, structurally relevant but lexically different cases are missed, and retrieved cases mislead when reused without adaptation.  
**Required change:** cases must be indexed by typed task structure (`tool`, `chain`, `asset`, `intent`, `outcome`, `failure mode`), not only full text.

**P2**, in my reading, is not “memory” in the classical sense; it is a crude **metalevel control element**, closest to Hayes-Roth’s blackboard control architecture (1985, *A Blackboard Architecture for Control*) and the control problem in Hearsay-II (Erman et al., 1980). Classical failure mode: opportunistic control thrash. A fixed every-N-turns nudger has no expected-value model, no agenda arbitration, and no conflict resolution among candidate memory operations.  
**Required change:** trigger retention by utility signals such as novelty, repeated reuse, contradiction, failure recovery, or explicit user correction, not by turn count alone.

**P3** is EBL/chunking. Mitchell, Keller, and Kedar-Cabelli (1986, *Explanation-Based Generalization: A Unifying View*), DeJong and Mooney (1986, *Explanation-Based Learning: An Alternative View*), Keller (1988, *Defining Operationality for Explanation-Based Learning*), and Minton (1990) all point to the same failure mode: **operationality and utility**. Learned rules become too specific, too general, or too expensive to match. Ottie’s skill drafting mechanism still lacks an operationality test.  
**Required change:** every learned skill needs explicit preconditions, applicability signature, expected benefit, and a success test.

**P4** is behavior cloning plus a proto-RL wrapper. Ross, Gordon, and Bagnell (2011, DAgger) give the imitation failure mode: covariate shift and compounding error. Sutton-style RL adds the second failure mode: no clear state, action, reward, or termination semantics means you do not yet have a serious learning environment.  
**Required change:** do not call Atropos integration “RL” until the design defines the MDP/POMDP interface for crypto tasks.

3. **Theory of forgetting gap.**  
The right theory is a hybrid: ACT-R for retrieval probability, case-base maintenance for pruning, and Soar chunking for abstraction. Anderson and Schooler (1991, *Reflections of the Environment in Memory*) and Anderson et al. (2004, ACT-R) give the right skeleton: memory usefulness depends on recency, frequency, and current context. Leake and Wilson (1998, *Categorizing Case-Base Maintenance*) and Leake and Wilson (2000, *Remembering Why to Remember*) add that retention must be tied to competence and performance, not raw age. Laird, Rosenbloom, and Newell (1986, *Chunking in Soar*) add that repeated successful subgoal resolutions should compile into more abstract units.

Concretely, Ottie should stop treating `MEMORY.md` and `USER.md` as the canonical store. Put canonical memory in SQLite with tables like `memory_items`, `memory_evidence`, and `memory_uses`. Each item gets: typed key, target (`user` or `env`), confidence, created_at, last_used_at, use_count, confirmation_count, contradiction_count, and supporting episodes. Retrieval score should be something like:  
`score = relevance(query,item) + α*ln(sum age^-0.5) + β*ln(1+successful_uses) - γ*contradictions - δ*size`.  
Then render the top-scoring items into the 15k/30k prompt budgets as a **materialized view**.

Maintenance job: merge duplicates by key, mark contradictions explicitly, promote repeated aligned episodes into abstract chunks, archive low-score items with zero marginal contribution, and keep rare high-risk items even if old.  
**Required change:** R3 must specify structured memory, contradiction handling, decay, and chunk promotion; markdown becomes output, not truth.

4. **Multi-agent coordination.**  
Right now there is no protocol, only spawning. Hearsay-II gave you a blackboard for shared partial results (Erman et al., 1980). Smith (1980, *The Contract Net Protocol*) gave you explicit task announcement, bidding, award, and completion. Cohen and Levesque (1991, *Teamwork*) and Tambe (1997, *Towards Flexible Teamwork*) added commitment semantics. Ottie should not pick pure blackboard and should not pretend a “swarm” is a protocol. It should pick **contract-net-lite backed by a shared board**, with a small amount of joint-commitment semantics.

That means every delegated task needs a schema: `task_id`, `parent_goal`, `artifact`, `acceptance_test`, `tool requirements`, `lease`, `budget`, `dependencies`, `owner`, `status`, `decommitment_reason`. Workers bid with capability, estimated cost, and confidence. The orchestrator awards work. Agents must explicitly notify on failure, blockage, or invalidated assumptions.  
Pitfalls are classical: auction overhead, blackboard flooding, stale bids, duplicated work, and brittle decommitment.  
**Required change:** write the protocol into R3. “Spawn subagent” is not a coordination architecture.

5. **Imitation learning compounding errors.**  
No, the current Phase 4 pipeline includes no DAgger-style correction, no counterfactual logging, and no principled expert intervention loop. Ross, Gordon, and Bagnell (2011) showed exactly why that is dangerous: expert-distribution behavior cloning can yield `O(T²ε)` degradation when the learned policy visits states the expert data never covered.

The minimal fix is **DAgger-lite**. Whenever Ottie enters a state it induced itself and one of these happens: user correction, tool validator rejection, safety block, backtrack, or failed plan, capture that state and ask an oracle for the better next action. The oracle can be a human, a stronger model plus rule validators, or a simulator-backed expert policy. Store both `chosen_action` and `corrected_action`, plus the correction source. Also log `top_k_candidates` so rejected alternatives exist as counterfactuals.  
**Required change:** trajectory capture must support correction labels from learner-induced states, not only successful production transcripts.

6. **Case-base maintenance.**  
Leake and Wilson (1998, 2000) and Smyth and McKenna’s competence work make the answer clear: the right eviction policy is **marginal competence under the actual query distribution**. Not LRU. Not age. Not sheer frequency. Keep the cases that uniquely cover important regions of problem space or preserve retrieval quality; evict cases that are redundant, confusing, or dead weight.

For Ottie, maintain per-case statistics: `retrieved_count`, `successful_reuse_count`, `unique_coverage`, `confusion_cost`, `query_clusters_hit`, and `risk_class`. Approximate `unique_coverage` by replaying recent recall queries and measuring whether removal of the case changes top-k successful retrieval. Approximate `confusion_cost` by counting cases frequently retrieved but never used or contradicted by later action. Archive cases with low marginal coverage and high confusion cost. Keep rare critical cases such as safety incidents even if seldom retrieved.  
**Required change:** add a case-maintenance job and define the utility formula in R3.

7. **Decision-structure loss in trajectories.**  
You are right: a flat message trace destroys planning signal. Newell’s problem-space view and Minton’s search-control work both assume that learning depends on **choice points**, not only outputs. Hermes’s ShareGPT format, as documented, keeps `<think>`, `<tool_call>`, and `<tool_response>` inside a flat conversational stream. That is fine for SFT on formatting and coarse behavior. It is not enough for learning search control or planning.

Minimal sidecar metadata should include: `decision_id`, `parent_decision_id`, `goal_id`, `state_summary_hash`, `retrieved_memory_ids`, `candidate_actions[]`, `candidate_scores_or_ranks`, `chosen_action`, `expected_outcome`, `actual_outcome`, `validator_verdict`, and `corrected_action` if intervention occurred. That gives you a decision DAG without polluting ShareGPT compatibility.  
Compared with Hermes: Hermes gives you the executed path only. Ottie should stay ShareGPT-compatible for the text stream but add a decision-graph sidecar if it wants to learn planning rather than mere response style.  
**Required change:** trajectory schema must include a decision sidecar before any “learning from trajectories” claim.

8. **Goal stack gap.**  
R2 is architecturally closer to a Brooks-style reactive tool agent than to a BDI system. Brooks (1986) is not wrong for low-latency reactive competence, but Rao and Georgeff (1995, *BDI Agents: From Theory to Practice*) and Cohen and Levesque (1990, *Intention Is Choice With Commitment*) explain what Ottie is missing: persistent intentions with commitment conditions.

These should live in **session state**, not in `MEMORY.md`. Add a session-local `goals` table: `goal_id`, `parent_goal_id`, `status`, `success_condition`, `failure_condition`, `next_action`, `blocked_on`, `adopted_at`, `updated_at`, `drop_reason`. Cheapest shippable version: after each turn, extract a tiny JSON intention update from the model or from a controller. On the next turn, inject only the top active intentions as a normal context/tool message, not as mutable system prompt. Commitment rule: a goal stays until achieved, impossible, irrelevant, or explicitly superseded.  
**Required change:** define an intention stack and lifecycle before adding more autonomous “learning.”

9. **User model under-determination.**  
Honcho “dialectic observation” is just rhetoric unless the design commits to a representation. Rich (1979, *User Modeling via Stereotypes*) and Kobsa (1989/1990, *User Models in Dialog Systems*; *User Modeling in Dialog Systems: Potentials and Hazards*) imply four properties a useful user model must have: it must be **individualized**, **uncertain**, **revisable over time**, and **behaviorally useful** for choosing explanations, plans, and dialog moves.

Ottie addresses only the weakest form of the first property: a bag of user facts. It does not represent uncertainty, does not resolve contradictions, does not model the user’s knowledge level or current plans, and does not define how the model changes action selection. A crypto agent needs at least: wallet sophistication, consent policy, risk tolerance, jurisdictional constraints, preferred chains/tools, and confidence/provenance for each claim.  
**Required change:** replace free-text `USER.md` as source-of-truth with a typed user model plus confidence, provenance, expiry, and contradiction resolution.

10. **Evaluation harness gap.**  
I would not accept any “learning” implementation without an offline evaluation stack. HELM (Bommasani et al., 2023) is useful as a methodology: multi-metric, scenario-based, transparent reporting. AgentBench (Liu et al., 2023) is useful as a reminder that interactive environments matter, but its stock tasks are mostly not crypto-specific. τ-bench (Yao et al., 2024) is directly relevant in spirit: simulated user, tool APIs, policy constraints, end-state evaluation, and `pass^k`. BFCL (Patil et al., 2025) is directly relevant for tool-use exactness, abstention, and stateful multi-step function calling.

For a crypto agent, the usable subset is: BFCL-style function-call evaluation on Ottie’s tool schemas; τ-bench-style interactive task evaluation with a simulated wallet/exchange/Lido world and policy rulebook; HELM-style reporting for success, calibration, robustness, safety, latency, and cost. AgentBench is mostly methodology, not dataset.  
**Required change:** define `Tau-Crypto`, `BFCL-Ottie`, a cross-session recall benchmark, and a safety/consent regression suite. No model or skill promotion without benchmark wins over a frozen baseline.

**R3 Deliverables**
- A “learning contract” section that replaces the current marketing claim with explicit objectives, promotion rules, rollback rules, and evaluation gates for memories, skills, and trajectories.
- A structured memory spec: canonical SQLite schema, ACT-R-style activation scoring, contradiction handling, chunk promotion, and markdown-as-view rendering.
- A coordination and intention spec: contract-net-lite protocol, shared board schema, leases/decommitment, and a session-local intention stack.
- A trajectory-v2 spec: ShareGPT text stream plus decision-graph sidecar, correction labels, and DAgger-lite intervention logging.
- An evaluation spec: BFCL-style tool tests, τ-bench-style crypto simulator, cross-session memory benchmark, safety/consent regressions, and HELM-style reporting.

**Sources**
- Allen Newell, *Précis of Unified Theories of Cognition* (1992): https://www.cambridge.org/core/journals/behavioral-and-brain-sciences/article/precis-of-unified-theories-of-cognition/0903DF1E0E9B600A16C6757561A928E6
- John R. Anderson & Lael J. Schooler, *Reflections of the Environment in Memory* (1991): https://journals.sagepub.com/doi/10.1111/j.1467-9280.1991.tb00174.x
- John R. Anderson et al., *An Integrated Theory of the Mind* (2004): https://www.cs.utexas.edu/~dana/ACT-R.pdf
- Janet Kolodner, *An Introduction to Case-Based Reasoning* (1992): https://www.researchgate.net/publication/226704111_An_introduction_to_case-based_reasoning
- David Leake & David Wilson, *Categorizing Case-Base Maintenance* (1998): https://homes.luddy.indiana.edu/leake/papers/a-98-03.html
- David Leake & David Wilson, *Remembering Why to Remember* (2000): https://homes.luddy.indiana.edu/leake/papers/a-00-03.html
- Lee Erman et al., *The Hearsay-II Speech Understanding System* (1980): https://mas.cs.umass.edu/paper/229
- Barbara Hayes-Roth, *A Blackboard Architecture for Control* (1985): https://www.sciencedirect.com/science/article/abs/pii/0004370285900633
- Reid G. Smith, *The Contract Net Protocol* (1980): https://reidgsmith.com/The_Contract_Net_Protocol_Dec-1980.pdf
- Anand Rao & Michael Georgeff, *BDI Agents: From Theory to Practice* (1995): https://osj.aaai.org/Library/ICMAS/1995/icmas95-042.php
- Philip Cohen & Hector Levesque, *Teamwork* (1991): https://www.sri.com/publication/teamwork/
- Gerald DeJong & Raymond Mooney, *Explanation-Based Learning: An Alternative View* (1986): https://link.springer.com/article/10.1007/BF00114116
- Richard Keller, *Defining Operationality for Explanation-Based Learning* (1988): https://www.sciencedirect.com/science/article/pii/0004370288900136
- Steven Minton, *Quantitative Results Concerning the Utility of Explanation-Based Learning* (1990): https://www.sciencedirect.com/science/article/pii/0004370290900599
- Stéphane Ross, Geoffrey Gordon, Drew Bagnell, *A Reduction of Imitation Learning and Structured Prediction to No-Regret Online Learning* (2011): https://proceedings.mlr.press/v15/ross11a.html
- Percy Liang et al., *Holistic Evaluation of Language Models (HELM)* (2023): https://huggingface.co/papers/2211.09110
- Xiao Liu et al., *AgentBench* (2023): https://arxiv.gg/abs/2308.03688
- Shunyu Yao et al., *τ-bench* (2024): https://huggingface.co/papers/2406.12045
- Shishir Patil et al., *BFCL* (2025): https://proceedings.mlr.press/v267/patil25a.html
- Hermes trajectory format docs: https://hermes-agent.nousresearch.com/docs/developer-guide/trajectory-format/

tokens used: 154848
