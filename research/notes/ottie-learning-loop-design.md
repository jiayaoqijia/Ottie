# Ottie Adaptive Context System — Clean Design (post-R6)

> Status: frozen after R6 / cleaned 2026-04-11. This doc folds six rounds of
> review (R1 original, R2 codex consult, R3 senior review, R4 third-round
> review, R5 fourth-round review, R6 learn-and-beat-hermes) into a single
> actionable plan. The full accumulated amendment history is preserved at
> `research/notes/ottie-design-history-R1-R6.md` for anyone who needs the raw
> review prose. Every decision below is already an R1-through-R6 decision —
> this is the settled version.
>
> Goal: **make Ottie structurally superior to NousResearch/hermes-agent on
> the axes that matter for a crypto agent** — deterministic replay, signed
> audit trail, per-principal safety, single-binary deploy — while stealing
> hermes's recall, memory-curation, and skill-ergonomics patterns wholesale.

## 0. TL;DR

Ottie and hermes-agent share OpenClaw ancestry. Hermes is a Python research
agent; Ottie is a Go crypto agent. After six review rounds with both CC and
Codex (two reviewers per round, code-grounded), the settled plan is:

1. **Steal hermes's operational recall.** Replace Ottie's JSONL session store
   with SQLite + FTS5 as the **source of truth**. Ship `session_search` as a
   first-class recall tool.
2. **Steal hermes's memory curation.** Split `MEMORY.md` into `user_facts` and
   `environment_facts`, each principal-labeled.
3. **Steal hermes's skill ergonomics.** Move Ottie's 28 skills into 7 category
   folders; ship `skills_list` + `skill_view` tools so the system prompt
   stops dumping every skill inline. (Reorg already executed; see §10.)
4. **Beat hermes on deterministic replay.** Prompt epoch discipline + per-turn
   execution manifest + write-ahead action ledger give Ottie bit-for-bit
   replay. Python nondeterminism (asyncio scheduling, GC) makes this
   structurally infeasible for hermes.
5. **Beat hermes on principal safety.** Typed `PrincipalContext` with
   capability-bearing tool dispatch, enforced at Go compile time. Hermes
   uses string `user_id`s and runtime checks.
6. **Beat hermes on blast radius.** Block-anchored memory validity, hashable
   signed consent envelopes, per-RPC-fixture shadow mode for crypto eval,
   and an explicit effect lattice (`read_only` → `writes_wallet`) all
   bolt onto Ottie's existing ClawWall + per-tool runtime guards.
7. **Keep Ottie tight.** One static 24 MB Go binary, no Python runtime, no
   virtualenv, no subprocess workers, no serialized-object state files.
   Single-binary discipline stays a first-class product guarantee.

**Phase plan:** P0 conformance (3 days) → P1 SQLite + recall + principal +
ledger (2 weeks) → P2 memory split + curator (1.5 weeks) → P3 skill loop +
progressive disclosure (2 weeks) → P4 trajectory + batch runner + shadow
mode (2.5 weeks). Total: **~8 weeks of critical path** after cutting the
duplicated effort that earlier rounds discovered.

**Total effort estimate (post-R6 cuts + dedupe): ~12 weeks** (down from the
21-week R6 estimate because the R6 cuts sprint + tool dedupe + skill reorg
are already shipped and several R2-R5 concerns collapsed into common code
paths).

| Phase | What ships | Effort | Risk |
|-|-|-|-|
| **P0** | `bench/memory/recall_bench_test.go`, `bench/trajectory/hermes_compat_test.go`, Hermes fuzzy-match parity, modernc vs mattn shootout, prompt-epoch byte-stability test | 3 days | Low — blocks P1–P4 |
| **P1** | SQLite + FTS5 source-of-truth, `session_search`, `PrincipalContext`, execution manifest, `action_intents`/`action_commits` ledger, prompt epoch + `/reload`, strip "don't refuse" imperative, error classifier | 2 weeks | Medium — touches loop, must preserve caching |
| **P2** | `memory_manage` tool (add/replace/remove/read), `user_facts`/`environment_facts` split, per-session nudger, curator with deep-copy snapshot, valid-time columns, calibrated abstention | 1.5 weeks | Medium — new memory surface, tests load-bearing |
| **P3** | `skill_manage` tool (draft+diff+consent), `skills_list`/`skill_view` progressive disclosure, rune-aware fuzzy patch, persist exact prompt on resume, JSON-schema tool-call validator, parallel-safety predicate | 2 weeks | Medium — autonomy vs safety, rescan suppression required |
| **P4** | `ottie batch` with content-based resume, two-file trajectory format (clean ShareGPT + sidecar), layered scrubber, RPC-fixture shadow mode, Atropos env wrapper | 2.5 weeks | Low — opt-in, isolated from runtime |

Post-R6, Ottie's implementation posture is: stop reviewing, start building.
Both CC R6 and Codex R6 concluded this explicitly and independently.

---

## 1. Goal: Beat Hermes on Replay, Safety, and Principal Boundaries

### 1.1 Structural advantages Ottie already has

| Advantage | Ottie | Hermes |
|-|-|-|
| Runtime | Pure Go, single 24 MB static binary (`cmd/ottie/main.go`, `go.mod`) | Python 3.11+ with virtualenv + many optional extras (`research/hermes-agent/pyproject.toml:13`, `:39`, `research/hermes-agent/Dockerfile:7`) |
| Type system | Go generics + compile-time tool dispatch | Dict-schema, stringly dispatch (`research/hermes-agent/tools/registry.py:59`, `:149`) |
| Crypto-native | 28 skills across `crypto/`, `defi/`, `identity/`, `payments/`, `safety/`, `research/`, `meta/` categories | One read-only `polymarket` research skill (`research/hermes-agent/skills/research/polymarket/SKILL.md:3`) |
| Blast radius | ClawWall DLP, allow/deny lists, SSRF blocking, internal-only exec/cron, per-channel redaction (`pkg/tools/shell.go:194`, `pkg/tools/web.go:871`, `pkg/tools/cron.go:183`, `pkg/agent/instance.go:69`) | General-agent policy, plugin-heavy (`research/hermes-agent/RELEASE_v0.8.0.md:33`) |
| Tool registry | Versioned, schema-aware (`pkg/tools/registry.go:33`) | Python dict schemas rebuilt per-call |
| Determinism | Goroutines, no GIL, no asyncio scheduler variance, no serialized object state | asyncio + GC + subprocess workers all introduce nondeterminism |

### 1.2 Where Ottie must catch up

