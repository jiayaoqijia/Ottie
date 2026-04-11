---
name: self-agent-id
description: >
  Integrate Self Protocol Agent ID for ZK-powered, human-backed AI agent identity.
  Covers registration, verification, soulbound NFTs, Sybil resistance, and A2A patterns.
  Use when asked about agent identity, proof-of-human, Self Protocol, or ZK identity.
---

# Self Agent ID

ZK-powered identity for AI agents via Self Protocol. Agents get soulbound ERC-721 NFTs verified through zero-knowledge passport authentication.

## How It Works

1. Human scans passport via Self app (ZK proof generated locally on phone)
2. Hub V2 verifies the proof on-chain and notifies the registry
3. Registry mints a soulbound NFT linking agent's key to unique human nullifier
4. Agents sign requests via SDK; services verify against on-chain registry

No personal data leaves the user's device. Only a ZK proof and nullifier are submitted on-chain.

## Registry Addresses

| Network | Chain ID | Registry Address |
|---------|----------|------------------|
| Celo Mainnet | 42220 | `0xaC3DF9ABf80d0F5c020C06B04Cced27763355944` |
| Celo Sepolia | 11142220 | `0x043DaCac8b0771DD5b444bCC88f2f8BBDBEdd379` |

Three on-chain registries (ERC-8004 compatible):
- **SelfAgentRegistry** — Core identity (soulbound ERC-721 + proof-of-human)
- **SelfReputationRegistry** — Aggregated reputation feedback
- **SelfValidationRegistry** — Third-party validation

## REST API

Base: `https://selfagentid.xyz/api`

**Get agent info**:
```
web_fetch url="https://selfagentid.xyz/api/agent/info/42220/AGENT_ID"
```

**List agents by wallet**:
```
web_fetch url="https://selfagentid.xyz/api/agent/agents/42220/0xWALLET_ADDRESS"
```

**Verify agent**:
```
web_fetch url="https://selfagentid.xyz/api/agent/verify/42220/AGENT_ID"
```

**Get agent card (A2A)**:
```
web_fetch url="https://selfagentid.xyz/api/cards/42220/AGENT_ID"
```

**Register agent**:
```
exec command="curl -s -X POST 'https://selfagentid.xyz/api/agent/register' -H 'Content-Type: application/json' -d '{\"mode\":\"linked\",\"network\":\"mainnet\",\"humanAddress\":\"0xWALLET\",\"disclosures\":{\"age\":true,\"ofac\":true}}'"
```

## Registration Modes

| Mode | Key Type | Wallet Required | Best For |
|------|----------|-----------------|----------|
| `linked` | ECDSA | Yes (human) | Autonomous agents with human oversight |
| `wallet-free` | ECDSA | No | Non-crypto users, embedded agents |
| `ed25519` | Ed25519 | No | OpenClaw, Eliza frameworks |
| `ed25519-linked` | Ed25519 | Yes (human) | Ed25519 agents with human custody |
| `privy` | Social login | No | Google/Twitter/email auth |
| `smartwallet` | Passkey | No | Biometric auth, gasless revocation |

## Verification Patterns

### Agent-to-Service
Agent signs HTTP requests with headers:
- `x-self-agent-address` — Agent's Ethereum address
- `x-self-agent-signature` — ECDSA signature
- `x-self-agent-timestamp` — Unix timestamp

### Agent-to-Agent
Mutual verification between two agents. `sameHuman()` detects if two agents share the same human backer without revealing identity.

### Agent-to-Chain
Smart contracts check `isVerifiedAgent()` on the registry directly. For gasless: agents sign EIP-712 typed data, relayer submits.

## SDK Installation

| Language | Package | Install |
|----------|---------|---------|
| TypeScript | `@selfxyz/agent-sdk` | `npm install @selfxyz/agent-sdk` |
| Python | `selfxyz-agent-sdk` | `pip install selfxyz-agent-sdk` |
| Rust | `self-agent-sdk` | `cargo add self-agent-sdk` |

## Integration with Ottie

Ottie can integrate Self Agent ID for:
- **Sybil-resistant identity**: each agent backed by unique human
- **A2A verification**: verify other agents before DeFi interactions
- **On-chain reputation**: build verifiable trust history
- **Proof-of-human gating**: restrict sensitive operations to human-verified agents
