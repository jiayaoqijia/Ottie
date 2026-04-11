# Solana Token Mint Addresses

Common SPL token mint addresses for Jupiter swaps and Solana DeFi.

## Major Tokens

| Token | Mint Address | Decimals |
|-------|-------------|----------|
| SOL (wrapped) | `So11111111111111111111111111111111111111112` | 9 |
| USDC | `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` | 6 |
| USDT | `Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB` | 6 |
| WBTC (Portal) | `3NZ9JMVBmGAqocybic2c7LQCJScmgsAZ6vQqTDzcqmJh` | 8 |
| WETH (Portal) | `7vfCXTUXx5WJV5JADk17DUJ4ksgau7utNKj4b963voxs` | 8 |

## DeFi Tokens

| Token | Mint Address | Decimals |
|-------|-------------|----------|
| JUP | `JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN` | 6 |
| RAY | `4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R` | 6 |
| ORCA | `orcaEKTdK7LKz57vaAYr9QeNsVEPfiu6QeMU1kektZE` | 6 |
| MNDE | `MNDEFzGvMt87ueuHvVU9VcTqsAP5b3fTGPsHuuPA5ey` | 9 |
| mSOL | `mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So` | 9 |
| jitoSOL | `J1toso1uCk3RLmjorhTtrVwY9HJ7X8V9yYac6Y7kGCPn` | 9 |
| bSOL | `bSo13r4TkiE4KumL71LsHTPpL2euBYLFx6h9HP3piy1` | 9 |
| DRIFT | `DriFtupJYLTosbwoN8koMbEYSx54aFAVLddWsbksjwg7` | 6 |
| PYTH | `HZ1JovNiVvGrGNiiYvEozEVgZ58xaU3RKwX8eACQBCt3` | 6 |
| JTO | `jtojtomepa8beP8AuQc6eXt5FriJwfFMwQx2v2f9mCL` | 9 |
| W | `85VBFQZC9TZkfaptBWjvUw7YbZjy52A6mjtPGjstQAmQ` | 6 |
| BONK | `DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263` | 5 |
| WIF | `EKpQGSJtjMFqKZ9KQanSqYXRcF8fBopzLHYxdM65zcjm` | 6 |

## Stablecoins

| Token | Mint Address | Decimals |
|-------|-------------|----------|
| USDC | `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` | 6 |
| USDT | `Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB` | 6 |
| PYUSD | `2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo` | 6 |
| UXD | `7kbnvuGBxxj8AG9qp8Scn56muWGaRaFqxg1FsRp3PaFT` | 6 |

## Liquid Staking Tokens

| Token | Mint Address | Decimals |
|-------|-------------|----------|
| mSOL (Marinade) | `mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So` | 9 |
| jitoSOL (Jito) | `J1toso1uCk3RLmjorhTtrVwY9HJ7X8V9yYac6Y7kGCPn` | 9 |
| bSOL (BlazeStake) | `bSo13r4TkiE4KumL71LsHTPpL2euBYLFx6h9HP3piy1` | 9 |
| INF (Sanctum) | `5oVNBeEEQvYi1cX3ir8Dx5n1P7pdxydbGF2X4TxVusJm` | 9 |

## Notes

- Use the Jupiter token list API for the most up-to-date addresses: `https://token.jup.ag/strict`
- Amounts are in the smallest unit (lamports for SOL = 10^9, micro-USDC = 10^6)
- Wrapped SOL (`So111...112`) is used in swap APIs; native SOL is auto-wrapped by Jupiter
- For new/meme tokens, search Jupiter: `https://token.jup.ag/all` (includes unverified)