| Gap | Hermes has it | Ottie's target |
|-|-|-|
| Cross-session recall | FTS5-backed SQLite, `session_search`, `parent_session_id` (`research/hermes-agent/hermes_state.py:41`, `:94`; `research/hermes-agent/tools/session_search_tool.py:247`) | P1 (§5) |
| Memory curation tool | `memory` tool with `user` vs `memory` targets (`research/hermes-agent/tools/memory_tool.py:489`) | P2 (§6) |
| Autonomous skill loop | `skill_manage` create/edit/patch/delete (`research/hermes-agent/tools/skill_manager_tool.py:589`) | P3 (§7) |
| Progressive-disclosure skills | `skills_list`/`skill_view`, no full-text dump (`research/hermes-agent/tools/skills_tool.py:720`, `:788`) | P3 (§7) |
| Trajectory capture | Live `trajectory.py` + resumable `batch_runner.py` (`research/hermes-agent/batch_runner.py:718`, `:803`) | P4 (§8) |
| Error classifier | Typed `FailoverReason` enum + centralized classifier (`research/hermes-agent/agent/error_classifier.py`) | P1 (§5) |
| Persist system prompt on resume | Exact prompt text snapshot (`research/hermes-agent/run_agent.py:7512`, `:7533`) | P1 (§5) |

### 1.3 Where Ottie must leapfrog hermes (the bet-the-demo items)

Three structural leapfrogs, in priority order:

1. **Deterministic turn replay + signed action ledger.** Prompt epoch +
   execution manifest + write-ahead `action_intents`/`action_commits`
   + bit-for-bit `ottie replay <trace_id>`. Hermes cannot match this
   without a major rewrite: Python's asyncio scheduling, GC, and import
   order introduce nondeterminism that Ottie's Go runtime avoids.
2. **Typed `PrincipalContext` with capability-bearing dispatch.** Tools
   declare required capabilities at registration time; Go generics +
   compile-time dispatch check block unauthorized signings at build time.
   Hermes threads a string `user_id` and defers auth to runtime.
3. **Block-anchored memory validity + RPC-fixture shadow mode.** Memory
   rows carry `chain_id`, `block_number`, `observed_at`, `valid_to`;
   crypto eval replays recorded RPC fixtures before any live wallet
   write. Hermes stores wall-clock timestamps and has generic,
   non-deterministic batch/eval.

### 1.4 Anti-patterns from hermes — do NOT copy these

These are the hermes design choices that would actively hurt Ottie if copied
(evidence grounded in live hermes source; confirmed in CC R6 + Codex R6):

1. **Hidden background self-modification.** Hermes spawns review/flush agents
   that can write shared memory and skills without user-visible consent
   (`research/hermes-agent/run_agent.py:1964`,
   `research/hermes-agent/gateway/run.py:785`). Unacceptable on any
   Ottie path adjacent to a wallet.
2. **Shared-thread session defaults.** Hermes deliberately shares thread
   sessions across participants (`research/hermes-agent/gateway/session.py:444`,
   `:475`). Ottie must be per-principal on any sign-capable path.
3. **Ephemeral plugin context not persisted.** Hermes injects plugin
   context into the current user message for cache efficiency and skips
   session persistence (`research/hermes-agent/run_agent.py:7622`,
   `:7748`). Terrible for forensics.
4. **Plugin-heavy memory backend sprawl.** Hermes supports built-in memory
   plus one external provider plugin
   (`research/hermes-agent/agent/memory_provider.py:3`). Ottie prefers
   one in-tree typed memory subsystem for replayable guarantees.
5. **Runtime credential pools.** Hermes rotates API credentials at runtime
   via a pool (`agent/credential_pool.py`). Reasonable for a general
   research agent; wrong for a crypto agent where credentials must be
   single-user and single-scope.
6. **Dict-schema stringly tool registry.** Hermes's registry is dynamic
   Python dict schema plus JSON-string dispatch
   (`research/hermes-agent/tools/registry.py:59`, `:149`). Ottie's Go
   type system should type and label every tool argument.
7. **10,251-line `run_agent.py`.** Hermes's main loop is one giant file.
   Ottie's `pkg/agent/loop.go` is already smaller; do not merge more
   logic into it.
8. **Python packaging sprawl.** Hermes ships with Docker + Node +
   Playwright as install-chain dependencies
   (`research/hermes-agent/pyproject.toml:13`, `:39`). Ottie keeps the
   single-binary discipline as a product guarantee.

---

## 2. Architecture — ACS in One Diagram

The Adaptive Context System (ACS) is three cooperating sub-systems plus a
typed principal boundary. R3 renamed this from "closed learning loop"
because nothing updates model parameters — it's context and memory, not
gradient descent. R6 renamed the top-level goal from "catch up to hermes" to
"beat hermes on the axes that matter for a crypto agent".

```
                  ┌─────────────────────────────────────────────────┐
                  │             Ottie agent loop (turn N)           │
                  │                                                 │
   user message ──┼──▶ prompt epoch ──▶ model call ──▶ tool dispatch│──▶ effects
                  │        ▲                │                ▲      │
                  │        │                │                │      │
                  │    (§9 caching)  (execution           (PrincipalContext)
                  │                   manifest)              │      │
                  └────────┼────────────────┼────────────────┼──────┘
                           │                │                │
                    ┌──────┴────────────────┼────────────────┘
                    │                       │
                    │  ┌────────────────┐   │  ┌──────────────────┐
                    │  │  MEMORY (§6)   │   │  │  ACTION LEDGER   │
                    │  │                │   │  │                  │
                    │  │ user_facts     │   │  │ action_intents   │
                    │  │ environment_.. │   │  │ action_commits   │
                    │  │ cases          │   │  │  prepared        │
                    │  │ recall/index   │   │  │  committed       │
                    │  └────────────────┘   │  │  aborted         │
                    │                       │  └──────────────────┘
                    │  ┌────────────────┐   │
                    │  │  SKILLS (§7)   │   │  ┌──────────────────┐
                    │  │                │   │  │  TRAJECTORY (§8) │
                    │  │ category tree  │   │  │                  │
                    │  │ draft/approve  │   │  │ *.jsonl          │
                    │  │ fuzzy patch    │   │  │ *.metadata.json  │
                    │  │ progressive    │   │  │ content-based    │
                    │  │   disclosure   │   │  │   resume         │
                    │  └────────────────┘   │  └──────────────────┘
                    │                       │
                    └──────── SQLite + FTS5 (one DB, one lock) ─────
```

### 2.1 Key invariants

