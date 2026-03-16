# Changelog

## 0.1.3 (2026-02-16)

### Added
- Discord v2 delivery guidance in `SKILL.md` for OpenClaw v2026.2.14+:
  - Compact first response with Critical/High focus
  - Component-style quick actions for follow-up workflows
  - Fallback numbered actions when components are unavailable

### Changed
- README: added "OpenClaw Discord v2 Ready" compatibility section.
- `SKILL.md` metadata tags now include `discord` and `discord-v2`.

## 0.1.2 (2026-02-15)

### Fixed
- **[HIGH]** `audit-npm.sh`: fixed jq compile error when parsing `fixAvailable`
- **[MEDIUM]** Cross-platform timeouts: added `timeout`/`gtimeout` fallback helpers (runs without time caps when unavailable)
- **[LOW]** Added MIT `LICENSE` file
- **[LOW]** Docs: clarified that per-tool time caps require `timeout`/`gtimeout` (Linux/macOS coreutils)

## 0.1.1 (2026-02-14)

### Fixed
- **[HIGH]** `audit-cargo.sh`: CVSS parser was extracting spec version (3.1) instead of actual score — now uses `.advisory.informational` field for honest severity classification
- **[HIGH]** `audit-go.sh`: jq pipeline lost original input after `$osv_db` binding — `.finding` lookup iterated wrong data, causing 0 vulns reported always. Fixed by saving original input as `$all`
- **[HIGH]** Removed false "jq fallback to grep/awk" claim from SKILL.md, README.md, SPEC.md, SECURITY.md — jq is now documented as a hard requirement for audit/aggregation
- **[MEDIUM]** Fixed JSON injection via unescaped `$DIR` in all heredoc fallbacks — now uses `jq --arg` or `json_escape` helper for safe interpolation
- **[MEDIUM]** `audit-pip.sh`: severity was hardcoded to "moderate" — changed to "unknown" with documented limitation; added `unknown` severity to summary counts and aggregate report
- **[MEDIUM]** `detect.sh`: `yarn.lock` and `pnpm-lock.yaml` no longer falsely classified as "npm" — now reported as `yarn` and `pnpm` ecosystems with documented audit limitation
- **[MEDIUM]** SKILL.md: `write: false` permission conflicted with `sbom.sh` — changed to `write: on-request`
- **[MEDIUM]** `aggregate.sh`: no longer hangs on stdin when called with no args — prints usage and exits
- **[LOW]** Removed dead `TEMPLATE_DIR` variable and unused `templates/report.md.tmpl` file
- **[LOW]** SKILL.md Step 3: added stdout/stderr guidance for aggregate.sh output
- **[LOW]** TESTING.md Test 3: fixed placeholder `/path/to/scripts/` → `scripts/`
- **[LOW]** TESTING.md Test 5: aggregate test now captures and validates both JSON and Markdown outputs
- **[LOW]** SECURITY.md: added note about sbom.sh symlink-resolved write path
- **[INFO]** SECURITY.md: corrected "all paths are quoted" claim to reference `jq --arg` usage
- Updated SPEC.md file tree, error handling table, and pip-audit severity documentation

## 0.1.0 (2026-02-14)

### Added
- Initial release
- Detection script: auto-discover lockfiles (npm, pip, Cargo, Go) and available tools
- npm audit script with normalized JSON output
- pip-audit script with normalized JSON output
- cargo audit script with normalized JSON output
- govulncheck script with normalized JSON output
- Aggregation script: merge per-ecosystem results into unified report
- SBOM generation via syft
- Markdown report template
- Safety-first design: report-only default, fix commands require explicit confirmation
- Graceful degradation: missing tools are reported with install instructions, not crashes
