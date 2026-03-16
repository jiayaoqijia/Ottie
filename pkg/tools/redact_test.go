package tools

import (
	"strings"
	"testing"
)

func TestRedactSensitive_TokenPrefixes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"anthropic key", "sk-ant-abc123def456ghi789"},
		{"openai key", "sk-abcdefghijklmnopqrstuvwxyz1234567890"},
		{"github PAT", "ghp_abcdefghijklmnopqrstuvwxyz1234567890"},
		{"github fine-grained", "github_pat_abcdefghijklmnopqrst"},
		{"slack bot token", "xoxb-1234567890-abcdefghijklmnop"},
		{"slack app token", "xapp-1234567890-abcdefghijklmnop"},
		{"google api key", "AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz123456"},
		{"aws access key", "AKIAIOSFODNN7EXAMPLE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactSensitive("Output: " + tt.input + " done")
			if strings.Contains(result, tt.input) {
				t.Errorf("expected %q to be redacted, got: %s", tt.input, result)
			}
			if !strings.Contains(result, "[REDACTED]") {
				t.Errorf("expected [REDACTED] in output, got: %s", result)
			}
		})
	}
}

func TestRedactSensitive_EnvAssignments(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"API_KEY assignment", "API_KEY=sk-supersecret123"},
		{"TOKEN assignment", "GITHUB_TOKEN=ghp_abc123"},
		{"SECRET colon", "SECRET: mysecretvalue"},
		{"PASSWORD assignment", "DB_PASSWORD=hunter2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactSensitive(tt.input)
			if strings.Contains(result, "supersecret") || strings.Contains(result, "hunter2") ||
				strings.Contains(result, "mysecretvalue") {
				t.Errorf("expected secret value to be redacted, got: %s", result)
			}
		})
	}
}

func TestRedactSensitive_PEMBlocks(t *testing.T) {
	pem := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/yGiNBHSQo
-----END RSA PRIVATE KEY-----`

	result := RedactSensitive("Here is key:\n" + pem + "\nDone")
	if strings.Contains(result, "MIIEpAIBAAK") {
		t.Errorf("expected PEM block to be redacted, got: %s", result)
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] placeholder, got: %s", result)
	}
}

func TestRedactSensitive_AuthHeaders(t *testing.T) {
	tests := []string{
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIx",
		"Authorization: sk-ant-abcdef123456789012345",
	}

	for _, input := range tests {
		result := RedactSensitive(input)
		if strings.Contains(result, "eyJhbGci") || strings.Contains(result, "sk-ant") {
			t.Errorf("expected auth header to be redacted, got: %s", result)
		}
	}
}

func TestRedactSensitive_LongBase64(t *testing.T) {
	b64 := "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkw"
	result := RedactSensitive("token=" + b64)
	if strings.Contains(result, b64) {
		t.Errorf("expected long base64 string to be redacted, got: %s", result)
	}
}

func TestRedactSensitive_SafeContent(t *testing.T) {
	safe := "Hello world, file count: 42, path: /home/user/project"
	result := RedactSensitive(safe)
	if result != safe {
		t.Errorf("expected safe content to be unchanged, got: %s", result)
	}
}
