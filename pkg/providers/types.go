package providers

import (
	"context"
	"fmt"

	"github.com/jiayaoqijia/ottie/pkg/providers/protocoltypes"
)

type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ExtraContent           = protocoltypes.ExtraContent
	GoogleExtra            = protocoltypes.GoogleExtra
	ContentBlock           = protocoltypes.ContentBlock
	CacheControl           = protocoltypes.CacheControl
)

type LLMProvider interface {
	Chat(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (*LLMResponse, error)
	GetDefaultModel() string
}

type StatefulProvider interface {
	LLMProvider
	Close()
}

// ThinkingCapable is an optional interface for providers that support
// extended thinking (e.g. Anthropic). Used by the agent loop to warn
// when thinking_level is configured but the active provider cannot use it.
type ThinkingCapable interface {
	SupportsThinking() bool
}

// FailoverReason classifies why an LLM request failed for fallback decisions.
// This is the R6 steal #1 from hermes-agent/agent/error_classifier.py —
// a single typed taxonomy that drives the retry ladder across every
// provider and replaces scattered inline string-matching.
//
// The constants here are the closed set; any new reason added must come
// with a matching classification path in error_classifier.go and a
// corresponding recovery hint in the methods below. Reasons are string-
// typed (not iota) so they are stable across serialization into the
// action ledger and traces tables without a migration.
type FailoverReason string

const (
	// Transient auth failures (401/403, stale token): refresh/rotate
	// credential and retry.
	FailoverAuth FailoverReason = "auth"

	// Auth failed after refresh: abort, do not retry. Distinct from
	// FailoverAuth because the retry ladder must not burn credits
	// trying a new credential if the permanent-auth signal is clear.
	FailoverAuthPermanent FailoverReason = "auth_permanent"

	// Rate limit or token-per-minute throttling (429 or pattern):
	// backoff then retry on same provider, then rotate.
	FailoverRateLimit FailoverReason = "rate_limit"

	// Billing exhausted / insufficient credits (402 or pattern):
	// rotate credential immediately, do not burn retries.
	FailoverBilling FailoverReason = "billing"

	// Connection or read timeout: rebuild client and retry.
	FailoverTimeout FailoverReason = "timeout"

	// Context window too large: compress history and retry in place,
	// do not failover. Different from payload_too_large because this
	// is model-side not transport-side.
	FailoverContextOverflow FailoverReason = "context_overflow"

	// Payload too large for transport (413): compress then retry.
	// Distinct from context_overflow because the fix is at a
	// different layer — strip images/attachments rather than
	// summarize history.
	FailoverPayloadTooLarge FailoverReason = "payload_too_large"

	// 400 bad request: abort or strip-and-retry once.
	FailoverFormat FailoverReason = "format"

	// 503/529 provider overloaded: backoff then retry.
	FailoverOverloaded FailoverReason = "overloaded"

	// 404 or "model not found": fallback to a different model.
	FailoverModelNotFound FailoverReason = "model_not_found"

	// Session token expired mid-call: refresh session and retry.
	FailoverSessionExpired FailoverReason = "session_expired"

	// Anthropic-specific: invalid thinking-block signature. Strip
	// thinking blocks from the request and retry once.
	FailoverThinkingSignature FailoverReason = "thinking_signature"

	// Anthropic-specific: "long context" extra-usage tier not
	// enabled. Either fall back to a smaller context or surface to
	// the user.
	FailoverLongContextTier FailoverReason = "long_context_tier"

	// Unclassifiable: retry with backoff as a last resort.
	FailoverUnknown FailoverReason = "unknown"
)

// FailoverError wraps an LLM provider error with classification metadata.
type FailoverError struct {
	Reason   FailoverReason
	Provider string
	Model    string
	Status   int
	Wrapped  error
}

func (e *FailoverError) Error() string {
	return fmt.Sprintf("failover(%s): provider=%s model=%s status=%d: %v",
		e.Reason, e.Provider, e.Model, e.Status, e.Wrapped)
}

func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}

// IsRetriable returns true if this error should trigger fallback to
// next candidate. Non-retriable reasons are those where the call will
// fail the same way on the next provider too (bad request format,
// model not found, permanent auth failure).
func (e *FailoverError) IsRetriable() bool {
	switch e.Reason {
	case FailoverFormat, FailoverModelNotFound, FailoverAuthPermanent:
		return false
	}
	return true
}

// ShouldCompress reports whether the retry layer should compress the
// request (history summarization or payload stripping) before
// retrying. Hermes separates this from the retry ladder because
// compression is a request-mutating recovery, not a provider swap.
func (e *FailoverError) ShouldCompress() bool {
	return e.Reason == FailoverContextOverflow || e.Reason == FailoverPayloadTooLarge
}

// ShouldRotateCredential reports whether the retry layer should swap
// to a different credential immediately rather than retrying on the
// same one. Used by billing failures (credit exhausted) where more
// retries will just fail faster.
func (e *FailoverError) ShouldRotateCredential() bool {
	return e.Reason == FailoverBilling || e.Reason == FailoverAuth
}

// ShouldFallback reports whether the retry layer should swap to a
// different provider/model. The default is "fallback" for any
// transient condition that is unlikely to resolve on the current
// target within the retry window: rate limits, overload, timeout,
// transient auth, billing exhaustion, model-not-found, permanent
// auth, long-context tier gates.
//
// Returns false for conditions where swapping providers is wasteful:
// compression-fixable errors (context_overflow, payload_too_large),
// Anthropic thinking-signature (fixed in-place by stripping
// thinking blocks), session expiry (refresh in-place), format
// errors (will fail the same way elsewhere), and the catch-all
// FailoverUnknown (retry in place once; aborting is safer than
// burning through every provider on an unclassifiable error).
func (e *FailoverError) ShouldFallback() bool {
	switch e.Reason {
	case FailoverModelNotFound, FailoverOverloaded, FailoverBilling,
		FailoverAuthPermanent, FailoverLongContextTier, FailoverRateLimit,
		FailoverTimeout, FailoverAuth:
		return true
	}
	return false
}

// IsAuth is a convenience predicate that covers both transient and
// permanent auth failures.
func (e *FailoverError) IsAuth() bool {
	return e.Reason == FailoverAuth || e.Reason == FailoverAuthPermanent
}

// ModelConfig holds primary model and fallback list.
type ModelConfig struct {
	Primary   string
	Fallbacks []string
}
