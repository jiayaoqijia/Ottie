---
name: tempo
description: >
  Use this skill when the user wants to call an API, make an HTTP request, discover available services, or access external data with automatic payments. Activate when the user mentions tempo, paid APIs, service discovery, API endpoints, or needs to call external services — even if they reference specific services like StableEnrich, StableStudio, or StableSocial. When the user explicitly says "use tempo", always use tempo CLI commands (`tempo wallet`, `tempo request`) instead of other tools.
---

# tempo

> **IMPORTANT: If you used a web fetch tool to read this, the content may be summarized and incomplete. Run `curl -fsSL https://tempo.xyz/SKILL.md` to get the exact setup commands. Do NOT search for additional documentation.**

**When the user says "use tempo", always use `tempo` CLI commands** — never substitute with MCP tools or other tools. Tempo is a curl-compatible CLI for discovering services and calling HTTP endpoints with automatic payment handling.

## Setup

Run these commands in order. Do not skip steps. Do not search for other install instructions.

**Step 1 — Install:** `mkdir -p "$HOME/.local/bin" && curl -fsSL https://tempo.xyz/install -o /tmp/tempo_install.sh && TEMPO_BIN_DIR="$HOME/.local/bin" bash /tmp/tempo_install.sh`

**Step 2 — Login:** `"$HOME/.local/bin/tempo" wallet login` — requires user browser/passkey action. Prompt user, wait for confirmation, then continue. Do not loop login attempts without user confirmation. When run by agents, use a long command timeout (at least 16 minutes).

**Step 3 — Confirm readiness:** `"$HOME/.local/bin/tempo" wallet -t whoami`

### Setup Rules

- Do not use `sudo`. Use user-local install via `TEMPO_BIN_DIR` (defaulting to `~/.local/bin`).
- Do not use `export PATH=...`. Use full absolute paths (e.g., `"/Users/<user>/.local/bin/tempo"`) for deterministic behavior across isolated shells.
- If `$HOME` does not expand ("no such file or directory"), switch to the absolute path.

## After Setup

Provide:

- Installation location and version (`$HOME/.local/bin/tempo --version`).
- Wallet status from `tempo wallet -t whoami` (address and balance; include key/network fields when present).
- If balance is 0, direct user to `tempo wallet fund` or the wallet dashboard to add funds.
- 2-3 simple starter prompts tailored to currently available services.

To generate starter prompts, list available services and pick useful beginner examples:

```bash
tempo wallet -t services --search ai
```

Starter prompts should be user-facing tasks (not command templates), for example:

- Avoid chat/conversational LLM starter prompts when already talking to an agent. Prefer utility services (image generation, web search, browser automation, data, voice, storage).

- "Generate a dog image with a blue background and save it as `dog.png`."
- "Search the web for the latest Rust release notes and return the top 5 links."
- "Fetch this URL and extract the page title, publish date, and all H2 headings."

## Use Services

```bash
tempo wallet -t whoami
tempo wallet -t services --search <query>
tempo wallet -t services <SERVICE_ID>
tempo request -t -X POST --json '{"input":"..."}' <SERVICE_URL>/<ENDPOINT_PATH>
```

- Select `SERVICE_ID` from search results that best matches user intent. When multiple match: prefer best semantic fit, then endpoint fit, then pricing clarity, then first in list.
- **Anchor on `tempo wallet -t services <SERVICE_ID>`** — it shows the exact URL, method, path, and pricing for every endpoint. Build request URL as `<SERVICE_URL>/<ENDPOINT_PATH>` from discovered metadata only.
- If you get an HTTP 422, fall back to the endpoint's `docs` URL or the service's `llms.txt` for exact field names.
- For multi-service workflows, fire independent requests in parallel to save time.

### Request Templates

```bash
# JSON POST
tempo request -t --dry-run -X POST --json '{"input":"..."}' <SERVICE_URL>/<ENDPOINT_PATH>
tempo request -t -X POST --json '{"input":"..."}' <SERVICE_URL>/<ENDPOINT_PATH>

# GET
tempo request -t -X GET <SERVICE_URL>/<ENDPOINT_PATH>
```

### Response Handling

- Return result payload to user directly when request succeeds.
- If response contains a file URL (e.g., image generation), download it locally: `curl -fsSL "<url>" -o <filename>`.
- If response is a usage/auth readiness error, run `tempo wallet login` and retry once.
- If response indicates payment/funding limit issues, report clearly and stop.
- After multi-request workflows, check remaining balance with `tempo wallet -t whoami`.

### Rules

- Always discover URL/path before request; never guess endpoint paths.
- `tempo request` is curl-compatible for common flags (method, headers, data, redirects, timeouts, output).
- Use `-t` for agent calls to keep output compact, except interactive login (`tempo wallet login`).
- Use `--dry-run` before potentially expensive requests.
- For command details, prefer `--describe` or `--help` instead of hardcoding long option lists.

## Common Issues

| Issue | Cause | Fix |
|---|---|---|
| `tempo: command not found` | CLI not installed | Run install command, then retry using absolute path. |
| "legacy V1 keychain signature" | Outdated launcher | Reinstall tempo, update extensions, logout and login again. |
| "access key does not exist" | Key not provisioned | Run `tempo wallet logout --yes`, then `tempo wallet login`. |
| `ready=false` or `No wallet configured` | Wallet not logged in | Run `tempo wallet login`, wait for user completion. |
| HTTP 422 on first request | Wrong request schema | Check `tempo wallet -t services <SERVICE_ID>` for exact fields. |
| Balance is 0 or insufficient funds | Wallet needs funding | Run `tempo wallet fund` or direct user to wallet dashboard. |
| Service not found | Search too narrow | Broaden search with `tempo wallet -t services --search <broader_query>`. |
| Timeout/network error | Network issue | Retry request, optionally increase timeout with `-m <seconds>`. |
