---
name: crypto-cex
description: >
  Get centralized exchange market data, order books, trading pairs, ticker prices, and 24h
  volume across Binance, Coinbase, Kraken, Bybit, Gate.io, and Bitget. Use when asked about
  CEX prices, exchange order book, trading pairs, CEX volume, Binance price, Kraken ticker,
  exchange comparison, or funding rates on centralized exchanges.
---

# Crypto CEX

Centralized exchange public market data across 6 major exchanges. All endpoints below are
public (no auth required) and return JSON.

## Symbol Format by Exchange

| Exchange | Format | Example (BTC/USDT) |
|----------|--------|-------------------|
| Binance | `BTCUSDT` | No separator |
| Coinbase | `BTC-USDT` | Hyphen |
| Kraken | `XBTUSDT` | XBT for BTC |
| Bybit | `BTCUSDT` | No separator |
| Gate.io | `BTC_USDT` | Underscore |
| Bitget | `BTCUSDT` | No separator |

## APIs

### Binance (Free, no auth)

Base: `https://api.binance.com`

**Ticker price**:
```
web_fetch url="https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT"
```

**24h ticker stats**:
```
web_fetch url="https://api.binance.com/api/v3/ticker/24hr?symbol=BTCUSDT"
```

**Order book** (depth):
```
web_fetch url="https://api.binance.com/api/v3/depth?symbol=BTCUSDT&limit=10"
```

**Klines (candlesticks)**:
```
web_fetch url="https://api.binance.com/api/v3/klines?symbol=BTCUSDT&interval=1d&limit=30"
```

**All trading pairs**:
```
web_fetch url="https://api.binance.com/api/v3/exchangeInfo"
```

**Futures funding rate**:
```
web_fetch url="https://fapi.binance.com/fapi/v1/fundingRate?symbol=BTCUSDT&limit=1"
```

### Coinbase (Free, no auth)

Base: `https://api.exchange.coinbase.com`

**Ticker**:
```
web_fetch url="https://api.exchange.coinbase.com/products/BTC-USDT/ticker"
```

**24h stats**:
```
web_fetch url="https://api.exchange.coinbase.com/products/BTC-USDT/stats"
```

**Order book**:
```
web_fetch url="https://api.exchange.coinbase.com/products/BTC-USDT/book?level=1"
```

**All products**:
```
web_fetch url="https://api.exchange.coinbase.com/products"
```

### Kraken (Free, no auth)

Base: `https://api.kraken.com/0/public`

**Ticker**:
```
web_fetch url="https://api.kraken.com/0/public/Ticker?pair=XBTUSDT"
```

**OHLC**:
```
web_fetch url="https://api.kraken.com/0/public/OHLC?pair=XBTUSDT&interval=1440"
```

**Order book**:
```
web_fetch url="https://api.kraken.com/0/public/Depth?pair=XBTUSDT&count=10"
```

**Asset pairs**:
```
web_fetch url="https://api.kraken.com/0/public/AssetPairs"
```

> Note: Kraken uses `XBT` instead of `BTC`, `XXBT` in some responses.

### Bybit (Free, no auth)

Base: `https://api.bybit.com`

**Ticker**:
```
web_fetch url="https://api.bybit.com/v5/market/tickers?category=spot&symbol=BTCUSDT"
```

**Order book**:
```
web_fetch url="https://api.bybit.com/v5/market/orderbook?category=spot&symbol=BTCUSDT&limit=10"
```

**Klines**:
```
web_fetch url="https://api.bybit.com/v5/market/kline?category=spot&symbol=BTCUSDT&interval=D&limit=30"
```

**Futures funding rate**:
```
web_fetch url="https://api.bybit.com/v5/market/tickers?category=linear&symbol=BTCUSDT"
```
> `fundingRate` field in response

### Gate.io (Free, no auth)

Base: `https://api.gateio.ws/api/v4`

**Ticker**:
```
web_fetch url="https://api.gateio.ws/api/v4/spot/tickers?currency_pair=BTC_USDT"
```

**Order book**:
```
web_fetch url="https://api.gateio.ws/api/v4/spot/order_book?currency_pair=BTC_USDT&limit=10"
```

**Candlesticks**:
```
web_fetch url="https://api.gateio.ws/api/v4/spot/candlesticks?currency_pair=BTC_USDT&interval=1d&limit=30"
```

**All pairs**:
```
web_fetch url="https://api.gateio.ws/api/v4/spot/currency_pairs"
```

### Bitget (Free, no auth)

Base: `https://api.bitget.com`

**Ticker**:
```
web_fetch url="https://api.bitget.com/api/v2/spot/market/tickers?symbol=BTCUSDT"
```

**Order book**:
```
web_fetch url="https://api.bitget.com/api/v2/spot/market/orderbook?symbol=BTCUSDT&limit=10"
```

**Candlesticks**:
```
web_fetch url="https://api.bitget.com/api/v2/spot/market/candles?symbol=BTCUSDT&granularity=1day&limit=30"
```

## External Tools (Optional)

- **Kraken CLI**: `github.com/krakenfx/kraken-cli` — official Go binary with built-in MCP, supports trading
- **Bitget Agent Hub**: `npx bitget-mcp-server` — official, 36 tools for trading and market data
- **Binance MCP**: `npx @snjyor/binance-mcp@latest` — Binance market data and trading via MCP
- **Bybit MCP**: `npx bybit-mcp-server` — Bybit market data via MCP

## Cross-Exchange Comparison

For price comparison across exchanges, fetch tickers from multiple exchanges in parallel
and present in a table. Example approach:

1. Fetch BTC price from all 6 exchanges simultaneously
2. Compare bid/ask spreads
3. Note volume differences
4. Flag significant price discrepancies (potential arbitrage)

## Usage Tips

- Public endpoints need no authentication — use for prices, order books, volume
- For private endpoints (trading, balances), see `references/cex-auth.md`
- Always use the correct symbol format per exchange (see table above)
- Binance has the highest volume for most pairs — use as price reference
- Funding rates indicate market sentiment: positive = longs pay shorts (bullish bias)
- Rate limits vary: Binance ~1200/min, Coinbase ~10/sec, Kraken ~1/sec
- For kline intervals: `1d` or `1D` or `1440` depending on exchange
