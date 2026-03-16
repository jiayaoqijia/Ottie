---
name: defi-lending
description: >
  Check DeFi lending and borrowing rates, supply/borrow APY, positions, health factors,
  and protocol comparisons. Use when asked about lending rates, borrowing costs, Aave rates,
  Compound APY, Morpho markets, lending protocol comparison, health factor, or liquidation risk.
---

# DeFi Lending

Query lending/borrowing rates, positions, and protocol data across major DeFi lending platforms.

## APIs

### DefiLlama Yields (Free, no auth) — Best starting point

Base: `https://yields.llama.fi`

**All lending pools with APY**:
```
web_fetch url="https://yields.llama.fi/pools"
```

> Returns ~10k+ pools. Filter client-side by `project` and `chain`.
> Key fields: `apy` (supply APY), `apyBorrow` (borrow APY), `tvlUsd`, `pool` (unique ID)

**Single pool detail + history**:
```
web_fetch url="https://yields.llama.fi/chart/POOL_ID"
```

**Filter tips**: Search response for `"project":"aave-v3"`, `"project":"compound-v3"`, `"project":"morpho"`, `"project":"spark"`.

### Aave V3 Subgraph (Free, no auth)

**Get all reserves with rates**:
```
exec command="curl -s -X POST 'https://api.thegraph.com/subgraphs/name/aave/protocol-v3' -H 'Content-Type: application/json' -d '{\"query\":\"{reserves(first:100){name symbol underlyingAsset liquidityRate variableBorrowRate stableBorrowRate availableLiquidity totalCurrentVariableDebt totalDeposits reserveFactor baseLTVasCollateral reserveLiquidationThreshold}}\"}"
```

> Rates are in RAY (27 decimals). Convert: `rate / 1e27 * 100` = APY%

**User positions on Aave**:
```
exec command="curl -s -X POST 'https://api.thegraph.com/subgraphs/name/aave/protocol-v3' -H 'Content-Type: application/json' -d '{\"query\":\"{userReserves(where:{user:\\\"0xUSER_ADDRESS_LOWERCASE\\\"}){reserve{symbol}currentATokenBalance currentVariableDebt currentStableDebt}}\"}"
```

**Multi-chain Aave subgraph endpoints**:
| Chain | Subgraph |
|-------|----------|
| Ethereum | `aave/protocol-v3` |
| Arbitrum | `aave/protocol-v3-arbitrum` |
| Optimism | `aave/protocol-v3-optimism` |
| Polygon | `aave/protocol-v3-polygon` |
| Base | `aave/protocol-v3-base` |
| Avalanche | `aave/protocol-v3-avalanche` |

See `references/aave-v3.md` for contract addresses per chain.

### Aave V3 On-Chain via RPC

**Get user account data** (health factor, total collateral/debt):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2\",\"data\":\"0xbf92857c000000000000000000000000USER_ADDRESS_NO_0x\"},\"latest\"],\"id\":1}'"
```

> Returns: totalCollateralBase, totalDebtBase, availableBorrowsBase, currentLiquidationThreshold, ltv, healthFactor
> All values in base currency (USD with 8 decimals). Health factor has 18 decimals.
> Health factor < 1.0 = liquidatable. Warn user if < 1.5.

### Morpho (GraphQL API)

**Get Morpho Blue markets**:
```
exec command="curl -s -X POST 'https://blue-api.morpho.org/graphql' -H 'Content-Type: application/json' -d '{\"query\":\"{markets(first:20 orderBy:TotalSupplyUsd orderDirection:Desc){items{uniqueKey loanAsset{symbol address} collateralAsset{symbol address} lltv state{supplyApy borrowApy totalSupplyUsd totalBorrowUsd utilization}}}}\"}"
```

**Morpho Vaults (curated lending)**:
```
exec command="curl -s -X POST 'https://blue-api.morpho.org/graphql' -H 'Content-Type: application/json' -d '{\"query\":\"{vaults(first:20 orderBy:TotalAssetsUsd orderDirection:Desc){items{name symbol address chain{id} state{totalAssetsUsd apy netApy fee} asset{symbol}}}}\"}"
```

### Compound V3 (Comet)

**Get base asset rates via RPC** (getSupplyRate, getBorrowRate):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xc3d688B66703497DAA19211EEdff47f25384cdc3\",\"data\":\"0xd955b861\"},\"latest\"],\"id\":1}'"
```

> Compound V3 Comet (USDC market) on Ethereum: `0xc3d688B66703497DAA19211EEdff47f25384cdc3`
> `0xd955b861` = `getSupplyRate(0)`, `0xa5b4ff79` = `getBorrowRate(0)`
> Rate is per-second. APY = `(1 + rate/1e18)^(365*24*3600) - 1`

## Usage Tips

- Start with DefiLlama yields for a quick comparison across all protocols
- Use Aave subgraph for detailed position data and historical rates
- Health factor = total collateral * liquidation threshold / total debt
- When comparing rates, note: supply APY includes incentive rewards on DefiLlama (`apyReward`)
- Morpho typically offers better rates than Aave/Compound by matching lenders to borrowers
- For large positions, check utilization rate — high utilization can affect withdrawal liquidity
