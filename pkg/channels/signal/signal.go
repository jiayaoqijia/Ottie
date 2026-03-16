package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
	"github.com/jiayaoqijia/ottie/pkg/logger"
)

// SignalChannel implements the channels.Channel interface for Signal messenger
// using signal-cli's JSON-RPC 2.0 API over HTTP.
type SignalChannel struct {
	*channels.BaseChannel
	config     config.SignalConfig
	httpClient *http.Client
	baseURL    string
	daemonCmd  *daemonProcess
	ctx        context.Context
	cancel     context.CancelFunc
	rpcID      atomic.Int64
}

// NewSignalChannel creates a new Signal channel instance.
func NewSignalChannel(cfg config.SignalConfig, messageBus *bus.MessageBus) (*SignalChannel, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("signal account (phone number) is required")
	}

	base := channels.NewBaseChannel("signal", cfg, messageBus, cfg.AllowFrom,
		channels.WithGroupTrigger(cfg.GroupTrigger),
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)

	host := cfg.HTTPHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.HTTPPort
	if port == 0 {
		port = 8080
	}

	return &SignalChannel{
		BaseChannel: base,
		config:      cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)),
	}, nil
}

// Start initializes the Signal channel, optionally starting the signal-cli daemon.
func (c *SignalChannel) Start(ctx context.Context) error {
	logger.InfoC("signal", "Starting Signal channel")

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Start daemon if auto-start is enabled
	if c.config.AutoStart {
		daemon, err := startDaemon(c.config)
		if err != nil {
			c.cancel()
			return fmt.Errorf("failed to start signal-cli daemon: %w", err)
		}
		c.daemonCmd = daemon

		if err := waitForDaemon(c.ctx, c.baseURL, 30*time.Second); err != nil {
			c.stopDaemon()
			c.cancel()
			return fmt.Errorf("signal-cli daemon failed to become ready: %w", err)
		}
		logger.InfoC("signal", "signal-cli daemon started and ready")
	}

	// Start SSE event listener
	go c.sseLoop()

	c.SetRunning(true)
	logger.InfoCF("signal", "Signal channel started", map[string]any{
		"account": c.config.Account,
		"baseURL": c.baseURL,
	})
	return nil
}

// Stop shuts down the Signal channel.
func (c *SignalChannel) Stop(ctx context.Context) error {
	logger.InfoC("signal", "Stopping Signal channel")

	if c.cancel != nil {
		c.cancel()
	}

	c.stopDaemon()

	c.SetRunning(false)
	logger.InfoC("signal", "Signal channel stopped")
	return nil
}

func (c *SignalChannel) stopDaemon() {
	if c.daemonCmd != nil {
		stopDaemon(c.daemonCmd)
		c.daemonCmd = nil
	}
}

// Send sends a message via Signal using JSON-RPC.
func (c *SignalChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	params := map[string]any{
		"account": c.config.Account,
		"message": msg.Content,
	}

	// Determine if this is a group or DM based on chatID format
	if strings.HasPrefix(msg.ChatID, "group:") {
		groupID := strings.TrimPrefix(msg.ChatID, "group:")
		params["groupId"] = groupID
	} else {
		params["recipients"] = []string{msg.ChatID}
	}

	_, err := c.rpcCall(ctx, "send", params)
	if err != nil {
		return fmt.Errorf("signal send: %w", err)
	}

	return nil
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int64  `json:"id"`
	Params  any    `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcCall performs a JSON-RPC 2.0 call to the signal-cli daemon.
func (c *SignalChannel) rpcCall(ctx context.Context, method string, params any) (json.RawMessage, error) {
	reqID := c.rpcID.Add(1)

	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      reqID,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/rpc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc call returned status %d", resp.StatusCode)
	}

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
