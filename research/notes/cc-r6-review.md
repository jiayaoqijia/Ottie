# CC Senior Review of Ottie ACS Design (R5) — Fifth Round

> Reviewer persona: same 30-year AI/agent veteran as R3/R4/R5. R6 framing shift: not "find gaps", but "learn everything useful from hermes, then beat it". The user's words verbatim: "Yeah, learn all from hermes and beat it." This round is opinionated about *where Ottie should leapfrog hermes* and *where Ottie should ruthlessly copy*.

## Opening

After reading hermes-agent against Ottie for the fifth round, two things are clear. **The single biggest thing to steal from hermes is its error-recovery taxonomy** — `research/hermes-agent/agent/error_classifier.py` centralizes provider failures into a `FailoverReason` enum (`auth`, `billing`, `rate_limit`, `overloaded`, `context_overflow`, `thinking_signature`, `long_context_tier`, etc.) that drives a single retry policy across every provider. Ottie today has scattered string-matching in retry code and loses half the information an experienced operator would act on. **The single biggest leapfrog lever** is deterministic turn replay. Go + prompt epoch (§14) + execution manifest (§15 C4) + write-ahead action ledger (§15 C1) gives Ottie the ability to say "replay turn 1234 bit-for-bit from its prompt hash and manifest" — an ability hermes cannot reach without a major rewrite because Python's non-determinism (dict ordering was fixed, but asyncio scheduling, GC pauses, thread interleaving, and import-order side effects are not) makes it structurally impossible. This is the hackathon demo that nothing else can copy.

The meta-framing: hermes paid for speed-to-market with a 10,251-line `run_agent.py` (`research/hermes-agent/run_agent.py`) and scattered module conventions. Ottie can afford to be cleaner because it came later. That "came later" advantage is only valuable if Ottie takes the hermes lessons that are actually load-bearing and leaves the hermes accidents where they lie.

---

## Subsystem comparison matrix

