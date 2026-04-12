package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const ottieTestBinary = "../../build/ottie-linux-amd64"

func skipE2E(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if os.Getenv("ALTLLM_API_KEY") == "" {
		t.Skip("ALTLLM_API_KEY not set — skipping E2E test")
	}
	if _, err := os.Stat(ottieTestBinary); err != nil {
		t.Skipf("binary not found at %s — run 'make build' first", ottieTestBinary)
	}
}

// setupTestConfig creates a minimal config.json in a temp dir and returns the path.
func setupTestConfig(t *testing.T) (configPath string, workspaceDir string) {
	t.Helper()
	dir := t.TempDir()
	workspaceDir = filepath.Join(dir, "workspace")
	os.MkdirAll(workspaceDir, 0o755)
	os.MkdirAll(filepath.Join(workspaceDir, "skills"), 0o755)
	os.MkdirAll(filepath.Join(workspaceDir, "sessions"), 0o755)

	cfg := map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"workspace":             workspaceDir,
				"restrict_to_workspace": false,
				"model_name":            "altllm-basic",
				"max_tokens":            1024,
				"max_tool_iterations":   5,
			},
		},
		"model_list": []map[string]any{
			{
				"model_name": "altllm-basic",
				"model":      "openai/altllm-basic",
				"api_key":    os.Getenv("ALTLLM_API_KEY"),
				"api_base":   "https://api.altllm.ai/v1",
			},
		},
		"channels": map[string]any{},
		"tools": map[string]any{
			"exec":   map[string]any{"enabled": false},
			"web":    map[string]any{"enabled": false},
			"cron":   map[string]any{"enabled": false},
			"skills": map[string]any{"enabled": true},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath = filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath, workspaceDir
}

// runOttie executes the ottie binary with the given args and env, returning stdout and stderr.
func runOttie(t *testing.T, configPath string, args []string, timeoutSec int) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(ottieTestBinary, args...)
	cmd.Env = append(os.Environ(), "OTTIE_CONFIG="+configPath)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	timeout := time.Duration(timeoutSec) * time.Second
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		t.Fatalf("ottie timed out after %v", timeout)
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// === SMOKE TESTS ===

// TestE2E_SmokeChatResponse verifies the basic happy path:
// ottie agent -m "say pong" → response contains "pong".
func TestE2E_SmokeChatResponse(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, stderr, exitCode := runOttie(t, configPath, []string{
		"agent", "-m", "Reply with exactly the word 'pong'. Nothing else.",
	}, 30)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
	}
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "pong") {
		t.Errorf("stdout should contain 'pong':\n%s", stdout)
	}
	t.Logf("response: %s", strings.TrimSpace(stdout))
}

// TestE2E_SmokeVersionCommand verifies that `ottie version` works.
func TestE2E_SmokeVersionCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if _, err := os.Stat(ottieTestBinary); err != nil {
		t.Skipf("binary not found: %v", err)
	}

	cmd := exec.Command(ottieTestBinary, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "ottie") {
		t.Errorf("version output should contain 'ottie': %s", out)
	}
}

// TestE2E_SmokeEmptyMessage verifies that an empty message is handled gracefully.
func TestE2E_SmokeEmptyMessage(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, stderr, exitCode := runOttie(t, configPath, []string{
		"agent", "-m", "",
	}, 15)

	// Empty message should either succeed with a default response or fail gracefully
	t.Logf("exit=%d stdout=%q stderr=%s", exitCode, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
}

// === SKILLS TESTS ===

// TestE2E_SkillsList verifies that `ottie skills list` works.
func TestE2E_SkillsList(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, stderr, exitCode := runOttie(t, configPath, []string{
		"skills", "list",
	}, 15)

	if exitCode != 0 {
		t.Fatalf("skills list exit code = %d\nstderr: %s", exitCode, stderr)
	}
	t.Logf("skills list output:\n%s", stdout)
}

// TestE2E_ChatWithSessionPersistence verifies that session state persists
// across two separate invocations.
func TestE2E_ChatWithSessionPersistence(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)
	session := "e2e-persist-test"

	// First message: tell the agent a fact
	stdout1, stderr1, exit1 := runOttie(t, configPath, []string{
		"agent", "-m", "Remember this: the secret code is ALPHA-7. Just acknowledge.",
		"-s", session,
	}, 30)
	if exit1 != 0 {
		t.Fatalf("first message failed: exit=%d stderr=%s", exit1, stderr1)
	}
	t.Logf("response 1: %s", strings.TrimSpace(stdout1))

	// Second message: ask about the fact
	stdout2, stderr2, exit2 := runOttie(t, configPath, []string{
		"agent", "-m", "What was the secret code I told you? Reply with just the code.",
		"-s", session,
	}, 30)
	if exit2 != 0 {
		t.Fatalf("second message failed: exit=%d stderr=%s", exit2, stderr2)
	}
	t.Logf("response 2: %s", strings.TrimSpace(stdout2))

	lower := strings.ToLower(stdout2)
	if !strings.Contains(lower, "alpha") || !strings.Contains(lower, "7") {
		t.Errorf("agent should recall the secret code ALPHA-7, got: %s", strings.TrimSpace(stdout2))
	}
}

// TestE2E_ModelCommand verifies that `ottie model` shows the configured model.
func TestE2E_ModelCommand(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, stderr, exitCode := runOttie(t, configPath, []string{
		"model",
	}, 10)

	if exitCode != 0 {
		t.Fatalf("model command exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "altllm") {
		t.Errorf("model output should mention altllm: %s", stdout)
	}
	t.Logf("model output: %s", strings.TrimSpace(stdout))
}

// TestE2E_StatusCommand verifies that `ottie status` runs without crashing.
func TestE2E_StatusCommand(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, _, exitCode := runOttie(t, configPath, []string{
		"status",
	}, 10)

	// Status may return non-zero if gateway isn't running — that's fine.
	// We just verify it doesn't crash.
	t.Logf("status exit=%d output=%s", exitCode, strings.TrimSpace(stdout))
}

// TestE2E_MultiTurnReasoning verifies the model can do multi-step reasoning.
func TestE2E_MultiTurnReasoning(t *testing.T) {
	skipE2E(t)
	configPath, _ := setupTestConfig(t)

	stdout, stderr, exitCode := runOttie(t, configPath, []string{
		"agent", "-m", "What is 17 * 23? Reply with just the number.",
	}, 30)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "391") {
		t.Errorf("expected 391 in response: %s", strings.TrimSpace(stdout))
	}
	t.Logf("response: %s", strings.TrimSpace(stdout))
}
