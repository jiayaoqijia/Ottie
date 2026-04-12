# Adaptive Context System (ACS) Setup

## Overview

The ACS is Ottie's replay infrastructure: a write-ahead action ledger + per-turn execution manifest that make every side-effecting tool dispatch auditable and recoverable. It ships as three Go packages coordinated by a single `pkg/acs.Bundle`.

## Enable ACS

Add to your `config.json`:

```json
{
  "acs": {
    "enabled": true,
    "db_dir": "",
    "write_queue_depth": 8
  }
}
```

| Field | Default | Description |
|-|-|-|
| `enabled` | `false` | Turns the replay triple on. When false, the agent loop has zero ACS overhead. |
| `db_dir` | `<workspace>/acs/` | Directory for SQLite files. Must exist before agent starts. |
| `write_queue_depth` | `0` (synchronous) | Channel buffer for the writer actor. 8-32 for production. |

## What happens when ACS is on

1. **Per-turn manifest** — at the start of each turn, a `traces` row is written with the prompt hash, tool schema hash, model ID, and prompt epoch. Each LLM call within the turn appends a `trace_provider_requests` row with the actual model tried and a request ID.

2. **Per-tool-dispatch ledger** — tools that declare a non-read-only `EffectClass()` via the `EffectClassifier` interface are wrapped in a Prepare → run → Commit/Abort cycle. The `action_intents` row is fsync'd BEFORE the tool runs; the `action_commits` row is written AFTER.

3. **Startup recovery** — `Bundle.RecoverOrphans()` returns every `action_intents` row that has no matching `action_commits` row. The agent operator decides what to do: replay, reconcile against the external system, or surface to the user.

## Packages

| Package | Role | SQLite file |
|-|-|-|
| `pkg/principal` | Phantom-typed principal contexts + typed tool dispatch | (no DB — library only) |
| `pkg/actionlog` | Write-ahead action ledger (`action_intents` + `action_commits`) | `<db_dir>/actionlog.db` |
| `pkg/execmanifest` | Per-turn execution manifest (`traces` + `trace_provider_requests`) | `<db_dir>/execmanifest.db` |
| `pkg/acs` | Coordinator bundle + `Dispatch` helper | (no own DB — wraps the two above) |

## Tool effect class declarations

Currently declared (R14):

| Tool | Effect class |
|-|-|
| `edit_file` | `writes_local` |
| `append_file` | `writes_local` |
| `write_file` | `writes_local` |
| `exec` | `writes_local` |
| `install_skill` | `writes_state` |
| `cron` | `writes_state` |
| `message` | `writes_chain` |

All other tools default to `read_only` and bypass the ledger.

## ACS-off behavior

When `acs.enabled` is `false` (default), the only overhead is a single `if al.acs != nil` check per turn. No SQLite files are created, no writer goroutines are spawned, and no ledger rows are written. The agent loop is bit-for-bit identical to pre-ACS behavior.