| # | Subsystem | Hermes today | Ottie today | Verdict | Action |
|---|-|-|-|-|-|
| 1 | Memory recall store | SQLite + FTS5, fuzzy match, auto-recall on session open, `agent/memory_manager.py` + `agent/memory_provider.py` split | JSONL file, no recall, R5 design targets SQLite + FTS5 | **CATCH-UP** | Ship P1 as designed. Hermes has a 2-year head start on the interface shape; steal its provider split. |
| 2 | User model / facts | Markdown `USER.md` + `MEMORY.md` + Honcho (for active users) | Passive markdown file read into prompt | **STEAL** | R5 §15.4 #6 already targets valid-time columns — go further and steal hermes's dual-file split (`USER.md` = what we know about the user; `MEMORY.md` = what we know about the environment). |
| 3 | Autonomous skill creation | Background review thread, agent-written, human-reviewed | Manual via `self-evolve` skill | **CATCH-UP** (P3) | R5 design ships P3. Steal hermes's review cadence (nightly), invert the default from auto-ship to human-review (crypto requires stricter). |
| 4 | Trajectory capture + batch runner | `agent/trajectory.py` (56 lines — thin writer) + `batch_runner.py` (1287 lines — orchestrator) + ShareGPT JSONL + Atropos env wrapper | None | **CATCH-UP** (P4) | Hermes kept trajectory.py tiny on purpose; the intelligence is in batch_runner. Do the same in Go: thin writer, fat orchestrator. Two-file split (`*.jsonl` + `*.metadata.json`) from R2 is the right call. |
| 5 | Session lineage + resume | `hermes_state.py` (1238 lines) — central state module | `pkg/session/`, `pkg/agent/instance.go` with JSONL fallback | **MATCH** | Ottie's Go types give stronger invariants than hermes's Python state module. Press this — see leapfrog delta #6. |
| 6 | Tool call reliability + schema validation | `agent/retry_utils.py`, `_should_parallelize_tool_batch` at `run_agent.py:267` refuses to parallelize on path overlap, non-parseable JSON, or non-dict args (fail-safe concurrency) | Parallelizes blindly at `pkg/agent/loop.go:1448` (codex R5 finding); silent JSON coercion at `pkg/providers/toolcall_utils.go:62` | **STEAL + LEAPFROG** | Steal hermes's parallel-safety predicate (non-parseable args → sequential). Leapfrog via R5 delta #5 JSON-schema validator + compile-time type guards Python cannot have. |
| 7 | Multi-tenant trust boundary | Per-user group isolation via `research/hermes-agent/run_agent.py:1179` + docs at `research/hermes-agent/website/docs/user-guide/sessions.md:337` | Workspace-global `USER.md`/`MEMORY.md` injection at `pkg/agent/context.go:177`, `:490`; per-room not per-user Telegram (`pkg/channels/telegram/telegram.go:568`) | **CATCH-UP** | R5 delta #1 already targets this. Hermes confirmed the shape — per-user, not per-room, not per-workspace. Ship this first. |
| 8 | Crypto / on-chain integration | Zero — hermes is general research | 8 crypto skills + Lido MCP + ERC-8004 + stETH treasury + Sepolia wallet | **LEAPFROG** | Ottie's biggest structural advantage by miles. Press via leapfrog delta #1. |
| 9 | Identity / principal binding | Session-level user_id, not propagated into tool context | `channel`/`chatID` only in tool context (`pkg/tools/base.go:28`); no principal at all | **MATCH (both weak)** | R5 C2 `PrincipalContext` is the leapfrog — hermes doesn't have it either. See leapfrog delta #2. |
| 10 | Prompt caching + epoch discipline | `agent/prompt_caching.py` 72 lines — simple cache-control emission; cache breaks on context edit | R5 §14 prompt epoch + `/reload`; rebuild on mtime drift removed | **LEAPFROG** | Ottie R5 already surpasses hermes here; the epoch discipline + demoted summaries + imperative-strip is a more disciplined design. Press it. |
| 11 | Eval harness + promotion criteria | Scattered tests + `mini_swe_runner.py` | R5 §15.4 #8 Goodhart defenses targeted; R3 P0.5 eval harness | **CATCH-UP** | Hermes has had 8+ months more operational feedback on what a real eval loop looks like. Steal their eval scaffolding wholesale, add R5 Goodhart defenses on top. |
| 12 | Deployment footprint | Python 3.11+, virtualenv, 100s of MB of deps, non-trivial cold start | Single 24 MB Go binary, instant start, no deps | **LEAPFROG** | Ottie's #2 structural advantage. Press via leapfrog delta #3 (air-gapped single-binary deploy). |
| 13 | Replay / incident forensics | None structural — logs only | R5 §15.4 #2 Dapper spine + #3 write-ahead ledger + C4 execution manifest | **LEAPFROG** | Ottie R5 design already exceeds hermes. The leverage is *shipping* it. See leapfrog delta #4. |
| 14 | Testing philosophy / load-bearing | `tests/` across 20+ subdirs, heavy on integration, real LLM calls gated by env var | `pkg/*/` test packages, golden-file heavy, no LLM integration tests in CI | **STEAL** | Hermes treats LLM-gated integration tests as a first-class signal. Ottie should too — behind a local-only nightly job. |
| 15 | Error classification + retry | `agent/error_classifier.py` centralized `FailoverReason` enum → drives `retry_utils.py` | Scattered string-match retry logic in `pkg/providers/*` | **STEAL** | Highest-value hermes steal. See learn-from-hermes #1. |

---

## Learn-from-hermes list (steal these)

Ordered by ROI.

1. **Error classifier with a `FailoverReason` enum.** `research/hermes-agent/agent/error_classifier.py` has a single `FailoverReason` enum (`auth`, `auth_permanent`, `billing`, `rate_limit`, `overloaded`, `server_error`, `timeout`, `context_overflow`, `payload_too_large`, `model_not_found`, `format_error`, `thinking_signature`, `long_context_tier`, `unknown`) and a priority-ordered classification pipeline. The retry loop consults one function; the retry logic is one place. Ottie port: add `pkg/providers/errclass/` with a typed `FailoverReason` Go iota + classifier func; replace every scattered `strings.Contains(err.Error(), "rate limit")` call. Compile-time exhaustiveness check via type switch = Python cannot match this.

