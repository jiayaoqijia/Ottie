# Multi-Agent Swarm

Ottie supports two optional collaboration modes. Both are entirely opt-in — your existing single-agent config works unchanged.

## Mode A: Sub-Agents (Single Process)

One orchestrator agent spawns specialized worker agents inside the same process. The orchestrator delegates tasks, workers execute them independently, and results flow back automatically.

### Step 1: Add `agents.list` to your config

Edit `~/.ottie/config.json`. Keep your existing `model_list` and `channels` — just add `agents.list` inside the `agents` block:

```json
{
  "agents": {
    "defaults": {
      "model_name": "your-model",
      "max_tokens": 128000
    },
    "list": [
      {
        "id": "orchestrator",
        "default": true,
        "identity": "You are the Orchestrator. Break complex tasks into subtasks and delegate them to specialized sub-agents using the sessions_spawn tool. Use sessions_control to monitor progress. Summarize results when all sub-agents report back.",
        "role": "orchestrator",
        "subagents": {
          "allow_agents": ["researcher", "coder"],
          "max_spawn_depth": 2,
          "max_children_per": 3
        }
      },
      {
        "id": "researcher",
        "identity": "You are a Researcher sub-agent. Find information using web search and report findings concisely. Always provide a clear summary.",
        "role": "leaf",
        "tools_allow": ["web_search", "web_fetch", "read_file", "list_dir"]
      },
      {
        "id": "coder",
        "identity": "You are a Coder sub-agent. Write, edit, and review code. Verify your work compiles or runs correctly before reporting back.",
        "role": "leaf",
        "tools_allow": ["read_file", "write_file", "edit_file", "append_file", "list_dir", "exec"]
      }
    ]
  },
  "model_list": [ ... ],
  "channels": { ... }
}
```

**Key fields:**

| Field | Description |
|-------|-------------|
| `id` | Unique agent identifier |
| `default` | Set `true` on the agent that handles incoming messages (usually orchestrator) |
| `identity` | System prompt that defines the agent's personality and instructions |
| `role` | `"orchestrator"` gets spawn/control tools; `"leaf"` does not |
| `subagents.allow_agents` | Which agent IDs this orchestrator can spawn |
| `subagents.max_spawn_depth` | Max nesting depth (orchestrator spawns agent that spawns agent...) |
| `subagents.max_children_per` | Max concurrent sub-agents per parent |
| `tools_allow` | Whitelist of tools this agent can use (empty = all tools) |
| `tools_deny` | Blacklist of tools (applied after allow list) |

### Step 2: Run Ottie

No special flags needed. Run the same way you always do:

```bash
# Via Telegram/Slack/etc.
./build/ottie gateway

# Interactive CLI (for testing)
./build/ottie agent

# One-shot CLI
./build/ottie agent -m "Spawn the researcher to find the latest Go version"
```

### Step 3: Talk to the orchestrator

The orchestrator automatically gets two new tools:

- **`sessions_spawn`** — Spawn a sub-agent with a task. Parameters: `task` (required), `agent_id` (optional), `label` (optional).
- **`sessions_control`** — Monitor sub-agents. Actions: `list`, `kill`, `steer`, `info`.

Example prompts:
- "Research the top 3 Go web frameworks and write a comparison"
- "Spawn researcher to find info about Kubernetes operators, then spawn coder to write an example"
- "Use sessions_control to list all running sub-agents"

### How it works internally

```
User message
  → Orchestrator (has sessions_spawn + sessions_control)
    → sessions_spawn(agent_id="researcher", task="find X")
      → Researcher runs independently with web_search, web_fetch, read_file
      → Result announced back to orchestrator
    → sessions_spawn(agent_id="coder", task="write Y")
      → Coder runs independently with read_file, write_file, exec
      → Result announced back to orchestrator
  → Orchestrator combines results and responds
```

- Sub-agents run as goroutines in the same process
- Each sub-agent has its own LLM conversation (no shared session history)
- Sub-agents use the same model and API key as the parent
- Results are automatically queued and injected into the orchestrator's next LLM turn
- The orchestrator can spawn multiple sub-agents concurrently

---

