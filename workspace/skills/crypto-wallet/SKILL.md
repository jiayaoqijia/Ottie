---
name: crypto-wallet
description: >
  Check cryptocurrency wallet balances, token holdings, transaction history, token approvals,
  ENS name resolution, and on-chain activity. Use when asked about wallet balance, what tokens
  an address holds, transaction history, pending transactions, token allowances, ENS lookup,
  or address portfolio.
---

# Crypto Wallet

Query on-chain wallet data: balances, tokens, transactions, approvals, and ENS resolution.

## Common ERC-20 Function Selectors

| Function | Selector | Params |
|----------|----------|--------|
| `balanceOf(address)` | `0x70a08231` | address (32 bytes, left-padded) |
| `allowance(address,address)` | `0xdd62ed3e` | owner + spender (each 32 bytes) |
| `decimals()` | `0x313ce567` | none |
| `symbol()` | `0x95d89b41` | none |
| `name()` | `0x06fdde03` | none |
| `totalSupply()` | `0x18160ddd` | none |

## APIs

### ETH Balance via Public RPC

**Native ETH balance** (eth_getBalance):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBalance\",\"params\":[\"0xADDRESS\",\"latest\"],\"id\":1}'"
```

**ERC-20 token balance** (eth_call with balanceOf):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xTOKEN_CONTRACT\",\"data\":\"0x70a08231000000000000000000000000WALLET_NO_0x\"},\"latest\"],\"id\":1}'"
```

> Result is hex. Convert: `parseInt(result, 16) / 10^decimals`

### Multi-chain Public RPCs

| Chain | RPC URL |
|-------|---------|
| Ethereum | `https://eth.llamarpc.com` |
| Arbitrum | `https://arb1.arbitrum.io/rpc` |
| Optimism | `https://mainnet.optimism.io` |
| Base | `https://mainnet.base.org` |
| Polygon | `https://polygon-rpc.com` |
| BSC | `https://bsc-dataseed.binance.org` |
| Avalanche | `https://api.avax.network/ext/bc/C/rpc` |

### Etherscan / Block Explorers (Free tier, API key optional)

Base URLs:
- Ethereum: `https://api.etherscan.io/api`
- Arbitrum: `https://api.arbiscan.io/api`
- Base: `https://api.basescan.org/api`
- Optimism: `https://api-optimistic.etherscan.io/api`
- Polygon: `https://api.polygonscan.com/api`
- BSC: `https://api.bscscan.com/api`

**Token balances for address** (requires free API key):
```
web_fetch url="https://api.etherscan.io/api?module=account&action=tokentx&address=0xADDRESS&page=1&offset=50&sort=desc&apikey=YOUR_KEY"
```

**Normal transactions**:
```
web_fetch url="https://api.etherscan.io/api?module=account&action=txlist&address=0xADDRESS&startblock=0&endblock=99999999&page=1&offset=20&sort=desc&apikey=YOUR_KEY"
```

**ETH balance** (no key needed, rate limited):
```
web_fetch url="https://api.etherscan.io/api?module=account&action=balance&address=0xADDRESS&tag=latest"
```

### ENS Resolution

**Forward (name to address)** via RPC:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x4976fb03C32e5B8cfe2b6cCB31c09Ba78EBaBa41\",\"data\":\"0x3b3b57de{NAMEHASH}\"},\"latest\"],\"id\":1}'"
```

**Simpler: use ENS public API**:
```
web_fetch url="https://ensdata.net/ADDRESS_OR_NAME"
```

### Token Approvals

**Check allowance** (how much a spender can use):
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0xTOKEN\",\"data\":\"0xdd62ed3e000000000000000000000000OWNER_NO_0x000000000000000000000000SPENDER_NO_0x\"},\"latest\"],\"id\":1}'"
```

> Unlimited approval = `0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff`

## External Tools (Optional)

- **EVM MCP Server**: `npx @mcpdotdirect/evm-mcp-server` — read balances, tokens, NFTs across 60+ EVM chains
- **Base MCP**: `github.com/base/base-mcp` — Coinbase-official Base chain tools

## Usage Tips

- Always pad addresses to 32 bytes (64 hex chars) in eth_call data fields
- Strip `0x` prefix from address when embedding in calldata, then left-pad with zeros
- Results from eth_call are hex-encoded; parse with `parseInt(hex, 16)`
- For ERC-20 balances, divide by `10^decimals` (USDC=6, most tokens=18)
- For multi-token portfolios, use Etherscan tokentx endpoint rather than individual RPC calls
- Verify chain before querying — same address can exist on multiple chains with different balances
