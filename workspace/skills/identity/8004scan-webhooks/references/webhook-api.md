# Webhook API Reference

## Endpoints

### POST /api/v1/webhooks — Register Webhook

Create a new webhook subscription.

**Request:**

```json
{
  "url": "https://yourserver.com/webhook",
  "events": ["feedback.received", "validation.completed"],
  "secret": "your-shared-secret",
  "description": "Optional description"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | HTTPS endpoint to receive events |
| `events` | string[] | Yes | Event types to subscribe to |
| `secret` | string | Yes | Shared secret for HMAC-SHA256 signing |
| `description` | string | No | Human-readable description |

**Response (201):**

```json
{
  "success": true,
  "data": {
    "id": "wh_abc123",
    "url": "https://yourserver.com/webhook",
    "events": ["feedback.received", "validation.completed"],
    "active": true,
    "createdAt": "2026-03-16T00:00:00Z"
  }
}
```

**Validation rules:**
- `url` must be HTTPS
- `events` must contain at least one valid event type
- `secret` must be at least 16 characters

---

### GET /api/v1/webhooks — List Webhooks

List all webhook subscriptions for the authenticated API key.

**Response (200):**

```json
{
  "success": true,
  "data": [
    {
      "id": "wh_abc123",
      "url": "https://yourserver.com/webhook",
      "events": ["feedback.received"],
      "active": true,
      "createdAt": "2026-03-16T00:00:00Z",
      "lastDeliveryAt": "2026-03-16T12:00:00Z",
      "failureCount": 0
    }
  ]
}
```

---

### PATCH /api/v1/webhooks/{webhookId} — Update Webhook

Update webhook URL, events, secret, or active status.

**Request (partial update):**

```json
{
  "events": ["feedback.received", "feedback.revoked", "star.received"],
  "active": true
}
```

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | New delivery URL |
| `events` | string[] | Replace event subscriptions |
| `secret` | string | New shared secret |
| `active` | boolean | Enable/disable webhook |
| `description` | string | Update description |

---

### DELETE /api/v1/webhooks/{webhookId} — Delete Webhook

Permanently remove a webhook subscription.

**Response (200):**

```json
{
  "success": true,
  "data": { "deleted": true }
}
```

---

### GET /api/v1/webhooks/{webhookId}/deliveries — Delivery History

View delivery attempts for a webhook.

**Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 20 | Results per page (1-100) |
| `offset` | int | 0 | Pagination offset |
| `status` | string | — | Filter: `success`, `failed`, `pending` |

**Response (200):**

```json
{
  "success": true,
  "data": [
    {
      "id": "del_xyz789",
      "event": "feedback.received",
      "status": "success",
      "responseCode": 200,
      "attempt": 1,
      "payload": { ... },
      "createdAt": "2026-03-16T12:00:00Z",
      "deliveredAt": "2026-03-16T12:00:01Z"
    }
  ],
  "meta": {
    "pagination": { "limit": 20, "offset": 0, "total": 45 }
  }
}
```

**Delivery statuses:**
- `success` — 2xx response received
- `failed` — All retry attempts exhausted
- `pending` — Awaiting next retry attempt
