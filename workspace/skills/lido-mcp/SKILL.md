---
name: lido-mcp
description: >
  Build and use an MCP server for Lido staking operations. Covers stETH staking, wstETH
  wrapping, withdrawal queue, balance queries, governance, and vault monitoring.
  Use when building Lido MCP tools, staking integrations, or monitoring Lido positions.
---

# Lido MCP Server

Reference for building or using an MCP server that makes Lido staking operations natively callable by AI agents.

## Core Operations

### 1. Stake ETH → stETH

Send ETH to the Lido contract's `submit()` function:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84\",\"data\":\"0xa1903eab0000000000000000000000000000000000000000000000000000000000000000\",\"value\":\"0xde0b6b3a7640000\"},\"latest\"],\"id\":1}'"
```
> `0xa1903eab` = `submit(address _referral)`. Send ETH as `value`. Returns shares minted.

### 2. Wrap stETH → wstETH

Approve stETH then call `wrap()`:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0\",\"data\":\"0xea598cb00000000000000000000000000000000000000000000000000000000000000000\"},\"latest\"],\"id\":1}'"
```
> `0xea598cb0` = `wrap(uint256 _stETHAmount)`. Returns wstETH amount.

### 3. Unwrap wstETH → stETH

```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0\",\"data\":\"0xde0e9a3e0000000000000000000000000000000000000000000000000000000000000000\"},\"latest\"],\"id\":1}'"
```
> `0xde0e9a3e` = `unwrap(uint256 _wstETHAmount)`. Returns stETH amount.

### 4. Request Withdrawal (Unstake)

```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x889edC2eDab5f40e902b864aD4d7AdE8E412F9B1\",\"data\":\"0xd6681042\"},\"latest\"],\"id\":1}'"
```
> Withdrawal Queue: `0x889edC2eDab5f40e902b864aD4d7AdE8E412F9B1`
> Key functions: `requestWithdrawals(uint256[] amounts, address owner)`, `claimWithdrawals(uint256[] requestIds, uint256[] hints)`

## Balance & Rate Queries

**stETH balance**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84\",\"data\":\"0x70a08231000000000000000000000000USER_ADDRESS_HERE\"},\"latest\"],\"id\":1}'"
```

**wstETH/stETH exchange rate**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0\",\"data\":\"0x035faf82\"},\"latest\"],\"id\":1}'"
```
> `0x035faf82` = `stEthPerToken()`. Divide result by 1e18.

**Shares balance** (underlying stETH shares):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84\",\"data\":\"0xf5eb42dc000000000000000000000000USER_ADDRESS_HERE\"},\"latest\"],\"id\":1}'"
```
> `0xf5eb42dc` = `sharesOf(address)`.

**Total pooled ETH**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84\",\"data\":\"0x37cfdaca\"},\"latest\"],\"id\":1}'"
```
> `0x37cfdaca` = `getTotalPooledEther()`.

## Lido REST APIs (Free, no auth)

### APR API (`https://eth-api.lido.fi`)

**Current APR (7-day SMA)**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
```

**Latest APR**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/apr/last"
```

**Protocol stats**:
```
web_fetch url="https://eth-api.lido.fi/v1/protocol/steth/stats"
```

### Withdrawal Queue API (`https://wq-api.lido.fi`)

**Queue wait time**:
```
web_fetch url="https://wq-api.lido.fi/v2/request-time/calculate"
```

**Estimated time for specific amount**:
```
web_fetch url="https://wq-api.lido.fi/v2/request-time/calculate?amount=32"
```

**Time for specific request IDs**:
```
web_fetch url="https://wq-api.lido.fi/v2/request-time?ids=1&ids=2"
```

### Reward History API (`https://reward-history-backend.lido.fi`)

**Staking rewards for an address**:
```
web_fetch url="https://reward-history-backend.lido.fi/?address=0xUSER&currency=USD&onlyRewards=true"
```

## Governance (Aragon DAO)

| Contract | Address |
|----------|---------|
| Lido DAO (Aragon) | `0xb8FFC3Cd6e7Cf5a098A1c92F48009765B24088Dc` |
| LDO Token | `0x5A98FcBEA516Cf06857215779Fd812CA3beF1B32` |
| Voting | `0x2e59A20f205bB85a89C53f1936454680651E618e` |
| Token Manager | `0xf73a1260d222f447210581DDf212D915c09a3249` |
| Finance | `0xB9E5CBB9CA5b0d659238807E84D0176930753d86` |
| Agent (Treasury) | `0x3e40D73EB977Dc6a537aF587D48316feE66E9C8c` |

**Query active proposals** via Snapshot:
```
exec command="curl -s -X POST 'https://hub.snapshot.org/graphql' -H 'Content-Type: application/json' -d '{\"query\":\"{proposals(first:5,where:{space:\\\"lido-snapshot.eth\\\",state:\\\"active\\\"},orderBy:\\\"created\\\",orderDirection:desc){id title state start end scores_total}}\"}'
```

