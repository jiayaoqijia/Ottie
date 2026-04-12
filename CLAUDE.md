# CLAUDE.md — Ottie Development Guide

## Commit Rules

- NEVER add `Co-Authored-By` lines mentioning Claude, Anthropic, or any AI assistant
- NEVER add `Co-Authored-By` trailers of any kind unless the user explicitly requests it
- Commit messages should be concise and describe WHAT changed and WHY
- Do not mention AI tools, agents, or assistants in commit messages

## What is Ottie?

A self-evolving AI agent runtime in Go. Every side-effecting action is auditable and crash-recoverable. One binary, 28 skills, 13+ channels.

## Build & Test

```bash
make build                    # builds to build/ottie-linux-amd64
go test ./pkg/... -short      # unit tests (44 packages)
ALTLLM_API_KEY=... go test ./pkg/providers/ -run "TestLive" -v  # live integration tests
ALTLLM_API_KEY=... go test ./cmd/ottie/ -run "TestE2E" -v       # CLI E2E tests
```

## Architecture

### Core packages (in dependency order)

| Package | Purpose | Key files |
|-|-|-|
| `pkg/principal` | Compile-time authorization via phantom-typed generics | `principal.go`, `typed_tool.go` |
| `pkg/actionlog` | Write-ahead action ledger (Prepare/Commit/Abort) | `actionlog.go` |
| `pkg/execmanifest` | Per-turn execution manifest for replay | `execmanifest.go` |
| `pkg/acs` | ACS bundle coordinator (principal + actionlog + manifest) | `acs.go`, `dispatch.go` |
| `pkg/providers` | LLM providers + error classifier + fallback chain | `error_classifier.go`, `fallback.go`, `factory.go` |
| `pkg/tools` | Tool registry + effect classification + file/shell/web tools | `registry.go`, `base.go`, `shell.go`, `filesystem.go`, `web.go` |
| `pkg/skills` | Self-evolving skill lifecycle (load, install, scan) | `loader.go`, `installer.go`, `scan.go` |
| `pkg/agent` | Agent loop orchestrating everything | `loop.go` (2500 LOC — the main loop) |
| `pkg/channels` | 13+ chat channel adapters | `manager.go`, `telegram/`, `discord/`, etc. |
| `pkg/routing` | Smart model routing (light vs primary) | `router.go` |
| `pkg/config` | Configuration loading, validation, migration | `config.go` |

### Key design decisions

- **ACS is optional**: `al.acs` is nil when disabled; every hook is a nil-check
- **Fail-open ACS**: if Prepare fails, tool still runs (observability layer, not a gate)
- **Single-writer SQLite**: actionlog and execmanifest each use a writer goroutine
- **Capability ladder**: read_only < writes_local < writes_state < writes_chain < writes_wallet

## Design Documents

### Architecture & Design
- [`docs/design/provider-refactoring.md`](docs/design/provider-refactoring.md) — Provider architecture redesign
- [`docs/design/provider-refactoring-tests.md`](docs/design/provider-refactoring-tests.md) — Provider test plan
- [`docs/agent-refactor/README.md`](docs/agent-refactor/README.md) — Agent refactoring notes
- [`docs/multi-agent.md`](docs/multi-agent.md) — Multi-agent swarm design

### Developer Guides
- [`docs/dev/adding-tools.md`](docs/dev/adding-tools.md) — How to add new tools
- [`docs/dev/adding-skills.md`](docs/dev/adding-skills.md) — How to add new skills
- [`docs/dev/acs-setup.md`](docs/dev/acs-setup.md) — ACS (Audit/Crash/Safety) setup guide
- [`docs/tools_configuration.md`](docs/tools_configuration.md) — Tool configuration reference
- [`docs/troubleshooting.md`](docs/troubleshooting.md) — Troubleshooting guide
- [`docs/debug.md`](docs/debug.md) — Debug mode guide

### Research & Reviews
- [`research/notes/ottie-test-plan.md`](research/notes/ottie-test-plan.md) — Comprehensive test plan (320 items)
- [`research/notes/ottie-test-plan-review.md`](research/notes/ottie-test-plan-review.md) — CC+Codex dual-agent review of test plan
- [`research/notes/ottie-self-review-findings.md`](research/notes/ottie-self-review-findings.md) — Ottie self-review bug findings
- [`research/notes/ottie-design-history-R1-R6.md`](research/notes/ottie-design-history-R1-R6.md) — Design history rounds 1-6
- [`research/notes/ottie-learning-loop-design.md`](research/notes/ottie-learning-loop-design.md) — Self-evolving skills design
- [`research/notes/ottie-positioning.md`](research/notes/ottie-positioning.md) — Competitive positioning

### Channel Setup Guides
- [`docs/channels/telegram/`](docs/channels/telegram/) — Telegram bot setup
- [`docs/channels/discord/`](docs/channels/discord/) — Discord bot setup
- [`docs/channels/slack/`](docs/channels/slack/) — Slack bot setup
- [`docs/channels/matrix/`](docs/channels/matrix/) — Matrix bot setup
- [`docs/channels/feishu/`](docs/channels/feishu/) — Feishu (Lark) bot setup
- (and more: qq, dingtalk, line, wecom, onebot)

### Migration
- [`docs/migration/model-list-migration.md`](docs/migration/model-list-migration.md) — Model list migration guide
- [`docs/ANTIGRAVITY_AUTH.md`](docs/ANTIGRAVITY_AUTH.md) — Google Antigravity auth setup
- [`docs/ANTIGRAVITY_USAGE.md`](docs/ANTIGRAVITY_USAGE.md) — Antigravity usage guide

## Code Conventions

- Tests use `testify/assert` + `testify/require` in skills; raw `testing` elsewhere
- Integration tests gated by `testing.Short()` + env vars (e.g., `ALTLLM_API_KEY`)
- SQLite stores use `WriteQueueDepth: 0` in tests for synchronous writes
- Tool effect classes: `EffectReadOnly`, `EffectWritesLocal`, `EffectWritesState`, `EffectWritesChain`, `EffectWritesWallet`
- Error classifier returns `FailoverError` with recovery hints; never returns nil for non-nil input
- Config loaded from `$OTTIE_CONFIG` or `~/.ottie/config.json`

## Known Limitations (documented, not bugs)

- `ToolSchemaHash` is a stand-in (set to `PromptHash`) — real hash not yet threaded in
- `hashMessagesForACS` uses FNV-1a over first 256 bytes — weak hash, sufficient for attribution
- ClawHub skill downloads are rate-limited (HTTP 429 on download endpoint)
- Web backend does not proxy `/api/ottie/token` to gateway for WebSocket chat
- `customAllowPatterns` in shell tool bypasses the entire denylist (by design, but risky)
