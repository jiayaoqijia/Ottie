<div align="center">

  <img src="docs/ottie_banner.jpg" alt="Ottie Banner" width="100%">

  <h1>Ottie</h1>
  <h3>Self-Evolving Agent for Ethereum and Crypto</h3>

  <p>
    <a href="https://ottie.xyz"><img src="https://img.shields.io/badge/website-ottie.xyz-blue?style=flat" alt="Website"></a>
    <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/license-AGPL--3.0-blue" alt="License">
    <a href="https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111"><img src="https://img.shields.io/badge/ERC--8004-Agent_%231988-purple?style=flat" alt="ERC-8004"></a>
    <a href="https://drive.google.com/drive/folders/137-dvzsrpH6FjilDD7hjby519rIEnTZc?usp=drive_link"><img src="https://img.shields.io/badge/demos-10_tracks-green?style=flat" alt="Demos"></a>
  </p>

</div>

---

Ottie is a purpose-built AI agent for Ethereum and crypto, written in pure Go. Single binary, 36 blockchain-native skills, multi-agent swarms, 13+ messaging channels, and verified on-chain presence via ERC-8004. Where general-purpose agents bolt on wallet plugins, Ottie treats every interaction as if it might involve real money — with real on-chain transactions, deployed contracts, and verifiable execution logs.

