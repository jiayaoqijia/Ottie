# SPEC.md — `dep-audit` (Unified Dependency Audit & CVE Alert Skill)

**Run ID:** scan-0214  
**Date:** 2026-02-14  
**Status:** Approved by ForgeJudge — ready for Builder  

---

## 1. Skill Identity

| Field | Value |
|-------|-------|
| **Name** | dep-audit |
| **Display Name** | Dependency Audit |
| **Version** | 0.1.0 |
| **Description** | Audit your project's dependencies for known vulnerabilities. Supports npm, pip, Cargo, and Go. Zero API keys. Safe-by-default (report-only). |
| **Author** | Anvil AI |
| **Tags** | `security`, `audit`, `dependencies`, `cve`, `supply-chain` |
| **License** | MIT |

---

## 2. What It Does

The agent detects lockfiles in the current directory (or a user-specified tree), runs the appropriate ecosystem audit tool, normalizes the output into a unified severity table, and presents actionable findings. Optionally generates an SBOM.

**Modes:**

| Mode | Trigger | Behavior |
|------|---------|----------|
| **Single-dir audit** | "Audit this project" | Detect lockfiles in `.`, run audits, report |
| **Tree audit** | "Audit all projects in ~/code" | Walk directory tree, find lockfiles, audit each, aggregate |
| **SBOM generation** | "Generate an SBOM" | Run `syft` or `cdxgen`, output CycloneDX JSON |
| **Fix suggestions** | "Fix the critical vulnerabilities" | Print fix commands, **require explicit user confirmation** before executing any |

---

## 3. Supported Ecosystems & Tools

Each ecosystem maps to one CLI tool. The skill auto-detects which are available.

| Ecosystem | Lockfile(s) Detected | Audit CLI | JSON Flag | Install If Missing |
|-----------|---------------------|-----------|-----------|-------------------|
| **Node/npm** | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml` | `npm audit` | `--json` | Pre-installed with Node |
| **Python** | `requirements.txt`, `Pipfile.lock`, `poetry.lock` | `pip-audit` | `--format json --output -` | `pip install pip-audit` |
| **Rust** | `Cargo.lock` | `cargo audit` | `--json` | `cargo install cargo-audit` |
| **Go** | `go.sum` | `govulncheck` | `-json` | `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| **SBOM** | any of the above | `syft` | `-o cyclonedx-json` | `curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh \| sh` |

**Docs:**
- npm audit: https://docs.npmjs.com/cli/commands/npm-audit
- pip-audit: https://github.com/pypa/pip-audit
- cargo audit: https://github.com/rustsec/rustsec/tree/main/cargo-audit
- govulncheck: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
- syft: https://github.com/anchore/syft

---

## 4. File Structure

```
dep-audit/
├── SKILL.md              # Skill definition (agent-facing)
├── scripts/
│   ├── detect.sh         # Detect lockfiles + available tools
│   ├── audit-npm.sh      # Run npm audit, output normalized JSON
│   ├── audit-pip.sh      # Run pip-audit, output normalized JSON
│   ├── audit-cargo.sh    # Run cargo audit, output normalized JSON
│   ├── audit-go.sh       # Run govulncheck, output normalized JSON
│   ├── aggregate.sh      # Merge per-ecosystem JSONs → unified report
│   └── sbom.sh           # Generate SBOM via syft
├── SPEC.md               # This file
├── README.md             # ClawHub listing (user-facing)
├── SECURITY.md           # Security model and threat analysis
├── TESTING.md            # Test plan
└── CHANGELOG.md          # Version history
```

---

## 5. SKILL.md Spec

```yaml
---
name: dep-audit
description: >
  Audit project dependencies for known vulnerabilities (CVEs).
  Supports npm, pip, Cargo, and Go. Zero API keys required.
  Safe-by-default: report-only mode, fix commands require confirmation.
version: 0.1.0
author: Anvil AI
tags: [security, audit, dependencies, cve, supply-chain]
---
```

### Activation Triggers

The skill activates when the user:
- Says "audit", "vulnerability", "CVE", "dependency check", "supply chain", "security scan"
- Asks to check dependencies, lockfiles, or packages for issues
- Asks to generate an SBOM

### Example Prompts (include all in SKILL.md)

1. `"Audit this project for vulnerabilities"`
2. `"Check all my repos in ~/projects for known CVEs"`
3. `"Are there any critical vulnerabilities I should fix right now?"`
4. `"Generate an SBOM for this project"`
5. `"What dependencies need updating in this project?"`
6. `"Audit only the Python dependencies"`

### Permissions

```yaml
permissions:
  exec: true          # Required to run audit CLIs
  read: true          # Read lockfiles
  write: on-request   # SBOM generation writes sbom.cdx.json when user asks
  network: true       # Tools fetch advisory DBs
```

