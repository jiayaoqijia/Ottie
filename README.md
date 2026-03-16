<div align="center">

  <img src="docs/ottie_banner.jpg" alt="Ottie Banner" width="100%">

  <h1>Ottie</h1>
  <h3>Self-Evolving Agent for Ethereum and Crypto</h3>

  <p>
    <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/license-AGPL--3.0-blue" alt="License">
  </p>

</div>

---

Ottie is a purpose-built AI agent for Ethereum and crypto, written in pure Go. Single binary, 22 blockchain-native skills, multi-agent swarms, 13+ messaging channels. Where general-purpose agents bolt on wallet plugins, Ottie treats every interaction as if it might involve real money.

## Why a Crypto-Specific Agent

General-purpose agents assume actions are reversible, networks are reliable, and authentication is optional. None of this holds on public blockchains. A bad email can be unsent. A bad transaction is permanent. Ottie is built from the ground up for an adversarial financial environment: constrained blast radius, self-evolving skills that adapt to protocol upgrades, and zero-dependency deployment.

## Features

- **Self-evolving skills** — learns from tasks and packages approaches as reusable skills with progressive 3-level disclosure
- **Ethereum-native** — 8 crypto/DeFi skills covering wallets, swaps, lending, staking, yield, CEX data, and research
- **Security first** — constrained blockchain domain (cannot access email, files, browser), prompt-injection guard, ClawWall DLP
- **Super light** — single Go binary (<10MB), zero CGO, sub-second startup, runs on a $5/month VPS
- **Multi-agent swarm** — Mode A (in-process goroutine workers) and Mode B (multi-bot Telegram coordination via Redis)
- **13+ channels** — Telegram, Discord, Slack, Signal, WhatsApp, Matrix, QQ, DingTalk, LINE, WeCom, Feishu, IRC
- **Multi-provider** — OpenAI, Anthropic, Zhipu, DeepSeek, Groq, Ollama with configurable fallback chains
- **Visual output** — render_html generates PNG charts/dashboards for data-heavy responses
- **MCP integration** — dynamically loads tools from Model Context Protocol servers
- **9 networks** — Ethereum, Arbitrum, Optimism, Base, Polygon, BSC, Avalanche, Fantom, and Solana

## Crypto Skills

Ottie ships with 8 crypto/DeFi skills covering the full stack. All use free, no-authentication APIs. Zero API keys required for read-only operations.

| Layer | Skill | Capabilities | APIs |
|-------|-------|-------------|------|
| **Market Intelligence** | `crypto-market-data` | Prices, trending, market cap, Fear & Greed, TVL | CoinGecko, DefiLlama |
| | `crypto-cex` | Order books, funding rates, tickers across 6 exchanges | Binance, Coinbase, Kraken, Bybit, Gate.io, Bitget |
| | `crypto-research` | Contract verification, perps, governance | Etherscan, Hyperliquid, Snapshot |
| **On-Chain Operations** | `crypto-wallet` | Balances, holdings, tx history, approvals, ENS | Public RPCs, Etherscan |
| | `defi-swap` | DEX swap quotes, price routing, slippage | ParaSwap, Jupiter, 1inch, 0x |
| **Yield & Risk** | `defi-lending` | Lending rates, APY, health factors | Aave, Morpho, Compound, DefiLlama |
| | `defi-staking` | Liquid staking APR, exchange rates | Lido, Rocket Pool, DefiLlama |
| | `defi-yield` | Yield farming, APY comparison | DefiLlama, Pendle, Curve |

**Supported networks:** Ethereum, Arbitrum, Optimism, Base, Polygon, BSC, Avalanche, Fantom, Solana

Skills are auto-loaded from `workspace/skills/`. Each includes reference files with token addresses, contract ABIs, and chain-specific configuration.

## Quick Start

```bash
# Build
git clone https://github.com/jiayaoqijia/ottie.git
cd ottie
make build

# Initialize
./build/ottie onboard

# Configure (~/.ottie/config.json) — add your API key
# Then run:
./build/ottie gateway
```

## Configuration

Config lives at `~/.ottie/config.json`. Minimal example:

```json
{
  "model_list": [
    {
      "model_name": "claude-sonnet-4.6",
      "model": "anthropic/claude-sonnet-4.6",
      "api_key": "sk-ant-your-key"
    }
  ],
  "agents": {
    "defaults": {
      "model": "claude-sonnet-4.6"
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_BOT_TOKEN",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

Use `vendor/model` format (e.g. `openai/gpt-5.4`, `zhipu/glm-4.7`, `ollama/llama3`). See `config.example.json` for all options.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OTTIE_HOME` | Root data directory | `~/.ottie` |
| `OTTIE_CONFIG` | Config file path | `~/.ottie/config.json` |

## CLI

| Command | Description |
|---------|-------------|
| `ottie onboard` | Initialize config & workspace |
| `ottie agent -m "..."` | One-shot chat |
| `ottie agent` | Interactive chat |
| `ottie gateway` | Start gateway (channels + heartbeat) |
| `ottie status` | Show status |
| `ottie cron list` | List scheduled jobs |

## Chat Channels

13+ messaging platforms through a single webhook server on `127.0.0.1:18790`:

**Telegram** · **Discord** · **Slack** · **Signal** · **WhatsApp** · **Matrix** · **QQ** · **DingTalk** · **LINE** · **WeCom** · **Feishu** · **IRC** · **Custom Webhooks**

Each channel gets a native adapter that handles platform-specific formatting, message splitting, and rich media embedding. The `render_html` tool converts charts and dashboards to PNG images that embed directly in any channel.