## Contract Addresses

### Ethereum Mainnet

| Contract | Address |
|----------|---------|
| Lido (stETH Proxy) | `0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84` |
| wstETH | `0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0` |
| Withdrawal Queue | `0x889edC2eDab5f40e902b864aD4d7AdE8E412F9B1` |
| Staking Router | `0xFdDf38947aFB03C621C71b06C9C70bce73f12999` |
| Accounting Oracle | `0x852deD011285fe67063a08005c71a85690503Cee` |
| Lido Locator | `0xC1d0b3DE6792Bf6b4b37EccdcC24e45978Cfd2Eb` |
| Vault Hub | `0x1d201BE093d847f6446530Efb0E8Fb426d176709` |

### Lido Earn Vaults (Mellow)

| Vault | Address |
|-------|---------|
| strETH Vault | `0x277C6A642564A91ff78b008022D65683cEE5CCC5` |
| DVstETH Vault | `0x5E362eb2c0706Bd1d134689eC75176018385430B` |

### wstETH on L2s

| Network | wstETH Address |
|---------|---------------|
| Optimism | `0x1F32b1c2345538c0c6f582fCB022739c4A194Ebb` |
| Arbitrum | `0x5979D7b546E38E414F7E9822514be443A4800529` |
| Base | `0xc1CBa3fCea344f92D9239c08C0568f6F2F0ee452` |
| Polygon | `0x03b54A6e9a984069379fae1a4fC4dBAE93B3bCCD` |
| Scroll | `0xf610A9dfB7C89644979b4A0f27063E9e7d7Cda32` |
| zkSync Era | `0x703b52F2b28fEbcB60E1372858AF5b18849FE867` |
| Mantle | `0x458ed78EB972a369799fb278c0243b25e5242A83` |
| Linea | `0xB5beDd42000b71FddE22D3eE8a79Bd49A568fC8F` |
| BNB Chain | `0x26c5e01524d2E6280A48F2c50fF6De7e52E9611C` |
| Mode | `0x98f96A4B34D03a2E6f225B28b8f8Cb1279562d81` |
| Zircuit | `0xf0e673Bc224A8Ca3ff67a61605814666b1234833` |
| Unichain | `0xc02fE7317D4eb8753a02c35fe019786854A92001` |

### Chainlink Price Feeds

| Feed | Address | Network |
|------|---------|---------|
| wstETH/USD | `0x8b6851156023f4f5a66f68bea80851c3d905ac93` | Ethereum |
| wstETH/stETH | `0xB88BAc61a4Ca37C43a3725912B1f472c9A5bc061` | Base |
| wstETH/stETH | `0xB1552C5e96B312d0Bf8b554186F846C40614a540` | Arbitrum |
| wstETH/stETH | `0xe59EBa0D492cA53C6f46015EEa00517F2707dc77` | Optimism |

## MCP Tool Schema

Recommended MCP tools for a Lido server:

| Tool | Description | dry_run |
|------|-------------|---------|
| `lido_stake` | Stake ETH → stETH | Yes |
| `lido_wrap` | Wrap stETH → wstETH | Yes |
| `lido_unwrap` | Unwrap wstETH → stETH | Yes |
| `lido_request_withdrawal` | Request unstake from queue | Yes |
| `lido_claim_withdrawal` | Claim completed withdrawal | Yes |
| `lido_balance` | Query stETH/wstETH balances | N/A |
| `lido_rewards` | Query staking rewards earned | N/A |
| `lido_apr` | Get current staking APR | N/A |
| `lido_exchange_rate` | Get wstETH/stETH rate | N/A |
| `lido_withdrawal_status` | Check withdrawal queue | N/A |
| `lido_governance_proposals` | List active governance votes | N/A |
| `lido_vote` | Cast governance vote | Yes |

All write operations must support `dry_run: true` to simulate without executing.

## Key Integration Notes

- **stETH rebases daily** — holder balances change. Use `sharesOf()` for stable accounting.
- **wstETH does NOT rebase** — preferred for DeFi, L2 bridging, and agent wallets.
- **1-2 wei rounding**: stETH transfers may lose 1-2 wei due to shares math. Always use `transferShares()` for exact amounts.
- **Withdrawal queue**: requests are NFTs (ERC-721). Min 100 wei, max 1000 stETH per request. Finalization takes 1-5 days.
- **Staking limits**: call `getCurrentStakeLimit()` before staking. If limit insufficient, swap on DEX instead.
- **L2 wstETH**: bridged via canonical bridges. Same non-rebasing behavior as mainnet.
- **Withdrawal functions**: `requestWithdrawalsWstETH()` accepts wstETH directly. Use `getWithdrawalStatus()` to check finalization.
- **LDO token quirk**: returns `false` instead of reverting on transfer failure — always check return value.
