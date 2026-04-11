// Additional tests for the R6 steal #1 extensions to FailoverReason.
// Covers the five hermes-derived reasons that were added after R6:
// FailoverAuthPermanent, FailoverContextOverflow, FailoverPayloadTooLarge,
// FailoverThinkingSignature, FailoverLongContextTier — plus the recovery
// hint methods (ShouldCompress, ShouldRotateCredential, ShouldFallback,
// IsAuth) that let the retry loop pick a recovery action from the
// classification alone.

package providers

import (
	"errors"
	"testing"
)

func TestClassifyError_ContextOverflowPatterns(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"openai maximum context", "This model's maximum context length is 8192 tokens"},
		{"anthropic prompt too long", "prompt is too long: 250000 tokens > 200000 maximum"},
		{"explicit messages too long", "messages_too_long"},
		{"generic input too long", "The input is too long for this model"},
		{"exceeds context window", "Request exceeds the context window limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(errors.New(tc.msg), "openai", "gpt-4")
			if got == nil {
				t.Fatalf("expected classification, got nil")
			}
			if got.Reason != FailoverContextOverflow {
				t.Fatalf("Reason = %q, want context_overflow", got.Reason)
			}
			if !got.ShouldCompress() {
				t.Fatal("context_overflow should trigger compression")
			}
			if got.ShouldFallback() {
				t.Fatal("context_overflow should NOT trigger failover (fix the request, don't swap providers)")
			}
		})
	}
}

func TestClassifyError_PayloadTooLargePatterns(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"413 status", "HTTP 413 Request Entity Too Large"},
		{"payload too large literal", "payload too large"},
		{"request body too large", "request body too large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(errors.New(tc.msg), "openai", "gpt-4")
			if got == nil {
				t.Fatalf("expected classification, got nil")
			}
			if got.Reason != FailoverPayloadTooLarge {
				t.Fatalf("Reason = %q, want payload_too_large", got.Reason)
			}
			if !got.ShouldCompress() {
				t.Fatal("payload_too_large should trigger compression")
			}
		})
	}
}

func TestClassifyError_ThinkingSignaturePatterns(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"anthropic thinking block signature", "invalid thinking block signature"},
		{"dot form", "thinking.signature is not valid"},
		{"extended thinking signature", "extended_thinking_signature verification failed"},
		{"short form", "invalid_thinking block"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(errors.New(tc.msg), "anthropic", "claude-sonnet")
			if got == nil {
				t.Fatalf("expected classification, got nil")
			}
			if got.Reason != FailoverThinkingSignature {
				t.Fatalf("Reason = %q, want thinking_signature", got.Reason)
			}
			// The recovery action for thinking_signature is "strip
			// thinking blocks and retry once" — it should NOT trigger
			// a compress or credential rotation.
			if got.ShouldCompress() {
				t.Fatal("thinking_signature should not compress")
			}
			if got.ShouldRotateCredential() {
				t.Fatal("thinking_signature should not rotate credential")
			}
		})
	}
}

func TestClassifyError_LongContextTierPatterns(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"anthropic long context", "long context feature not enabled"},
		{"extra usage tier", "extra usage tier required"},
		{"explicit tier", "long_context_tier not enabled"},
		{"pattern form", "context size tier not enabled for this account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(errors.New(tc.msg), "anthropic", "claude-sonnet")
			if got == nil {
				t.Fatalf("expected classification, got nil")
			}
			if got.Reason != FailoverLongContextTier {
				t.Fatalf("Reason = %q, want long_context_tier", got.Reason)
			}
			if !got.ShouldFallback() {
				t.Fatal("long_context_tier should trigger fallback to smaller context model")
			}
		})
	}
}

func TestClassifyError_AuthPermanentPatterns(t *testing.T) {
	// Note: "plan does not include" is classified as billing (not
	// auth_permanent) per hermes convention — covered in the
	// billing test below, not here.
	cases := []struct {
		name string
		msg  string
	}{
		{"deactivated account", "account is deactivated"},
		{"suspended account", "account has been suspended"},
		{"org disabled", "organization has been disabled"},
		{"user not authorized", "user is not authorized for this resource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(errors.New(tc.msg), "openai", "gpt-4")
			if got == nil {
				t.Fatalf("expected classification, got nil")
			}
			if got.Reason != FailoverAuthPermanent {
				t.Fatalf("Reason = %q, want auth_permanent", got.Reason)
			}
			if got.IsRetriable() {
				t.Fatal("auth_permanent must not be retriable")
			}
			if got.ShouldRotateCredential() {
				t.Fatal("auth_permanent should NOT rotate credential — the issue is permanent")
			}
			if !got.ShouldFallback() {
				t.Fatal("auth_permanent should fallback to a different provider")
			}
			if !got.IsAuth() {
				t.Fatal("auth_permanent should register as an auth failure")
			}
		})
	}
}

