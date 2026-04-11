---
name: defi-yield
description: >
  Find yield farming opportunities, compare APY across DeFi protocols, analyze yield strategies,
  and check risk metrics. Use when asked about best yields, yield farming, APY comparison,
  Pendle fixed yield, Curve pools, liquidity mining rewards, yield strategy, or where to earn
  the highest return on crypto.
---

# DeFi Yield

Yield farming opportunities, APY comparisons, strategy guidance, and risk metrics.

## APIs

### DefiLlama Yields (Free, no auth) — Primary source

Base: `https://yields.llama.fi`

**All yield pools** (10k+ pools across all DeFi):
```
web_fetch url="https://yields.llama.fi/pools"
```

Key response fields:
- `pool` — unique pool ID (use for chart endpoint)
- `project` — protocol name (e.g., `aave-v3`, `curve-dex`, `pendle`)
- `chain` — chain name
- `symbol` — pool token(s)
- `tvlUsd` — total value locked in USD
- `apy` — total APY (base + reward)
- `apyBase` — base yield (swap fees, interest)
- `apyReward` — incentive/farming rewards
- `apyMean30d` — 30-day average APY
- `stablecoin` — whether pool is stablecoin-only
- `ilRisk` — impermanent loss risk level
- `exposure` — `single` or `multi` asset

**Pool history** (daily APY over time):
```
web_fetch url="https://yields.llama.fi/chart/POOL_ID"
```

**Filtering strategies** (client-side):
- Stablecoin yields: filter `stablecoin: true`
- Low IL risk: filter `ilRisk: "no"` or `exposure: "single"`
- High TVL (safer): filter `tvlUsd > 10000000`
- Specific chain: filter `chain: "Ethereum"` or `chain: "Arbitrum"`

### Pendle API (Free, no auth)

Base: `https://api-v2.pendle.finance/core`

**Active markets** (fixed yield opportunities):
```
web_fetch url="https://api-v2.pendle.finance/core/v1/1/markets?order_by=name&skip=0&limit=20"
```

**Market details** (implied APY, maturity):
```
web_fetch url="https://api-v2.pendle.finance/core/v1/1/markets/MARKET_ADDRESS"
```

**Multi-chain markets**:
| Chain | Chain ID |
|-------|----------|
| Ethereum | 1 |
| Arbitrum | 42161 |
| BSC | 56 |
| Optimism | 10 |
| Mantle | 5000 |

Replace `1` in URL with chain ID.

Key Pendle concepts:
- **PT (Principal Token)** — buy at discount, redeem at par at maturity = fixed yield
- **YT (Yield Token)** — leveraged yield exposure until maturity
- **Implied APY** — the fixed rate from buying PT
- **Underlying APY** — current floating yield of the underlying asset

### Curve Finance API (Free, no auth)

**All pools with APY**:
```
web_fetch url="https://api.curve.fi/v1/getPools/all/ethereum"
```

**Volume and fees**:
```
web_fetch url="https://api.curve.fi/v1/getVolumes/ethereum"
```

Supported chains: `ethereum`, `arbitrum`, `optimism`, `base`, `polygon`, `avalanche`, `fantom`

### Convex Finance

**All pools (Curve + Convex rewards)**:
```
web_fetch url="https://www.convexfinance.com/api/curve-apys"
```

## Yield Strategy Guide

### Low Risk (Stablecoins)

1. **Lending** — Supply USDC/USDT to Aave/Compound (2-8% APY)
2. **Curve stable pools** — USDC/USDT/DAI pools (2-6% base + CRV rewards)
3. **Pendle PT** — Buy PT-aUSDC for fixed yield until maturity (4-10%)
4. **Morpho vaults** — Curated lending vaults (3-10% APY)

### Medium Risk (Blue-chip)

1. **LST yield** — Hold wstETH/rETH (3-5% staking APY)
2. **LST LP** — wstETH/ETH pools on Curve/Balancer (3-8%)
3. **Pendle PT-wstETH** — Fixed staking yield at premium (5-10%)
4. **Aave recursive** — Supply wstETH, borrow ETH, re-supply (leveraged staking)

### Higher Risk (Maximizing)

1. **Incentive farming** — New protocol token rewards (volatile, high APY)
2. **Pendle YT** — Leveraged yield exposure (can go to zero at maturity)
3. **Concentrated LP** — Uniswap V3 narrow range (high fees, high IL)
4. **Points farming** — EigenLayer, Ethena, new protocols (uncertain value)

## Risk Assessment Checklist

When recommending yields, always consider:

1. **Smart contract risk** — Is the protocol audited? How long has it been live?
2. **Impermanent loss** — Multi-asset pools lose value if prices diverge
3. **Reward sustainability** — High APY from token emissions decreases over time
4. **TVL trend** — Declining TVL can signal problems
5. **Depeg risk** — Stablecoin or LST depegging from underlying
6. **Maturity risk** (Pendle) — PT has fixed maturity; early exit may lose yield
7. **Protocol-specific** — Governance attacks, oracle failures, admin keys

## Usage Tips

- DefiLlama is the single best source for cross-protocol APY comparison
- `apyBase` is more sustainable than `apyReward` (rewards get diluted)
- `apyMean30d` is more reliable than current `apy` for forecasting
- For "where should I put my USDC", filter DefiLlama for stablecoin pools with TVL > $10M
- Always note the chain — same protocol can have very different yields on different chains
- When reporting yields, distinguish between base APY and reward APY
- Warn users about IL risk for volatile pair pools
