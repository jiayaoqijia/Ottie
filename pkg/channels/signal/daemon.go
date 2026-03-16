package signal

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/config"
	"github.com/jiayaoqijia/ottie/pkg/logger"
)

// daemonProcess wraps an os/exec.Cmd for the signal-cli daemon.
type daemonProcess struct {
	cmd *exec.Cmd
}

// startDaemon spawns the signal-cli daemon process.
func startDaemon(cfg config.SignalConfig) (*daemonProcess, error) {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "signal-cli"
	}

	host := cfg.HTTPHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.HTTPPort
	if port == 0 {
		port = 8080
	}

	args := []string{}
	if cfg.Account != "" {
		args = append(args, "-a", cfg.Account)
	}
	args = append(args, "daemon",
		"--http", fmt.Sprintf("%s:%d", host, port),
		"--receive-mode", "on-start",
	)

	cmd := exec.Command(cliPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.InfoCF("signal", "Starting signal-cli daemon", map[string]any{
		"path": cliPath,
		"args": args,
	})

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start signal-cli: %w", err)
	}

	return &daemonProcess{cmd: cmd}, nil
}

// stopDaemon gracefully shuts down the signal-cli daemon.
// It sends SIGTERM first, waits up to 5 seconds, then SIGKILL if needed.
func stopDaemon(d *daemonProcess) {
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return
	}

	logger.InfoC("signal", "Stopping signal-cli daemon")

	// Send SIGTERM
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		logger.ErrorCF("signal", "Failed to send SIGTERM to signal-cli", map[string]any{
			"error": err.Error(),
		})
		return
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		_, err := d.cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
		logger.InfoC("signal", "signal-cli daemon stopped gracefully")
	case <-time.After(5 * time.Second):
		logger.WarnC("signal", "signal-cli daemon did not stop in time, sending SIGKILL")
		if err := d.cmd.Process.Kill(); err != nil {
			logger.ErrorCF("signal", "Failed to kill signal-cli", map[string]any{
				"error": err.Error(),
			})
		}
	}
}

// waitForDaemon polls the daemon's HTTP endpoint until it becomes ready
// or the timeout expires.
func waitForDaemon(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Use a JSON-RPC version call as health check since /api/v1/about
		// is not available in all signal-cli versions.
		body := []byte(`{"jsonrpc":"2.0","method":"version","id":0}`)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/rpc", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create health check request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("signal-cli daemon not ready after %s", timeout)
}
