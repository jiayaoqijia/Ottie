#!/usr/bin/env bash
# audit-cargo.sh — Run cargo audit and output normalized JSON
# Usage: bash audit-cargo.sh <directory>
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
  printf '{"error":"%s","ecosystem":"rust","directory":"%s"}\n' \
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

if ! command -v jq >/dev/null 2>&1; then
  emit_error "jq not found"
  exit 1
fi

if ! command -v cargo >/dev/null 2>&1 || ! cargo audit --help >/dev/null 2>&1; then
  emit_error "cargo-audit not found. Install with: cargo install cargo-audit"
  exit 1
fi

if [ ! -f "$DIR/Cargo.lock" ]; then
  emit_error "Cargo.lock not found in target directory"
  exit 1
fi

RAW=""
AUDIT_RC=0
if RAW=$(cd "$DIR" && run_timeout 30 cargo audit --json 2>/dev/null); then
  AUDIT_RC=0
else
  AUDIT_RC=$?
fi

if [ "$AUDIT_RC" -eq 124 ]; then
  emit_error "cargo audit timed out after 30s"
  exit 1
fi

if [ -z "$RAW" ]; then
  emit_error "cargo audit returned no output"
  exit 1
fi

if ! echo "$RAW" | jq -e 'type == "object"' >/dev/null 2>&1; then
  emit_error "cargo audit returned unexpected JSON format"
  exit 1
fi

jq -r --arg dir "$DIR" --arg time "$SCAN_TIME" '
  # cargo audit JSON: .vulnerabilities.list[] has .advisory and .versions
  [
    (.vulnerabilities.list // [])[] |
    select(.advisory.kind == "vulnerability" or .advisory.kind == null) |
    {
      id: (.advisory.id // "unknown"),
      package: (.advisory.package // "unknown"),
      installed_version: (.package.version // "unknown"),
      severity: (
        # cargo-audit usually provides CVSS vector but not numeric base score.
        # Keep conservative mapping until explicit numeric scoring is added.
        if (.advisory.informational // null) != null then "low"
        elif (.advisory.cvss // null) != null then "moderate"
        else "moderate"
        end
      ),
      title: (.advisory.title // "See advisory"),
      url: (.advisory.url // ("https://rustsec.org/advisories/" + (.advisory.id // "unknown") + ".html")),
      fix_available: ((.versions.patched // []) | length > 0),
      fix_command: (
        if ((.versions.patched // []) | length > 0)
        then "Update " + (.advisory.package // "package") + " to " + (.versions.patched[0] // "latest")
        else "No patched version available"
        end
      ),
      patched_version: ((.versions.patched // []) | join(", "))
    }
  ] as $entries |

  {
    ecosystem: "rust",
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