### Safety Section (must appear in SKILL.md)

```markdown
## Safety

- **Default mode is report-only.** The skill never modifies files unless you explicitly ask for a fix and confirm.
- Audit tools read lockfiles — they do not execute project code.
- Fix commands (`npm audit fix`, `pip install --upgrade`) are printed as suggestions. The agent will ask for confirmation before running them.
- This skill checks known advisory databases (OSV, GitHub Advisory DB, RustSec). It does not detect zero-days or runtime vulnerabilities.
```

---

## 6. Script Specifications

### 6.1 `scripts/detect.sh`

**Purpose:** Discover lockfiles and available audit tools in a given directory.

**Input:** `$1` = target directory (default `.`)  
**Output:** JSON to stdout

```json
{
  "directory": "/home/user/project",
  "lockfiles": [
    { "ecosystem": "npm", "path": "package-lock.json" },
    { "ecosystem": "python", "path": "requirements.txt" }
  ],
  "tools": {
    "npm": { "available": true, "version": "10.9.2" },
    "pip-audit": { "available": true, "version": "2.7.3" },
    "cargo-audit": { "available": false, "install": "cargo install cargo-audit" },
    "govulncheck": { "available": false, "install": "go install golang.org/x/vuln/cmd/govulncheck@latest" },
    "syft": { "available": false, "install": "curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh" }
  }
}
```

**Logic:**
1. `find "$dir" -maxdepth 3` for each known lockfile name
2. `command -v` for each tool; if found, capture `--version`
3. Emit JSON (use `jq` if available, else raw `printf`)

### 6.2 `scripts/audit-npm.sh`

**Purpose:** Run `npm audit --json` against a directory containing `package-lock.json`.

**Input:** `$1` = directory containing the lockfile  
**Output:** Normalized JSON to stdout

```json
{
  "ecosystem": "npm",
  "directory": "/home/user/project",
  "scan_time": "2026-02-14T22:30:00Z",
  "summary": { "critical": 1, "high": 3, "moderate": 5, "low": 2, "info": 0, "total": 11 },
  "vulnerabilities": [
    {
      "id": "GHSA-xxxx-yyyy-zzzz",
      "package": "lodash",
      "installed_version": "4.17.15",
      "severity": "critical",
      "title": "Prototype Pollution in lodash",
      "url": "https://github.com/advisories/GHSA-xxxx-yyyy-zzzz",
      "fix_available": true,
      "fix_command": "npm audit fix",
      "patched_version": ">=4.17.21"
    }
  ]
}
```

**Logic:**
1. `cd "$1"` and run `npm audit --json 2>/dev/null`
2. Parse with `jq`: extract `.vulnerabilities | to_entries[]` 
3. Map npm severity levels → normalized `critical|high|moderate|low|info`
4. Compute summary counts
5. Exit 0 always (findings are data, not errors). Store raw exit code in `.exit_code`.

**Timeout:** 30 seconds (`timeout 30 npm audit --json`)

### 6.3 `scripts/audit-pip.sh`

**Purpose:** Run `pip-audit --format json` for Python projects.

**Input:** `$1` = directory  
**Output:** Same normalized JSON schema as npm (ecosystem: "python")

**Logic:**
1. Detect `requirements.txt` vs `Pipfile.lock` vs `poetry.lock`
2. For `requirements.txt`: `pip-audit -r "$1/requirements.txt" --format json --output - 2>/dev/null`
3. For `Pipfile.lock`/`poetry.lock`: `cd "$1" && pip-audit --format json --output - 2>/dev/null`
4. Map pip-audit fields → normalized schema (pip-audit uses `id`, `name`, `version`, `fix_versions`, `description`)
5. Severity mapping: pip-audit does not provide severity natively; severity is set to "unknown". Future: cross-reference OSV API for CVSS scores to map → critical(≥9)/high(≥7)/moderate(≥4)/low(<4)

**Timeout:** 30 seconds

### 6.4 `scripts/audit-cargo.sh`

**Purpose:** Run `cargo audit --json` for Rust projects.

**Input:** `$1` = directory containing `Cargo.lock`  
**Output:** Normalized JSON (ecosystem: "rust")

**Logic:**
1. `cd "$1" && cargo audit --json 2>/dev/null`
2. Parse `.vulnerabilities.list[]` with `jq`
3. Map RustSec advisory fields → normalized schema
4. RustSec provides `informational` vs `vulnerability` kind; only `vulnerability` entries are reported

**Timeout:** 30 seconds

### 6.5 `scripts/audit-go.sh`

**Purpose:** Run `govulncheck -json` for Go projects.

