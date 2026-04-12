package providers

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Common patterns in Go HTTP error messages
var httpStatusPatterns = []*regexp.Regexp{
	regexp.MustCompile(`status[:\s]+(\d{3})`),
	regexp.MustCompile(`http[/\s]+\d*\.?\d*\s+(\d{3})`),
	regexp.MustCompile(`\b([3-5]\d{2})\b`),
}

// errorPattern defines a single pattern (string or regex) for error classification.
type errorPattern struct {
	substring string
	regex     *regexp.Regexp
}

func substr(s string) errorPattern { return errorPattern{substring: s} }
func rxp(r string) errorPattern    { return errorPattern{regex: regexp.MustCompile("(?i)" + r)} }

// Error patterns organized by FailoverReason, matching OpenClaw production (~40 patterns).
var (
	rateLimitPatterns = []errorPattern{
		rxp(`rate[_ ]limit`),
		substr("too many requests"),
		substr("429"),
		substr("exceeded your current quota"),
		rxp(`exceeded.*quota`),
		rxp(`resource has been exhausted`),
		rxp(`resource.*exhausted`),
		substr("resource_exhausted"),
		substr("quota exceeded"),
		substr("usage limit"),
	}

	overloadedPatterns = []errorPattern{
		rxp(`overloaded_error`),
		rxp(`"type"\s*:\s*"overloaded_error"`),
		substr("overloaded"),
	}

	timeoutPatterns = []errorPattern{
		substr("timeout"),
		substr("timed out"),
		substr("deadline exceeded"),
		substr("context deadline exceeded"),
	}

	billingPatterns = []errorPattern{
		rxp(`\b402\b`),
		substr("payment required"),
		substr("insufficient credits"),
		substr("credit balance"),
		substr("plans & billing"),
		substr("insufficient balance"),
		substr("insufficient_quota"),
		substr("credits have been exhausted"),
		substr("top up your credits"),
		substr("billing hard limit"),
		// "plan does not include" is a billing concern in hermes —
		// the fix is to rotate to a credential with a bigger plan,
		// not to fall through to auth handling.
		substr("plan does not include"),
		// Note: "exceeded your current quota" lives in rateLimitPatterns
		// because hermes treats it as a transient throttle, not a
		// hard billing exhaustion.
	}

	authPatterns = []errorPattern{
		rxp(`invalid[_ ]?api[_ ]?key`),
		substr("incorrect api key"),
		substr("invalid token"),
		substr("authentication"),
		substr("re-authenticate"),
		substr("oauth token refresh failed"),
		substr("unauthorized"),
		substr("forbidden"),
		substr("access denied"),
		substr("expired"),
		substr("token has expired"),
		rxp(`\b401\b`),
		rxp(`\b403\b`),
		substr("no credentials found"),
		substr("no api key found"),
	}

	formatPatterns = []errorPattern{
		substr("string should match pattern"),
		substr("tool_use.id"),
		substr("tool_use_id"),
		substr("messages.1.content.1.tool_use.id"),
		substr("invalid request format"),
	}

	imageDimensionPatterns = []errorPattern{
		rxp(`image dimensions exceed max`),
	}

	imageSizePatterns = []errorPattern{
		rxp(`image exceeds.*mb`),
	}

	modelNotFoundPatterns = []errorPattern{
		substr("model not found"),
		substr("does not exist"),
		substr("model_not_found"),
		rxp(`model.*not.*found`),
	}

	sessionExpiredPatterns = []errorPattern{
		substr("session expired"),
		substr("session_expired"),
		substr("session has expired"),
	}

	// Context overflow: the request's history is too large for the
	// model's context window. Triggers compression, not failover.
	contextOverflowPatterns = []errorPattern{
		rxp(`context[_ ]length`),
		substr("maximum context length"),
		substr("prompt is too long"),
		substr("messages_too_long"),
		substr("input is too long"),
		rxp(`exceed.*context.*window`),
		// Hermes patterns ported in R8:
		substr("token limit"),
		substr("too many tokens"),
		substr("max_tokens"),
		substr("maximum number of tokens"),
	}

	// Payload too large: 413 or transport-layer limits. The fix is
	// different from context_overflow (strip attachments, not
	// summarize history).
	payloadTooLargePatterns = []errorPattern{
		rxp(`\b413\b`),
		substr("payload too large"),
		substr("request entity too large"),
		substr("request body too large"),
	}

	// Anthropic-specific: thinking block signature invalid. The fix
	// is to strip thinking blocks and retry once.
	thinkingSignaturePatterns = []errorPattern{
		substr("thinking block signature"),
		substr("thinking.signature"),
		substr("extended_thinking_signature"),
		substr("invalid_thinking"),
	}

	// Anthropic-specific: long-context tier gate. The account needs
	// an "extra usage" entitlement that isn't enabled; fallback to a
	// smaller context window or surface to the user.
	//
	// These patterns deliberately require specific multi-word
	// phrases rather than bare "long context" or bare "extra usage"
	// (both of which appear in billing errors too). Hermes only
	// classifies this reason when the message explicitly names the
	// tier gate; otherwise the billing/rate-limit path takes it.
	longContextTierPatterns = []errorPattern{
		substr("long_context_tier"),
		substr("long context tier"),
		rxp(`context.*tier.*not.*enabled`),
		substr("long context feature not enabled"),
		substr("extra usage tier required"),
		substr("extra_usage_tier"),
	}

	// Patterns that indicate a permanent auth failure (not transient).
	// Distinct from authPatterns because these should not trigger a
	// credential rotation — the credential is already bad.
	//
	// Note: "plan does not include" is classified as billing (not
	// auth_permanent) because hermes-agent/agent/error_classifier.py
	// treats plan-tier errors as a billing concern that rotates to a
	// different credential with a bigger plan.
	authPermanentPatterns = []errorPattern{
		substr("account is deactivated"),
		substr("account has been suspended"),
		substr("organization has been disabled"),
		substr("user is not authorized"),
	}

	// Transient HTTP status codes that map to timeout (500/502/408
	// — genuine server-side failures that often recover).
	// 503/529 are split into overloadedStatusCodes below because
	// they carry specific "provider is overloaded" semantics in both
	// anthropic and openai, and map to FailoverOverloaded → the
	// recovery path is "backoff and retry on same provider", not
	// "swap to a different model".
	transientStatusCodes = map[int]bool{
		500: true, 502: true,
		521: true, 522: true, 523: true, 524: true,
	}

	// Overloaded HTTP status codes — classifier returns
	// FailoverOverloaded, which enables the retry-with-backoff
	// recovery path without triggering a provider swap.
	overloadedStatusCodes = map[int]bool{
		503: true, 529: true,
	}
)

