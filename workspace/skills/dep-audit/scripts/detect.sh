#!/usr/bin/env bash
# detect.sh — Discover lockfiles and available audit tools
# Usage: bash detect.sh [target_directory]
# Output: JSON to stdout

set -euo pipefail

INPUT_DIR="${1:-.}"
DIR=""

# --- Helper: escape a string for safe JSON embedding ---
json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/\t/\\t/g' -e 's/$/\\n/g' | tr -d '\n' | sed 's/\\n$//'
}

if ! DIR="$(cd "$INPUT_DIR" 2>/dev/null && pwd -P)"; then
  safe_input="$(json_escape "$INPUT_DIR")"
  printf '{"error":"Directory not found or not accessible","directory":"%s"}\n' "$safe_input" >&2
  exit 1
fi

# --- Helper: check if command exists, get version ---
tool_info() {
  local cmd="$1" version_flag="$2" install_hint="$3"
  if command -v "$cmd" >/dev/null 2>&1; then
    local ver
    local -a version_args
    read -r -a version_args <<< "$version_flag"
    ver=$("$cmd" "${version_args[@]}" 2>/dev/null | head -1 | tr -cd '[:print:]' || echo "unknown")
    ver=$(json_escape "$ver")
    printf '{"available":true,"version":"%s"}' "$ver"
  else
    local hint
    hint=$(json_escape "$install_hint")
    printf '{"available":false,"install":"%s"}' "$hint"
  fi
}

# --- Find lockfiles (maxdepth 3) ---
# Build lockfile array as raw JSON string (no jq dependency)
lockfile_entries=""

add_lockfile() {
  local eco="$1" path="$2"
  local safe_eco safe_path entry
  safe_eco=$(json_escape "$eco")
  safe_path=$(json_escape "$path")
  entry=$(printf '{"ecosystem":"%s","path":"%s"}' "$safe_eco" "$safe_path")
  if [ -n "$lockfile_entries" ]; then
    lockfile_entries="${lockfile_entries},${entry}"
  else
    lockfile_entries="${entry}"
  fi
}

while IFS= read -r -d '' f; do
  base="$(basename "$f")"
  rel="${f#$DIR/}"
  case "$base" in
    package-lock.json) add_lockfile "npm" "$rel" ;;
    yarn.lock)         add_lockfile "yarn" "$rel" ;;
    pnpm-lock.yaml)    add_lockfile "pnpm" "$rel" ;;
    requirements.txt)  add_lockfile "python" "$rel" ;;
    Pipfile.lock)      add_lockfile "python" "$rel" ;;
    poetry.lock)       add_lockfile "python" "$rel" ;;
    Cargo.lock)        add_lockfile "rust" "$rel" ;;
    go.sum)            add_lockfile "go" "$rel" ;;
  esac
done < <(find "$DIR" -maxdepth 3 \( \
  -name "package-lock.json" -o \
  -name "yarn.lock" -o \
  -name "pnpm-lock.yaml" -o \
  -name "requirements.txt" -o \
  -name "Pipfile.lock" -o \
  -name "poetry.lock" -o \
  -name "Cargo.lock" -o \
  -name "go.sum" \
\) -print0 2>/dev/null)

lockfiles="[${lockfile_entries}]"

# --- Check tools ---
npm_info=$(tool_info "npm" "--version" "Pre-installed with Node")
pip_audit_info=$(tool_info "pip-audit" "--version" "pip install pip-audit")
cargo_audit_info=$(tool_info "cargo-audit" "--version" "cargo install cargo-audit")
govulncheck_info=$(tool_info "govulncheck" "-version" "go install golang.org/x/vuln/cmd/govulncheck@latest")
syft_info=$(tool_info "syft" "version" "curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh")

# --- Output JSON ---
# Use jq for safe interpolation if available, otherwise escaped raw output
if command -v jq >/dev/null 2>&1; then
  jq -n --arg dir "$DIR" \
    --argjson lockfiles "$lockfiles" \
    --argjson npm "$npm_info" \
    --argjson pip_audit "$pip_audit_info" \
    --argjson cargo_audit "$cargo_audit_info" \
    --argjson govulncheck "$govulncheck_info" \
    --argjson syft "$syft_info" \
    '{
      directory: $dir,
      lockfiles: $lockfiles,
      tools: {
        "npm": $npm,
        "pip-audit": $pip_audit,
        "cargo-audit": $cargo_audit,
        "govulncheck": $govulncheck,
        "syft": $syft
      }
    }'
else
  safe_dir=$(json_escape "$DIR")
  cat <<ENDJSON
{
  "directory": "${safe_dir}",
  "lockfiles": $lockfiles,
  "tools": {
    "npm": $npm_info,
    "pip-audit": $pip_audit_info,
    "cargo-audit": $cargo_audit_info,
    "govulncheck": $govulncheck_info,
    "syft": $syft_info
  }
}
ENDJSON
fi
