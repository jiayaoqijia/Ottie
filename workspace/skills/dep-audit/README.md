# dep-audit — Dependency Audit Skill for OpenClaw

**Audit your project's dependencies for known vulnerabilities.** Supports npm, pip, Cargo, and Go. Zero API keys. Safe-by-default.

## Install

```bash
clawhub install dep-audit
```

## What It Does

Point it at a project directory. It detects lockfiles, runs the appropriate ecosystem audit tool, normalizes the output into a unified severity table, and presents actionable findings.

### Supported Ecosystems

| Ecosystem | Lockfiles Detected | Audit Tool |
|-----------|-------------------|------------|
| **Node/npm** | `package-lock.json` | `npm audit` |
| **Yarn** *(detection only)* | `yarn.lock` | `yarn audit` (manual) |
| **pnpm** *(detection only)* | `pnpm-lock.yaml` | `pnpm audit` (manual) |
| **Python** | `requirements.txt`, `Pipfile.lock`, `poetry.lock` | `pip-audit` |
| **Rust** | `Cargo.lock` | `cargo audit` |
| **Go** | `go.sum` | `govulncheck` |

### Features

- **Auto-detect** lockfiles in your project (searches up to 3 levels deep)
- **Unified report** across all ecosystems with severity breakdown
- **Strict path safety** — invalid target directories fail fast (no false "clean" reports)
- **SBOM generation** via `syft` (CycloneDX JSON)
- **Tree scan** — audit all projects under a directory
- **Fix suggestions** with explicit confirmation gate (never auto-modifies)

## OpenClaw Discord v2 Ready

Compatible with OpenClaw Discord channel behavior documented for v2026.2.14+:
- First message focuses on total findings and Critical/High vulnerabilities
- Full detail is sent on demand to avoid noisy channel spam
- Supports component-style follow-ups when available (`Show Full Report`, `Show Fix Commands`, `Generate SBOM`)

## Usage

Just ask:

> "Audit this project for vulnerabilities"
> "Check all my repos in ~/projects for known CVEs"
> "Are there any critical vulnerabilities I should fix right now?"
> "Generate an SBOM for this project"

## Requirements

- **bash** (Linux/macOS)
- **jq** (required for audit and aggregation scripts; detection works without it)
- Optional (recommended): `timeout`/`gtimeout` for per-tool time caps (Linux: `timeout`, macOS with coreutils: `gtimeout`)
- At least one audit tool for your ecosystem:
  - Node: `npm` (pre-installed with Node)
  - Python: `pip-audit` (`pip install pip-audit`)
  - Rust: `cargo-audit` (`cargo install cargo-audit`)
  - Go: `govulncheck` (`go install golang.org/x/vuln/cmd/govulncheck@latest`)

The skill will tell you exactly what's missing and how to install it.

## Safety

- **Report-only by default.** Never modifies files unless you explicitly ask and confirm.
- Audit tools read lockfiles — they don't execute your project code.
- Fix commands are printed as suggestions; the agent asks for confirmation before running any.
- No telemetry. No tracking. No phone-home. No API keys.

## More from Anvil AI

This skill is part of the **Anvil AI** open-source skill suite.

| Skill | What it does |
|-------|-------------|
| **[vibe-check](https://clawhub.com/skills/vibe-check)** | AI code quality + security review scorecard. |
| **[prom-query](https://clawhub.com/skills/prom-query)** | Prometheus metrics + alert triage from natural language. |
| **[dep-audit](https://clawhub.com/skills/dep-audit)** | This skill — unified dependency vulnerability auditing |
| **[rug-checker](https://clawhub.com/skills/rug-checker)** | Solana token rug-pull risk analysis |


---

Built by **[Anvil AI](https://anvil-ai.io)**.


## License

MIT