// ClassifyError classifies an error into a FailoverError with reason.
// Returns nil for errors that should NOT trigger the failover path:
// a nil input, or a context.Canceled (user abort). Every other error
// is always classified; unrecognized errors get FailoverUnknown so
// the retry layer has a concrete reason to reason about.
//
// The R6 steal-from-hermes rule: always classify. A nil return used
// to mean "can't tell, skip failover," which left FailoverUnknown
// dead and made the recovery layer inconsistent with hermes. The
// classifier now returns FailoverUnknown for unmatched errors and
// the retry layer uses IsRetriable() and the Should* hints to decide
// whether to actually swap providers.
func ClassifyError(err error, provider, model string) *FailoverError {
	if err == nil {
		return nil
	}

	// Context cancellation: user abort, never fallback.
	if errors.Is(err, context.Canceled) {
		return nil
	}

	// Context deadline exceeded: treat as timeout, always fallback.
	if errors.Is(err, context.DeadlineExceeded) {
		return &FailoverError{
			Reason:   FailoverTimeout,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	// Traverse error chain — check wrapped errors too.
	current := err
	for current != nil {
		if fe, ok := current.(*FailoverError); ok {
			return fe
		}
		current = errors.Unwrap(current)
	}

	msg := strings.ToLower(err.Error())

	// Image dimension/size errors: non-retriable, non-fallback.
	if IsImageDimensionError(msg) || IsImageSizeError(msg) {
		return &FailoverError{
			Reason:   FailoverFormat,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	// Try HTTP status code extraction first.
	if status := extractHTTPStatus(msg); status > 0 {
		if reason := classifyByStatus(status); reason != "" {
			return &FailoverError{
				Reason:   reason,
				Provider: provider,
				Model:    model,
				Status:   status,
				Wrapped:  err,
			}
		}
	}

	// Message pattern matching (priority order from OpenClaw).
	if reason := classifyByMessage(msg); reason != "" {
		return &FailoverError{
			Reason:   reason,
			Provider: provider,
			Model:    model,
			Wrapped:  err,
		}
	}

	// No pattern matched — classify as Unknown so the retry layer
	// has a concrete reason to decide on. The recovery hints for
	// FailoverUnknown are "retriable true, no compress, no rotate,
	// no fallback" — a safe default that lets the retry loop try
	// once more without swapping providers or credentials.
	return &FailoverError{
		Reason:   FailoverUnknown,
		Provider: provider,
		Model:    model,
		Wrapped:  err,
	}
}

// classifyByStatus maps HTTP status codes to FailoverReason.
// Status-based classification is more reliable than message-pattern
// matching because bodies drift but status codes are load-bearing
// HTTP contract.
func classifyByStatus(status int) FailoverReason {
	switch {
	case status == 401 || status == 403:
		return FailoverAuth
	case status == 402:
		return FailoverBilling
	case status == 404:
		// Hermes maps bare 404 to model_not_found; the pattern path
		// will refine it further if the message carries more context.
		return FailoverModelNotFound
	case status == 408:
		return FailoverTimeout
	case status == 413:
		return FailoverPayloadTooLarge
	case status == 429:
		return FailoverRateLimit
	case status == 400:
		// A bare 400 with no message body is unambiguously a format
		// error. If the caller's message carries more context (e.g.,
		// "context length exceeded"), the pattern path runs AFTER
		// status classification in ClassifyError and would only hit
		// this if a specific pattern does not match.
		return FailoverFormat
	case overloadedStatusCodes[status]:
		return FailoverOverloaded
	case transientStatusCodes[status]:
		return FailoverTimeout
	}
	return ""
}

// classifyByMessage matches error messages against patterns.
// Priority order matters — more specific patterns must come before
// more general ones so a "thinking signature" error does not get
// misclassified as format, and "context overflow" is not mistaken
// for a generic format error. Compression hints come first because
// they are request-mutating recoveries, not provider swaps.
//
// Returns "" (empty FailoverReason) on no match; the caller decides
// whether to map that to FailoverUnknown or return nil. This lets
// ClassifyError distinguish "definitely not a failover" from "can't
// tell, retry with backoff."
func classifyByMessage(msg string) FailoverReason {
	// Compression hints first — the classifier should flag these
	// before any pattern that would trigger a provider swap.
	if matchesAny(msg, contextOverflowPatterns) {
		return FailoverContextOverflow
	}
	if matchesAny(msg, payloadTooLargePatterns) {
		return FailoverPayloadTooLarge
	}
	// Anthropic-specific errors before generic format/auth because
	// they carry specific recovery actions.
	if matchesAny(msg, thinkingSignaturePatterns) {
		return FailoverThinkingSignature
	}
	// Billing patterns must run before longContextTier because some
	// "extra usage" strings are billing concerns (hermes classifies
	// them as billing) — the tier-specific patterns are narrower.
	if matchesAny(msg, billingPatterns) {
		return FailoverBilling
	}
	if matchesAny(msg, longContextTierPatterns) {
		return FailoverLongContextTier
	}
	if matchesAny(msg, rateLimitPatterns) {
		return FailoverRateLimit
	}
	// Overloaded must be checked BEFORE falling into rate_limit
	// (which was the old behavior) so the reason surface is
	// actually reachable.
	if matchesAny(msg, overloadedPatterns) {
		return FailoverOverloaded
	}
	if matchesAny(msg, timeoutPatterns) {
		return FailoverTimeout
	}
	if matchesAny(msg, modelNotFoundPatterns) {
		return FailoverModelNotFound
	}
	if matchesAny(msg, sessionExpiredPatterns) {
		return FailoverSessionExpired
	}
	// Permanent-auth patterns must be checked before the generic
	// authPatterns list — otherwise a "account deactivated" error
	// would be classified as transient FailoverAuth and trigger a
	// wasted credential rotation.
	if matchesAny(msg, authPermanentPatterns) {
		return FailoverAuthPermanent
	}
	if matchesAny(msg, authPatterns) {
		return FailoverAuth
	}
	if matchesAny(msg, formatPatterns) {
		return FailoverFormat
	}
	return ""
}

// extractHTTPStatus extracts an HTTP status code from an error message.
// Looks for patterns like "status: 429", "status 429", "http/1.1 429", "http 429", or standalone "429".
func extractHTTPStatus(msg string) int {
	for _, p := range httpStatusPatterns {
		if m := p.FindStringSubmatch(msg); len(m) > 1 {
			return parseDigits(m[1])
		}
	}
	return 0
}

// IsImageDimensionError returns true if the message indicates an image dimension error.
func IsImageDimensionError(msg string) bool {
	return matchesAny(msg, imageDimensionPatterns)
}

// IsImageSizeError returns true if the message indicates an image file size error.
func IsImageSizeError(msg string) bool {
	return matchesAny(msg, imageSizePatterns)
}

// matchesAny checks if msg matches any of the patterns.
func matchesAny(msg string, patterns []errorPattern) bool {
	for _, p := range patterns {
		if p.regex != nil {
			if p.regex.MatchString(msg) {
				return true
			}
		} else if p.substring != "" {
			if strings.Contains(msg, p.substring) {
				return true
			}
		}
	}
	return false
}

// parseDigits converts a string of digits to an int.
func parseDigits(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
