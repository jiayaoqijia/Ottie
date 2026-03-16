# SECURITY.md — dep-audit Security Model

## What Data This Skill Touches

| Data | Access | Notes |
|------|--------|-------|
| Lockfiles (`package-lock.json`, `requirements.txt`, etc.) | **Read-only** | Parsed to identify packages and versions |
| Advisory databases (OSV, GitHub Advisory DB, RustSec) | **Read-only, network** | Fetched by upstream audit tools, not by this skill directly |
| SBOM output files | **Write** (only when explicitly requested) | Written to project directory as `sbom.cdx.json` |
| Project source code | **Never accessed** | Audit tools read lockfiles only, not source |

## Network Behavior

This skill does **not** make any network requests itself. The underlying audit tools (`npm audit`, `pip-audit`, `cargo audit`, `govulncheck`) each contact their respective advisory databases:

- npm → `registry.npmjs.org` (npm Advisory DB)
- pip-audit → `osv.dev` (Open Source Vulnerability DB)
- cargo audit → `github.com/rustsec/advisory-db`
- govulncheck → `vuln.go.dev`

No data is sent to CacheForge or any third party beyond what these standard tools do.

## Abuse Cases & Mitigations

| Abuse Case | Risk | Mitigation |
|------------|------|------------|
| User asks to "fix everything" → unreviewed package upgrades | **Medium** | Fix commands are never auto-executed. Agent must print commands and get explicit "yes" before running any. |
| Malicious lockfile tricks parser into code execution | **Low** | Scripts use `jq` for JSON parsing, not `eval`. No lockfile content is ever executed. |
| Scanning untrusted directories exposes file paths | **Low** | Paths appear only in the report shown to the user. No external transmission. |
| SBOM written to symlink-resolved path | **Low** | `sbom.sh` resolves symlinks via `cd && pwd`. A symlinked dir could cause writes to the target. Only runs when user explicitly requests SBOM generation. |
| Denial of service via huge monorepo (thousands of lockfiles) | **Low** | Per-tool time caps are enforced when `timeout`/`gtimeout` is available (Linux/macOS coreutils). Tree scans limited to `maxdepth 3`. Users can interrupt long-running scans. |
| Shell injection via directory names with special characters | **Low** | All paths are quoted in scripts. Audit scripts use `jq --arg` for safe JSON interpolation of paths and variables. |

## Input Validation

- **Directory paths:** Validated via `cd "$DIR" && pwd -P` with explicit error handling. Invalid/non-existent paths return hard errors (no fallback to `$HOME`, no false "clean" reports).
- **Lockfile detection:** Only known filenames are matched (`package-lock.json`, etc.) — no glob expansion of user input.
- **JSON parsing:** All output parsing uses `jq` with explicit field access — no `eval`, no `source`, no backtick execution.

## Rate Limiting

Not applicable — this skill runs locally on user request. The upstream advisory databases have their own rate limits (npm: generous; pip-audit/OSV: generous; RustSec: git clone; Go vulncheck: generous).

## What This Skill Does NOT Do

- Does not scan container images or running services
- Does not detect zero-day vulnerabilities
- Does not phone home or send telemetry
- Does not modify any files without explicit user confirmation
- Does not execute project code (only reads lockfiles)
- Does not cache or store vulnerability data between runs
