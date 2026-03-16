#!/usr/bin/env bash
# aggregate.sh — Merge per-ecosystem audit JSONs into unified report
# Usage: bash aggregate.sh <file1.json> <file2.json> ...
#   or:  cat results/*.json | bash aggregate.sh
# Output: Unified JSON to stdout, Markdown report to stderr

set -euo pipefail

SCAN_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
INPUT_FILE="$(mktemp)"
trap 'rm -f "$INPUT_FILE"' EXIT

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required for aggregation" >&2
  exit 1
fi

json_error() {
  local msg="$1"
  printf '{"error":"%s"}\n' "$(printf '%s' "$msg" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')" >&2
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

# Collect all input JSONs into a single array file
if [ $# -gt 0 ]; then
  # Files passed as arguments
  if ! jq -s '.' "$@" > "$INPUT_FILE" 2>/dev/null; then
    json_error "Failed to parse one or more input files as JSON"
    exit 1
  fi
elif [ ! -t 0 ]; then
  # Stdin is piped/redirected — read it with a timeout
  if ! run_timeout 15 jq -s '.' > "$INPUT_FILE" 2>/dev/null; then
    json_error "Failed to read valid JSON from stdin (or timed out)"
    exit 1
  fi
else
  echo 'Usage: aggregate.sh <file1.json> [file2.json] ...' >&2
  echo '  or:  cat results/*.json | aggregate.sh' >&2
  json_error "No input files provided"
  exit 1
fi

if ! jq -e 'type == "array"' "$INPUT_FILE" >/dev/null 2>&1; then
  json_error "Aggregated input is not a JSON array"
  exit 1
fi

if [ "$(jq 'length' "$INPUT_FILE")" -eq 0 ]; then
  json_error "No audit results to aggregate"
  exit 1
fi

# Generate unified JSON
UNIFIED="$(jq -r --arg time "$SCAN_TIME" '
  def is_valid_result:
    type == "object"
    and (.ecosystem | type == "string")
    and (.directory | type == "string")
    and (.summary | type == "object")
    and (.vulnerabilities | type == "array");

  def normalized_summary($s; $v):
    {
      critical: ($s.critical // 0),
      high: ($s.high // 0),
      moderate: ($s.moderate // 0),
      low: ($s.low // 0),
      info: ($s.info // 0),
      unknown: ($s.unknown // 0),
      total: ($s.total // ($v | length) // 0)
    };

  . as $all |

  [
    $all[] |
    select(is_valid_result) |
    {
      ecosystem: .ecosystem,
      directory: .directory,
      summary: normalized_summary(.summary; .vulnerabilities),
      vulnerabilities: (.vulnerabilities // [])
    }
  ] as $results |

  [
    $all[] |
    select(is_valid_result | not) |
    {
      ecosystem: (.ecosystem // "unknown"),
      directory: (.directory // null),
      error: (
        if (.error // null) != null
        then (.error | tostring)
        else "Invalid result schema (expected ecosystem,directory,summary,vulnerabilities)"
        end
      )
    }
  ] as $errors |

  {
    scan_time: $time,
    status: (if ($results | length) > 0 then "ok" else "error" end),
    directories_scanned: ([$results[]?.directory] | unique | length),
    ecosystems: ([$results[]?.ecosystem] | unique),
    summary: {
      critical: ([$results[]?.summary.critical] | add // 0),
      high: ([$results[]?.summary.high] | add // 0),
      moderate: ([$results[]?.summary.moderate] | add // 0),
      low: ([$results[]?.summary.low] | add // 0),
      info: ([$results[]?.summary.info] | add // 0),
      unknown: ([$results[]?.summary.unknown] | add // 0),
      total: ([$results[]?.summary.total] | add // 0)
    },
    by_ecosystem: $results,
    top_findings: (
      [$results[]?.vulnerabilities[]?] |
      sort_by(
        if .severity == "critical" then 0
        elif .severity == "high" then 1
        elif .severity == "moderate" then 2
        elif .severity == "low" then 3
        elif .severity == "info" then 4
        elif .severity == "unknown" then 5
        else 6
        end
      ) |
      .[0:10]
    ),
    errors: $errors
  }
' "$INPUT_FILE")"

# Output JSON to stdout
echo "$UNIFIED"

# Generate Markdown report to stderr
echo "$UNIFIED" | jq -r '
  "# 🔒 Dependency Audit Report\n" +
  "**Scanned:** " +
    (if (.by_ecosystem | length) > 0
     then ([.by_ecosystem[].directory] | unique | join(", "))
     else "none"
     end) + "  \n" +
  "**Time:** " + .scan_time + "  \n" +
  "**Status:** " + .status + "  \n" +
  "**Ecosystems:** " +
    (if (.ecosystems | length) > 0 then (.ecosystems | join(", ")) else "none" end) +
  "\n\n" +

  "## Summary\n\n" +
  "| Severity | Count |\n|----------|-------|\n" +
  "| 🔴 Critical | " + (.summary.critical | tostring) + " |\n" +
  "| 🟠 High | " + (.summary.high | tostring) + " |\n" +
  "| 🟡 Moderate | " + (.summary.moderate | tostring) + " |\n" +
  "| 🔵 Low | " + (.summary.low | tostring) + " |\n" +
  "| ⚪ Unknown | " + (.summary.unknown | tostring) + " |\n" +
  "| **Total** | **" + (.summary.total | tostring) + "** |\n\n" +

  (if (.summary.critical + .summary.high) > 0 then
    "## Critical & High Findings\n\n" +
    "| Package | Version | Severity | Advisory | Fix |\n|---------|---------|----------|----------|-----|\n" +
    (
      [.top_findings[] |
        select(.severity == "critical" or .severity == "high") |
        "| " + (.package // "unknown") +
        " | " + (.installed_version // "unknown") +
        " | " + (if .severity == "critical" then "🔴 Critical" else "🟠 High" end) +
        " | " + (.id // "unknown") +
        " | `" + (.fix_command // "manual review") + "` |"
      ] | join("\n")
    ) + "\n\n"
  else
    ""
  end) +

  (if (.errors | length) > 0 then
    "## Skipped / Error Inputs\n\n" +
    (
      [.errors[] |
        "- **" + (.ecosystem | tostring) + "**" +
        (if .directory != null then " (`" + (.directory | tostring) + "`)" else "" end) +
        ": " + (.error | tostring)
      ] | join("\n")
    ) + "\n\n"
  else
    ""
  end) +

  (if (.by_ecosystem | length) > 0 then
    ([.by_ecosystem[] |
      "## " + (.ecosystem | ascii_upcase) + " (" + (.summary.total | tostring) + " findings)\n\n" +
      (if (.vulnerabilities | length) > 0 then
        "| Package | Version | Severity | Advisory |\n|---------|---------|----------|----------|\n" +
        ([.vulnerabilities[] |
          "| " + (.package // "unknown") +
          " | " + (.installed_version // "unknown") +
          " | " + (.severity // "unknown") +
          " | " + (.id // "unknown") + " |"
        ] | join("\n")) + "\n"
      else
        "✅ No known vulnerabilities found.\n"
      end)
    ] | join("\n"))
  else
    "## Results\n\nNo valid ecosystem reports were provided.\n"
  end) +

  "\n---\n⚠️ This report covers known vulnerabilities in public advisory databases. It does not detect zero-days or runtime security issues."
' >&2
