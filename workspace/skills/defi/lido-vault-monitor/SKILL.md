---
name: lido-vault-monitor
description: >
  Monitor Lido Earn vault positions (EarnETH, EarnUSD) and staking positions. Track yield
  against benchmarks, detect allocation shifts, alert on health changes. Use when asked about
  Lido vault monitoring, yield tracking, stETH position alerts, or DeFi position management.
---

# Lido Vault Position Monitor

Monitor Lido staking positions and Earn vault allocations with plain-language alerts.

## Monitoring Targets

### 1. stETH Staking Position

**Current APR** (benchmark):
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
```

**Withdrawal queue depth** (affects unstaking time):
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/withdrawals/status"
```

**Protocol-wide stats**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/stats"
```

### 2. Benchmark Yield Comparison

Compare stETH APR against alternatives via DefiLlama:

**All staking/lending yields**:
```
web_fetch url="https://yields.llama.fi/pools"
```
> Filter for relevant pools:
> - `project: lido` — stETH base APR
> - `project: aave-v3`, `symbol: WETH` — Aave ETH supply rate
> - `project: compound-v3`, `symbol: WETH` — Compound ETH rate
> - `project: morpho-blue` — Morpho ETH vaults
> - `project: rocket-pool` — rETH APR

**Specific pool history** (30-day trend):
```
web_fetch url="https://yields.llama.fi/chart/POOL_UUID"
```

### 3. Lido Earn Vaults (Mellow)

Lido Earn vaults use Mellow Protocol to allocate across Aave, Morpho, Pendle, Gearbox, and Maple.

**Track vault allocations on-chain** — query vault contract for current strategy:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"VAULT_ADDRESS\",\"data\":\"0x01e1d114\"},\"latest\"],\"id\":1}'"
```
> `0x01e1d114` = `totalAssets()`. Returns total value in underlying token.

**Compare vault yield to benchmarks**:
```
web_fetch url="https://yields.llama.fi/pools"
```
> Filter: `project: mellow` for Mellow vault yields

### 4. DeFi Position Health (wstETH as collateral)

**Aave V3 health factor** for a wstETH position:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2\",\"data\":\"0xbf92857c000000000000000000000000USER_ADDRESS\"},\"latest\"],\"id\":1}'"
```
> `0xbf92857c` = `getUserAccountData(address)`. Returns: totalCollateral, totalDebt, availableBorrow, liquidationThreshold, LTV, healthFactor.

## Alert Conditions

Monitor and alert when:

| Condition | Threshold | Severity |
|-----------|-----------|----------|
| stETH APR drops below benchmark | APR < Aave WETH supply rate | Warning |
| APR drops >20% from 30-day average | Significant yield compression | Warning |
| Withdrawal queue grows >50k ETH | Extended unstaking delays | Info |
| Health factor < 1.5 (Aave collateral) | Liquidation risk rising | Critical |
| Health factor < 1.1 (Aave collateral) | Imminent liquidation | Emergency |
| Vault allocation shift detected | Strategy rebalance | Info |
| wstETH/stETH rate anomaly | >0.1% deviation from expected | Warning |

## Alert Message Format

Alerts should be plain language, explaining:
1. **What changed** — specific metric and direction
2. **Why it matters** — impact on the user's position
3. **What to do** — actionable recommendation (or "no action needed")

Example: "Your stETH APR dropped to 3.1% overnight, below the Aave WETH supply rate of 3.4%. This means holding stETH currently yields less than lending WETH on Aave. Consider whether to maintain your staking position or reallocate. No urgent action needed — APR fluctuations are normal."

## MCP Tool Schema

Expose as MCP-callable tools for other agents:

| Tool | Description |
|------|-------------|
| `lido_vault_health` | Get comprehensive vault position summary |
| `lido_yield_comparison` | Compare stETH APR against DeFi benchmarks |
| `lido_withdrawal_eta` | Estimate withdrawal completion time |
| `lido_position_summary` | Full position report (balance, yield, health) |
| `lido_alert_check` | Run all alert conditions, return triggered alerts |

## Monitoring Schedule

- **APR & yield comparison**: every 6 hours
- **Withdrawal queue**: every 1 hour
- **Health factor** (if collateral): every 15 minutes
- **Vault allocations**: every 12 hours
- **Exchange rate**: every 1 hour

## Usage Tips

- Use DefiLlama as primary yield data source — it normalizes APR/APY across protocols
- stETH APR includes consensus + execution layer rewards; varies daily due to MEV tips
- For Lido Earn vaults, track both the vault's composite yield AND the underlying protocol allocations
- When alerting on yield drops, check if it's a protocol-wide event (e.g., ETH staking APR compression) or vault-specific
- Withdrawal queue depth is a leading indicator of market stress — spikes often precede sell pressure
