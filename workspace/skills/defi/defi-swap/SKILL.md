---
name: defi-swap
description: >
  Get DEX swap quotes, price routes, and trade execution data across multiple chains. Use when
  asked about swapping tokens, best swap price, DEX aggregator quotes, token swap routes,
  trading on Uniswap/Jupiter/ParaSwap/1inch, slippage, or comparing swap rates across DEXes.
---

# DeFi Swap

DEX swap quotes and routing across EVM chains and Solana using free aggregator APIs.

## APIs

### ParaSwap (Free, no auth)

Base: `https://api.paraswap.io`

**Get swap quote** (prices only, no tx data):
```
web_fetch url="https://api.paraswap.io/prices?srcToken=0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE&destToken=0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48&amount=1000000000000000000&srcDecimals=18&destDecimals=6&side=SELL&network=1"
```

**Get transaction data** (for execution):
```
exec command="curl -s -X POST 'https://api.paraswap.io/transactions/1' -H 'Content-Type: application/json' -d '{\"srcToken\":\"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE\",\"destToken\":\"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\",\"srcAmount\":\"1000000000000000000\",\"slippage\":100,\"userAddress\":\"0xUSER\",\"priceRoute\":PRICE_ROUTE_FROM_ABOVE}'"
```

Supported chains (network IDs):
| Chain | ID | Chain | ID |
|-------|----|-------|----|
| Ethereum | 1 | Arbitrum | 42161 |
| Optimism | 10 | Base | 8453 |
| Polygon | 137 | BSC | 56 |
| Avalanche | 43114 | Fantom | 250 |

Native token address (all EVM): `0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE`

### Jupiter / Solana (Free, no auth)

Base: `https://quote-api.jup.ag/v6`

**Get swap quote**:
```
web_fetch url="https://quote-api.jup.ag/v6/quote?inputMint=So11111111111111111111111111111111111111112&outputMint=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&amount=1000000000&slippageBps=50"
```

**Get swap transaction** (for execution):
```
exec command="curl -s -X POST 'https://quote-api.jup.ag/v6/swap' -H 'Content-Type: application/json' -d '{\"quoteResponse\":QUOTE_FROM_ABOVE,\"userPublicKey\":\"USER_PUBKEY\",\"wrapAndUnwrapSol\":true}'"
```

**Token list** (for mint address lookup):
```
web_fetch url="https://token.jup.ag/strict"
```

### Uniswap Trading API (Requires API key)

Base: `https://trade-api.gateway.uniswap.org/v1`

> Requires header `x-api-key: YOUR_API_KEY` — get one at https://developers.uniswap.org/dashboard/

**Check token approval**:
```
exec command="curl -s -X POST 'https://trade-api.gateway.uniswap.org/v1/check_approval' -H 'Content-Type: application/json' -H 'x-api-key: YOUR_API_KEY' -d '{\"walletAddress\":\"0xUSER\",\"token\":\"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\",\"amount\":\"1000000000\",\"chainId\":1}'"
```

**Get swap quote**:
```
exec command="curl -s -X POST 'https://trade-api.gateway.uniswap.org/v1/quote' -H 'Content-Type: application/json' -H 'x-api-key: YOUR_API_KEY' -d '{\"type\":\"EXACT_INPUT\",\"amount\":\"1000000000000000000\",\"tokenIn\":\"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE\",\"tokenOut\":\"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48\",\"tokenInChainId\":1,\"tokenOutChainId\":1,\"swapper\":\"0xUSER\",\"slippageTolerance\":0.5}'"
```

**Execute swap (gasful, on-chain)**:
```
exec command="curl -s -X POST 'https://trade-api.gateway.uniswap.org/v1/swap' -H 'Content-Type: application/json' -H 'x-api-key: YOUR_API_KEY' -d '{\"quote\":QUOTE_FROM_ABOVE,\"signature\":\"0xSIGNED_PERMIT2\",\"permitData\":PERMIT_DATA_FROM_QUOTE}'"
```

**Execute swap (gasless, UniswapX)**:
```
exec command="curl -s -X POST 'https://trade-api.gateway.uniswap.org/v1/order' -H 'Content-Type: application/json' -H 'x-api-key: YOUR_API_KEY' -d '{\"quote\":QUOTE_FROM_ABOVE,\"signature\":\"0xSIGNED_ORDER\"}'"
```

**Check swap status**:
```
web_fetch url="https://trade-api.gateway.uniswap.org/v1/swaps?txHash=0xTX_HASH" headers="x-api-key: YOUR_API_KEY"
```

Supported chains (17+ mainnets):
| Chain | ID | Chain | ID |
|-------|----|-------|----|
| Ethereum | 1 | Arbitrum | 42161 |
| Optimism | 10 | Base | 8453 |
| Polygon | 137 | BSC | 56 |
| Avalanche | 43114 | zkSync | 324 |
| Blast | 81457 | Zora | 7777777 |
| Unichain | 130 | Celo | 42220 |
| Linea | 59144 | World Chain | 480 |

UniswapX gasless swaps available on: Ethereum (1), Arbitrum (42161), Base (8453), Unichain (130).

Native token address: `0x0000000000000000000000000000000000000000`

### 1inch (Requires API key)

Base: `https://api.1inch.dev/swap/v6.0/{chainId}`

**Get quote**:
```
web_fetch url="https://api.1inch.dev/swap/v6.0/1/quote?src=0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE&dst=0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48&amount=1000000000000000000"
```
> Requires header `Authorization: Bearer YOUR_API_KEY`

### 0x / Matcha (Requires API key)

**Get quote**:
```
web_fetch url="https://api.0x.org/swap/v1/quote?buyToken=USDC&sellToken=ETH&sellAmount=1000000000000000000"
```
> Requires header `0x-api-key: YOUR_KEY`

## Common Token Addresses

See `references/evm-tokens.md` and `references/solana-tokens.md` for full lists.

### Quick Reference — Ethereum Mainnet

| Token | Address |
|-------|---------|
| WETH | `0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2` |
| USDC | `0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48` |
| USDT | `0xdAC17F958D2ee523a2206206994597C13D831ec7` |
| DAI | `0x6B175474E89094C44Da98b954EedeAC495271d0F` |
| WBTC | `0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599` |

### Quick Reference — Solana

| Token | Mint |
|-------|------|
| SOL (wrapped) | `So11111111111111111111111111111111111111112` |
| USDC | `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` |
| USDT | `Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB` |

## External Tools (Optional)

- **EVM MCP Server**: `npx @mcpdotdirect/evm-mcp-server` — swap execution across 60+ EVM chains
- **Solana Agent Kit**: `github.com/sendaifun/solana-mcp` — Solana swap execution via Jupiter

## Usage Tips

- Always quote amounts in smallest unit (wei for ETH = 10^18, lamports for SOL = 10^9)
- `slippage` in ParaSwap is basis points (100 = 1%). Jupiter uses `slippageBps`.
- ParaSwap is best free option for EVM; Jupiter is best for Solana
- For price-only queries, use quote endpoints (no tx data needed)
- Compare ParaSwap vs 1inch quotes for best execution on large trades
- Native token address `0xEeee...` is the same across all EVM chains in ParaSwap
