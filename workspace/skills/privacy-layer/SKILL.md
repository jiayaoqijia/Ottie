---
name: privacy-layer
description: >
  Privacy-preserving operations for crypto agents. Covers ZK-SNARK private transfers via
  Railgun Protocol, shielded balances, network egress monitoring, and confidential transaction
  patterns. Use when asked about private transfers, shielded transactions, or agent privacy.
---

# Privacy Layer

Privacy-preserving operations for AI agents handling sensitive financial data.

## ZK-SNARK Private Transfers (Railgun Protocol)

Railgun enables fully private ERC-20 transfers using ZK-SNARKs on Ethereum. Three core operations:

### Shield (Public → Private)
Move tokens from a public address into the Railgun privacy pool:
- Tokens become shielded — balance hidden from chain observers
- Agent receives a private note commitment
- On-chain: only the deposit amount and token are visible

### Private Transfer (Private → Private)
Transfer tokens within the privacy pool:
- Sender and receiver are hidden on-chain
- Amount is hidden via ZK proof
- Only the proof validity is verifiable

### Unshield (Private → Public)
Withdraw tokens from the privacy pool to a public address:
- Converts shielded balance back to standard ERC-20
- Receiver address becomes public

## Railgun Contract Addresses

| Network | Contract | Address |
|---------|----------|---------|
| Ethereum | Relay Adapt | `0xc3f2C8F9d5F0705De706b1302B7a039e1e11aC88` |
| Arbitrum | Relay Adapt | `0x5aD95C537b002770a39dea342c4bb2b68B1497aA` |
| Polygon | Relay Adapt | `0xc7FfA542736321A3dd69246d73987566a5486968` |
| BSC | Relay Adapt | `0x19B620929f97b7b990801496c3b361CA5bbC8E71` |

## Network Egress Monitoring

Monitor all outbound connections from the agent to detect data leaks:

- Track all RPC calls, HTTP requests, and WebSocket connections
- Classify targets as `external` vs `loopback`
- Log destination hosts, ports, and protocols
- Alert on unexpected outbound connections

## Agent Privacy Architecture

### 1. Secret Isolation
- Private keys never sent to LLM — all signing handled in isolated process
- Mnemonic stored with `0600` permissions (owner-only read)
- Config directory `0700` permissions

### 2. LLM Security Fence
- System prompt forbids revealing private keys, mnemonics, API keys
- Reduced tool surface — only expose necessary operations
- ClawWall DLP catches secrets before they reach the LLM

### 3. Private Inference
- Use Venice AI or local Ollama for zero-retention inference
- Sensitive data (portfolio values, strategies) processed without provider storage
- No training on agent queries

### 4. On-Chain Privacy
- Shield balances via Railgun before agent operations
- Private transfers between agent sub-wallets
- Unshield only when public settlement is needed

## Privacy Checklist for Agents

| Risk | Mitigation |
|------|------------|
| LLM sees private key | ClawWall DLP + system prompt fence |
| LLM provider stores queries | Venice (zero retention) or local Ollama |
| On-chain activity traced | Railgun shielded transfers |
| Network connections leak data | Egress monitoring + allowlist |
| Config files exposed | 0600/0700 permissions, fail-loud validation |
| Agent overshares in chat | Domain constraint — blockchain-only access |

## Usage Tips

- Shield tokens before any sensitive agent operations
- Use private transfers for inter-agent payments
- Monitor network egress during development to catch unexpected leaks
- Always use Venice or local models for strategy-sensitive reasoning
- Never expose raw portfolio data to third-party APIs without shielding
