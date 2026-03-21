# Ottie Agent Guide

This file is the repo-level agent handoff for Ottie. It supersedes the old `CLAUDE.md` as the canonical project guide.

## Identity

- Project: **Ottie** — self-evolving AI agent for Ethereum and crypto
- Repo/module: `github.com/jiayaoqijia/ottie`
- Primary binary: `ottie`
- Note: the checkout path may still be named `aintern`, but the codebase, module path, binary, docs, and branding are all `ottie`

## Architecture Snapshot

- `cmd/ottie/` contains the Cobra CLI entrypoint and user-facing commands such as `agent`, `gateway`, `skills`, `model`, `onboard`, and `status`
- `pkg/agent/` owns the runtime loop, context assembly, routing, memory, timeouts, and per-agent registry wiring
- `pkg/tools/` contains tool implementations and the shared `ToolRegistry`; shared tools are registered from `pkg/agent/loop.go`
- `pkg/skills/` implements skill loading, validation, registry search, and install flows
- `pkg/mcp/` manages Model Context Protocol servers and wraps their tools via `pkg/tools/mcp_tool.go`
- `pkg/session/` stores session history and archival data
- `pkg/swarm/board/` and `pkg/tools/swarm*.go` implement optional multi-agent coordination modes
- `pkg/channels/` contains channel adapters for Telegram, Discord, Slack, Matrix, QQ, WeCom, Feishu, IRC, and others
- `web/backend/` serves the launcher/web API; `web/frontend/` is the React/TypeScript frontend
- `workspace/skills/` is the live repo-local skill set; this checkout currently has **31** skills
- `cmd/ottie/internal/onboard/workspace/skills/` is the onboard template; it currently ships **22** builtin starter skills
- `workspace/AGENTS.md` is the minimal onboard intern-behavior file generated into user workspaces; it is separate from this repo guide

## Repo Behavior

- Use noreply Git identity before commit/push:
  - `git config user.name "jiayaoqijia"`
  - `git config user.email "jiayaoqijia@users.noreply.github.com"`
- When committing, set both author and committer to the noreply email
- Never add AI attribution in commit messages or commit trailers
- Keep commit messages short and action-oriented
- After non-trivial work, check whether the approach should become a reusable skill via `workspace/skills/self-evolve/SKILL.md`
- Ask before creating new skills

## Skills And MCP

- Skill resolution priority is `workspace` -> `global` -> `builtin`
- Repo-local skills live under `workspace/skills/<name>/SKILL.md`
- Skill metadata is validated in `pkg/skills/loader.go`
- Skill search/install flows live in `pkg/tools/skills_search.go`, `pkg/tools/skills_install.go`, and `cmd/ottie/internal/skills/`
- MCP server loading is opt-in through config and handled by `pkg/mcp/manager.go`
- Relative MCP `env_file` paths are resolved against the workspace path
- MCP tools are exposed as sanitized names prefixed with `mcp_`
- Shared tools are registered only when enabled in config, then filtered per agent via `ToolsAllow` and `ToolsDeny`
- `render_html` is only registered when a browser is actually present

## Build, Test, Lint

- `make build`
- `make build-all`
- `make test`
- `golangci-lint run ./...`
- Focused suites:
  - `go test ./pkg/skills/...`
  - `go test ./pkg/tools/...`
  - `go test ./pkg/session/...`
  - `go test ./web/backend/...`

## Config Notes

- Swarm mode is fully opt-in
- Mode A activates when `agents.list` defines subagents
- Mode B activates when `swarm.enabled` is true
- No `swarm` config means no project board, no Redis requirement, and no extra swarm tools
- `workspace/skills/` is the main repo-local skill surface; edit the onboard template only when you want new installs to inherit a change
- MCP servers are configured under `tools.mcp.servers` in config.json (set `tools.mcp.enabled: true`)
- Lido MCP server: `python3 mcp-servers/lido-mcp/server.py` (requires `fastmcp`, `aiohttp`)

## On-Chain Artifacts

Scripts and contracts for hackathon track submissions:

- `mcp-servers/lido-mcp/server.py` — 14-tool FastMCP server for Lido staking
- `scripts/uniswap-swap.js` — Uniswap Trading API swap on Sepolia
- `scripts/erc8004-register.js` — ERC-8004 agent identity registration via agent0-sdk
- `contracts/AgentTreasury.sol` — yield-only treasury with MockWstETH
- `scripts/deploy-treasury.js` — compile + deploy treasury to Sepolia
- `scripts/venice-demo.sh` — Venice AI zero-retention inference demo
- `scripts/autonomous-demo.sh` — 5-step autonomous agent execution
- `scripts/vault-monitor-demo.sh` — vault health monitoring with Telegram alerts
- `scripts/self-agent-register.js` — Self Protocol Agent ID registration
- `scripts/erc8183-job.js` — ERC-8183 job escrow on BSC Testnet
- `workspace/agent.json` — ERC-8004 agent manifest
- `demos/test-onchain.sh` — 37-test E2E suite across all 10 tracks

## Claude Migration Inventory

Recovered from `~/.claude` so the prior Claude environment can be reconstructed deliberately instead of rediscovered from session logs.

### Installed Claude plugins and skills

- `agentic-dev@local-skills`
  - provides the `agentic-dev` skill with proposal/apply/archive prompts
- `frontend-design@claude-plugins-official`
  - provides the `frontend-design` skill
- `railway@railway-skills`
  - provides Railway deployment skills:
  - `central-station`
  - `database`
  - `deploy`
  - `deployment`
  - `domain`
  - `environment`
  - `metrics`
  - `new`
  - `projects`
  - `railway-docs`
  - `service`
  - `status`
  - `templates`

### Other Claude plugins observed

- `ralph-loop@claude-plugins-official`
- `code-review@claude-plugins-official`
- `code-simplifier@claude-plugins-official`
- `figma@claude-plugins-official`
- `rust-analyzer-lsp@claude-plugins-official`

### Skills referenced in Claude history

- `ui-ux-pro-max-skill`
- Railway skills from `railwayapp/railway-skills`
- Anthropics `pptx` skill
- repeated requests for agent-team, math, and quant-oriented skills

### MCP servers explicitly added or requested in Claude history

- `vercel` via `https://mcp.vercel.com`
- project-scoped Vercel variant (`vercel-awesome-ai`)
- `Railway` via `npx @railway/mcp-server`
- `neon` via `https://mcp.neon.tech/mcp`
- `cloudflare` via `https://bindings.mcp.cloudflare.com/mcp`
- `upstash` via `npx -y @upstash/mcp-server`
- `open-webSearch` was requested for AltLLM work

### How to re-apply them in Ottie

- Skills should be imported intentionally into `workspace/skills/` or `~/.ottie/skills/` only if they fit Ottie's scope
- MCPs should be translated into Ottie config under `tools.mcp.servers`
- Do not assume Claude-home plugins automatically affect Ottie; Ottie only sees what is installed in its own skill roots or configured in its own MCP settings