2. **Path-scoped parallel-tool safety.** `research/hermes-agent/run_agent.py:267` `_should_parallelize_tool_batch` refuses to parallelize on (a) any tool in `_NEVER_PARALLEL_TOOLS`, (b) non-parseable args JSON, (c) non-dict args, (d) overlapping scoped paths via `_paths_overlap`, (e) any tool not in `_PARALLEL_SAFE_TOOLS`. Ottie today parallelizes blindly at `pkg/agent/loop.go:1448`. Port to Go: add a `ParallelSafety` field on tool registration (`parallel_safe`, `never_parallel`, `path_scoped`). Registry rejects parallel batches that violate the policy. Much cleaner than hermes because Go's type system catches this at compile time.

3. **Memory split into two markdown files, not one.** `USER.md` = agent's model of the user. `MEMORY.md` = agent's model of the environment. Hermes operates this way in `agent/memory_manager.py`. Ottie's current single `MEMORY.md` conflates them. Splitting makes invalidation much easier — environment facts decay faster than user facts (the R5 §15.4 #6 valid-time work gets cleaner if the two files have different default policies).

4. **Centralized `memory_provider.py` abstraction.** `research/hermes-agent/agent/memory_provider.py` is the interface; `memory_manager.py` is one implementation. Two files, clean separation. Ottie's `pkg/memory/store.go` interface is already good — match hermes's provider/manager split in the SQLite backend.

5. **`subdirectory_hints.py`.** Hermes injects per-subdirectory context hints into the prompt when the agent is working in a given directory (`research/hermes-agent/agent/subdirectory_hints.py`). This is a lightweight way to surface "you are currently in `pkg/agent/`, the memory system lives here" type hints without stuffing everything into the global system prompt. Ottie port: add a per-directory `.ottie/hint.md` file that gets surfaced when tools reference files in that subtree. Costs one tool call's worth of complexity, buys a huge context-efficiency win.

6. **Manual compression feedback.** `research/hermes-agent/agent/manual_compression_feedback.py` lets the user say "compress this part of the history, it's no longer relevant" and the agent actually does it rather than blindly sliding-window. Ottie port: a new `compact --scope <range>` or `/forget` command that reads a selection and rewrites history. This is surprisingly small code for surprisingly large UX value.

7. **Insights command.** `research/hermes-agent/agent/insights.py` generates historical usage reports — token consumption, cost estimates, tool usage patterns, session metrics. It's inspired by Claude Code's `/insights`. Ottie port: `ottie insights` subcommand. Trivial to build on top of R5's `traces` + `action_intents` tables; massive operator value.

