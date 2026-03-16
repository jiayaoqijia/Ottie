---
name: crypto-research
description: >
  Research cryptocurrency tokens, verify smart contracts, check perpetual futures data,
  governance proposals, and on-chain analytics. Use when asked about token research, contract
  verification, is this token safe, rug pull check, Hyperliquid perps, funding rates, governance
  vote, Dune analytics, token due diligence, or security audit status.
---

# Crypto Research

Token due diligence, contract verification, perpetual futures data, governance, and analytics.

## APIs

### Etherscan Contract Verification (Free tier)

Base: `https://api.etherscan.io/api`

**Check if contract is verified**:
```
web_fetch url="https://api.etherscan.io/api?module=contract&action=getabi&address=0xCONTRACT&apikey=YOUR_KEY"
```
> `status: "1"` = verified, `status: "0"` = not verified. Unverified contracts are a red flag.

**Get contract source code**:
```
web_fetch url="https://api.etherscan.io/api?module=contract&action=getsourcecode&address=0xCONTRACT&apikey=YOUR_KEY"
```
> Returns: source code, compiler version, optimization settings, license

**Token info** (supply, holders):
```
web_fetch url="https://api.etherscan.io/api?module=token&action=tokeninfo&contractaddress=0xCONTRACT&apikey=YOUR_KEY"
```

**Multi-chain explorers** (same API format):
| Chain | Base URL |
|-------|----------|
| Ethereum | `api.etherscan.io` |
| Arbitrum | `api.arbiscan.io` |
| Base | `api.basescan.org` |
| Optimism | `api-optimistic.etherscan.io` |
| Polygon | `api.polygonscan.com` |
| BSC | `api.bscscan.com` |

### Hyperliquid (Free, no auth)

Base: `https://api.hyperliquid.xyz`

**All perpetual markets (meta)**:
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"meta\"}'"
```

**All market stats (24h volume, funding, open interest)**:
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"metaAndAssetCtxs\"}'"
```

**Order book for a specific asset**:
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"l2Book\",\"coin\":\"BTC\"}'"
```

**Funding rate history**:
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"fundingHistory\",\"coin\":\"BTC\",\"startTime\":START_UNIX_MS}'"
```

**User positions** (if address known):
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"clearinghouseState\",\"user\":\"0xUSER_ADDRESS\"}'"
```

**Candle data**:
```
exec command="curl -s -X POST https://api.hyperliquid.xyz/info -H 'Content-Type: application/json' -d '{\"type\":\"candleSnapshot\",\"coin\":\"BTC\",\"interval\":\"1d\",\"startTime\":START_UNIX_MS,\"endTime\":END_UNIX_MS}'"
```

### Snapshot Governance (Free, no auth)

Base: `https://hub.snapshot.org/graphql`

**Active proposals for a DAO**:
```
exec command="curl -s -X POST https://hub.snapshot.org/graphql -H 'Content-Type: application/json' -d '{\"query\":\"{proposals(first:5 where:{space:\\\"aave.eth\\\" state:\\\"active\\\"} orderBy:\\\"created\\\" orderDirection:desc){id title body choices start end state scores scores_total}}\"}"
```

**Recent proposals across all DAOs**:
```
exec command="curl -s -X POST https://hub.snapshot.org/graphql -H 'Content-Type: application/json' -d '{\"query\":\"{proposals(first:10 orderBy:\\\"created\\\" orderDirection:desc where:{state:\\\"active\\\"}){id title space{id name} state end scores_total}}\"}"
```

Common Snapshot space IDs:
| Protocol | Space ID |
|----------|----------|
| Aave | `aave.eth` |
| Uniswap | `uniswap` |
| Compound | `comp-vote.eth` |
| Arbitrum | `arbitrumfoundation.eth` |
| Optimism | `opcollective.eth` |
| ENS | `ens.eth` |
| Lido | `lido-snapshot.eth` |
| Curve | `curve.eth` |

### Dune Analytics (API key required)

**Execute saved query**:
```
exec command="curl -s -X POST 'https://api.dune.com/api/v1/query/QUERY_ID/execute' -H 'X-Dune-API-Key: YOUR_KEY'"
```

**Get query results**:
```
web_fetch url="https://api.dune.com/api/v1/query/QUERY_ID/results?api_key=YOUR_KEY"
```

> Popular public query IDs change — search Dune.com for the latest.

### CoinGecko Token Detail (for research)

**Comprehensive token info** (description, links, dev activity, community):
```
web_fetch url="https://api.coingecko.com/api/v3/coins/TOKEN_ID?localization=false&tickers=false&community_data=true&developer_data=true"
```

## Token Due Diligence Checklist

When researching a token, check the following:

### 1. Contract Verification
- Is the contract verified on Etherscan? (Unverified = major red flag)
- Is it a proxy contract? Check implementation address too
- What license is it under?

### 2. Token Distribution
- Check top holders on Etherscan token page
- Is ownership renounced or is there an admin/owner?
- Are there mint functions or pause functions?

### 3. Liquidity
- Check DEX liquidity on DexPaprika or DexScreener
- Is liquidity locked? For how long?
- What's the daily trading volume?

### 4. Protocol Metrics
- TVL trend on DefiLlama (declining = warning)
- Revenue vs token emissions (check if sustainable)
- Audit reports (check project website/GitHub)

### 5. Community & Development
- GitHub activity (use CoinGecko developer_data)
- Social following (CoinGecko community_data)
- Governance activity on Snapshot

### 6. Red Flags
- Unverified contract
- Top holder owns >20% of supply
- No audit
- Anonymous team with no track record
- Unrealistic APY promises
- Honeypot (can buy but can't sell)

## External Tools (Optional)

- **Hyperliquid MCP**: `github.com/edkdev/hyperliquid-mcp` — trade and query Hyperliquid
- **BNB Chain MCP**: `npx @bnb-chain/mcp@latest` — BSC/opBNB chain data
- **Arbitrum Vibekit**: `github.com/EmberAGI/arbitrum-vibekit` — Arbitrum DeFi framework

## Usage Tips

- Always check contract verification before interacting with any new token
- Cross-reference prices across CoinGecko, DEX, and CEX for consistency
- For perps research, compare Hyperliquid funding rates with CEX funding rates
- Governance proposals can signal major protocol changes — monitor for invested positions
- Use DefiLlama TVL trends as a leading indicator of protocol health
- When analyzing memecoins, focus on liquidity and holder distribution over fundamentals