**[Website](https://ottie.xyz)** · **[One-Click Launch](https://claw.altllm.ai/)** · **[Demo Videos](https://drive.google.com/drive/folders/137-dvzsrpH6FjilDD7hjby519rIEnTZc?usp=drive_link)** · **[8004scan Profile](https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111)**

## Features

- **Self-evolving skills** — learns from tasks and packages approaches as reusable skills
- **36 blockchain-native skills** — wallets, swaps, lending, staking, yield, CEX data, research, Lido MCP, vault monitoring, privacy, identity, treasury, payments
- **On-chain verified** — [ERC-8004 Agent #1988](https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111), deployed contracts, real Uniswap swaps on Sepolia
- **Lido MCP Server** — 14 tools: stake, wrap, unwrap, withdraw, balance, APR, governance, vault health — all with `dry_run`
- **Autonomous execution** — discover → plan → execute → verify → report pipeline
- **Privacy layer** — Venice AI zero-retention inference, Railgun ZK-SNARK private transfers
- **Agent identity** — ERC-8004 on-chain identity, Self Protocol ZK proof-of-human
- **13+ channels** — Telegram, Discord, Slack, Signal, WhatsApp, Matrix, QQ, DingTalk, LINE, WeCom, Feishu, IRC
- **Super light** — single Go binary (<10MB), zero CGO, sub-second startup
- **Multi-agent swarm** — sub-agent spawning and multi-bot Telegram coordination

## On-Chain Artifacts

| Artifact | Explorer |
|----------|----------|
| **ERC-8004 Agent #1988** | [8004scan.io](https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111) |
| **Uniswap ETH→USDC Swap** | [sepolia.etherscan.io](https://sepolia.etherscan.io/tx/0xfa31963bea66cd22318262090f79d716d8f9dd470326936462c1824cd12e001f) |
| **AgentTreasury Contract** | [sepolia.etherscan.io](https://sepolia.etherscan.io/address/0xc4EB945689E0A13832004a44C0A3292a33E2Fec0) |
| **ERC-8004 Registration TX** | [sepolia.etherscan.io](https://sepolia.etherscan.io/tx/0xd374384682074831fd1549ec89a564f5a34acd59148285cec84d32e41f4cfd7c) |

## Synthesis Hackathon Demos

**[All 10 track demo videos](https://drive.google.com/drive/folders/137-dvzsrpH6FjilDD7hjby519rIEnTZc?usp=drive_link)** — each recorded with real Ottie agent calls, step-by-step operations.

| Track | Demo |
|-------|------|
| Lido MCP Server | 10 ops: APR, stats, balance, stake, wrap, unwrap, withdraw, governance, vault health |
| Uniswap Swap | Real ETH→USDC swap on Sepolia via Trading API |
| Venice AI Private Agents | Model listing, zero-retention inference, on-chain action |
| Let the Agent Cook | 5-step autonomous: discover → plan → execute → verify → report |
| ERC-8004 Agent Identity | NFT mint, 8004scan verification, reputation registry |
| stETH Agent Treasury | Contract deploy, deposit, yield query, permission controls |
| Vault Monitor + Telegram | APR vs Aave benchmark, alert generation |
| Self Agent ID | ZK proof-of-human registration flow |
| ERC-8183 Agentic Commerce | Job escrow creation, evaluator attestation |
| Open Track | Full capability showcase with live data |

37/37 E2E tests passing: `bash demos/test-onchain.sh`

## Quick Start

```bash
git clone https://github.com/jiayaoqijia/ottie.git && cd ottie
make build
./build/ottie onboard
./build/ottie gateway
```

<details>
<summary><b>Lido MCP Server (14 tools)</b></summary>

A FastMCP server (`mcp-servers/lido-mcp/server.py`) implementing the full Lido staking protocol:

| Tool | Type | Description |
|------|------|-------------|
| `lido_apr` | read | stETH APR (7-day SMA) from Lido API |
| `lido_stats` | read | Protocol stats: TVL, holders, market cap |
| `lido_balance` | read | On-chain stETH balance for any address |
| `lido_exchange_rate` | read | wstETH/stETH exchange rate (on-chain) |
| `lido_rewards` | read | Reward history for an address |
| `lido_withdrawal_status` | read | Withdrawal queue status |
| `lido_governance_proposals` | read | Snapshot governance proposals |
| `lido_stake` | write+dry_run | Stake ETH → stETH |
| `lido_wrap` | write+dry_run | Wrap stETH → wstETH |
| `lido_unwrap` | write+dry_run | Unwrap wstETH → stETH |
| `lido_request_withdrawal` | write+dry_run | Request stETH withdrawal |
| `lido_vote` | write+dry_run | Vote on Snapshot proposals |
| `lido_vault_health` | read | Compare APR vs Aave benchmark |
| `lido_alert_check` | read | Evaluate alert conditions |

All write operations support `dry_run=true` for simulation via `eth_call`.

</details>

<details>
<summary><b>AgentTreasury Smart Contract</b></summary>

Deployed Solidity contract (`contracts/AgentTreasury.sol`) — humans deposit wstETH, agents can only spend accrued yield. Principal is locked.

- `deposit(amount, agent)` — human deposits, authorizes agent
- `queryYield(depositor)` — calculate available yield from exchange rate delta
- `spendYield(depositor, recipient, amount)` — agent spends yield only
- `withdrawPrincipal()` — depositor-only withdrawal
- `setAllowedRecipient(address, bool)` — whitelist spending targets
- `setPerTxCap(uint256)` — maximum per-transaction cap

Deployed on Sepolia: [0xc4EB945689E0A13832004a44C0A3292a33E2Fec0](https://sepolia.etherscan.io/address/0xc4EB945689E0A13832004a44C0A3292a33E2Fec0)

</details>

<details>
<summary><b>Crypto Skills (36 total)</b></summary>

| Layer | Skill | Capabilities | APIs |
|-------|-------|-------------|------|
| **Market Intelligence** | `crypto-market-data` | Prices, trending, market cap, Fear & Greed, TVL | CoinGecko, DefiLlama |
| | `crypto-cex` | Order books, funding rates, tickers across 6 exchanges | Binance, Coinbase, Kraken, Bybit, Gate.io, Bitget |
| | `crypto-research` | Contract verification, perps, governance | Etherscan, Hyperliquid, Snapshot |
| **On-Chain Operations** | `crypto-wallet` | Balances, holdings, tx history, approvals, ENS | Public RPCs, Etherscan |
| | `defi-swap` | DEX swap quotes, price routing, slippage | Uniswap, ParaSwap, Jupiter, 1inch |
| **Yield & Risk** | `defi-lending` | Lending rates, APY, health factors | Aave, Morpho, Compound, DefiLlama |
| | `defi-staking` | Liquid staking APR, exchange rates | Lido, Rocket Pool, DefiLlama |
| | `defi-yield` | Yield farming, APY comparison | DefiLlama, Pendle, Curve |
| **Infrastructure** | `lido-mcp` | 14-tool MCP server for Lido staking | Lido API, On-chain RPCs, Snapshot |
| | `lido-vault-monitor` | Vault position monitoring & alerts | DefiLlama, Lido API, Aave |
| | `steth-treasury` | Yield-bearing agent budgets, principal-protected | wstETH, Lido |
| **Privacy & Identity** | `venice-private-ai` | Zero-retention LLM inference | Venice AI API |
| | `privacy-layer` | ZK-SNARK private transfers, egress monitoring | Railgun Protocol |
| | `self-agent-id` | ZK proof-of-human agent identity, soulbound NFTs | Self Protocol, Celo |
| | `8004` | ERC-8004 agent identity, reputation, validation | Agent0 SDK, 8004scan |
| **Payments** | `mpp` | Machine-to-machine payments via HTTP 402 | MPP Protocol, Tempo |
| | `tempo` | Paid API discovery and requests with auto-payment | Tempo Wallet |

**Supported networks:** Ethereum, Arbitrum, Optimism, Base, Polygon, BSC, Avalanche, Fantom, Solana

</details>

## Configuration

Config lives at `~/.ottie/config.json`:

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
  },
  "tools": {
    "mcp": {
      "enabled": true,
      "servers": {
        "lido-mcp": {
          "enabled": true,
          "command": "python3",
          "args": ["mcp-servers/lido-mcp/server.py"]
        }
      }
    }
  }
}
```

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

13+ messaging platforms: **Telegram** · **Discord** · **Slack** · **Signal** · **WhatsApp** · **Matrix** · **QQ** · **DingTalk** · **LINE** · **WeCom** · **Feishu** · **IRC** · **Custom Webhooks**

## Multi-Agent Swarm

**Mode A — Sub-Agents** (single process): one orchestrator spawns specialized workers internally via `sessions_spawn`.

**Mode B — Multi-Bot Telegram** (multiple processes): separate Ottie instances coordinate via shared ProjectBoard in a Telegram group.

```json
{
  "agents": {
    "list": [
      { "id": "orchestrator", "default": true, "role": "orchestrator",
        "subagents": { "allow_agents": ["researcher", "coder"] } },
      { "id": "researcher", "role": "leaf",
        "tools_allow": ["web_search", "web_fetch", "read_file"] },
      { "id": "coder", "role": "leaf",
        "tools_allow": ["read_file", "write_file", "edit_file", "exec"] }
    ]
  }
}
```

<details>
<summary><b>Docker & Build</b></summary>

```bash
# Docker
git clone https://github.com/jiayaoqijia/ottie.git && cd ottie
docker compose -f docker/docker-compose.yml --profile gateway up

# Build from source
make build              # Current platform
make build-all          # All platforms
make test               # Run tests
```

</details>

## Acknowledgments

Built on [nanobot](https://github.com/HKUDS/nanobot), [OpenClaw](https://github.com/openclaw/openclaw), and [PicoClaw](https://github.com/sipeed/picoclaw). Integrates with [CoinGecko](https://www.coingecko.com/) · [DefiLlama](https://defillama.com/) · [Uniswap](https://uniswap.org/) · [Lido](https://lido.fi/) · [Aave](https://aave.com/) · [Venice AI](https://venice.ai/) · [ERC-8004](https://8004scan.io/) · [Self Protocol](https://self.xyz/) · [Snapshot](https://snapshot.org/) · [FastMCP](https://gofastmcp.com/) and more.

## License

AGPL-3.0 — see [LICENSE](LICENSE) for details.
