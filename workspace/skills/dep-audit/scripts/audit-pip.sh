#!/usr/bin/env bash
# audit-pip.sh — Run pip-audit and output normalized JSON
# Usage: bash audit-pip.sh <directory>
# Output: Normalized JSON to stdout

set -euo pipefail

SCAN_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
INPUT_DIR="${1:-.}"
DIR=""

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

emit_error() {
  local msg="$1"
  local err_dir="${2:-$INPUT_DIR}"
  printf '{"error":"%s","ecosystem":"python","directory":"%s"}\n' \
    "$(json_escape "$msg")" "$(json_escape "$err_dir")" >&2
}

# --- Helper: run a command with a timeout when supported ---
run_timeout() {
  local secs="$1"; shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$secs" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$secs" "$@"
  else
    "$@"
  fi
}

if ! DIR="$(cd "$INPUT_DIR" 2>/dev/null && pwd -P)"; then
  emit_error "Directory not found or not accessible: $INPUT_DIR" "$INPUT_DIR"
  exit 1
fi

# Check required binaries
if ! command -v pip-audit >/dev/null 2>&1; then
  emit_error "pip-audit not found. Install with: pip install pip-audit"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  emit_error "jq not found — cannot parse pip-audit output"
  exit 1
fi

# Detect lockfile type and run pip-audit
RAW=""
AUDIT_RC=0
if [ -f "$DIR/requirements.txt" ]; then
  if RAW=$(run_timeout 30 pip-audit -r "$DIR/requirements.txt" --format json --output - 2>/dev/null); then
    AUDIT_RC=0
  else
    AUDIT_RC=$?
  fi
elif [ -f "$DIR/Pipfile.lock" ] || [ -f "$DIR/poetry.lock" ]; then
  if RAW=$(cd "$DIR" && run_timeout 30 pip-audit --format json --output - 2>/dev/null); then
    AUDIT_RC=0
  else
    AUDIT_RC=$?
  fi
else
  emit_error "No Python lockfile found (requirements.txt, Pipfile.lock, poetry.lock)"
  exit 1
fi

if [ "$AUDIT_RC" -eq 124 ]; then
  emit_error "pip-audit timed out after 30s"
  exit 1
fi

if [ -z "$RAW" ]; then
  emit_error "pip-audit returned no output"
  exit 1
fi

if ! echo "$RAW" | jq -e 'type == "object" and has("dependencies")' >/dev/null 2>&1; then
  emit_error "pip-audit returned unexpected JSON format"
  exit 1
fi

# pip-audit JSON format: { "dependencies": [ ... ] }
# Each entry: { "name": "...", "version": "...", "vulns": [ { "id": "...", "fix_versions": [...], "description": "..." } ] }
jq -r --arg dir "$DIR" --arg time "$SCAN_TIME" '

  # pip-audit does not include severity data natively.
  # Use "unknown" rather than a misleading default.
  # Future improvement: optionally enrich from OSV for CVSS-backed severity.
  def severity_from_id:
    "unknown";

  # Flatten: one entry per vuln per package
  [
    (.dependencies // [])[] |
    select((.vulns // []) | length > 0) |
    .name as $pkg | .version as $ver |
    .vulns[] |
    {
      id: (.id // "unknown"),
      package: $pkg,
      installed_version: ($ver // "unknown"),
      severity: ((.id // "unknown") | severity_from_id),
      title: ((.description // "See advisory for details") | split("\n")[0] | .[0:200]),
      url: ("https://osv.dev/vulnerability/" + (.id // "unknown")),
      fix_available: ((.fix_versions // []) | length > 0),
      fix_command: (if ((.fix_versions // []) | length > 0)
                    then "pip install " + $pkg + ">=" + (.fix_versions[0] // "latest")
                    else "No fix version available"
                    end),
      patched_version: ((.fix_versions // []) | join(", "))
    }
  ] as $entries |

  {
    ecosystem: "python",
    directory: $dir,
    scan_time: $time,
    summary: {
      critical: ([$entries[] | select(.severity == "critical")] | length),
      high: ([$entries[] | select(.severity == "high")] | length),
      moderate: ([$entries[] | select(.severity == "moderate")] | length),
      low: ([$entries[] | select(.severity == "low")] | length),
      info: ([$entries[] | select(.severity == "info")] | length),
      unknown: ([$entries[] | select(.severity == "unknown")] | length),
      total: ($entries | length)
    },
    vulnerabilities: $entries
  }
' <<< "$RAW"