**Input:** `$1` = directory containing `go.sum`  
**Output:** Normalized JSON (ecosystem: "go")

**Logic:**
1. `cd "$1" && govulncheck -json ./... 2>/dev/null`
2. Parse JSON output — govulncheck emits newline-delimited JSON messages
3. Extract `finding` entries with `osv` IDs
4. Map to normalized schema; Go doesn't assign severity levels natively, so derive from CVSS in the OSV entry or default to "moderate"

**Timeout:** 60 seconds (Go analysis is slower)

### 6.6 `scripts/aggregate.sh`

**Purpose:** Merge multiple per-ecosystem JSON files into one unified report.

**Input:** One or more JSON files as arguments, OR `stdin` (newline-delimited JSON)  
**Output:** Unified JSON + Markdown report

```json
{
  "scan_time": "2026-02-14T22:30:00Z",
  "directories_scanned": 1,
  "ecosystems": ["npm", "python"],
  "summary": { "critical": 1, "high": 5, "moderate": 8, "low": 3, "total": 17 },
  "by_ecosystem": [ { "ecosystem": "npm", "summary": { ... }, "vulnerabilities": [...] } ],
  "top_findings": [ ... ]  
}
```

Also generates a Markdown report inline via jq — the agent presents this to the user.

### 6.7 `scripts/sbom.sh`

**Purpose:** Generate a CycloneDX SBOM for the project.

**Input:** `$1` = directory  
**Output:** CycloneDX JSON file at `$1/sbom.cdx.json`, summary to stdout

**Logic:**
1. Check `syft` available; if not, print install instructions and exit 1
2. `syft dir:"$1" -o cyclonedx-json > "$1/sbom.cdx.json"`
3. Print component count and file location to stdout

---

## 7. Agent Workflow (SKILL.md instruction to the agent)

The SKILL.md should instruct the agent to follow this sequence:

```
1. Run scripts/detect.sh <target_dir> to discover lockfiles and tools
2. For each ecosystem found:
   a. If tool is available → run scripts/audit-<ecosystem>.sh <dir>
   b. If tool is missing → note it; suggest install command; skip
3. Run scripts/aggregate.sh on all results
4. Present the Markdown report to the user
5. If user asks for fixes:
   a. List fix commands with package names and versions
   b. Ask for explicit confirmation before running ANY fix command
   c. Suggest "git stash" or "git checkout -b dep-audit-fixes" first
6. If user asks for SBOM:
   a. Run scripts/sbom.sh <dir>
   b. Report file location and component count
```

### Error Handling

- **Tool not found:** Print which tool is missing, provide install command, continue with available tools
- **Audit tool fails:** Capture stderr, report "audit failed for [ecosystem]: [error]", continue with others
- **Timeout:** Report "audit timed out for [ecosystem] (>30s), skipping", continue
- **No lockfiles found:** Report "No lockfiles found in <dir>. This skill works with npm, pip, Cargo, and Go projects."
- **jq not available:** Detection works without jq. Audit and aggregation scripts **require** jq and will exit with an error if it is not installed.

---

## 8. Unified Report Format (Markdown)

```markdown
# 🔒 Dependency Audit Report

**Scanned:** /home/user/project  
**Time:** 2026-02-14 22:30 UTC  
**Ecosystems:** npm, python  

## Summary

| Severity | Count |
|----------|-------|
| 🔴 Critical | 1 |
| 🟠 High | 3 |
| 🟡 Moderate | 5 |
| 🔵 Low | 2 |
| **Total** | **11** |

## Critical & High Findings

| Package | Version | Severity | CVE/Advisory | Fix |
|---------|---------|----------|-------------|-----|
| lodash | 4.17.15 | 🔴 Critical | GHSA-xxxx | `npm audit fix` → ≥4.17.21 |
| requests | 2.28.0 | 🟠 High | PYSEC-2023-xxx | `pip install requests>=2.31.0` |

## All Findings by Ecosystem

### npm (8 findings)
[detailed table]

### python (3 findings)
[detailed table]

---

⚠️ This report covers known vulnerabilities in public advisory databases (OSV, GitHub Advisory DB, RustSec). It does not detect zero-days or runtime security issues.
```

---

## 9. Test Plan

### Test 1: Detection (no audit tools needed)

```bash
# Create a test directory with a lockfile
mkdir -p /tmp/dep-audit-test && cd /tmp/dep-audit-test
echo '{}' > package-lock.json
echo 'requests==2.28.0' > requirements.txt

# Run detection
bash scripts/detect.sh /tmp/dep-audit-test

# Verify: JSON output lists both lockfiles, shows tool availability
```

**Pass criteria:** JSON includes both `npm` and `python` entries in `lockfiles[]`.

### Test 2: npm audit (real project)

