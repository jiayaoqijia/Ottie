---
name: crypto-market-data
description: >
  Get cryptocurrency prices, market cap, trading volume, trending tokens, market sentiment,
  Fear & Greed Index, total value locked (TVL), and DeFi protocol rankings. Use when asked about
  token price, crypto market overview, what's trending, market sentiment, TVL by chain or protocol,
  top gainers/losers, or price history.
---

# Crypto Market Data

Retrieve real-time and historical crypto market data using free, no-auth APIs.

## APIs

### CoinGecko (Free, no auth for basic endpoints)

Base: `https://api.coingecko.com/api/v3`

**Current price** (up to 250 IDs per call):
```
web_fetch url="https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,solana&vs_currencies=usd&include_24hr_change=true&include_market_cap=true"
```

**Token detail** (includes description, links, market data):
```
web_fetch url="https://api.coingecko.com/api/v3/coins/bitcoin"
```

**Market list** (top N by market cap, paginated):
```
web_fetch url="https://api.coingecko.com/api/v3/coins/markets?vs_currency=usd&order=market_cap_desc&per_page=50&page=1&sparkline=false"
```

**Trending tokens** (top 7 by search volume):
```
web_fetch url="https://api.coingecko.com/api/v3/search/trending"
```

**Price history** (daily OHLC, max 365 days free):
```
web_fetch url="https://api.coingecko.com/api/v3/coins/bitcoin/ohlc?vs_currency=usd&days=30"
```

**Search by name/symbol**:
```
web_fetch url="https://api.coingecko.com/api/v3/search?query=pepe"
```

**Categories** (list all categories with market data):
```
web_fetch url="https://api.coingecko.com/api/v3/coins/categories"
```

> Rate limit: ~30 req/min (free). Use `include_market_cap=true` to avoid extra calls.
> See `references/coingecko-ids.md` for top 200 token IDs.

### DexPaprika (Free, no auth)

Base: `https://api.dexpaprika.com`

**Token price on DEXes** (by chain + address):
```
web_fetch url="https://api.dexpaprika.com/networks/ethereum/tokens/0xdac17f958d2ee523a2206206994597c13d831ec7"
```

**Top DEX pools**:
```
web_fetch url="https://api.dexpaprika.com/networks/ethereum/pools?sort=volume_usd&order=desc&limit=20"
```

**Supported networks**:
```
web_fetch url="https://api.dexpaprika.com/networks"
```

**OHLCV for a pool**:
```
web_fetch url="https://api.dexpaprika.com/networks/ethereum/pools/{pool_address}/ohlcv?interval=1d&limit=30"
```

### Fear & Greed Index (Free, no auth)

```
web_fetch url="https://api.alternative.me/fng/?limit=1"
```

Response: `{ "data": [{ "value": "73", "value_classification": "Greed", "timestamp": "..." }] }`

Historical (last 30 days):
```
web_fetch url="https://api.alternative.me/fng/?limit=30"
```

### DefiLlama (Free, no auth)

Base: `https://api.llama.fi`

**Total TVL across all protocols**:
```
web_fetch url="https://api.llama.fi/v2/historicalChainTvl"
```

**TVL by chain**:
```
web_fetch url="https://api.llama.fi/v2/chains"
```

**Protocol TVL + details**:
```
web_fetch url="https://api.llama.fi/protocol/aave"
```

**Top protocols by TVL**:
```
web_fetch url="https://api.llama.fi/protocols"
```

**Stablecoins market cap**:
```
web_fetch url="https://stablecoins.llama.fi/stablecoins?includePrices=true"
```

**DEX volumes** (24h):
```
web_fetch url="https://api.llama.fi/overview/dexs?excludeTotalDataChart=true&excludeTotalDataChartBreakdown=true"
```

## Usage Tips

- Always use CoinGecko IDs (not symbols) — e.g., `bitcoin` not `BTC`. See references for mapping.
- For on-chain DEX data (price by contract address), use DexPaprika.
- For protocol-level TVL and DeFi analytics, use DefiLlama.
- Fear & Greed Index is BTC-focused; mention this when reporting sentiment.
- Combine CoinGecko trending + Fear & Greed for a quick market overview.
