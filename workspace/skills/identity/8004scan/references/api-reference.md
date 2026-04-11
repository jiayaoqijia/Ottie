# 8004scan Public API — Full Reference

Base URL: `https://www.8004scan.io/api/v1/public`

OpenAPI spec: `https://www.8004scan.io/api/v1/public/docs/openapi.json`

---

## GET /agents

List agents with pagination, filtering, and sorting.

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 20 | Results per page (1-100) |
| `offset` | integer | 0 | Skip N results |
| `chainId` | integer | — | Filter by blockchain network |
| `ownerAddress` | string | — | Filter by owner wallet (0x...) |
| `protocol` | string | — | Filter by protocol type (mcp, a2a) |
| `sortBy` | string | — | Sort field (registeredAt, feedbackCount, averageValue) |
| `sortOrder` | string | desc | Sort direction (asc, desc) |

### Response

```json
{
  "success": true,
  "data": [
    {
      "chainId": 8453,
      "tokenId": 17,
      "name": "Agent Name",
      "description": "What it does",
      "owner": "0x...",
      "active": true,
      "mcpEndpoint": "https://...",
      "a2aEndpoint": "https://...",
      "feedbackCount": 12,
      "averageValue": 85,
      "registeredAt": "2026-01-15T10:30:00Z"
    }
  ],
  "meta": {
    "version": "1.0",
    "timestamp": "2026-03-16T00:00:00Z",
    "requestId": "uuid",
    "pagination": { "limit": 20, "offset": 0, "total": 500 }
  }
}
```

---

## GET /agents/{chainId}/{tokenId}

Retrieve full details for a specific agent.

### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `chainId` | integer | Blockchain network chain ID |
| `tokenId` | integer | Agent token ID on the Identity Registry |

### Response

Returns the complete agent profile including:
- Identity: name, description, image, agentURI, owner
- Services: MCP, A2A, ENS, DID, email, web endpoints
- Reputation: feedbackCount, averageValue, recent feedback
- Metadata: skills, domains, x402, custom fields
- On-chain: registeredAt, updatedAt, wallet

---

## GET /agents/search

Semantic and keyword search for agents.

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `q` | string | **required** | Search query |
| `limit` | integer | 20 | Results per page (1-100) |
| `offset` | integer | 0 | Skip N results |
| `chainId` | integer | — | Filter by chain |
| `semanticWeight` | float | 0.5 | Balance between keyword (0) and semantic (1) search |

### Notes

- `semanticWeight=0` → pure keyword matching
- `semanticWeight=1` → pure semantic/embedding search
- `semanticWeight=0.5` → balanced (default)

---

## GET /accounts/{address}/agents

List agents owned by a wallet address.

### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `address` | string | Wallet address matching `^0x[a-fA-F0-9]{40}$` |

### Response

Array of agent summaries owned by the address. Same structure as `/agents` list items.

---

## GET /stats

Global platform statistics.

### Response

```json
{
  "success": true,
  "data": {
    "totalAgents": 5000,
    "totalChains": 30,
    "totalFeedbacks": 15000,
    "activeAgents": 3500,
    "recentRegistrations": 120
  }
}
```

---

## GET /chains

List all blockchain networks supported by 8004scan.

### Response

```json
{
  "success": true,
  "data": [
    {
      "chainId": 1,
      "name": "Ethereum Mainnet",
      "explorer": "https://etherscan.io",
      "active": true
    }
  ]
}
```

---

## GET /feedbacks

Paginated list of feedback entries.

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 20 | Results per page (1-100) |
| `offset` | integer | 0 | Skip N results |
| `minScore` | integer | — | Minimum score filter (0-5) |
| `maxScore` | integer | — | Maximum score filter (0-5) |

---

## Error Responses

All errors follow a consistent format:

```json
{
  "success": false,
  "error": {
    "code": "BadRequest",
    "message": "Invalid chainId parameter",
    "details": { "param": "chainId", "received": "abc" }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|------------|-------------|
| `BadRequest` | 400 | Invalid parameters |
| `NotFound` | 404 | Agent/resource not found |
| `RateLimitExceeded` | 429 | Rate limit hit |
| `InternalError` | 500 | Server error |
