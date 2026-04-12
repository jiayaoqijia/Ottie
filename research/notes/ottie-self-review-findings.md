# Ottie Self-Review Findings

> Ottie was used to review its own source code for bugs, security issues,
> and improvements. Each finding was produced by `ottie agent -m` pointed
> at its own codebase with read_file and exec tools enabled.

## Agent Loop (pkg/agent/loop.go)

### 1. Unsynchronized config access — data race
- **Line 611**: `sendTranscriptionFeedback()` reads `al.cfg` directly without `al.mu` lock
- **Impact**: Data race if `ReloadProviderAndConfig()` swaps `al.cfg` concurrently
- **Severity**: HIGH

### 2. Nil-pointer dereference on tool result
- **Lines 1031, 1044, 1067**: `r.result` dereferenced without nil check
- **Impact**: Panic kills the request if a tool returns nil
- **Severity**: HIGH

### 3. Unbounded goroutine fan-out from LLM tool calls
- **Lines 922-1022**: One goroutine per tool call, no semaphore
- **Impact**: Adversarial prompt inducing many tool calls = resource exhaustion
- **Severity**: MEDIUM

## Error Classifier (pkg/providers/error_classifier.go)

### 4. HTTP 400 classified as Format before message check
- **Line 295**: `status == 400` returns `FailoverFormat` before `classifyByMessage` runs
- **Impact**: Context-window errors returned as HTTP 400 become non-retriable format errors instead of triggering compression
- **Severity**: HIGH

### 5. HTTP 404 classified too early as ModelNotFound
- **Line 284**: Same issue — status wins before message refinement
- **Impact**: Transient 404 errors treated as permanent model-not-found
- **Severity**: MEDIUM

### 6. Regex matches random 3-digit numbers as HTTP status
- **Line 14**: `\b([3-5]\d{2})\b` matches any 300-599 number in error text
- **Impact**: Token counts, ports, limits misread as HTTP status codes
- **Severity**: MEDIUM

## Fallback Chain (pkg/providers/fallback.go)

### 7. Unknown errors never advance to next candidate
- **Line 175-180**: `ShouldFallback()==false` returns immediately for Unknown errors
- **Impact**: Transient unknown errors abort the chain instead of trying next provider
- **Severity**: HIGH

### 8. Cooldown skip reason hardcoded to RateLimit
- **Line 112**: All cooldown skips recorded as `FailoverRateLimit`
- **Impact**: Poisons attempt history and debugging
- **Severity**: LOW

## Action Ledger (pkg/actionlog/actionlog.go)

### 9. Writer deadlock on closed channel
- **Lines 312-317**: Receives from `writeCh` ignore `ok` boolean
- **Impact**: Closed channel returns zero-value op with nil reply channel = deadlock
- **Severity**: MEDIUM

### 10. Context cancel after enqueue reports failure despite committed write
- **Lines 549-558**: Context cancel races with reply delivery
- **Impact**: Caller believes Prepare failed but ledger has the row = lost intent_id
- **Severity**: HIGH

### 11. Checkpoint failure silently downgrades durability guarantee
- **Lines 392-394, 432-435, 458-461**: Checkpoint failure logged but success returned
- **Impact**: Violates documented durability contract (row may be in WAL only)
- **Severity**: MEDIUM

## Skills System (pkg/skills/installer.go)

### 12. Concurrent install race — check-then-create not atomic
- **Lines 109-112**: `os.Stat` check before `os.Mkdir` is not atomic
- **Impact**: Two concurrent installs of same skill = partial/mixed state
- **Severity**: MEDIUM

### 13. Installer doesn't validate skill name
- **Lines 104-107**: `skillName` from untrusted input, no `namePattern` check
- **Impact**: Invalid names installed that loader later silently rejects
- **Severity**: MEDIUM

### 14. Arbitrary file write via unvalidated GitHub item names
- **Lines 154, 161-169**: `item.Name` from API response used directly in filepath.Join
- **Impact**: Path traversal via malicious repo response
- **Severity**: HIGH

### 15. No content validation after download
- **Lines 124-129**: Only checks SKILL.md presence, not validity
- **Impact**: Invalid skills install successfully but are silently ignored
- **Severity**: LOW

### 16. Partial install cleanup missing
- **Lines 120-122**: Failed installs leave partial directory, blocking retry
- **Impact**: Broken state requires manual cleanup
- **Severity**: LOW
