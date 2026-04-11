# CoinGecko Top 200 Token IDs

Use these IDs with the CoinGecko API `ids` parameter. IDs are lowercase slug format.

## Top 50 by Market Cap

| Symbol | CoinGecko ID | Symbol | CoinGecko ID |
|--------|-------------|--------|-------------|
| BTC | bitcoin | ETH | ethereum |
| USDT | tether | BNB | binancecoin |
| SOL | solana | USDC | usd-coin |
| XRP | ripple | DOGE | dogecoin |
| ADA | cardano | TRX | tron |
| AVAX | avalanche-2 | SHIB | shiba-inu |
| DOT | polkadot | LINK | chainlink |
| TON | the-open-network | BCH | bitcoin-cash |
| NEAR | near | UNI | uniswap |
| LTC | litecoin | MATIC | matic-network |
| ICP | internet-computer | DAI | dai |
| APT | aptos | ETC | ethereum-classic |
| RNDR | render-token | ATOM | cosmos |
| HBAR | hedera-hashgraph | FIL | filecoin |
| IMX | immutable-x | ARB | arbitrum |
| STX | blockstack | OP | optimism |
| MKR | maker | INJ | injective-protocol |
| VET | vechain | GRT | the-graph |
| THETA | theta-token | FTM | fantom |
| ALGO | algorand | RUNE | thorchain |
| SEI | sei-network | SUI | sui |
| AAVE | aave | FLOW | flow |
| AXS | axie-infinity | SAND | the-sandbox |
| MANA | decentraland | SNX | havven |

## DeFi Tokens (51-100)

| Symbol | CoinGecko ID | Symbol | CoinGecko ID |
|--------|-------------|--------|-------------|
| CRV | curve-dao-token | LDO | lido-dao |
| COMP | compound-governance-token | YFI | yearn-finance |
| SUSHI | sushi | BAL | balancer |
| 1INCH | 1inch | DYDX | dydx |
| ENS | ethereum-name-service | RPL | rocket-pool |
| FXS | frax-share | PENDLE | pendle |
| GMX | gmx | MORPHO | morpho |
| ENA | ethena | EIGEN | eigenlayer |
| JUP | jupiter-exchange-solana | RAY | raydium |
| ORCA | orca | DRIFT | drift-protocol |
| JTO | jito-governance-token | PYTH | pyth-network |
| W | wormhole | TIA | celestia |
| STRK | starknet | ZK | zksync |
| MANTA | manta-network | BLAST | blast |
| MODE | mode | SCROLL | scroll |

## Layer 1 & Infrastructure (101-150)

| Symbol | CoinGecko ID | Symbol | CoinGecko ID |
|--------|-------------|--------|-------------|
| KAS | kaspa | EGLD | elrond-erd-2 |
| KAVA | kava | ZIL | zilliqa |
| ROSE | oasis-network | ONE | harmony |
| CELO | celo | MINA | mina-protocol |
| XTZ | tezos | EOS | eos |
| NEO | neo | IOTA | iota |
| QTUM | qtum | ZEN | zencash |
| XLM | stellar | CKB | nervos-network |
| CFX | conflux-token | BEAM | beam-2 |
| TONCOIN | the-open-network | TAO | bittensor |
| FET | fetch-ai | AGIX | singularitynet |
| OCEAN | ocean-protocol | OLAS | autonolas |
| WLD | worldcoin-wld | ARKM | arkham |
| GALA | gala | APE | apecoin |
| BLUR | blur | MAGIC | magic |

## Stablecoins & Wrapped

| Symbol | CoinGecko ID |
|--------|-------------|
| USDT | tether |
| USDC | usd-coin |
| DAI | dai |
| FRAX | frax |
| LUSD | liquity-usd |
| crvUSD | crvusd |
| GHO | gho |
| PYUSD | paypal-usd |
| WBTC | wrapped-bitcoin |
| WETH | weth |
| stETH | staked-ether |
| wstETH | wrapped-steth |
| rETH | rocket-pool-eth |
| cbETH | coinbase-wrapped-staked-eth |

## Meme Tokens

| Symbol | CoinGecko ID |
|--------|-------------|
| DOGE | dogecoin |
| SHIB | shiba-inu |
| PEPE | pepe |
| FLOKI | floki |
| BONK | bonk |
| WIF | dogwifcoin |
| BOME | book-of-meme |
| MEW | cat-in-a-dogs-world |
| POPCAT | popcat |
| TURBO | turbo |

## Notes

- IDs can change; verify with `https://api.coingecko.com/api/v3/search?query=TOKEN_NAME`
- For tokens not listed, search first then cache the ID
- Some tokens share symbols — always use the CoinGecko ID, not the ticker
- Wrapped/bridged versions have separate IDs (e.g., `weth` vs `ethereum`)
