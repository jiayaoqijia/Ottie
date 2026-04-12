# Adding Skills to Ottie

## Quick reference

| What | Where |
|-|-|
| Skill directory | `workspace/skills/<category>/<name>/SKILL.md` |
| Categories | `crypto/`, `defi/`, `identity/`, `payments/`, `safety/`, `research/`, `meta/` |
| Loader | `pkg/skills/loader.go` — `ListSkills` (recursive `WalkDir`), `LoadSkill` |
| Validation | Name regex `^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`, description < 1024 chars |
| Onboard copy | `cmd/ottie/internal/onboard/workspace/skills/` — mirrors repo via `go generate` |

## Steps to add a new skill

1. Choose the appropriate category folder under `workspace/skills/`.

2. Create `workspace/skills/<category>/<name>/SKILL.md` with YAML frontmatter:
   ```yaml
   ---
   name: my-new-skill
   description: >
     One-line description of when to activate this skill.
   ---
   ```

3. Write the skill body after the frontmatter. Include:
   - Trigger phrases (when should the agent activate this skill?)
   - API usage examples using `web_fetch` (GET) or `exec curl` (POST)
   - Reference files in a `references/` subdirectory if needed

4. Mirror the new skill to the onboard template:
   ```bash
   cp -r workspace/skills/<category>/<name> cmd/ottie/internal/onboard/workspace/skills/<category>/
   ```
   Or run `go generate ./cmd/ottie/internal/onboard/...` to regenerate the entire embedded copy.

5. Verify the loader picks it up:
   ```bash
   go test ./pkg/skills/...
   ```

## Skill resolution priority

1. **workspace** — project-level skills (`workspace/skills/`)
2. **global** — user-level skills (`~/.ottie/skills/`)
3. **builtin** — binary-embedded skills (from onboard template)

## Category guidelines

| Category | What goes here |
|-|-|
| `crypto/` | Market data, wallet queries, CEX APIs, smart money signals |
| `defi/` | Swap, lending, staking, yield, protocol-specific (Lido, Aave, etc.) |
| `identity/` | ERC-8004, Self Agent ID, on-chain identity protocols |
| `payments/` | Machine-to-machine payments (Tempo/MPP, HTTP 402) |
| `safety/` | DLP, privacy, prompt injection guards, zero-knowledge proofs |
| `research/` | Prediction markets, social protocols, research tools |
| `meta/` | Self-evolve, skill management, skill discovery |
