---
name: defi-staking
description: >
  Check liquid staking APR, exchange rates, withdrawal queues, and staking rewards for ETH
  and other tokens. Use when asked about staking ETH, Lido stETH, Rocket Pool rETH, staking
  APR, liquid staking rates, staking rewards, unstaking queue, or staking comparison.
---

# DeFi Staking

Liquid staking APR, exchange rates, withdrawal queues, and protocol comparisons.

## APIs

### Lido API (Free, no auth)

Base: `https://eth-api.lido.fi`

**Current stETH APR**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
```

**Withdrawal queue status**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/withdrawals/status"
```

**Lido stats (TVL, stakers, etc.)**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/stats"
```

### DefiLlama (Free, no auth)

**All liquid staking pools with APY**:
```
web_fetch url="https://yields.llama.fi/pools"
```
> Filter by `project` containing: `lido`, `rocket-pool`, `coinbase-wrapped-staked-eth`, `frax-ether`, `ether.fi`, `mantle-staked-eth`, `swell`, `binance-staked-eth`, `stakewise-v3`

**Specific protocol TVL**:
```
web_fetch url="https://api.llama.fi/protocol/lido"
```

### On-Chain Exchange Rates via RPC

**wstETH/stETH rate** (how much stETH per wstETH):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0\",\"data\":\"0x035faf82\"},\"latest\"],\"id\":1}'"
```
> `0x035faf82` = `stEthPerToken()`. Result is 18 decimals. Divide by 1e18.

**rETH/ETH rate** (how much ETH per rETH):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xae78736Cd615f374D3085123A210448E74Fc6393\",\"data\":\"0xe6aa216c\"},\"latest\"],\"id\":1}'"
```
> `0xe6aa216c` = `getExchangeRate()`. Result is 18 decimals. Divide by 1e18.

**cbETH/ETH rate**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xBe9895146f7AF43049ca1c1AE358B0541Ea49704\",\"data\":\"0xd68b2cb6\"},\"latest\"],\"id\":1}'"
```
> `0xd68b2cb6` = `exchangeRate()`. Result is 18 decimals.

## Contract Addresses

### Ethereum Mainnet

| Token | Address | Type |
|-------|---------|------|
| stETH | `0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84` | Lido staked ETH |
| wstETH | `0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0` | Wrapped stETH |
| rETH | `0xae78736Cd615f374D3085123A210448E74Fc6393` | Rocket Pool ETH |
| cbETH | `0xBe9895146f7AF43049ca1c1AE358B0541Ea49704` | Coinbase staked ETH |
| eETH | `0x35fA164735182de50811E8e2E824cFb9B6118ac2` | ether.fi staked ETH |
| weETH | `0xCd5fE23C85820F7B72D0926FC9b05b43E359b7ee` | Wrapped eETH |
| sfrxETH | `0xac3E018457B222d93114458476f3E3416Abbe38F` | Frax staked ETH |
| swETH | `0xf951E335afb289353dc249e82926178EaC7DEd78` | Swell staked ETH |
| mETH | `0xd5F7838F5C461fefF7FE49ea5ebaF7728bB0ADfa` | Mantle staked ETH |
| ETHx | `0xA35b1B31Ce002FBF2058D22F30f95D405200A15b` | Stader staked ETH |

### Lido Withdrawal Queue

| Contract | Address |
|----------|---------|
| WithdrawalQueue | `0x889edC2eDab5f40e902b864aD4d7AdE8E412F9B1` |

## Comparison Framework

When comparing liquid staking options, report:

1. **APR/APY** — current yield (from DefiLlama or protocol API)
2. **TVL** — total value locked (confidence indicator)
3. **Exchange rate** — current rate vs ETH (on-chain)
4. **DeFi composability** — which protocols accept the LST as collateral
5. **Withdrawal time** — instant vs queued vs epoch-based
6. **Fee** — protocol fee on rewards (Lido 10%, Rocket Pool 14%, etc.)

### Quick Composability Guide

| LST | Aave V3 | Morpho | Pendle | Curve |
|-----|---------|--------|--------|-------|
| wstETH | Yes | Yes | Yes | Yes |
| rETH | Yes | Yes | Yes | Yes |
| cbETH | Yes | Limited | Yes | Yes |
| weETH | Yes | Yes | Yes | Yes |
| sfrxETH | No | Yes | Yes | Yes |

## Usage Tips

- stETH rebases daily (balance changes); wstETH does not (value accrues to exchange rate)
- For DeFi use, wstETH > stETH because most protocols don't handle rebasing tokens well
- rETH has no rebase — exchange rate increases over time
- When reporting APR, specify whether it includes MEV tips and execution layer rewards
- Withdrawal times vary: Lido ~1-5 days, Rocket Pool varies, cbETH instant on Coinbase
- Check DefiLlama for the most accurate cross-protocol APR comparison
