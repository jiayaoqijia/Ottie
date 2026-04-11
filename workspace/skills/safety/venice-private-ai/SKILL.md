---
name: venice-private-ai
description: >
  Use Venice AI for private, no-data-retention LLM inference. OpenAI-compatible API with
  privacy guarantees — no prompts, completions, or user data stored. Use when asked about
  private AI inference, confidential reasoning, Venice API, or zero-retention LLM providers.
---

# Venice Private AI

Private LLM inference via Venice AI — an OpenAI-compatible API with zero data retention.

## Why Venice for Agents

Venice provides "private cognition" — the LLM processes sensitive data (portfolio values, trading strategies, wallet addresses) without retaining any of it. This is critical for agents handling:
- Confidential treasury management
- Private governance analysis
- Sensitive due diligence on DeFi positions
- Deal negotiation without data leakage

## API (OpenAI-compatible)

Base: `https://api.venice.ai/api/v1`

> Uses standard OpenAI API format. Requires `Authorization: Bearer YOUR_VENICE_API_KEY`.

**Chat completion** (private, zero retention):
```
exec command="curl -s -X POST 'https://api.venice.ai/api/v1/chat/completions' -H 'Content-Type: application/json' -H 'Authorization: Bearer YOUR_KEY' -d '{\"model\":\"venice-uncensored\",\"messages\":[{\"role\":\"user\",\"content\":\"Analyze this DeFi position...\"}]}'"
```

**List models**:
```
web_fetch url="https://api.venice.ai/api/v1/models" headers="Authorization: Bearer YOUR_KEY"
```

## Available Models

| Model | Type | Use Case |
|-------|------|----------|
| `venice-uncensored` | Text | General reasoning, uncensored |
| `llama-3.3-70b` | Text | Strong general purpose |
| `qwen-2.5-coder` | Code | Code generation |
| `deepseek-r1-671b` | Text | Deep reasoning |

## Ottie Integration

Venice works as a drop-in provider via Ottie's OpenAI-compatible provider. Config:

```json
{
  "model_list": [{
    "model_name": "venice-private",
    "model": "venice/venice-uncensored",
    "api_base": "https://api.venice.ai/api/v1",
    "api_key": "YOUR_VENICE_KEY"
  }]
}
```

## Privacy Guarantees

- **Zero data retention**: no prompts, completions, or metadata stored
- **No training on user data**: queries never used for model training
- **No logging**: inference requests are not logged
- **Decentralized**: inference runs on distributed GPU infrastructure

## Privacy Architecture for Agents

Combine Venice with Ottie's security model:

1. **ClawWall DLP** — prevents sensitive data (private keys, mnemonics) from reaching any LLM
2. **Venice inference** — zero-retention processing of financial data
3. **Domain constraint** — agent cannot access email, files, browser — only blockchain
4. **On-chain verification** — all actions produce verifiable on-chain receipts

## Use Cases

- **Private treasury copilot**: analyze portfolio without exposing holdings to LLM provider
- **Confidential governance**: evaluate DAO proposals using private voting preferences
- **Risk assessment**: process position data without metadata leakage
- **Multi-agent coordination**: agents share sensitive analysis via private inference channels
