#!/usr/bin/env bash
# sbom.sh — Generate a CycloneDX SBOM for a project
# Usage: bash sbom.sh <directory>
# Output: Creates sbom.cdx.json in the target directory, summary to stdout

set -euo pipefail

INPUT_DIR="${1:-.}"
DIR=""
OUTPUT=""

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

emit_error() {
  local msg="$1"
  local err_dir="${2:-$INPUT_DIR}"
  printf '{"error":"%s","directory":"%s"}\n' \
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

OUTPUT="$DIR/sbom.cdx.json"

if ! command -v syft >/dev/null 2>&1; then
  emit_error "syft not found. Install with: curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin" "$DIR"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  emit_error "jq not found — required for SBOM summary output" "$DIR"
  exit 1
fi

echo "Generating SBOM for $DIR ..." >&2

SBOM_RC=0
if run_timeout 60 syft "dir:$DIR" -o cyclonedx-json > "$OUTPUT" 2>/dev/null; then
  SBOM_RC=0
else
  SBOM_RC=$?
fi

if [ "$SBOM_RC" -eq 124 ]; then
  emit_error "syft timed out after 60s" "$DIR"
  exit 1
fi

if [ "$SBOM_RC" -ne 0 ]; then
  emit_error "syft failed to generate SBOM" "$DIR"
  exit 1
fi

COMPONENT_COUNT=$(jq '.components | length' "$OUTPUT" 2>/dev/null || echo "0")
jq -n --arg file "$OUTPUT" --argjson count "$COMPONENT_COUNT" '{
  status: "success",
  file: $file,
  format: "CycloneDX JSON",
  components: $count
}'

echo "✅ SBOM written to $OUTPUT ($COMPONENT_COUNT components)" >&2