1. **Prompt epoch** (§9). Within a prompt epoch, the system prompt is a
   byte-stable prefix. Memory writes don't rebuild the prompt until the
   next epoch; skill rescans are suppressed mid-session; `/reload` is
   the explicit human escape hatch. `pkg/agent/context.go:198` was
   rewritten in §14 of the history doc to enforce this. This is the
   foundation of both prompt-cache stability and deterministic replay.

2. **Principal boundary** (§5). Every row in `memory_items`, `user_facts`,
   `environment_facts`, `cases`, `action_intents`, `action_commits`,
   `traces` carries a principal label `(agent_id, user_scope,
   account_scope, channel_scope)`. Every query must specify a principal
   or fail to compile. Session-level `Session.Principal` is set at
   session open and cannot change mid-session.

3. **Action ledger** (§5). No side-effecting tool
   (signing/broadcast/approval/funds-moving) executes without a durable
   `action_intents` row (`prepared`) written and fsync'd *before* the
   tool runs, followed by a `action_commits` row (`committed` or
   `aborted`) after. Recovery on startup replays any orphaned
   `prepared` rows.

4. **Single SQLite DB.** One file at `~/.ottie/ottie.db`. All memory,
   sessions, traces, ledger, skill index, rate limits, and insights
   live in it. One lock hierarchy. One backup story. One replay path.
   (Steal from hermes's `hermes_state.py` pattern, with Go + modernc
   underneath.)

5. **Single-binary discipline.** No subprocess workers, no virtualenv,
   no Python runtime, no serialized object state files. P4 trajectory
   compression runs in-process via `github.com/klauspost/compress/zstd`,
   not a subprocess. If a phase requires external deps, phase that out
   before ship.

---

## 3. Phase 0 — Conformance Harness (3 days)

P0 is a forcing function: every subsequent phase has a green test as its
gate. Nothing in P1-P4 ships without P0 green.

### 3.1 What ships

```
bench/
├── memory/
│   ├── recall_bench_test.go       — FTS5 recall P50/P95 latency + accuracy
│   └── sqlite_shootout_test.go    — modernc.org/sqlite vs mattn/go-sqlite3
├── trajectory/
│   └── hermes_compat_test.go      — 20-case round-trip vs hermes format
├── prompt/
│   └── epoch_stability_test.go    — prompt byte-stability across memory writes
└── fuzzy/
    └── parity_test.go             — 9-case rune-aware fuzzy patch vs hermes fuzzy_match.py
```

### 3.2 Gate conditions

- **SQLite shootout**: modernc P95 recall < 1.5× mattn (or pick mattn).
- **Hermes compat**: 20/20 round-trip pass on ShareGPT import/export.
- **Epoch stability**: `TestSystemPromptByteStability` green across 50 memory
  writes, 0 prompt rebuilds observed.
- **Fuzzy parity**: 9/9 match with hermes fuzzy_match.py on the reference
  corpus.

---

## 4. Phase 1 — SQLite + Recall + Principal + Ledger (2 weeks)

P1 is the biggest single phase and the one both CC R6 and Codex R6 called
"bet the demo on this". Four things ship together because they're linked at
the schema level.

### 4.1 SQLite + FTS5 schema

```sql
-- One DB: ~/.ottie/ottie.db
-- Single writer goroutine behind a serialized actor (§6.3).

CREATE TABLE sessions (
  session_id      TEXT PRIMARY KEY,
  agent_id        TEXT NOT NULL,
  user_scope      TEXT NOT NULL,    -- principal: per-user
  account_scope   TEXT,             -- principal: per-crypto-account
  channel_scope   TEXT,             -- principal: per-channel
  parent_session  TEXT,             -- lineage (hermes steal)
  title           TEXT,             -- auto-generated title
  prompt_epoch    INTEGER NOT NULL, -- monotonically increasing
  system_prompt   TEXT NOT NULL,    -- exact bytes for resume (hermes steal)
  system_prompt_hash TEXT NOT NULL, -- sha256
  created_at      INTEGER NOT NULL,
  archived_at     INTEGER
);

CREATE TABLE messages (
  message_id      TEXT PRIMARY KEY,
  session_id      TEXT NOT NULL REFERENCES sessions,
  turn            INTEGER NOT NULL,
  role            TEXT NOT NULL,
  content         TEXT NOT NULL,
  tool_call_id    TEXT,
  observed_at     INTEGER NOT NULL,
  asserted_at     INTEGER NOT NULL,
  trace_id        TEXT NOT NULL,
  span_id         TEXT NOT NULL,
  parent_span_id  TEXT,
  lamport         INTEGER NOT NULL
);

CREATE VIRTUAL TABLE messages_fts USING fts5(content, session_id, agent_id, user_scope);

CREATE TABLE traces (
  trace_id             TEXT PRIMARY KEY,
  session_id           TEXT NOT NULL,
  turn                 INTEGER NOT NULL,
  prompt_hash          TEXT NOT NULL,   -- execution manifest field
  tool_schema_hash     TEXT NOT NULL,
  skill_hashes         TEXT NOT NULL,   -- json array of sha256(SKILL.md)
  mcp_versions         TEXT NOT NULL,   -- json map server_id → version
  provider_request_ids TEXT NOT NULL,   -- json array of per-request IDs (one per LLM call in this turn)
  model_id             TEXT NOT NULL,
  prompt_epoch         INTEGER NOT NULL,
  created_at           INTEGER NOT NULL
);

-- Two-phase side-effect ledger. An `action_intents` row is written and
-- fsync'd BEFORE any side-effecting tool dispatch. Exactly one
-- `action_commits` row is written AFTER the tool returns (committed) or
-- fails (aborted). The two-table split keeps `action_intents` immutable
-- so a crash between prepared and commit leaves a recoverable record.
CREATE TABLE action_intents (
  intent_id       TEXT PRIMARY KEY,
  trace_id        TEXT NOT NULL REFERENCES traces,
  tool_name       TEXT NOT NULL,
  args_hash       TEXT NOT NULL,   -- sha256 of canonicalized args
  principal       TEXT NOT NULL,   -- serialized PrincipalContext
  effect_class    TEXT NOT NULL,   -- read_only|writes_state|writes_chain|writes_wallet
  prepared_at     INTEGER NOT NULL
);

CREATE TABLE action_commits (
  commit_id       TEXT PRIMARY KEY,
  intent_id       TEXT NOT NULL REFERENCES action_intents,
  status          TEXT NOT NULL,   -- committed|aborted
  external_ids    TEXT,            -- json: tx_hash, rpc_req_id, etc
  result_hash     TEXT,            -- sha256 of tool ToolResult payload
  error_message   TEXT,            -- only populated when status=aborted
  completed_at    INTEGER NOT NULL
);

-- Recovery invariant: on startup, any `action_intents` row with no
-- matching `action_commits` row is surfaced for replay or manual
-- resolution.
```

