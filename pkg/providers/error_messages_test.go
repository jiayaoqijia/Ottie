package providers

import (
	"errors"
	"fmt"
	"testing"
)

func TestUserFacingError_Nil(t *testing.T) {
	if got := UserFacingError(nil); got != "" {
		t.Errorf("expected empty string for nil error, got %q", got)
	}
}

func TestUserFacingError_FailoverReasons(t *testing.T) {
	tests := []struct {
		reason   FailoverReason
		provider string
		model    string
		contains string
	}{
		{FailoverAuth, "openai", "gpt-4", "authentication error"},
		{FailoverAuth, "", "", "authentication error"},
		{FailoverRateLimit, "openai", "gpt-4", "rate limit reached"},
		{FailoverBilling, "openai", "gpt-4", "billing error"},
		{FailoverBilling, "", "", "billing error"},
		{FailoverTimeout, "openai", "gpt-4", "took too long"},
		{FailoverModelNotFound, "openai", "gpt-99", `"gpt-99" was not found`},
		{FailoverModelNotFound, "", "", "configured model was not found"},
		{FailoverFormat, "openai", "gpt-4", "request format was rejected"},
		{FailoverSessionExpired, "openai", "gpt-4", "session has expired"},
		{FailoverOverloaded, "openai", "gpt-4", "temporarily overloaded"},
		{FailoverUnknown, "openai", "gpt-4", "Something went wrong"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			err := &FailoverError{
				Reason:   tt.reason,
				Provider: tt.provider,
				Model:    tt.model,
				Wrapped:  errors.New("raw error"),
			}
			got := UserFacingError(err)
			if got == "" {
				t.Fatal("expected non-empty message")
			}
			if !contains(got, tt.contains) {
				t.Errorf("expected message to contain %q, got %q", tt.contains, got)
			}
		})
	}
}

func TestUserFacingError_ProviderLabel(t *testing.T) {
	// With provider and model.
	err := &FailoverError{
		Reason:   FailoverAuth,
		Provider: "openai",
		Model:    "gpt-4",
		Wrapped:  errors.New("unauthorized"),
	}
	got := UserFacingError(err)
	if !contains(got, "openai (gpt-4)") {
		t.Errorf("expected provider label in message, got %q", got)
	}

	// Without provider/model.
	err2 := &FailoverError{
		Reason:  FailoverAuth,
		Wrapped: errors.New("unauthorized"),
	}
	got2 := UserFacingError(err2)
	if !contains(got2, "API provider") {
		t.Errorf("expected 'API provider' fallback label, got %q", got2)
	}
}

func TestUserFacingError_FallbackExhausted(t *testing.T) {
	err := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "gpt-4", Reason: FailoverTimeout},
			{Provider: "anthropic", Model: "claude-3", Reason: FailoverBilling},
		},
	}
	got := UserFacingError(err)
	// Should pick billing (non-timeout) as the dominant reason.
	if !contains(got, "billing error") {
		t.Errorf("expected billing message for dominant reason, got %q", got)
	}
}

func TestUserFacingError_FallbackExhaustedAllTimeout(t *testing.T) {
	err := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "gpt-4", Reason: FailoverTimeout},
			{Provider: "anthropic", Model: "claude-3", Reason: FailoverTimeout},
		},
	}
	got := UserFacingError(err)
	if !contains(got, "took too long") {
		t.Errorf("expected timeout message, got %q", got)
	}
}

func TestUserFacingError_FallbackExhaustedSkipped(t *testing.T) {
	err := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "gpt-4", Skipped: true, Reason: FailoverRateLimit},
			{Provider: "anthropic", Model: "claude-3", Reason: FailoverAuth},
		},
	}
	got := UserFacingError(err)
	// Should skip the cooldown entry and pick auth.
	if !contains(got, "authentication error") {
		t.Errorf("expected auth message, got %q", got)
	}
}

func TestUserFacingError_ContextWindow(t *testing.T) {
	patterns := []string{
		"context_length_exceeded",
		"maximum context length exceeded",
		"too many tokens in request",
		"prompt is too long",
		"request too large",
	}
	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			err := errors.New(p)
			got := UserFacingError(err)
			if !contains(got, "Context overflow") {
				t.Errorf("expected context overflow message for %q, got %q", p, got)
			}
		})
	}
}

func TestUserFacingError_UnknownError(t *testing.T) {
	err := errors.New("some completely unknown error")
	got := UserFacingError(err)
	if !contains(got, "Something went wrong") {
		t.Errorf("expected generic fallback message, got %q", got)
	}
}

func TestUserFacingError_WrappedFailoverError(t *testing.T) {
	inner := &FailoverError{
		Reason:   FailoverAuth,
		Provider: "openai",
		Model:    "gpt-4",
		Wrapped:  errors.New("401 unauthorized"),
	}
	wrapped := fmt.Errorf("LLM call failed after retries: %w", inner)
	got := UserFacingError(wrapped)
	if !contains(got, "authentication error") {
		t.Errorf("expected auth message through wrapping, got %q", got)
	}
}

func TestProviderLabel(t *testing.T) {
	tests := []struct {
		provider, model, expected string
	}{
		{"openai", "gpt-4", "openai (gpt-4)"},
		{"openai", "", "openai"},
		{"", "gpt-4", "gpt-4"},
		{"", "", "API provider"},
	}
	for _, tt := range tests {
		got := providerLabel(tt.provider, tt.model)
		if got != tt.expected {
			t.Errorf("providerLabel(%q, %q) = %q, want %q", tt.provider, tt.model, got, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
