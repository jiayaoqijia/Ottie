package providers

import (
	"errors"
	"fmt"
	"strings"
)

// UserFacingError converts a provider/fallback error into a friendly,
// actionable message for end users. Uses emoji prefixes + explanation +
// recovery action, following OpenClaw's formatAssistantErrorText pattern.
func UserFacingError(err error) string {
	if err == nil {
		return ""
	}

	// 1. Check for FailoverError (single-provider failure).
	var failErr *FailoverError
	if errors.As(err, &failErr) {
		return messageForReason(failErr.Reason, failErr.Provider, failErr.Model)
	}

	// 2. Check for FallbackExhaustedError (all candidates failed).
	var exhausted *FallbackExhaustedError
	if errors.As(err, &exhausted) {
		reason, provider, model := dominantReason(exhausted.Attempts)
		return messageForReason(reason, provider, model)
	}

	// 3. Check for context window / token limit errors via string matching.
	errMsg := strings.ToLower(err.Error())
	if isContextWindowError(errMsg) {
		return "Context overflow: prompt too large for the model. Try starting a new conversation or clearing the history."
	}

	// 4. Fallback: generic message.
	return "\u26a0\ufe0f Something went wrong while processing your message. Please try again later."
}

// messageForReason maps a FailoverReason to a user-friendly string.
func messageForReason(reason FailoverReason, provider, model string) string {
	label := providerLabel(provider, model)

	switch reason {
	case FailoverAuth:
		return fmt.Sprintf(
			"\u26a0\ufe0f %s returned an authentication error — the API key is missing or invalid. Please check your configuration.",
			label,
		)
	case FailoverRateLimit:
		return "\u26a0\ufe0f API rate limit reached. Please try again in a moment."
	case FailoverBilling:
		return fmt.Sprintf(
			"\u26a0\ufe0f %s returned a billing error — your API key has run out of credits or has an insufficient balance. Check your provider's billing dashboard and top up or switch to a different API key.",
			label,
		)
	case FailoverTimeout:
		return "\u26a0\ufe0f The AI service took too long to respond. Please try again."
	case FailoverModelNotFound:
		if model != "" {
			return fmt.Sprintf(
				"\u26a0\ufe0f The configured model %q was not found. Please check your model settings.",
				model,
			)
		}
		return "\u26a0\ufe0f The configured model was not found. Please check your model settings."
	case FailoverFormat:
		return "\u26a0\ufe0f The request format was rejected by the provider. This is likely a bug — please report it."
	case FailoverSessionExpired:
		return "\u26a0\ufe0f Your session has expired. Please start a new conversation."
	case FailoverOverloaded:
		return "The AI service is temporarily overloaded. Please try again in a moment."
	default:
		return "\u26a0\ufe0f Something went wrong while processing your message. Please try again later."
	}
}

// providerLabel returns a human-readable label like "openai (gpt-4)" or
// "API provider" when provider/model info is unavailable.
func providerLabel(provider, model string) string {
	if provider == "" && model == "" {
		return "API provider"
	}
	if provider != "" && model != "" {
		return fmt.Sprintf("%s (%s)", provider, model)
	}
	if provider != "" {
		return provider
	}
	return model
}

// dominantReason picks the most representative reason from a list of
// fallback attempts. It returns the reason, provider, and model from the
// first non-skipped attempt, preferring non-timeout reasons.
func dominantReason(attempts []FallbackAttempt) (FailoverReason, string, string) {
	var firstReason FailoverReason
	var firstProvider, firstModel string

	for _, a := range attempts {
		if a.Skipped {
			continue
		}
		if firstReason == "" {
			firstReason = a.Reason
			firstProvider = a.Provider
			firstModel = a.Model
		}
		// Prefer a non-timeout, non-empty reason over timeout.
		if a.Reason != "" && a.Reason != FailoverTimeout {
			return a.Reason, a.Provider, a.Model
		}
	}

	if firstReason != "" {
		return firstReason, firstProvider, firstModel
	}
	return FailoverUnknown, "", ""
}

// isContextWindowError detects context window / token limit errors.
// Reuses the same patterns checked in loop.go.
func isContextWindowError(msg string) bool {
	patterns := []string{
		"context_length_exceeded",
		"context window",
		"maximum context length",
		"token limit",
		"too many tokens",
		"max_tokens",
		"invalidparameter",
		"prompt is too long",
		"request too large",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