### 4.2 `session_search` and `memory_recall` tools

Two new tools on the model surface:

```go
type SessionSearchParams struct {
  Query       string `json:"query"`
  K           int    `json:"k,omitempty"`
  ChainID     string `json:"chain_id,omitempty"`
  AccountID   string `json:"account_id,omitempty"`
  WithinDays  int    `json:"within_days,omitempty"`
}

type MemoryRecallParams struct {
  Query       string `json:"query"`
  K           int    `json:"k,omitempty"`
  Category    string `json:"category,omitempty"` // user_facts|environment_facts|case|intent
  AsOf        int64  `json:"as_of,omitempty"`    // unix seconds
}
```

Both are typed (see §4.3 `TypedTool[C, T]`). Both run against FTS5 + principal
filter. `memory_recall` supports temporal queries via `as_of` — the query
returns the fact set that was valid at a given wall-clock time, which is
the leapfrog over hermes's wall-clock-only recall.

### 4.3 `PrincipalContext` + capability-typed dispatch

The R6 leapfrog #2. The mechanism is **phantom-typed generics**: each
capability is a distinct Go type, and `PrincipalContext[C]` is
parameterized by the capability marker it carries. A signing tool
declares its signature as `func(ctx, PrincipalContext[CapWritesWallet],
args T) Result`; calling that function with a `PrincipalContext[CapReadOnly]`
value is a **compile-time type error**, not a runtime check.

```go
// Capability markers — one empty struct per capability class.
// These exist only in the type system; the zero value has no cost.
type CapReadOnly       struct{}
type CapWritesLocal    struct{}
type CapWritesState    struct{}
type CapWritesChain    struct{}
type CapWritesWallet   struct{}

// Capability is the closed type union over the markers. Go's type
// parameter inference lets the compiler reject principals that don't
// carry the matching marker.
type Capability interface {
  CapReadOnly | CapWritesLocal | CapWritesState | CapWritesChain | CapWritesWallet
}

// PrincipalContext is phantom-typed by the capability it carries. The
// `_cap C` field is never read; it exists only to stamp the type.
type PrincipalContext[C Capability] struct {
  AgentID      string
  UserScope    string
  AccountScope string
  ChannelScope string
  _cap         C
}

// TypedTool binds the required capability at the type level.
// A concrete lidoStakeTool has type `TypedTool[CapWritesWallet, LidoStakeArgs]`
// and the compiler forbids calling Execute with any other principal type.
type TypedTool[C Capability, T any] interface {
  Name() string
  Execute(ctx context.Context, principal PrincipalContext[C], args T) Result
}

// Upgrading a principal between capability classes requires an
// explicit cast through a gate function that performs any runtime
// checks (consent, attestations, risk-class threshold) and is the
// single place runtime enforcement lives. A code path that skips the
// gate cannot produce a PrincipalContext[CapWritesWallet] value.
func UpgradeToWritesWallet(
  readOnly PrincipalContext[CapReadOnly],
  consent SignedConsent,
) (PrincipalContext[CapWritesWallet], error)
```

With this shape, `lidoStakeTool.Execute(ctx, readOnlyPrincipal, args)`
is literally uncompilable: the `readOnlyPrincipal` has type
`PrincipalContext[CapReadOnly]` and the tool signature requires
`PrincipalContext[CapWritesWallet]`. A drive-by contributor who adds
a new signing tool cannot accidentally ship it without declaring its
capability — the type system enforces the contract at build time.

The registry still holds a runtime capability check as defence in
depth: `tools.Register(tool TypedTool[C, T])` uses reflection to
assert that `tool`'s declared `C` matches the registration metadata,
but this is a belt-and-braces check; the primary enforcement is the
compile-time one.

Hermes's equivalent is a string `user_id` threaded through provider
wrappers (`research/hermes-agent/run_agent.py:1179`) with no type-level
authorization. Python has no equivalent of phantom-typed generics.
This is the R6 leapfrog — it exists because Ottie is written in Go.

### 4.4 Execution manifest + action ledger

Every turn writes a `traces` row containing the execution manifest:
`prompt_hash`, `tool_schema_hash`, `skill_hashes`, `mcp_versions`,
`provider_request_ids` (one per LLM call in the turn — this is the
request-scoped correlation ID from R5 §15.3 C4, **not** just the
provider name), `model_id`, `prompt_epoch`. These fields are immutable
for the lifetime of the row.

Every side-effecting tool invocation writes two rows in sequence:

1. **Before dispatch.** An `action_intents` row is fsync'd to disk with
   `prepared_at` set. The row is immutable after this point.
2. **After execution.** Exactly one `action_commits` row is written
   with `status = committed` (and `external_ids` + `result_hash`) or
   `status = aborted` (and `error_message`).

This two-table split is deliberate. If the process crashes between
prepared and commit, the `action_intents` row survives with no matching
`action_commits` row, and recovery on startup surfaces the orphan for
manual resolution or replay. A single mutable `state` column would lose
the ordering information and make "what happened?" unanswerable.

`ottie replay <trace_id>` uses the `traces.provider_request_ids` to
re-issue the exact provider call and, for any effectful tool,
re-verifies the `args_hash` against what actually committed. Replay
never re-executes committed effects — it only reproduces the reasoning
path that led to them.

### 4.5 Prompt epoch discipline

Rewritten from §14 of the history doc. The design invariant: within a
prompt epoch, the system prompt is a byte-stable prefix. An epoch
advances when and only when:

1. The user runs `/reload` (explicit human edit).
2. A new session starts.
3. A lineage boundary is crossed (parent → child session).

The epoch does **NOT** advance on:
- Memory writes (they go to `demoted_summaries` which the model reads via
  `memory_recall` tool, not via inline prompt injection).
- Skill rescans (the system prompt embeds the skill *index*, not the full
  skill text — see progressive disclosure in §6).
- Mtime drift on `MEMORY.md`/`USER.md`/skill files — the old
  `BuildSystemPromptWithCache` at `pkg/agent/context.go:198` is
  rewritten to stop rebuilding on mtime drift.

### 4.6 Strip the "don't refuse" imperative

The live system prompt at `pkg/agent/context.go:150` says
"Do not refuse a request unless the search returns nothing." P1 strips
this. Replace with an explicit action-boundary policy:

