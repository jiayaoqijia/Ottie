# Aave V3 Contract Addresses

## Core Protocol Contracts

### Pool (main entry point for supply/borrow/repay/withdraw)

| Chain | Pool Address |
|-------|-------------|
| Ethereum | `0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2` |
| Arbitrum | `0x794a61358D6845594F94dc1DB02A252b5b4814aD` |
| Optimism | `0x794a61358D6845594F94dc1DB02A252b5b4814aD` |
| Polygon | `0x794a61358D6845594F94dc1DB02A252b5b4814aD` |
| Base | `0xA238Dd80C259a72e81d7e4664a9801593F98d1c5` |
| Avalanche | `0x794a61358D6845594F94dc1DB02A252b5b4814aD` |
| BSC | `0x6807dc923806fE8Fd134338EABCA509979a7e0cB` |

### PoolDataProvider (read reserve/user data)

| Chain | Address |
|-------|---------|
| Ethereum | `0x7B4EB56E7CD4b454BA8ff71E4518426c73786d18` |
| Arbitrum | `0x69FA688f1Dc47d4B5d8029D5a35FB7a548310654` |
| Optimism | `0x69FA688f1Dc47d4B5d8029D5a35FB7a548310654` |
| Polygon | `0x69FA688f1Dc47d4B5d8029D5a35FB7a548310654` |
| Base | `0x2d8A3C5677189723C4cB8873CfC9C8976FDF38Ac` |
| Avalanche | `0x69FA688f1Dc47d4B5d8029D5a35FB7a548310654` |

### Oracle (price feeds for collateral valuation)

| Chain | Address |
|-------|---------|
| Ethereum | `0x54586bE62E3c3580375aE3723C145253060Ca0C2` |
| Arbitrum | `0xb56c2F0B653B2e0b10C9b928C8580Ac5Df02C7C7` |
| Optimism | `0xb023e699F5a33916Ea823A16485e259257cA8Bd1` |
| Polygon | `0xb023e699F5a33916Ea823A16485e259257cA8Bd1` |

## Key Function Selectors

| Function | Selector | Description |
|----------|----------|-------------|
| `getUserAccountData(address)` | `0xbf92857c` | Returns collateral, debt, health factor |
| `getReserveData(address)` | `0x35ea6a75` | Returns reserve config, rates, tokens |
| `supply(address,uint256,address,uint16)` | `0x617ba037` | Supply asset to pool |
| `borrow(address,uint256,uint256,uint16,address)` | `0xa415bcad` | Borrow from pool |
| `repay(address,uint256,uint256,address)` | `0x573ade81` | Repay borrowed asset |
| `withdraw(address,uint256,address)` | `0x69328dec` | Withdraw supplied asset |

## getUserAccountData Return Values

Returns 6 uint256 values (each 32 bytes):
1. `totalCollateralBase` — total collateral in base currency (USD, 8 decimals)
2. `totalDebtBase` — total debt in base currency (USD, 8 decimals)
3. `availableBorrowsBase` — remaining borrow capacity (USD, 8 decimals)
4. `currentLiquidationThreshold` — weighted avg liquidation threshold (4 decimals, e.g., 8250 = 82.5%)
5. `ltv` — weighted avg loan-to-value (4 decimals)
6. `healthFactor` — health factor (18 decimals, < 1e18 = liquidatable)

## Aave V3 aToken Addresses (Ethereum)

| Asset | aToken |
|-------|--------|
| WETH | `0x4d5F47FA6A74757f35C14fD3a6Ef8E3C9BC514E8` |
| USDC | `0x98C23E9d8f34FEFb1B7BD6a91B7FF122F4e16F5c` |
| USDT | `0x23878914EFE38d27C4D67Ab83ed1b93A74D4086a` |
| DAI | `0x018008bfb33d285247A21d44E50697654f754e63` |
| WBTC | `0x5Ee5bf7ae06D1Be5997A1A72006FE6C607eC6DE8` |
| wstETH | `0x0B925eD163218f6662a35e0f0371Ac234f9E9371` |
| LINK | `0x5E8C8A7243651DB1384C0dDfDbE39761E8e7E51a` |
| GHO | Borrowable only (no aToken) |

## Rate Conversions

- Rates from subgraph are in **RAY** (1e27). To get APY%: `rate / 1e27 * 100`
- Rates from RPC `getReserveData` are also in RAY
- Health factor from `getUserAccountData` is 18 decimals: divide by 1e18
- Base currency values (USD) have 8 decimals: divide by 1e8
