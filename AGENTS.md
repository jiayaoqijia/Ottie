# AGENTS.md — Instructions for AI Agents Working on Ottie

## Commit Rules (MANDATORY)

- NEVER add `Co-Authored-By` trailers mentioning any AI assistant
- NEVER reference Claude, Copilot, Codex, or any AI tool in commit messages
- Keep commit messages focused on the change itself

## Repository Overview

Ottie is a Go-based AI agent runtime with auditable replay, crash-proof actions, and compile-time authorization. See [CLAUDE.md](CLAUDE.md) for architecture details.

## Quick Start for Agents

```bash
make build                         # Build binary
go test ./pkg/... -short           # Run all unit tests
source .env                        # Load ALTLLM credentials (if present)
go test ./pkg/providers/ -run "TestLive" -v  # Live integration tests
```

## Where Things Are

| What | Where |
|-|-|
| Main agent loop | `pkg/agent/loop.go` |
| ACS (audit/crash/safety) | `pkg/acs/`, `pkg/actionlog/`, `pkg/execmanifest/` |
| Authorization system | `pkg/principal/` |
| LLM providers | `pkg/providers/` |
| Tool implementations | `pkg/tools/` |
| Skill system | `pkg/skills/` |
| Channel adapters | `pkg/channels/` (13 subdirectories) |
| CLI commands | `cmd/ottie/internal/` |
| TUI launcher | `cmd/ottie-launcher-tui/` |
| Web UI | `web/frontend/` (React) + `web/backend/` (Go) |
| Config | `pkg/config/config.go`, example at `config/config.example.json` |
| Design docs | `docs/design/`, `docs/dev/`, `research/notes/` |
| Test plan | `research/notes/ottie-test-plan.md` (320 items) |
| Self-review findings | `research/notes/ottie-self-review-findings.md` |

## Testing Guidelines

- Unit tests: `go test ./pkg/<package>/ -v`
- Integration tests need `ALTLLM_API_KEY` env var and skip with `-short`
- CLI E2E tests: `go test ./cmd/ottie/ -run TestE2E -v` (needs `ALTLLM_API_KEY` + built binary)
- Use `WriteQueueDepth: 0` in test ACS bundles for synchronous writes
- Use `t.TempDir()` for all file-based test isolation
- Race detector: `go test ./pkg/<package>/ -race`

## Key Patterns to Follow

### Adding a new tool
See `docs/dev/adding-tools.md`. Implement the `Tool` interface. If side-effecting, implement `EffectClassifier` to declare the blast-radius class.

### Adding a new skill
See `docs/dev/adding-skills.md`. Create `workspace/skills/<name>/SKILL.md` with YAML frontmatter (`name`, `description`).

### Adding a new channel
Follow the pattern in `pkg/channels/telegram/`. Implement the `Channel` interface. Register in `pkg/channels/manager.go`.

### Error handling in providers
Use `ClassifyError()` — it returns `*FailoverError` with recovery hints. The fallback chain uses `ShouldFallback()`, `ShouldCompress()`, `ShouldRotateCredential()` to decide next action.

## Do NOT

- Add `Co-Authored-By` or any AI attribution in git commits
- Add temperature to ALTLLM API calls (returns 400)
- Use `http.MaxBytesReader` with nil ResponseWriter (use `io.LimitReader` instead)
- Mutate `websocket.DefaultDialer` directly (copy it first)
- Include `response_url` or similar signed credentials in log metadata
- Log raw tool args without `RedactArgs()` — use `tools.RedactArgs(args)`
- Rely on `customAllowPatterns` for security — it bypasses the entire denylist
