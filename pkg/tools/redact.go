package tools

import "regexp"

const redactedPlaceholder = "[REDACTED]"

// redactPatterns are compiled regexps that match sensitive data in tool output.
var redactPatterns = []*regexp.Regexp{
	// Token prefixes (Anthropic, OpenAI, GitHub, Slack, Google, AWS)
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`xoxb-[A-Za-z0-9\-]{20,}`),
	regexp.MustCompile(`xapp-[A-Za-z0-9\-]{20,}`),
	regexp.MustCompile(`AIza[A-Za-z0-9_\-]{30,}`),
	regexp.MustCompile(`AKIA[A-Z0-9]{16,}`),

	// Environment variable assignments containing secrets
	regexp.MustCompile(`(?i)\b[A-Z_]*KEY\s*=\s*\S+`),
	regexp.MustCompile(`(?i)\b[A-Z_]*TOKEN\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)\b[A-Z_]*SECRET\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)\b[A-Z_]*PASSWORD\s*[=:]\s*\S+`),

	// PEM private key blocks
	regexp.MustCompile(`(?s)-----BEGIN[A-Z ]*PRIVATE KEY-----.*?-----END[A-Z ]*PRIVATE KEY-----`),

	// Authorization headers
	regexp.MustCompile(`(?i)Authorization:\s*(Bearer\s+)?\S+`),

	// Long base64-like strings (40+ chars) that likely represent tokens/keys
	regexp.MustCompile(`[A-Za-z0-9+/=]{40,}`),
}

// RedactSensitive replaces sensitive patterns in s with [REDACTED].
func RedactSensitive(s string) string {
	for _, pat := range redactPatterns {
		s = pat.ReplaceAllString(s, redactedPlaceholder)
	}
	return s
}
