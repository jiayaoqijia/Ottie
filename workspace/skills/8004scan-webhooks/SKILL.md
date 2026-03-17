---
name: 8004scan-webhooks
description: Register and manage 8004scan webhooks for real-time event notifications — agent registrations, feedback submissions, validation responses, and star events. Covers webhook CRUD, event types, HMAC-SHA256 signature verification, delivery monitoring, and retry behavior. Consult this skill when the user wants to receive real-time notifications from 8004scan, set up webhooks, verify webhook signatures, or monitor webhook deliveries.
version: 1.0.0
allowed-tools: "Bash(curl:*) Bash(jq:*)"
metadata:
  openclaw:
    requires:
      bins:
        - curl
      env:
        - EIGHTSCAN_API_KEY
    primaryEnv: EIGHTSCAN_API_KEY
    emoji: "🪝"
    homepage: https://www.8004scan.io/developers/docs
---

# 8004scan Webhooks — Real-Time Event Skill

Subscribe to real-time events from the 8004scan platform. Receive HTTP callbacks when agents are registered, feedback is submitted, validations complete, or stars are given.

## Reference Map

| File | When to read |
|------|-------------|
| `{baseDir}/references/webhook-api.md` | Webhook CRUD endpoints, request/response schemas |
| `{baseDir}/references/events.md` | All event types, payload schemas, trigger conditions |
| `{baseDir}/references/verification.md` | HMAC-SHA256 signature verification, security best practices |

---

## Base URL

```
https://www.8004scan.io/api/v1
```

**Note**: Webhook endpoints require authentication. Include `X-API-Key` header.

## Authentication

All webhook management endpoints require a valid API key:

```bash
-H "X-API-Key: $EIGHTSCAN_API_KEY"
```

---

## Request Classification

1. **Register webhook** ("set up a webhook", "notify me when...") → Register Webhook endpoint.
2. **List webhooks** ("show my webhooks", "what webhooks do I have?") → List Webhooks endpoint.
3. **Update webhook** ("change webhook URL", "add events to webhook") → Update Webhook endpoint.
4. **Delete webhook** ("remove webhook", "stop notifications") → Delete Webhook endpoint.
5. **Check deliveries** ("webhook delivery history", "failed deliveries") → Deliveries endpoint.
6. **Verify signature** ("how to verify webhook?", "HMAC validation") → Read `references/verification.md`.

---

## Event Types

| Event | Trigger |
|-------|---------|
| `validation.requested` | A validation request is submitted for an agent |
| `validation.completed` | A validator submits their attestation response |
| `feedback.received` | New feedback is submitted for an agent |
| `feedback.revoked` | Existing feedback is revoked by its submitter |
| `star.received` | An agent receives a star rating |
| `star.removed` | A star rating is removed |

---

## Quick Reference

### Register a Webhook

```bash
curl -s -X POST "https://www.8004scan.io/api/v1/webhooks" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://yourserver.com/webhook",
    "events": ["feedback.received", "validation.completed"],
    "secret": "your-webhook-secret"
  }' | jq .
```

### List Webhooks

```bash
curl -s "https://www.8004scan.io/api/v1/webhooks" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" | jq .
```

### Update a Webhook

```bash
curl -s -X PATCH "https://www.8004scan.io/api/v1/webhooks/{webhookId}" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "events": ["feedback.received", "feedback.revoked", "star.received"],
    "active": true
  }' | jq .
```

### Delete a Webhook

```bash
curl -s -X DELETE "https://www.8004scan.io/api/v1/webhooks/{webhookId}" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" | jq .
```

### Check Delivery History

```bash
curl -s "https://www.8004scan.io/api/v1/webhooks/{webhookId}/deliveries?limit=10" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" | jq .
```

---

## Webhook Payload Format

All webhook deliveries include:

```json
{
  "event": "feedback.received",
  "timestamp": "2026-03-16T12:00:00Z",
  "data": {
    "chainId": 8453,
    "tokenId": 17,
    "agentId": "8453:17",
    ...event-specific fields...
  }
}
```

Headers on delivery:
- `X-Webhook-Signature`: HMAC-SHA256 hex digest
- `X-Webhook-Event`: Event type
- `X-Webhook-Delivery-Id`: Unique delivery ID
- `Content-Type`: `application/json`

---

## Signature Verification

Verify every incoming webhook to ensure authenticity:

```bash
# The signature is: HMAC-SHA256(secret, request_body)
EXPECTED=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | awk '{print $2}')
if [ "$EXPECTED" = "$RECEIVED_SIGNATURE" ]; then
  echo "Valid"
fi
```

---

## Retry Behavior

Failed deliveries (non-2xx response or timeout) are retried with exponential backoff:

| Attempt | Delay |
|---------|-------|
| 1 | Immediate |
| 2 | 1 minute |
| 3 | 5 minutes |
| 4 | 30 minutes |
| 5 | 2 hours |

After 5 failed attempts, the delivery is marked as permanently failed. Webhooks with repeated failures may be automatically disabled.

---

## Examples

**Example 1: Set up feedback notifications**
User: "Notify me when my agent gets feedback"
```bash
curl -s -X POST "https://www.8004scan.io/api/v1/webhooks" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://myserver.com/hook","events":["feedback.received","feedback.revoked"],"secret":"mysecret"}' | jq .
```

**Example 2: Monitor validation progress**
User: "Alert me when validation completes"
```bash
curl -s -X POST "https://www.8004scan.io/api/v1/webhooks" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://myserver.com/hook","events":["validation.requested","validation.completed"],"secret":"mysecret"}' | jq .
```

**Example 3: Debug failed deliveries**
User: "Why aren't my webhooks working?"
```bash
curl -s "https://www.8004scan.io/api/v1/webhooks/{id}/deliveries?limit=5" \
  -H "X-API-Key: $EIGHTSCAN_API_KEY" | jq '.data[] | {status, responseCode, attempt, createdAt}'
```
