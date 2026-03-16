#!/usr/bin/env bash
# audit-npm.sh — Run npm audit and output normalized JSON
# Usage: bash audit-npm.sh <directory>
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
  printf '{"error":"%s","ecosystem":"npm","directory":"%s"}\n' \
    "$(json_escape "$msg")" "$(json_escape "$err_dir")" >&2
}

# --- Helper: run a command with a timeout when supported ---
# - Linux usually has `timeout` (coreutils)
# - macOS users may have `gtimeout` (coreutils)
# - If neither exists, run without a time limit (still safe; user can interrupt)
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

# Check npm available
if ! command -v npm >/dev/null 2>&1; then
  emit_error "npm not found"
  exit 1
fi

# Check if jq is available (required for all JSON output)
if ! command -v jq >/dev/null 2>&1; then
  emit_error "jq not found — cannot parse npm audit output"
  exit 1
fi

# Require npm lockfile for deterministic scanning
if [ ! -f "$DIR/package-lock.json" ]; then
  emit_error "package-lock.json not found in target directory"
  exit 1
fi

# Run npm audit with timeout
RAW=""
AUDIT_RC=0
if RAW=$(cd "$DIR" && run_timeout 30 npm audit --json 2>/dev/null); then
  AUDIT_RC=0
else
  AUDIT_RC=$?
fi

if [ "$AUDIT_RC" -eq 124 ]; then
  emit_error "npm audit timed out after 30s"
  exit 1
fi

if [ -z "$RAW" ]; then
  # npm audit returned nothing — emit explicit failure instead of false "clean"
  emit_error "npm audit returned no output"
  exit 1
fi

# npm returns non-zero when vulnerabilities are found; that's expected.
# Treat explicit npm audit runtime errors as failures.
if echo "$RAW" | jq -e '.error? != null' >/dev/null 2>&1; then
  npm_err=$(echo "$RAW" | jq -r '.error.summary // .error.code // "npm audit failed"')
  emit_error "npm audit failed: $npm_err"
  exit 1
fi

# Parse npm audit JSON output
# npm audit --json v7+ format has .vulnerabilities as object keyed by package.
jq -r --arg dir "$DIR" --arg time "$SCAN_TIME" '
  (.vulnerabilities // {}) as $vulns |

  # Flatten vulnerability object advisories from .via[]
  [
    $vulns | to_entries[] as $pkg |
    ($pkg.value.via // [])[]? |
    select(type == "object") |
    {
      id: ((.url // .source // .name // "unknown") | tostring |
        (if test("^https://github.com/advisories/") then split("/")[-1] else . end)),
      package: $pkg.key,
      installed_version: ($pkg.value.range // "unknown"),
      severity: (.severity // $pkg.value.severity // "unknown"),
      title: ((.title // .name // "Unknown vulnerability") | tostring),
      url: (.url // ""),
      fix_available: (
        if ($pkg.value.fixAvailable | type) == "boolean" then $pkg.value.fixAvailable
        elif ($pkg.value.fixAvailable | type) == "object" then true
        else false
        end
      ),
      fix_command: (
        if ($pkg.value.fixAvailable | type) == "object"
          and ($pkg.value.fixAvailable.name // "") != ""
          and ($pkg.value.fixAvailable.version // "") != ""
        then "npm install " + $pkg.value.fixAvailable.name + "@" + $pkg.value.fixAvailable.version
        elif ($pkg.value.fixAvailable | type) == "boolean" and $pkg.value.fixAvailable == true then
          "npm audit fix"
        elif ($pkg.value.fixAvailable | type) == "object" then
          "npm audit fix"
        else
          "Manual upgrade required"
        end
      ),
      patched_version: (
        if ($pkg.value.fixAvailable | type) == "object" and ($pkg.value.fixAvailable.version // "") != ""
        then $pkg.value.fixAvailable.version
        elif (.range // "") != ""
        then .range
        else "see advisory"
        end
      )
    }
  ] as $object_entries |

  # Fallback entry when npm provides package severity but no advisory object
  [
    $vulns | to_entries[] |
    select((.value.via // []) | all(type != "object")) |
    {
      id: "unknown",
      package: .key,
      installed_version: (.value.range // "unknown"),
      severity: (.value.severity // "unknown"),
      title: "Unknown vulnerability (npm advisory metadata missing)",
      url: "",
      fix_available: (
        if (.value.fixAvailable | type) == "boolean" then .value.fixAvailable
        elif (.value.fixAvailable | type) == "object" then true
        else false
        end
      ),
      fix_command: (
        if (.value.fixAvailable | type) == "object"
          and (.value.fixAvailable.name // "") != ""
          and (.value.fixAvailable.version // "") != ""
        then "npm install " + .value.fixAvailable.name + "@" + .value.fixAvailable.version
        elif (.value.fixAvailable | type) == "boolean" and .value.fixAvailable == true then
          "npm audit fix"
        elif (.value.fixAvailable | type) == "object" then
          "npm audit fix"
        else
          "Manual upgrade required"
        end
      ),
      patched_version: (
        if (.value.fixAvailable | type) == "object" and (.value.fixAvailable.version // "") != ""
        then .value.fixAvailable.version
        else "see advisory"
        end
      )
    }
  ] as $fallback_entries |

  ($object_entries + $fallback_entries | unique_by(.package + "|" + .id + "|" + .title)) as $entries |

  {
    ecosystem: "npm",
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