## Docker

```bash
git clone https://github.com/jiayaoqijia/ottie.git && cd ottie

# First run — generates config
docker compose -f docker/docker-compose.yml --profile gateway up

# Edit docker/data/config.json with your API keys, then:
docker compose -f docker/docker-compose.yml --profile gateway up -d
```

## Chromium (Optional)

The `render_html` tool renders HTML/CSS to PNG images for rich visual output (dashboards, styled cards, charts). It requires Chromium or Google Chrome.

```bash
# macOS — Chrome is auto-detected from /Applications
brew install --cask google-chrome

# Debian/Ubuntu
apt install chromium-browser

# Alpine (Docker)
apk add chromium

# Verify
ottie agent -m "render a hello world card"
```

If no browser is found, the tool is silently skipped — Ottie still works, but sends text instead of images.

## Multi-Agent Swarm

Ottie supports two optional collaboration modes. Both are entirely opt-in — your existing config works unchanged.

### Mode A: Sub-Agents (single process)

One orchestrator spawns specialized workers internally. Add `agents.list` to your config:

```json
{
  "agents": {
    "defaults": { "model_name": "your-model" },
    "list": [
      {
        "id": "orchestrator", "default": true,
        "identity": "You are the Orchestrator. Delegate tasks to sub-agents using sessions_spawn.",
        "role": "orchestrator",
        "subagents": { "allow_agents": ["researcher", "coder"], "max_spawn_depth": 2 }
      },
      {
        "id": "researcher",
        "identity": "You are a Researcher. Find information and report back concisely.",
        "role": "leaf",
        "tools_allow": ["web_search", "web_fetch", "read_file", "list_dir"]
      },
      {
        "id": "coder",
        "identity": "You are a Coder. Write and edit code to complete tasks.",
        "role": "leaf",
        "tools_allow": ["read_file", "write_file", "edit_file", "exec", "list_dir"]
      }
    ]
  }
}
```

Then run normally — the orchestrator gets `sessions_spawn` and `sessions_control` tools automatically:

```bash
./build/ottie gateway          # via Telegram/Slack/etc.
./build/ottie agent            # interactive CLI
./build/ottie agent -m "Spawn researcher to find the latest Go version"
```

### Mode B: Multi-Bot Telegram Group (multiple processes)

Multiple Ottie instances run as separate bots in the same Telegram group, coordinating via a shared ProjectBoard.

**Bot 1** (`~/.ottie/config.json`):
```json
{
  "swarm": { "enabled": true, "instance_id": "coder-bot" },
  "agents": { "list": [{ "id": "main", "identity": "You are the Coder." }] },
  "channels": { "telegram": { "enabled": true, "token": "BOT1_TOKEN" } }
}
```

**Bot 2** (`~/.ottie-researcher/config.json`):
```json
{
  "swarm": { "enabled": true, "instance_id": "researcher-bot" },
  "agents": { "list": [{ "id": "main", "identity": "You are the Researcher." }] },
  "channels": { "telegram": { "enabled": true, "token": "BOT2_TOKEN" } }
}
```

Run both:
```bash
OTTIE_HOME=~/.ottie ./build/ottie gateway &
OTTIE_HOME=~/.ottie-researcher ./build/ottie gateway &
```

Each bot gets a `project_board` tool for posting/claiming tasks, sharing artifacts, and handing off work via @mentions.

See [docs/multi-agent.md](docs/multi-agent.md) for the full guide.

## Build Targets

```bash
make build              # Current platform
make build-all          # All platforms
make build-linux-arm64  # Raspberry Pi 64-bit
make test               # Run tests
make install            # Install to ~/.local/bin
```

## Acknowledgments

Ottie is purpose-built for Ethereum and crypto, standing on the shoulders of these open-source projects:

| Project | Contribution |
|---------|-------------|
| [OpenClaw](https://github.com/openclaw/openclaw) | Core agent architecture foundations. Ottie diverges with domain-constrained security, self-evolving skills, and blockchain-native capabilities |
| [PicoClaw](https://github.com/sipeed/picoclaw) | Original project that Ottie evolved from: multi-channel chat, skills engine, and embedded device support |
| [autoresearch](https://github.com/karpathy/autoresearch) | Automated research skill for literature review and experiment pipelines |
| [agency-agents](https://github.com/msitarzewski/agency-agents) | Multi-agent role patterns used in the agency-roles skill |
| [chromedp](https://github.com/chromedp/chromedp) | Go library for driving headless Chrome, powering the render_html tool |
| [chromedp/headless-shell](https://github.com/chromedp/docker-headless-shell) | Minimal headless Chrome Docker image for the `ottie:full` container |
| [Lightpanda](https://github.com/lightpanda-io/browser) | Next-gen headless browser (Zig-based), tracked as a future lightweight alternative |

### Skills & References

The crypto/DeFi skills integrate with free APIs from:
[CoinGecko](https://www.coingecko.com/) ·
[DefiLlama](https://defillama.com/) ·
[DexScreener](https://dexscreener.com/) ·
[ParaSwap](https://www.paraswap.io/) ·
[Jupiter](https://jup.ag/) ·
[Aave](https://aave.com/) ·
[Lido](https://lido.fi/) ·
[Etherscan](https://etherscan.io/) ·
[Hyperliquid](https://hyperliquid.xyz/) ·
[Snapshot](https://snapshot.org/)

## License

AGPL-3.0 — see [LICENSE](LICENSE) for details.