8. **Error-driven context compression.** `research/hermes-agent/agent/context_compressor.py` is 766 lines. It's invoked when the classifier returns `context_overflow` and compresses the oldest half of the history before retrying. Ottie port: compression is a recovery action in the retry ladder, not a separate lifecycle event. This integrates naturally with R5's fail-operational matrix (§15.4 #10).

9. **Explicit `retry_utils.py` with single-responsibility.** Hermes keeps retry decisions in one place and invokes them from many call sites. Ottie has retry logic scattered. Port: one `pkg/providers/retry/` package that is the only place retry happens.

10. **SQLite-backed session state (`hermes_state.py`, 1238 lines).** Hermes chose a single SQLite file for *all* session state — not just conversation history but also rate limits, preferences, cached model metadata, credentials state, insights cache, usage pricing cache. Ottie's R5 design already targets SQLite for memory; extend that single-DB discipline to everything else. Go has `modernc.org/sqlite` (already used in P0 bench) — one DB, one lock hierarchy, one backup story.

11. **ACP adapter as a first-class interface.** `research/hermes-agent/acp_adapter/` (with `auth.py`, `permissions.py`, `events.py`, `session.py`, `server.py`, `tools.py`) is the Agent Context Protocol adapter. Hermes treats ACP as a deployment target, not an afterthought. Ottie port: a top-level `pkg/acp/` that lets Ottie run as an ACP server for agentic-pipeline compatibility. This opens the door to Ottie being called *by* other agents, not just being the agent.

12. **Title generator for sessions.** `research/hermes-agent/agent/title_generator.py` auto-generates a human-readable title for each session using a tiny LLM call. Ottie could use this for the web UI session list. Small win, clear value.

13. **Skill-category hierarchy.** Hermes's `skills/` is organized by domain (`apple`, `creative`, `data-science`, `devops`, `diagramming`, `productivity/google-workspace`, `productivity/linear`, `productivity/notion`, `research/arxiv`, `research/polymarket`, `software-development/plan`, etc.). Ottie's `workspace/skills/` is a flat list of 35 items. Reorganize Ottie into `crypto/`, `defi/`, `identity/`, `meta/`, `security/`, `utility/` and stop cramming unrelated skills together.

14. **`copilot_acp_client.py`.** Hermes has a Copilot ACP client built-in (`research/hermes-agent/agent/copilot_acp_client.py`) — it can talk *to* GitHub Copilot's ACP server. Interesting because it shows ACP is bidirectional. For Ottie, the immediate steal is the ACP client protocol; the leapfrog is an on-chain agent-to-agent handshake (see leapfrog delta #5).

15. **Smart model routing.** `research/hermes-agent/agent/smart_model_routing.py` routes different query types to different models dynamically. Worth stealing for Ottie's multi-provider config — the same prompt might go to Venice (private cognition) vs altllm (public action) depending on whether secrets are involved.

---

## Ten leapfrog deltas — where Ottie beats hermes and hermes cannot catch up

**Top 3 are bet-the-hackathon-demo items.**

### #1. Crypto-native decision graph — Ottie signs its own reasoning
Every signing action emits, at the moment the signed tx goes on-chain, a companion EIP-712 attestation over the decision graph: `{prompt_epoch, execution_manifest, decision_id, retrieved_memory_hashes, intention_id, confidence, risk_class}`. The attestation goes into the `action_commits` ledger (R5 §15.4 #3) and is also posted on-chain as a low-cost blob via EIP-4844 or published to 8004scan. Result: anyone auditing an Ottie-signed tx can verify, cryptographically, which reasoning trace produced it. **This is impossible for hermes** because (a) hermes is not crypto-native, (b) even if it were, Python's nondeterminism makes the "replay-the-reasoning-to-re-verify" half impossible. Ottie's Go runtime + prompt epoch + execution manifest makes it structurally feasible. Hackathon demo: "here is a signing tx, here is the on-chain proof-of-reasoning, here is the bit-for-bit replay of the turn that produced it".

### #2. `PrincipalContext` with capability-bearing tool dispatch
R5 C2 `PrincipalContext` becomes a Go interface threaded through every tool invocation. Tools declare required capabilities at registration time; the registry compile-time-checks that only a `PrincipalContext` with the matching capability can dispatch. Example: `lido_stake` requires `caps.TxSigning[mainnet]`; a tool call from a context that only holds `caps.ReadOnly` refuses dispatch at the type level. Hermes cannot match this without adding a type system — Python's duck typing punts this to runtime. Ottie's `pkg/tools/registry.go` already has the registration hook; this is 2 days of work for a permanent architectural advantage.

### #3. Air-gapped deployment story
Single static Go binary + SQLite state + optional MCP servers. Ottie can run in an air-gapped environment that can't fetch Python dependencies, can't run a virtualenv, can't spawn subprocess workers. Hermes requires Python runtime, 150+ MB of pip deps, and subprocess workers for several features (trajectory compressor, batch runner). For crypto users running cold-wallet-adjacent agents, air-gap deploy is the winning feature. Hackathon demo: `scp ottie user@airgap:~/; ssh user@airgap 'ottie agent -m "analyze this tx"'`.

### #4. Deterministic turn replay
Combine R5 §14 prompt epoch + R5 §15.4 #2 trace spine + execution manifest + R5 §15.4 #3 write-ahead ledger. Result: `ottie replay <trace_id>` reconstructs turn inputs bit-for-bit (prompt hash match → fetch the exact prompt; tool schema hashes match → reconstruct the tool surface; manifest has provider request IDs → replay with the same provider call IDs). Hermes cannot do this because Python's `dict` order was fixed in 3.7 but asyncio task scheduling, GC, and imports are not. Ottie's Go determinism (no GC pauses visible to user code, goroutine scheduling is at least annotatable, provider calls are purely stateless if we persist request IDs) makes this feasible. Plus: deterministic replay is the foundation for offline RL on real traces, which opens the R1 "real learning loop" door that R3 shut.

### #5. On-chain agent-to-agent handshake via ERC-8004
Ottie runs as an ACP server (from steal #11). Two Ottie instances talking to each other present their ERC-8004 identity and reputation on-chain *before* accepting a task. This is the crypto-native version of ACP: instead of trusting the caller based on header auth, trust them based on their on-chain identity + reputation score. Hermes cannot match this — hermes's ACP adapter has `auth.py` based on traditional tokens, not on-chain identity. The hackathon track alignment: ERC-8004 registration (Track 5) + Self Agent ID (Track 8) + Agent Cook (Track 4) all line up.

### #6. SQLite JSONB + prepared statements — type-safe crypto state
Go's `modernc.org/sqlite` supports JSONB columns, and Go's `database/sql` + `sqlc` lets Ottie generate type-safe query bindings at compile time. Ottie stores wallet addresses, token balances, and ABI fragments as typed JSONB fields, queried through generated Go functions. Hermes uses raw SQL strings because Python doesn't have a good sqlc-equivalent. Compile-time crypto-state query safety beats runtime query safety every time.

### #7. Build-time skill manifest checking
Ottie's `go:embed` lets every default skill be a compile-time embedded asset. A custom `go generate` pass validates every skill's frontmatter (name regex, description length, schema validity) at build time — an invalid skill refuses to compile. Hermes has to do this at runtime because Python doesn't embed. Leverage: CI never ships an invalid skill. Ottie's `workspace/skills/self-evolve/SKILL.md` checking can become fully static.

### #8. Per-channel ClawWall policy as a Go type parameter
Ottie already has ClawWall DLP (`pkg/channels/`). Make the channel type *parameterize* the ClawWall policy at compile time: `Channel[PublicPolicy]` cannot accidentally send a message with `PrivatePolicy` data. Go generics (introduced in 1.18) are exactly the right tool. Hermes's Python ClawWall equivalent would be a runtime check; Ottie's is a compile error. Wallet-draining DLP bug in production = literally impossible if the types line up.

### #9. Cost-of-operation accounting on every tool call
Ottie's tool registry captures `estimated_gas`, `estimated_api_cost_usd`, `estimated_latency_ms` at call time and aggregates per session. A `/budget` command shows "this session has spent $0.42 in API calls + 0.0012 ETH in gas". Hermes has `usage_pricing.py` for API costs but has no concept of gas costs because it's not crypto-native. This is a small win on its own, but combined with R5 §15.4 #4 metareasoning budget (which already targets API cost) it becomes a unified cost-aware reasoning layer — the agent can say "I would answer, but that would cost $0.08 in API calls + 0.001 ETH in gas. Is that OK?"

### #10. Zero-alloc tool dispatch hot path
Ottie's Go tool registry can be made zero-alloc for the hot path (no map lookups, no interface boxing, no runtime type assertions) using `sync.Pool` + generated dispatch tables from `go generate`. Hermes's Python dispatch allocates a dict lookup + function call + arg unpacking on every tool call. For a long-running swarm/nightly-monitor use case, the allocation delta adds up. Not the highest-leverage delta, but free compute once the machinery is in place.

---

## Cuts — trim Ottie's default skill set to match hermes's leanness

Ottie has 35 skills in `workspace/skills/`. Hermes has 27 domain *categories* with multiple skills per category, but not 35 flat defaults. Ottie's flat list is too long, too undifferentiated, and includes pure cruft.

### High-confidence cuts (remove immediately)

| Skill | Reason | Breakage risk | Confidence |
|-|-|-|-|
| `tmux` | Ottie is an agent, not a sysadmin tool. Terminal multiplexing is not an agent capability. | None — no other skill depends on this. | **high** |
| `tempo` | Music-tempo? Time-tracking? Unclear purpose; looks like a half-finished experiment. | None — not referenced anywhere. | **high** |
| `mpp` | Microsoft Project file format? Obsolete for an agent use case; no DeFi user has ever asked for this. | None — not referenced. | **high** |
| `weather` | Generic utility, duplicates what a single `web_fetch` call can do. | None — trivially replaceable. | **high** |
| `summarize` | Duplicates what the LLM already does natively. Skills should add capability the LLM lacks. | Possibly breaks demo scripts; verify `grep -rn summarize workspace/` first. | **medium** |
| `browser` | Duplicates the MCP Playwright/Chrome DevTools servers. Ottie should use those, not a hand-rolled browser skill. | None — MCP covers this. | **high** |

### Medium-confidence cuts (discuss before removing)

| Skill | Reason | Breakage risk | Confidence |
|-|-|-|-|
| `code-security-audit` | Useful but not crypto-adjacent — a general-purpose code audit skill. Can live as a community skill, not a default. | Possible demo references. | **medium** |
| `dep-audit` | Same story — general-purpose dependency audit. Keep only if it can be rescoped to crypto-specific (e.g., scan ABI imports for malicious patches). | Possible CI integration. | **medium** |
| `farcaster` | Social-network skill. Useful for the social-agent use case, but not in the hackathon critical path. | None — opt-in. | **medium** |
| `github` | General code-collaboration. Not crypto-native. But many crypto users use GitHub for smart contract repos — keep as a crypto-adjacent skill rather than a general default. | Possibly referenced in agent workflows. | **low** |
| `tempo` (if kept) | same as above | — | — |

### Load-bearing — DO NOT cut

Keep these. They define Ottie's crypto-native identity:
- `8004`, `8004scan`, `8004scan-webhooks` (ERC-8004 identity — hackathon Track 5)
- `agency-roles`, `self-agent-id` (identity tracks — Track 8)
- `autoresearch`, `crypto-research`, `crypto-market-data`, `crypto-cex`, `crypto-wallet` (core crypto research + trading)
- `defi-lending`, `defi-staking`, `defi-swap`, `defi-yield`, `lido-mcp`, `lido-vault-monitor` (DeFi operations — Track 1, 2, 7)
- `polymarket`, `smart-money-signals` (prediction + on-chain intel)
- `steth-treasury` (Track 6)
- `venice-private-ai` (Track 3)
- `clawwall`, `privacy-layer`, `prompt-injection-guard` (security — critical for a crypto agent)
- `self-evolve`, `skill-creator`, `skill-finder` (meta — the autonomous skill loop needs these)

### Target shape

From 35 skills to ~22, organized into categories matching Hermes's pattern:

```
workspace/skills/
├── crypto/
│   ├── market-data/     (was crypto-market-data)
│   ├── research/        (was crypto-research)
│   ├── wallet/          (was crypto-wallet)
│   ├── cex/             (was crypto-cex)
│   └── smart-money/     (was smart-money-signals)
├── defi/
│   ├── swap/
│   ├── lending/
│   ├── staking/
│   ├── yield/
│   ├── lido-mcp/
│   ├── lido-vault-monitor/
│   └── steth-treasury/
├── identity/
│   ├── 8004/
│   ├── 8004scan/
│   ├── 8004scan-webhooks/
│   ├── agency-roles/
│   └── self-agent-id/
├── security/
│   ├── clawwall/
│   ├── privacy-layer/
│   ├── prompt-injection-guard/
│   └── venice-private-ai/
├── research/
│   ├── autoresearch/
│   └── polymarket/
└── meta/
    ├── self-evolve/
    ├── skill-creator/
    └── skill-finder/
```

This makes the default skill set **visibly crypto-native** (no weather, no tmux), organized by domain (a user asking "what can Ottie do?" sees five clean categories, not a flat 35-item list), and 36% smaller.

### Tool cuts — `pkg/tools/`

Survey the tool registry carefully. Candidates for removal: `render_html` (covered by MCP Chrome DevTools if needed), possibly `send_file` depending on usage. But most Ottie tools (`edit`, `filesystem`, `shell`, `web`, `message`, `spawn`, `cron`, `mcp_tool`, `skills_search`, `skills_install`, `subagent`, `swarm_*`, `project_board_tool`, `search_tool`, `redact`) are load-bearing.

**Recommendation:** run `grep -rn "render_html\|send_file" pkg/ web/ cmd/` before removing either, and ship them as a second, smaller commit after the skill cuts.

---

## Anti-patterns — do NOT copy these from hermes

1. **10,251-line `run_agent.py`.** Hermes's main loop lives in one giant file. Ottie's `pkg/agent/loop.go` is already smaller; do not merge more logic into it just to match hermes's "everything in one place" convention. Go's package boundaries are a feature, use them.

2. **Runtime credential pools (`agent/credential_pool.py`).** Hermes rotates API credentials at runtime by maintaining a pool and swapping on `FailoverReason.billing`. Reasonable for a general-purpose research agent, but for a crypto agent this is a red flag — credentials on a crypto agent should be single-user, single-scope, and explicit, not pooled. Do not port this; if Ottie ever needs credential rotation, it should be explicit + audit-logged, not a silent pool.

3. **Smart model routing as a default (`agent/smart_model_routing.py`).** Hermes routes different query types to different models dynamically. For crypto, this introduces ambiguity about *which model produced a signing decision*. Do port it (it's in the steal list as a controlled feature) but gate it behind explicit config — do not let it be default-on.

4. **General research skills as defaults.** Hermes has `apple`, `creative`, `gaming`, `gifs`, `leisure`, `smart-home`, `social-media`, `email`, `feeds`, `media`, `note-taking` as default skill categories. Crypto Ottie should not ship any of these. A user asking "what can Ottie do?" should not see `gifs` in the answer.

5. **ACP adapter with token auth as default.** Steal the ACP shape; do not steal the auth model. On-chain identity (leapfrog delta #5) is the right auth for a crypto agent.

6. **Subprocess trajectory compressor.** Hermes uses a Python subprocess to compress trajectories. Ottie's P4 should do this in-process in Go — `github.com/klauspost/compress/zstd` is all you need, no subprocess. Subprocess-shelling is a cost hermes pays for language reasons Ottie doesn't have.

7. **Python's `hermes_state.py` 1238-line state module.** Hermes conflates 10+ concerns in this one file because Python discourages tiny files. Ottie should split state management along the lines of its existing package boundaries: `pkg/session/`, `pkg/memory/`, `pkg/rate_limit/`, etc. Do not let one file become the dumping ground.

8. **Insights engine that hits the LLM for every summary.** `agent/insights.py` hints at LLM-powered session summaries. For an auditable crypto agent, insights should be deterministic SQL aggregates over the R5 `traces` table, not LLM hallucinations over log text. Steal the command, not the implementation.

---

## Closing: ship vs refine

**This round should be the last deep review round before P1 implementation starts.** 

Six rounds in, the review is converging. R6's additions (error classifier steal, parallel-tool safety, multi-level decision attestations, PrincipalContext with capabilities, deterministic replay, on-chain agent handshake) are all feasible and make Ottie measurably better than hermes on axes crypto users care about. But each subsequent round is adding marginal deltas, not core architecture. The 17-week effort from R5 + R6 leapfrog work (~2 more weeks for steal list + ~2 more weeks for leapfrog deltas + 0.5 week for skill cuts + cleanup) totals ~21 weeks. That is already longer than the time most crypto-agent projects had when they reached user adoption.

Recommendation:

1. **Freeze the design doc after merging R6.** Add a note at the top: "Review complete. No further R7 unless reality disagrees."
2. **Ship the skill cuts immediately.** 6 high-confidence cuts can land today, non-destructively (keep the files, just remove them from the default set). The category reorganization is a separate follow-up.
3. **Start P1 on Monday.** SQLite + FTS5 + P0 bench first, per R3 phase plan. Every week of continued design review is a week the hackathon demos do not get built.
4. **Treat the leapfrog deltas as *features to ship*, not *things to review*.** The on-chain decision attestation (leapfrog #1), PrincipalContext with capabilities (leapfrog #2), and air-gapped deploy (leapfrog #3) should be in the P1-P2 milestone, not deferred.
5. **Treat the steal list as a 2-week hermes-steal sprint before P1.** Error classifier, parallel-tool safety, memory split, memory_provider abstraction, subdirectory hints, insights command — none are blocked on anything. All are achievable in 2 weeks by one engineer.

**Six-round summary:** R2 fixed R1's structural mistakes. R3 renamed the concept honestly (ACS, not closed learning loop) and added the Learning Contract. R4 found that the design contradicted the live code. R5 added observability, calibration, forensics, and multi-tenant enforcement. R6 names the architectural leapfrogs that make Ottie not just competitive but structurally superior on crypto-native axes. Any R7 would be diminishing returns. **Stop reviewing, start shipping.**