func TestClassifyError_PermanentAuthBeatsTransientAuth(t *testing.T) {
	// The classification order must check permanent-auth patterns
	// BEFORE generic auth patterns. A message mentioning both
	// "deactivated" and "unauthorized" should classify as
	// auth_permanent, not auth.
	err := errors.New("account is deactivated and user is unauthorized")
	got := ClassifyError(err, "openai", "gpt-4")
	if got == nil {
		t.Fatal("expected classification, got nil")
	}
	if got.Reason != FailoverAuthPermanent {
		t.Fatalf("Reason = %q, want auth_permanent (must win over generic auth)", got.Reason)
	}
}

func TestClassifyError_Status413MapsToPayloadTooLarge(t *testing.T) {
	err := errors.New("upstream returned status: 413 request entity too large")
	got := ClassifyError(err, "openai", "gpt-4")
	if got == nil {
		t.Fatal("expected classification, got nil")
	}
	if got.Reason != FailoverPayloadTooLarge {
		t.Fatalf("Reason = %q, want payload_too_large", got.Reason)
	}
}

func TestRecoveryHintsCoverEveryReason(t *testing.T) {
	// Exhaustiveness check: every FailoverReason should have a
	// defined answer to each of the four hint methods, even if the
	// answer is "false". This test guards against a future reason
	// being added without updating the hint methods.
	reasons := []FailoverReason{
		FailoverAuth,
		FailoverAuthPermanent,
		FailoverRateLimit,
		FailoverBilling,
		FailoverTimeout,
		FailoverContextOverflow,
		FailoverPayloadTooLarge,
		FailoverFormat,
		FailoverOverloaded,
		FailoverModelNotFound,
		FailoverSessionExpired,
		FailoverThinkingSignature,
		FailoverLongContextTier,
		FailoverUnknown,
	}

	// Known ground truth for the recovery hints — one row per reason.
	type expected struct {
		retriable       bool
		shouldCompress  bool
		shouldRotate    bool
		shouldFallback  bool
		isAuth          bool
	}
	want := map[FailoverReason]expected{
		FailoverAuth:              {retriable: true, shouldRotate: true, shouldFallback: true, isAuth: true},
		FailoverAuthPermanent:     {retriable: false, shouldFallback: true, isAuth: true},
		FailoverRateLimit:         {retriable: true, shouldFallback: true},
		FailoverBilling:           {retriable: true, shouldRotate: true, shouldFallback: true},
		FailoverTimeout:           {retriable: true, shouldFallback: true},
		FailoverContextOverflow:   {retriable: true, shouldCompress: true},
		FailoverPayloadTooLarge:   {retriable: true, shouldCompress: true},
		FailoverFormat:            {retriable: false},
		FailoverOverloaded:        {retriable: true, shouldFallback: true},
		FailoverModelNotFound:     {retriable: false, shouldFallback: true},
		FailoverSessionExpired:    {retriable: true},
		FailoverThinkingSignature: {retriable: true},
		FailoverLongContextTier:   {retriable: true, shouldFallback: true},
		FailoverUnknown:           {retriable: true},
	}

	if len(want) != len(reasons) {
		t.Fatalf("recovery hint matrix has %d entries but there are %d reasons; a new reason was added without updating this test", len(want), len(reasons))
	}

	for _, r := range reasons {
		t.Run(string(r), func(t *testing.T) {
			fe := &FailoverError{Reason: r}
			w := want[r]
			if got := fe.IsRetriable(); got != w.retriable {
				t.Errorf("IsRetriable() = %v, want %v", got, w.retriable)
			}
			if got := fe.ShouldCompress(); got != w.shouldCompress {
				t.Errorf("ShouldCompress() = %v, want %v", got, w.shouldCompress)
			}
			if got := fe.ShouldRotateCredential(); got != w.shouldRotate {
				t.Errorf("ShouldRotateCredential() = %v, want %v", got, w.shouldRotate)
			}
			if got := fe.ShouldFallback(); got != w.shouldFallback {
				t.Errorf("ShouldFallback() = %v, want %v", got, w.shouldFallback)
			}
			if got := fe.IsAuth(); got != w.isAuth {
				t.Errorf("IsAuth() = %v, want %v", got, w.isAuth)
			}
		})
	}
}