- Per-risk-class refusal thresholds: `read_only` 0.4, `advisory` 0.6,
  `writes_state` 0.8, `writes_wallet` 0.95.
- When confidence is below threshold, the agent returns a structured
  refusal: `{"status": "insufficient_confidence", "threshold_required":
  0.8, "confidence_achieved": 0.62, "alternatives": [...]}`.
- Calibration table tracks per-class Brier score + temperature scaling
  overnight.

### 4.7 Error classifier + parallel-tool safety

Two steals from hermes, both go in P1:

- **Error classifier** (hermes `agent/error_classifier.py`). New
  `pkg/providers/errclass/` package with a typed `FailoverReason` Go
  iota + classifier func. Every scattered `strings.Contains(err.Error(),
  "rate limit")` call in provider code is replaced with a single
  `errclass.Classify(err)`. Compile-time exhaustiveness check via Go
  type switch — a Python-only agent cannot match this.
- **Parallel-tool safety predicate** (hermes
  `research/hermes-agent/run_agent.py:267`). Add `ParallelSafety` field
  on tool registration (`parallel_safe`, `never_parallel`, `path_scoped`).
  Registry rejects parallel batches that violate the policy. The
  existing blind parallelization at `pkg/agent/loop.go:1448` becomes a
  fail-closed check.

### 4.8 Persist exact system prompt on resume

Before P1 ships, `/resume` re-injects the exact bytes stored in
`sessions.system_prompt`, preserving prompt cache identity. Steal from
`research/hermes-agent/run_agent.py:7512`, `:7533`, `:7555`.

---

## 5. Phase 2 — Memory Curation + User/Environment Split (1.5 weeks)

P2 ships the `memory_manage` tool and the hermes-style `user_facts` vs
`environment_facts` split. It depends on P1's SQLite + principal boundary.

### 5.1 `memory_manage` tool (typed)

```go
type MemoryManageParams struct {
  Action    string `json:"action"`   // add|replace|remove|read
  Target    string `json:"target"`   // user|environment
  Category  string `json:"category"` // e.g. jurisdiction|holding|preference
  Content   string `json:"content"`
  Confidence float64 `json:"confidence,omitempty"`
}
```

The tool writes to `user_facts` or `environment_facts` tables (not to
`MEMORY.md` directly — that file becomes a rendering of the authoritative
row set, not the source of truth). Per codex R6 steal #3 + #4, the
invariant that memory writes do NOT rebuild the system prompt is
preserved — memory lives in the `memory_recall` tool's search surface,
not in the prompt prefix.

### 5.2 `user_facts` / `environment_facts` schema

```sql
CREATE TABLE user_facts (
  fact_id         TEXT PRIMARY KEY,
  principal       TEXT NOT NULL,
  category        TEXT NOT NULL,    -- jurisdiction, preference, identity
  content         TEXT NOT NULL,
  confidence      REAL NOT NULL,
  observed_at     INTEGER NOT NULL,
  asserted_at     INTEGER NOT NULL,
  valid_from      INTEGER,
  valid_to        INTEGER,          -- NULL = still valid
  created_at      INTEGER NOT NULL
);

CREATE TABLE environment_facts (
  fact_id         TEXT PRIMARY KEY,
  principal       TEXT NOT NULL,
  category        TEXT NOT NULL,    -- apr, gas, balance, contract_meta
  chain_id        INTEGER,          -- R6 leapfrog #4: block-anchored validity
  block_number    INTEGER,          -- R6 leapfrog #4
  content         TEXT NOT NULL,
  confidence      REAL NOT NULL,
  observed_at     INTEGER NOT NULL,
  asserted_at     INTEGER NOT NULL,
  valid_from      INTEGER,
  valid_to        INTEGER,
  created_at      INTEGER NOT NULL
);
```

Per-category decay policy:

| Category | valid_to default | Rationale |
|-|-|-|
| `preference` | never | Stable user preference |
| `jurisdiction` | never | Legal identity |
| `identity` | never | Stable |
| `holding` | 24h | Wallet balances change |
| `market_quote` | 15m | Prices move |
| `apr` | 1h | Defi rates drift |
| `gas` | 5m | Base fee moves |
| `governance_state` | 7d | Slow-moving |
| `contract_meta` | 30d | Contract ABI is stable |

