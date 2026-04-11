# CEX Authentication Reference

Private endpoints (placing orders, checking balances) require API key authentication.
Each exchange uses HMAC-based request signing.

## General Setup

1. Create API key on the exchange website
2. Store credentials securely (environment variables recommended)
3. Never share API secrets in logs or messages

Environment variables convention:
```
BINANCE_API_KEY, BINANCE_API_SECRET
COINBASE_API_KEY, COINBASE_API_SECRET, COINBASE_API_PASSPHRASE
KRAKEN_API_KEY, KRAKEN_API_SECRET
BYBIT_API_KEY, BYBIT_API_SECRET
GATEIO_API_KEY, GATEIO_API_SECRET
BITGET_API_KEY, BITGET_API_SECRET, BITGET_API_PASSPHRASE
```

## Signing Methods

### Binance
- Method: HMAC-SHA256
- Headers: `X-MBX-APIKEY: API_KEY`
- Query params: append `timestamp=UNIX_MS&signature=HMAC_SHA256(query_string, secret)`

### Coinbase
- Method: HMAC-SHA256
- Headers: `CB-ACCESS-KEY`, `CB-ACCESS-SIGN`, `CB-ACCESS-TIMESTAMP`, `CB-ACCESS-PASSPHRASE`
- Signature: `HMAC_SHA256(timestamp + method + path + body, base64_decode(secret))`

### Kraken
- Method: HMAC-SHA512
- Headers: `API-Key`, `API-Sign`
- Signature: `HMAC_SHA512(path + SHA256(nonce + postdata), base64_decode(secret))`
- Body includes `nonce` (incrementing integer)

### Bybit
- Method: HMAC-SHA256
- Headers: `X-BAPI-API-KEY`, `X-BAPI-SIGN`, `X-BAPI-TIMESTAMP`, `X-BAPI-RECV-WINDOW`
- Signature: `HMAC_SHA256(timestamp + api_key + recv_window + query_or_body, secret)`

### Gate.io
- Method: HMAC-SHA512
- Headers: `KEY`, `SIGN`, `Timestamp`
- Signature: `HMAC_SHA512(method + \n + path + \n + query + \n + SHA512(body) + \n + timestamp, secret)`

### Bitget
- Method: HMAC-SHA256 (base64)
- Headers: `ACCESS-KEY`, `ACCESS-SIGN`, `ACCESS-TIMESTAMP`, `ACCESS-PASSPHRASE`
- Signature: `base64(HMAC_SHA256(timestamp + method + path + body, secret))`

## Security Notes

- Use read-only API keys when only querying data
- Enable IP whitelisting on exchange API settings
- Never enable withdrawal permission unless absolutely necessary
- Rotate keys periodically
- For trading via Ottie, prefer using official MCP tools (Kraken CLI, Bitget Agent Hub)
  which handle signing internally
