#!/usr/bin/env bash
# run-experiment.sh — timeout wrapper for autoresearch experiments
# Usage: ./run-experiment.sh <timeout_seconds> <command...>
#
# Runs the given command with a timeout. Exits with:
#   0   — command succeeded
#   124 — command timed out
#   *   — command failed with its own exit code

set -euo pipefail

TIMEOUT="${1:?Usage: run-experiment.sh <timeout_seconds> <command...>}"
shift

exec timeout --signal=TERM --kill-after=10 "${TIMEOUT}" "$@"