```bash
# Clone a known-vulnerable project or create one
mkdir -p /tmp/npm-test && cd /tmp/npm-test
npm init -y
npm install lodash@4.17.15 --save  # Known vulnerable version

# Run audit
bash scripts/audit-npm.sh /tmp/npm-test

# Verify: JSON output contains vulnerabilities with severity levels
```

**Pass criteria:** At least one vulnerability found; JSON schema matches spec; summary counts are correct.

### Test 3: pip-audit (real project)

```bash
mkdir -p /tmp/pip-test && cd /tmp/pip-test
echo 'requests==2.25.0' > requirements.txt

bash scripts/audit-pip.sh /tmp/pip-test
```

**Pass criteria:** Findings include known CVEs for requests 2.25.0.

### Test 4: Aggregation

```bash
# Feed two ecosystem results into aggregate
bash scripts/aggregate.sh npm-results.json pip-results.json

# Verify: unified JSON has correct totals, Markdown report is generated
```

**Pass criteria:** `summary.total` equals sum of per-ecosystem totals. Markdown renders correctly.

### Test 5: Missing tools graceful degradation

```bash
# Rename cargo-audit to simulate absence
bash scripts/detect.sh /tmp/cargo-project
# audit-cargo.sh should report tool missing and exit cleanly
```

**Pass criteria:** No crash; user sees install suggestion; other ecosystems still audited.

### Test 6: Tree scan

```bash
# Create nested structure
mkdir -p /tmp/tree/{project-a,project-b,project-c}
echo '{}' > /tmp/tree/project-a/package-lock.json
echo 'flask==2.0.0' > /tmp/tree/project-b/requirements.txt

# Agent walks tree, finds 2 projects, audits both
```

**Pass criteria:** Report covers both project-a (npm) and project-b (python).

### Test 7: Fix confirmation gate

```
User: "Fix the critical vulnerabilities"
Agent: "I found these fix commands:
  1. cd /home/user/project && npm audit fix
  
  Shall I run them? I recommend creating a branch first:
    git checkout -b dep-audit-fixes
  
  Confirm? (yes/no)"
```

**Pass criteria:** Agent NEVER runs fix commands without explicit "yes" from user.

### Test 8: SBOM generation

```bash
bash scripts/sbom.sh /tmp/npm-test
# Verify: sbom.cdx.json exists, contains components array
```

**Pass criteria:** Valid CycloneDX JSON file produced.

---

## 10. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Empty directory (no lockfiles) | "No lockfiles found" message, list supported ecosystems |
| Lockfile present but 0 vulnerabilities | "✅ No known vulnerabilities found" |
| Very large monorepo (100+ lockfiles) | Scan with 4 concurrent audits, 30s timeout each, report partial results |
| No internet access | Report that advisory DB fetch failed, suggest checking connectivity |
| `jq` not installed | Detection works without jq; audit/aggregation require jq and exit with an error |
| Lockfile is malformed/corrupt | Report parse error for that ecosystem, continue with others |
| Mixed ecosystems in one directory | Run all detected auditors, aggregate results |

---

## 11. Build Priorities

### MVP (Day 1) — Ship This

1. `scripts/detect.sh` — lockfile + tool detection
2. `scripts/audit-npm.sh` — npm audit (largest user base)
3. `scripts/audit-pip.sh` — pip-audit (second largest)
4. `scripts/aggregate.sh` — unified report
5. `SKILL.md` — agent instructions + safety section + example prompts
6. `templates/report.md.tmpl` — Markdown report template

### Fast Follow (Day 2)

7. `scripts/audit-cargo.sh` — Rust support
8. `scripts/audit-go.sh` — Go support
9. `scripts/sbom.sh` — SBOM generation
10. Tree-scan support (multi-directory)
11. Fix suggestions with confirmation gate
12. `README.md` + `CHANGELOG.md` for ClawHub listing

---

## 12. Non-Goals (explicitly out of scope)

- **Runtime vulnerability scanning** (e.g., container image scanning) — different tool, different skill
- **License compliance auditing** — adjacent but separate concern
- **Automatic PR creation** — future feature, not MVP
- **Scheduled/cron scanning** — premium tier, not MVP
- **Custom advisory databases** — use public DBs only
- **Windows support** — Linux/macOS only for MVP (bash scripts)

---

## 13. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Stars at 7 days | ≥15 | ClawHub dashboard |
| Stars at 30 days | 80-120 | ClawHub dashboard |
| Install errors reported | <3 issues | GitHub issues |
| Time to first audit (new user) | <60 seconds | Manual test |
| Ecosystems working at launch | ≥2 (npm + pip) | Test suite |

---

*This spec is decision-complete. A Builder agent should be able to implement the full skill from this document alone.*