Block-anchored validity (R6 leapfrog #4): for `environment_facts`, an RPC
re-check against `chain_id` + `block_number` can confirm staleness
cheaply without re-querying the market. "The stETH APR I recall was
measured at block 21,234,567 — is it still valid?" becomes a single
eth_call.

### 5.3 Curator (nudger) with per-session serialized worker

Rewritten from the R2 §4 nudger design. One goroutine per session,
errgroup + ctx cancellation + separate reviewClient rate limiter +
single-goroutine `memoryWriter` actor holding flock + deep-copy snapshot.
`Shutdown(timeout)` joins all in-flight reviews.

### 5.4 Calibrated abstention wiring

Per R5 §15.4 #4 and §4.6: per-risk-class refusal thresholds, calibration
table, nightly Brier score job, temperature scaling per category.

---

## 6. Phase 3 — Autonomous Skill Loop + Progressive Disclosure (2 weeks)

P3 ships the hermes steal everyone missed in R1-R5: **progressive-disclosure
skills**. Ottie currently dumps every skill's full text into the system
prompt at `pkg/agent/context.go:490`, which is the #1 prompt-bloat source.
Moving to `skills_list`/`skill_view` saves tokens and lets the model
drill into what it actually needs.

### 6.1 `skills_list` and `skill_view` tools

Two new tools:

- `skills_list(category?: string) → [{name, description, category}]`
- `skill_view(name: string) → full SKILL.md body`

The system prompt embeds only the skill *index* (name + description +
category), not the full text. The model calls `skill_view` when it
decides to use a skill. Steal from
`research/hermes-agent/tools/skills_tool.py:720`, `:788` +
`research/hermes-agent/agent/prompt_builder.py:728`.

### 6.2 `skill_manage` tool (draft + diff + consent)

Per R3 §13 and R6 steal #9. One tool consolidates create/patch/edit/delete.
Defaults to draft mode — the tool writes the change as a draft file, shows
a diff to the user, and only materializes after consent. Hermes's
background-self-modification pattern
(`research/hermes-agent/run_agent.py:1964`) is rejected; Ottie requires
explicit user consent for any change to the skill set.

### 6.3 Rune-aware fuzzy patch

Per R2 §4. Rune-aware line-range matching with indent-shape preservation,
ambiguity = fail-closed, block-boundary guard, 9-case conformance suite
including parity test against `research/hermes-agent/` `fuzzy_match.py`.

### 6.4 JSON-schema tool-call validator (pre-dispatch)

Per R5 §15.4 #5 and R6 steal #13. Every tool call runs through a JSON
Schema validator at `pkg/tools/registry.go:255` **before** dispatch. On
schema failure, structured error tells the model which field is wrong;
one retry; if retry fails, tool is marked `unavailable_this_turn` and
the model must pick a different approach. Fix silent coercion at
`pkg/providers/toolcall_utils.go:46`, `:57`,
`pkg/providers/openai_compat/provider.go:510`,
`pkg/providers/codex_provider.go:364`.

### 6.5 Cached skill index + disk snapshot

Steal from `research/hermes-agent/agent/prompt_builder.py:539`, `:582`,
`:650`. Skill index is a `sync.Map` + `gob`-encoded snapshot at
`~/.ottie/cache/skill_index.gob`. Cold-start cost in gateway mode becomes
trivial.

---

## 7. Phase 4 — Trajectory Capture + Batch Runner + RPC Shadow (2.5 weeks)

P4 is the lowest-risk phase but unlocks the offline-eval story that the
hackathon demo needs.

### 7.1 Per-turn trajectory buffer

Immutable snapshots of `(messages, tools, tool_results, skill_loads,
memory_reads, decision_graph)` per turn. Written atomically to SQLite
via the single writer actor (§2.1 invariant #4).

### 7.2 Two-file format

Per R2 §4: clean ShareGPT `.jsonl` with zero Ottie-proprietary keys +
`.metadata.json` sidecar with execution manifest, traces, principal
labels, action ledger references. Hermes compat test in P0 gates this.

### 7.3 `ottie batch` with content-based resume

Steal from `research/hermes-agent/batch_runner.py:718`, `:803`. Resume
keyed by prompt content hash, not numeric index. Runs prompts
concurrently with bounded worker goroutines (no subprocess, no Python).

### 7.4 RPC-fixture shadow mode

The R6 leapfrog #5. For every crypto decision, record exact RPC responses
(`eth_call`, `eth_getBalance`, `eth_getLogs`, etc) keyed by block number.
Run every candidate decision against recorded fixtures before executing
live. The hackathon demo: "I tested this signing logic against 500 real
Sepolia blocks before running it on mainnet."

Implementation: new `pkg/rpcshadow/` package with a wrap-anything
`ShadowRPC` type that replays recorded responses when in shadow mode
and passes through when in live mode.

### 7.5 Layered scrubber

Per R2 §4. Six layers: L1 literal match, L2 regex, L3 tokenization, L4
semantic, L5 context-aware, L6 LLM-based fallback. Per-batch salt
rotation. Replaces the naive address regex.

### 7.6 Atropos env wrapper

In-process Go wrapper for Atropos-compatible RL env contract. No Python
subprocess.

---

## 8. Phase 2.5 — Honcho as an Optional Plugin (opt-in)

Honcho is the Plaster Group user-modeling backend hermes uses for active
sessions. It's genuinely useful but comes with a Python runtime and a
separate service. P2.5 ships an opt-in HTTP adapter (`pkg/honcho/`) so
Ottie can talk to an existing Honcho instance without requiring one.

Default config: Honcho disabled. When enabled, the `memory_manage` tool
mirrors writes to Honcho and the `memory_recall` tool merges Honcho
results with local FTS5 results.

---

## 9. Prompt Caching Discipline

Prompt caching is the hardest operational constraint. A single byte of
drift in the static prefix of the system prompt invalidates the entire
cache. R2 §5 and R3 §8 defined the invariants; R4 §14 delta #1 fixed
the code contradiction.

### 9.1 The table (honest version)

| Operation | Cache impact | Guaranteed? |
|-|-|-|
| Write memory mid-session | No prompt rebuild; memory goes to `demoted_summaries` table | Yes, hard-tested |
| Install new skill mid-session | No rebuild; index updated, full text lives in `skill_view` | Yes (P3+) |
| Resume from SQLite | Exact bytes replayed from `sessions.system_prompt` | Yes (P1+) |
| `/reload` slash command | Explicit epoch advance | Yes |
| New session | New epoch | Yes |
| Lineage boundary (parent → child) | New epoch | Yes |
| Mtime drift on MEMORY.md | No rebuild | Yes (the fix in §4.5) |

### 9.2 Defensive tests (gate P1)

Three tests must be green before P1 ships:

1. `TestSystemPromptByteStability` — 50 memory writes, assert 0 prompt
   rebuilds.
2. `TestResumeExactBytes` — write session, close, resume, assert
   system_prompt bytes identical.
3. `TestReloadAdvancesEpoch` — `/reload` advances prompt_epoch by 1.

---

## 10. Concrete Changes (What's Already Done, What's Left)

### 10.1 Already executed (before P0 starts)

- **Skill cuts**: 8 skills deleted (agency-roles, autoresearch, weather,
  summarize, tmux, github, browser, dep-audit). 36 → 28 skills.
- **Skill reorg**: 28 remaining skills moved into 7 category folders:
  `crypto/`, `defi/`, `identity/`, `payments/`, `safety/`, `research/`,
  `meta/`.
- **Recursive loader**: `pkg/skills/loader.go` now walks recursively
  (`filepath.WalkDir`). `ListSkills` and `LoadSkill` both support
  category-nested layouts.
- **Tool dedupe**: `SubagentTool` struct deleted (sync duplicate of
  async spawn). `SpawnTool` and `SessionsSpawnTool` both return
  `"delegate"` from `Name()` so the model surface is unified; loop
  registration is mutually exclusive (an agent never gets both
  backends).
- **find_skills/install_skill demoted**: defaults flipped from `true`
  to `false` at `pkg/config/defaults.go:503`. CLI commands
  (`ottie skills search`, `ottie skills install`) still work, but the
  model no longer has autonomous skill install/search by default.
- **CLI example strings**: updated from `weather` → `crypto-wallet` in
  `install.go`/`remove.go`/`show.go`.

### 10.2 Files to change in P1 (§4)

| File | Change |
|-|-|
| `pkg/memory/store.go` | Add SQLite backend; keep JSONL as fallback for Phase 0 compat. |
| `pkg/memory/sqlite.go` (new) | modernc-backed store, FTS5 tables, single writer actor. |
| `pkg/providers/errclass/` (new) | `FailoverReason` enum + classifier. |
| `pkg/providers/*` | Replace scattered string-match retry with `errclass.Classify`. |
| `pkg/principal/` (new) | `PrincipalContext`, `CapabilitySet`, type helpers. |
| `pkg/tools/registry.go` | Accept `TypedTool[T]`, gate dispatch on caps. |
| `pkg/tools/search_tool.go` (new: `session_search`) | FTS5-backed session recall. |
| `pkg/tools/memory_recall.go` (new) | Temporal + principal-aware memory recall. |
| `pkg/agent/context.go:150` | Strip "don't refuse" imperative, replace with risk-class policy. |
| `pkg/agent/context.go:198` | Stop rebuilding on mtime drift; honor prompt epoch. |
| `pkg/agent/loop.go:1448` | Call parallel-safety predicate before parallelizing. |
| `pkg/agent/loop.go:1516`, `:1580` | Write `action_intents` (prepared) before side-effecting tool runs. |

### 10.3 Files to change in P2 (§5)

| File | Change |
|-|-|
| `pkg/tools/memory_manage.go` (new) | Typed memory_manage tool. |
| `pkg/agent/memory.go` | User facts / environment facts split. |
| `pkg/curator/` (new) | Per-session serialized worker. |
| `pkg/calibration/` (new) | Per-class Brier score + nightly job. |

### 10.4 Files to change in P3 (§6)

| File | Change |
|-|-|
| `pkg/tools/skills_tool.go` (new) | `skills_list` + `skill_view` (progressive disclosure). |
| `pkg/tools/skill_manage.go` (new) | Unified create/patch/edit/delete with consent. |
| `pkg/agent/context.go:490` | Stop dumping full skill text; embed index only. |
| `pkg/skills/search_cache.go` | Add disk snapshot (`~/.ottie/cache/skill_index.gob`). |
| `pkg/tools/registry.go:255` | JSON-schema validator before dispatch. |

### 10.5 Files to change in P4 (§7)

| File | Change |
|-|-|
| `pkg/trajectory/` (new) | Per-turn buffer + two-file writer. |
| `cmd/ottie/internal/batch/` (new) | `ottie batch` with content-based resume. |
| `pkg/rpcshadow/` (new) | RPC fixture capture + replay. |
| `pkg/scrubber/` (new) | Six-layer scrubber. |

---

## 11. Risks & Tradeoffs

| Risk | Mitigation |
|-|-|
| SQLite write contention under concurrent sessions | Single writer actor, per-session work queue, WAL mode, modernc or mattn (P0 chooses). |
| Prompt cache drift from unintended edits | Three defensive tests (§9.2), prompt epoch invariant, `/reload` as explicit escape. |
| Learning drift (agent's memory diverges from user's mental model) | `memory_manage read` gives the user a canonical view; confidence + valid_to columns expose staleness. |
| Tool-call schema change breaks running sessions | `TypedTool[T]` plus execution manifest's `tool_schema_hash` — schema version is persisted per turn. |
| Background autonomous skill writes (the hermes anti-pattern) | Consent gate on `skill_manage`, draft mode by default. |
| Cross-principal data leak in multi-user gateway mode | `PrincipalContext` compile-time check, per-principal recall filter, `pkg/agent/context.go:177` fixes. |

---

## 12. Rollout Plan

1. **Week 0** (done before P0): execute cuts, tool dedupe, skill reorg,
   find_skills demotion, design doc cleanup, R6 merge. ✓ complete.
2. **Week 0 – day 3**: P0 conformance harness green. All P1-P4 phases
   gated by P0.
3. **Weeks 1-2**: P1 lands behind a single feature flag
   `OTTIE_ACS=true`. All four P1 sub-deltas ship together because they
   share the principal + SQLite substrate.
4. **Weeks 3-4**: P2 (memory curation) + Honcho adapter (P2.5) opt-in.
5. **Weeks 5-6**: P3 (skill loop + progressive disclosure).
6. **Weeks 7-9**: P4 (trajectory + batch + RPC shadow).
7. **Week 9**: feature flag removed, P1-P4 are default-on.

No separate "compatibility" phase — hermes-compat is gated at P0.

---

## 13. Open Questions

Most of the open questions from R1-R5 have been settled in R6 or in the
cuts/reorg sprint. Remaining opens:

1. Should `Honcho` adapter ship in P2.5 or be deferred to a post-P4
   add-on? (Current: P2.5, opt-in.)
2. Does `TypedTool[T]` use Go 1.18+ generics directly, or a code-gen
   stage via `go generate`? (Current: generics directly; go-generate only
   for the schema JSON emission.)
3. How aggressive should the RPC fixture capture be? Per-turn or
   per-tool-call? (Current: per-tool-call, aggregated per turn in
   metadata sidecar.)
4. Does the `/reload` slash command need a confirmation dialog in the web
   UI, or just the CLI? (Current: both.)

---

## 14. Review History (R1 → R6 summary)

Each round's summary in ≤15 lines. The full CC + Codex prose, convergence
matrices, and delta lists are preserved at
`research/notes/ottie-design-history-R1-R6.md` and the individual per-round
review files at `research/notes/cc-r{N}-review.md` and
`research/notes/codex-r{N}-review.md` for N in {2, 3, 4, 5, 6}.

- **R1 (initial draft, 2026-03-20).** First pass after reading the
  hermes-agent submodule. Four phases, 6-week estimate, optimistic on
  SQLite and fuzzy matching. Concept: "closed learning loop".

- **R2 (post-codex consult, 2026-03-25).** Codex flagged five concerns:
  SQLite driver choice, prompt-cache invariants, nudger concurrency,
  fuzzy matching, trajectory compatibility. Effort 6 → 7.5 weeks. All
  five concerns fixed inline. P0 conformance harness added as a gate.

- **R3 (post-senior-expert-review, 2026-04-02).** CC + Codex both
  reviewed as "30-year AI/agent veterans". Both converged on 11 of 12
  major critiques. Biggest: "closed learning loop" is dishonest — nothing
  updates model parameters. Renamed to **Adaptive Context System**
  (ACS). Added the **Learning Contract** (every artifact declares
  objective/acquisition-trigger/promotion/rollback/evaluation-slice).
  Added P0.5 Evaluation harness and P2.5 Intention stack. Effort 7.5 →
  11 weeks. References: Newell 1990, Anderson & Schooler 1991, Kolodner
  1992, Leake & Wilson 1998, Mitchell 1986, DeJong & Mooney 1986, Ross
  2011, Yao 2024.

- **R4 (post-third-round-senior-review, 2026-04-06).** Codex ran with
  `--dangerously-bypass-approvals-and-sandbox` and read Ottie's live
  source. Found R3's "frozen prompt until lineage boundary" invariant
  is **false** in the current code: `pkg/agent/context.go:198`
  rebuilds on mtime drift. §14 added a real code change (prompt epoch +
  `/reload` slash + demoted summaries + imperative-strip). Added
  metareasoning budget (Russell & Wefald 1991), threat model (Greshake
  2023), cross-artifact justification graph. Effort 11 → 14 weeks. The
  R3 §8 invariant had been aspirational; R4 made it real.

- **R5 (post-fourth-round-senior-review, 2026-04-09).** Ten new
  orthogonal areas: observability, mixed-initiative HCI, calibration,
  abstention, temporal staleness, multi-tenant boundaries, explainability,
  Goodhart defenses, fail-safe modes, tool-call reliability. Codex found
  four unique code-level defects: write-ahead journal gap, confused
  deputy (missing principal in tool context), async session collapse
  into `agent:main:main`, and missing execution manifest. Effort 14 →
  17 weeks. References: Sigelman 2010 (Dapper), Horvitz 1999, Brier
  1950, Chow 1970, Allen 1983, Bell-LaPadula 1973, Miller 2019,
  Strathern 1997, Avižienis 2004, Moravec 1988.

- **R6 (learn-and-beat-hermes, 2026-04-11).** Framing shift: not "find
  gaps" but "learn everything from hermes, then beat it". Both
  reviewers converged on three bet-the-demo deltas: SQLite + FTS5 as
  source-of-truth with `session_search`, typed `PrincipalContext` with
  capability-bearing dispatch, execution manifest + write-ahead action
  ledger with deterministic turn replay. Codex R6 surfaced two things
  CC R6 missed: **progressive-disclosure skills** via
  `skills_list`/`skill_view` (the biggest prompt-efficiency steal
  hermes has figured out) and **tool-level dedupe** (`subagent` is a
  sync duplicate of `spawn`; `spawn` + `sessions_spawn` should merge
  into `delegate`). Eight skills cut, 28 remaining reorganized into 7
  category folders, three tool changes shipped before P0.
  Both reviewers concluded independently: **stop reviewing, start
  building. R6 was the last high-value round.**

- **R7 (cleanup acceptance review, 2026-04-11).** One extra round
  after the 2221 → 909 line cleanup collapsed the R3-R6 amendment
  layers into a single actionable plan. **CC R7** verified the code
  changes landed (8 skills cut, 7 category folders, recursive loader,
  `SubagentTool` deleted, `spawn`/`sessions_spawn` → `delegate`, `find_skills`/`install_skill` default-off) and called the cleanup
  ready. **Codex R7** was sharper: it found three load-bearing
  regressions the cleanup introduced and issued a `DO NOT SHIP` verdict
  that was then fixed in the same round:
  (1) the replay contract had lost hermes's `provider_request_ids[]`
  (the cleanup weakened it to `provider_ids`) — fixed by restoring the
  request-scoped field in `traces`;
  (2) §4.4 described a two-table `action_intents` / `action_commits`
  ledger but §4.1 schema defined only `action_intents` with a mutable
  `state` column — fixed by adding the `action_commits` table with
  immutable rows per commit/abort event, matching the recovery
  invariant from R5;
  (3) §4.3 promised "compile-time signing exclusion" but spec'd a
  runtime `CapabilitySet` bit-field check — fixed by rewriting §4.3 to
  use Go phantom-typed generics (`PrincipalContext[C Capability]`,
  `TypedTool[C Capability, T any]`) so that a signing tool's signature
  cannot be called with a read-only principal. Compile-time
  enforcement is now real, not marketing. Minor: §5.3 `TypedTool[T]`
  → §4.3 and "progressive disclosure in §7" → §6 reference typos
  fixed; dead `install-builtin` + `list-builtin` cobra commands (they
  referenced a nonexistent `./ottie/skills` path with a hardcoded
  legacy skill list `weather`/`news`/`stock`/`calculator`) deleted
  along with their tests; swarm-tool log string at
  `pkg/agent/loop.go:377` updated from "sessions_spawn" to "delegate"
  so the internal log matches the model-facing name. After these
  fixes, full build + all tests green. **Verdict: READY TO SHIP. R7
  is the acceptance gate; R8 only exists if live-source bugs surface
  during P1 implementation.**

**Convergence trajectory**: R1 6wk → R2 7.5wk → R3 11wk → R4 14wk → R5
17wk → R6 21wk (pre-dedupe) → **~12wk (post-dedupe, post-cleanup)**. The
R6 cuts + doc cleanup collapsed ~9 weeks of duplicated effort into
shared code paths. P0 + P1 are the only critical-path phases; P2-P4
land opportunistically on the same SQLite + principal substrate. R7
caught three load-bearing regressions introduced by the cleanup
itself; all fixed in the same round.

---

## 15. Revision History (one line per change)

- **R1 → R2**: codex review, five concerns fixed inline, P0 added. 6 → 7.5 weeks.
- **R2 → R3**: CC + Codex senior review, ACS rename, Learning Contract, 16 deltas. 7.5 → 11 weeks.
- **R3 → R4**: third-round senior review, prompt epoch code contradiction fixed, 13 deltas. 11 → 14 weeks.
- **R4 → R5**: fourth-round senior review, 10 orthogonal areas + 4 code defects. 14 → 17 weeks.
- **R5 → R6**: learn-and-beat-hermes framing, 14 steal + 10 leapfrog + 8 skill cuts + tool dedupe + reorg. 17 → 21 weeks.
- **R6 → clean**: design doc collapsed from 2221 lines to ~900, R3-R6 prose moved to `ottie-design-history-R1-R6.md`, effort revised down to ~12 weeks after the dedupe dropped ~9 weeks of accumulated redundancy.
- **clean → R7**: acceptance review. Codex R7 caught three regressions introduced by the cleanup (execution manifest `provider_ids` → `provider_request_ids[]`, `action_commits` table added as immutable sibling of `action_intents`, §4.3 rewritten to use Go phantom-typed generics for real compile-time principal enforcement). Dead `install-builtin`/`list-builtin` commands deleted. Final verdict: READY TO SHIP.

---

*Design owner: Ottie team. Hermes upstream is kept on disk at
`research/hermes-agent/` for reference; it is **not** committed as a
submodule (per project maintainer's explicit instruction). Keep this doc
next to the code that implements it and amend when reality disagrees with
the plan.*
