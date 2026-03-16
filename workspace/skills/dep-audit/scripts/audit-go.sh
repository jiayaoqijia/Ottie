#!/usr/bin/env bash
# audit-go.sh — Run govulncheck and output normalized JSON
# Usage: bash audit-go.sh <directory>
# Output: Normalized JSON to stdout

set -euo pipefail

SCAN_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
INPUT_DIR="${1:-.}"
DIR=""
RESULT_FILE="$(mktemp)"
trap 'rm -f "$RESULT_FILE"' EXIT

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

emit_error() {
  local msg="$1"
  local err_dir="${2:-$INPUT_DIR}"
  printf '{"error":"%s","ecosystem":"go","directory":"%s"}\n' \
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

if ! command -v govulncheck >/dev/null 2>&1; then
  emit_error "govulncheck not found. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  emit_error "jq not found"
  exit 1
fi

if [ ! -f "$DIR/go.sum" ]; then
  emit_error "go.sum not found in target directory"
  exit 1
fi

# govulncheck needs a Go project — run from the directory
RAW=""
AUDIT_RC=0
if RAW=$(cd "$DIR" && run_timeout 60 govulncheck -json ./... 2>/dev/null); then
  AUDIT_RC=0
else
  AUDIT_RC=$?
fi

if [ "$AUDIT_RC" -eq 124 ]; then
  emit_error "govulncheck timed out after 60s"
  exit 1
fi

if [ -z "$RAW" ]; then
  emit_error "govulncheck returned no output"
  exit 1
fi

# govulncheck -json emits newline-delimited JSON objects.
# We look for objects with .finding and .osv fields and correlate by OSV ID.
if ! echo "$RAW" | jq -rs --arg dir "$DIR" --arg time "$SCAN_TIME" '
  def norm_severity($osv):
    if (($osv.database_specific.severity // "") | ascii_upcase) == "CRITICAL" then "critical"
    elif (($osv.database_specific.severity // "") | ascii_upcase) == "HIGH" then "high"
    elif (($osv.database_specific.severity // "") | ascii_upcase) == "MODERATE" then "moderate"
    elif (($osv.database_specific.severity // "") | ascii_upcase) == "LOW" then "low"
    elif (($osv.severity // []) | length) > 0 then "moderate"
    else "unknown"
    end;

  . as $all |

  # Collect OSV metadata keyed by ID.
  ([$all[] | select(.osv != null)] | map({(.osv.id): .osv}) | add // {}) as $osv_db |

  # Collect findings and map each to an OSV entry.
  (
    [$all[] | select(.finding != null) | .finding.osv] |
    map(select(. != null and . != "")) |
    unique
  ) as $finding_ids |

  [
    $finding_ids[] as $id |
    ($osv_db[$id] // {}) as $osv |
    {
      id: $id,
      package: ($osv.affected[0].package.name // "unknown"),
      installed_version: "see go.sum",
      severity: norm_severity($osv),
      title: ($osv.summary // "See advisory"),
      url: ("https://pkg.go.dev/vuln/" + $id),
      fix_available: (($osv.affected[0].ranges[0].events // []) | map(select(.fixed != null)) | length > 0),
      fix_command: (
        ($osv.affected[0].ranges[0].events // []) | map(select(.fixed != null)) |
        if length > 0 then
          "go get " + ($osv.affected[0].package.name // "") + "@" + (.[0].fixed // "latest")
        else
          "No fix version available"
        end
      ),
      patched_version: (
        ($osv.affected[0].ranges[0].events // []) |
        map(select(.fixed != null)) |
        map(.fixed) |
        join(", ")
      )
    }
  ] as $entries |

  {
    ecosystem: "go",
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
' > "$RESULT_FILE" 2>/dev/null; then
  emit_error "govulncheck output could not be parsed"
  exit 1
fi

cat "$RESULT_FILE"
