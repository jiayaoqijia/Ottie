---
name: mpp
description: >
  Machine Payments Protocol (MPP) for agent-to-service payments. Pay for APIs, LLM inference,
  image generation, and web search via HTTP 402. Use when asked about machine payments,
  paying for API calls, MPP protocol, Tempo wallet, or agent commerce.
---

# Machine Payments Protocol (MPP)

The open protocol for machine-to-machine payments. Agents pay for services in the same HTTP request — no API keys, no signups, no billing accounts.

## How It Works

1. Client requests a paid resource
2. Server returns `402 Payment Required` with supported payment methods
3. Client pays and retries with a `Credential` (proof of payment)
4. Server verifies payment, returns the resource with a `Receipt` (proof of delivery)

## Quick Start with Tempo Wallet (Recommended)

Paste this into your agent to set up Tempo Wallet:
```
Read https://tempo.xyz/SKILL.md and set up tempo
```

Or manually:

**Install CLI**:
```
exec command="curl -fsSL https://tempo.xyz/install | bash"
```

**Connect wallet**:
```
exec command="tempo wallet login"
```

**List available services**:
```
exec command="tempo wallet services"
```

**Make a paid request**:
```
exec command="tempo request -X POST --json '{\"prompt\": \"a sunset over the ocean\"}' https://fal.mpp.tempo.xyz/fal-ai/flux/dev"
```

## Quick Start with mppx CLI

**Install**:
```
exec command="npm install -g mppx"
```

**Create account**:
```
exec command="mppx account create"
```

**Make a paid request**:
```
exec command="mppx https://mpp.dev/api/ping/paid"
```

## Payment Methods

| Method | Type | Use Case |
|--------|------|----------|
| **Tempo** | Stablecoin (USDC) | Recommended. One-time charges and sessions |
| **Stripe** | Cards, wallets | Traditional payment methods |
| **Lightning** | Bitcoin (BOLT11) | Bitcoin payments over Lightning Network |
| **Card** | Encrypted tokens | Direct card payments |
| **Custom** | Build your own | Extend with custom payment flows |

## Payment Intents

| Intent | Description |
|--------|-------------|
| **Charge** | One-time payment per request |
| **Session** | Pay-as-you-go with payment channels (low-cost, high-throughput) |
| **Stream** | Per-token billing over Server-Sent Events |

## SDKs

| Language | Package | Install |
|----------|---------|---------|
| TypeScript | `mppx` | `npm install mppx` |
| Python | `pympp` | `pip install pympp` |
| Rust | `mpp` | `cargo add mpp` |

## TypeScript Client Example

```typescript
import { Mppx, Method } from 'mppx'

// Create payment-aware fetch
const mppx = await Mppx.create({
  methods: [Method.tempo.charge(), Method.tempo.session()]
})

// Fetch automatically handles 402 → pay → retry
const res = await fetch('https://api.example.com/generate', {
  method: 'POST',
  body: JSON.stringify({ prompt: 'hello' })
})
```

## Use Cases for Agents

- **Pay for LLM inference** — call LLM providers via MPP, pay per token
- **Generate images** — request from fal.ai, pay per request
- **Search the web** — query Parallel for real-time results, pay per query
- **Agent-to-agent commerce** — agents pay each other for services (ERC-8183 compatible)
- **Self-sustaining agents** — combine with stETH treasury to fund operations from yield

## Integration with Ottie

Ottie agents can use MPP to:
1. Pay for external API calls autonomously (LLM, image gen, search)
2. Accept payments for Ottie's own skills as an MCP service
3. Combine with `steth-treasury` for yield-funded autonomous spending
4. Use `venice-private-ai` for private inference + MPP for paid public APIs

## Resources

- **Protocol spec**: https://paymentauth.org
- **Documentation**: https://mpp.dev
- **Tempo Wallet**: https://wallet.tempo.xyz
- **SDKs**: https://mpp.dev/sdk
