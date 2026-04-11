# Codex Review of Ottie Learning Loop Design

Session ID: `019d7d19-e357-7d62-a8a1-fe26fe8db00c`
Tokens: 229,197
Reasoning: xhigh
Date: 2026-03-25

> **Note on verification coverage:** Codex's sandbox blocked local file reads in this run (`bwrap: loopback: Failed RTM_NEWADDR`). It verified Hermes claims against the public repo and public issue trackers but could not diff our actual Go code. Any "Hermes does X" claim in the review below is from public sources; any claim about Ottie is from the design summary alone.

## Concern 1 — SQLite + FTS5 via modernc is not a free win

Hermes being "SQLite+FTS5" does not validate the Go driver choice because Hermes uses native SQLite from Python, not a pure-Go port. The real risk isn't FTS5 correctness (no public evidence of contentless-table or tokenizer bugs) — it's operational behavior:

- Public modernc history shows recurring `SQLITE_BUSY` pain under concurrency
- WAL quirks like `wal_autocheckpoint` behaving badly
- Issues referenced: [modernc #232 `SQLITE_BUSY`](https://gitlab.com/cznic/sqlite/-/issues/232), [modernc #179 WAL autocheckpoint](https://gitlab.com/cznic/sqlite/-/issues/179)

The 10 ms/query target is handwaving unless benchmarked with BM25 ranking, WAL enabled, Go `database/sql` pool configured sanely, warm and cold paths. Multiple write-capable pooled connections will churn locks.

## Concern 2 — Prompt-caching invariant is overstated and probably false

Hermes policy: never rebuild system prompts, reload memories, or change toolsets mid-conversation; only compression touches that. The design summary already violates that:

- **`/resume`** — unless the exact prompt snapshot is persisted and reused, resuming regenerates prompt state and takes a cache miss.
- **Memory writes (Phase 2)** — bad tradeoff the design pretends doesn't exist: either the next turn keeps stale memory to preserve cache, or it reloads fresh memory and breaks cache. Pick one, document it.
- **Autonomous skill create (Phase 3)** — if skill creation causes a directory rescan, command refresh, tool registry change, or skill index injection mid-session, caching is broken outright. Hermes uses user-message injection for skill payloads precisely to avoid this.
- **Trajectory recorder (Phase 4)** — classic bug: if trajectories capture references to mutable prompt/message structures instead of immutable copies, background mutation poisons saved data.

## Concern 3 — Nudger goroutine design is sloppy and unsafe

"Fire-and-forget at end of turn" is not a concurrency model. Concrete failures:

- Two turns finishing close together spawn overlapping reviewers racing on the same session, same memory files, same API quota
- No root context cancellation + no tracked worker lifecycle = goroutine leaks on shutdown
- Reusing the parent `SessionStore` is only safe if the store is fully concurrency-safe end to end — `database/sql` being safe does not rescue wrapped maps, caches, buffers, or session-local state
- Concurrent `MEMORY.md` writes need serialization or file locking; otherwise last-writer-wins corruption is the happy path
- Same LLM client + same rate budget for foreground turns and background review: the nudger steals capacity, amplifies retries, creates self-inflicted latency spikes exactly when the user is active

## Concern 4 — Fuzzy patch algorithm is underdesigned and will misapply edits

"Normalize whitespace, find in normalized text, map back to original byte offsets" sounds neat and breaks immediately:

- `def foo` matching `def  foo` has no unique inverse mapping after whitespace collapse — boundary selection is ambiguous, off-by-one or over-capture inevitable
- If one patch rewrites whitespace, the next patch sees a different original/normalized correspondence — drift compounds
- Byte offsets instead of rune-aware spans: Unicode will bite — `§`, `✓`, emoji, ZWJ, decomposed forms make byte slicing unsafe
- Even in plain ASCII markdown, collapsing newlines can jump block boundaries and replace across code fences or list structure
- Don't claim "Hermes does this" without reading `tools/fuzzy_match.py`

## Concern 5 — Trajectory "full compatibility" claim is unearned

"ShareGPT JSONL compatible, but with extra metadata" is how people quietly make things not compatible:

- Unknown metadata keys are untested — unless we round-trip Ottie trajectories through Hermes's `batch_runner.py` + `trajectory_compressor.py`, we don't know whether they're ignored, stripped, or mishandled
- Even if they don't crash, `ottie_version / toolset / chain_ids / cost_usd` in trainable payloads = distribution pollution
- Wallet scrubbing is weak: hashing + redacting `0x` addresses does not protect users when ENS names, tx hashes, contract addresses, timestamps, token balances, and counterparties still make the graph re-identifiable
- Reasoning/tool formatting likely differs: Hermes has explicit `scratchpad -> <think>` handling; if Ottie emits tool calls or tool responses in a different shape, compressor silently degrades or breaks

## Highest-priority fixes before implementation

1. Stop claiming Hermes parity or ShareGPT compatibility until round-trip tests exist against Hermes prompt caching, trajectories, and batch/compression tools.
2. Freeze a per-session prompt/tool snapshot. No memory reloads, no skill rescans, no toolset changes until a new session or an explicit compression boundary.
3. Replace the nudger goroutine with a per-session serialized worker: bounded queue, context cancellation, shutdown join, file/write locking, and a separately rate-limited LLM path.
4. Kill the normalize-and-remap patch idea. Use anchored spans or structured diffs with rune-aware offsets and fail closed when a patch is ambiguous.
5. Benchmark `modernc.org/sqlite` against `mattn/go-sqlite3` on actual recall workload under WAL, busy timeouts, and concurrent turns. If modernc cannot survive cleanly, accept CGO.

## Sources codex actually verified

- [Hermes GitHub](https://github.com/NousResearch/hermes-agent)
- [Hermes extracted repo index](https://gitextract.com/NousResearch/hermes-agent)
- [Hermes AGENTS mirror (prompt-caching policy)](https://mdgrok.com/files/80091)
- [modernc SQLITE_BUSY issue #232](https://gitlab.com/cznic/sqlite/-/issues/232)
- [modernc WAL autocheckpoint issue #179](https://gitlab.com/cznic/sqlite/-/issues/179)