## Mode B: Multi-Bot Telegram Group (Multiple Processes)

Multiple independent Ottie instances — each running as a separate Telegram bot — collaborate in the same group chat. They coordinate via a shared ProjectBoard.

### Step 1: Create two Telegram bots

Talk to [@BotFather](https://t.me/BotFather) on Telegram and create two bots:
1. A "Coder" bot (get its token)
2. A "Researcher" bot (get its token)

Disable privacy mode for both bots so they can see group messages:
- `/mybots` → select bot → Bot Settings → Group Privacy → Turn off

### Step 2: Create config for each bot

Each bot needs its own `OTTIE_HOME` directory with a separate `config.json`.

**Bot 1 — Coder** (`~/.ottie-coder/config.json`):

```json
{
  "swarm": {
    "enabled": true,
    "instance_id": "coder-bot"
  },
  "agents": {
    "defaults": {
      "model_name": "your-model",
      "max_tokens": 128000
    },
    "list": [
      {
        "id": "main",
        "identity": "You are the Coder bot in a multi-agent team. Your specialty is writing, reviewing, and debugging code. Use the project_board tool to coordinate with other bots — check for tasks assigned to you, post results as artifacts, and hand off work when needed."
      }
    ]
  },
  "model_list": [
    {
      "model_name": "your-model",
      "model": "openai/gpt-5.4",
      "api_key": "your-api-key",
      "api_base": "https://api.openai.com/v1"
    }
  ],
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "CODER_BOT_TOKEN_FROM_BOTFATHER"
    }
  }
}
```

**Bot 2 — Researcher** (`~/.ottie-researcher/config.json`):

```json
{
  "swarm": {
    "enabled": true,
    "instance_id": "researcher-bot"
  },
  "agents": {
    "defaults": {
      "model_name": "your-model",
      "max_tokens": 128000
    },
    "list": [
      {
        "id": "main",
        "identity": "You are the Researcher bot in a multi-agent team. Your specialty is finding information, analyzing data, and summarizing findings. Use the project_board tool to coordinate with other bots — post research findings as artifacts, and hand off coding tasks to @coder_bot."
      }
    ]
  },
  "model_list": [
    {
      "model_name": "your-model",
      "model": "openai/gpt-5.4",
      "api_key": "your-api-key",
      "api_base": "https://api.openai.com/v1"
    }
  ],
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "RESEARCHER_BOT_TOKEN_FROM_BOTFATHER"
    }
  }
}
```

### Step 3: Create a Telegram group and add both bots

1. Create a new Telegram group
2. Add both bots to the group
3. Make both bots admins (optional but recommended for reliability)

### Step 4: Run both instances

```bash
# Terminal 1 — Coder bot
OTTIE_HOME=~/.ottie-coder ./build/ottie gateway

# Terminal 2 — Researcher bot
OTTIE_HOME=~/.ottie-researcher ./build/ottie gateway
```

Or run both in the background:

```bash
OTTIE_HOME=~/.ottie-coder ./build/ottie gateway &
OTTIE_HOME=~/.ottie-researcher ./build/ottie gateway &
```

### Step 5: Interact in the group

Each bot gets a `project_board` tool with these actions:

| Action | Parameters | Description |
|--------|-----------|-------------|
| `read_tasks` | — | List all tasks on the board |
| `post_task` | `title`, `description` | Create a new task |
| `claim_task` | `task_id` | Claim a task (sets status to "claimed") |
| `update_task` | `task_id`, `status`, `title`, `description` | Update a task |
| `put_artifact` | `key`, `value` | Store a shared artifact |
| `get_artifact` | `key` | Retrieve an artifact |
| `list_artifacts` | — | List all artifacts |
| `put_context` | `key`, `value` | Store shared context |
| `get_context` | `key` | Retrieve shared context |
| `handoff` | `target_bot`, `message` | Send an @mention handoff to another bot |

Example group conversation:
1. You: "@researcher_bot find the top 3 Rust web frameworks"
2. Researcher bot searches the web, posts findings as an artifact, then uses `handoff` to @mention the coder bot
3. Coder bot sees the handoff, reads the artifact, and writes example code

### How it works internally

```
Telegram Group
  ├── User sends message @mentioning researcher-bot
  ├── Researcher bot processes the message
  │   ├── Uses web_search to find info
  │   ├── project_board put_artifact(key="research", value="...")
  │   └── project_board handoff(target="@coder_bot", message="Please write example code")
  │       → Sends "@coder_bot Please write example code" + JSON handoff block
  ├── Coder bot detects the handoff block targeting it
  │   ├── project_board get_artifact(key="research")
  │   ├── Writes code
  │   └── Responds in group
  └── User sees the collaborative result
```

- Each bot runs in its own process with its own config
- The ProjectBoard is currently in-memory per process (shared state requires Redis — planned)
- Bots detect handoff blocks (JSON in code fences) targeting their username
- Messages from other bots that don't @mention this bot are ignored (no loops)

---

## Combining Mode A + Mode B

You can use both modes together. An orchestrator bot in Mode A can also have `swarm.enabled: true` to access the `project_board` tool:

```json
{
  "swarm": { "enabled": true, "instance_id": "orchestrator-bot" },
  "agents": {
    "list": [
      {
        "id": "orchestrator", "default": true,
        "identity": "You are the Orchestrator with internal sub-agents and access to a shared project board.",
        "role": "orchestrator",
        "subagents": { "allow_agents": ["researcher", "coder"] }
      },
      { "id": "researcher", "role": "leaf", "tools_allow": ["web_search", "web_fetch", "read_file"] },
      { "id": "coder", "role": "leaf", "tools_allow": ["read_file", "write_file", "exec"] }
    ]
  }
}
```

This gives the orchestrator `sessions_spawn`, `sessions_control`, AND `project_board`.

---

## Configuration Reference

### Agent Config Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique agent identifier (required) |
| `default` | bool | Handle incoming messages (one agent should be default) |
| `name` | string | Display name (optional) |
| `identity` | string | System prompt for this agent |
| `role` | string | `"orchestrator"` or `"leaf"` — controls which tools are registered |
| `workspace` | string | Custom workspace path (optional, auto-generated if omitted) |
| `model` | string/object | Override model for this agent |
| `skills` | []string | Filter which skills this agent can use |
| `tools_allow` | []string | Whitelist of tool names (empty = all) |
| `tools_deny` | []string | Blacklist of tool names |
| `subagents` | object | Sub-agent spawn configuration (orchestrators only) |

### Subagents Config

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `allow_agents` | []string | — | Agent IDs this orchestrator can spawn |
| `model` | string/object | parent's model | Override model for sub-agents |
| `max_spawn_depth` | int | 3 | Max nesting depth |
| `max_children_per` | int | 5 | Max concurrent sub-agents per parent |
| `auto_announce` | bool | true | Auto-inject sub-agent results into parent's conversation |

### Swarm Config

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | false | Enable Mode B (multi-bot collaboration) |
| `instance_id` | string | — | Unique identifier for this bot instance |
| `redis_url` | string | — | Redis URL for cross-process ProjectBoard (planned) |
| `project_board_ttl` | int | 86400 | TTL for board entries in seconds |

---

## FAQ

**Q: Does this change anything for existing users?**
A: No. Without `agents.list` or `swarm.enabled`, Ottie behaves identically to before. No new tools are registered, no extra overhead.

**Q: What model does the sub-agent use?**
A: The same model as the parent agent, including `force_stream` settings. You can override it with `subagents.model`.

**Q: Is Redis required?**
A: No. The in-memory ProjectBoard works for single-process Mode B testing. Redis is planned for production multi-process deployments where bots need to share state.

**Q: Can Mode A and Mode B coexist?**
A: Yes. Set `swarm.enabled: true` on an orchestrator to give it both internal sub-agents and the shared project board.

**Q: What about session isolation?**
A: Each sub-agent runs with its own ephemeral context — no shared session history. Only the task description and tools are passed to the sub-agent.

**Q: What tools do leaf agents get?**
A: Leaf agents get all standard tools by default. Use `tools_allow` to restrict them (e.g., researcher only gets search tools, coder only gets file tools). The `sessions_spawn` and `sessions_control` tools are only given to agents with `subagents` configured.
