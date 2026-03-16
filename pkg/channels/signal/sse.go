package signal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/identity"
	"github.com/jiayaoqijia/ottie/pkg/logger"
)

// SSE envelope types from signal-cli

type sseEnvelope struct {
	Envelope signalEnvelope `json:"envelope"`
	Account  string         `json:"account"`
}

type signalEnvelope struct {
	Source       string       `json:"source"`
	SourceUUID   string       `json:"sourceUuid"`
	SourceName   string       `json:"sourceName"`
	Timestamp    int64        `json:"timestamp"`
	DataMessage  *dataMessage `json:"dataMessage"`
	SyncMessage  *syncMessage `json:"syncMessage"`
	SourceNumber string       `json:"sourceNumber"`
}

type syncMessage struct {
	SentMessage *sentMessage `json:"sentMessage"`
}

type sentMessage struct {
	Timestamp   int64      `json:"timestamp"`
	Message     string     `json:"message"`
	GroupInfo   *groupInfo `json:"groupInfo"`
	Mentions    []mention  `json:"mentions"`
	Destination string     `json:"destination"`
}

type dataMessage struct {
	Timestamp int64      `json:"timestamp"`
	Message   string     `json:"message"`
	GroupInfo *groupInfo `json:"groupInfo"`
	Mentions  []mention  `json:"mentions"`
}

type groupInfo struct {
	GroupID string `json:"groupId"`
	Type    string `json:"type"`
}

type mention struct {
	Start  int    `json:"start"`
	Length int    `json:"length"`
	UUID   string `json:"uuid"`
}

// sseLoop connects to the signal-cli SSE endpoint and processes incoming messages.
// It automatically reconnects with exponential backoff on failures.
func (c *SignalChannel) sseLoop() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		err := c.connectSSE()
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			logger.ErrorCF("signal", "SSE connection error", map[string]any{
				"error":   err.Error(),
				"backoff": backoff.String(),
			})

			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
			}

			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on successful connection
		backoff = time.Second
	}
}

// connectSSE establishes a single SSE connection and reads events until the
// connection drops or the context is canceled.
func (c *SignalChannel) connectSSE() error {
	url := c.baseURL + "/api/v1/events"

	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Use a separate client with no timeout for the long-lived SSE connection
	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE returned status %d", resp.StatusCode)
	}

	logger.InfoC("signal", "SSE connection established")

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			dataLines = append(dataLines, data)
			continue
		}

		// Empty line signals end of an event
		if line == "" && len(dataLines) > 0 {
			fullData := strings.Join(dataLines, "")
			dataLines = nil

			c.handleEnvelope(fullData)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE read: %w", err)
	}

	return fmt.Errorf("SSE connection closed")
}

// handleEnvelope processes a single Signal envelope from the SSE stream.
func (c *SignalChannel) handleEnvelope(data string) {
	var env sseEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		logger.ErrorCF("signal", "Failed to parse envelope", map[string]any{
			"error": err.Error(),
		})
		return
	}

	// Try dataMessage first (messages from others), then syncMessage (messages
	// from the primary device seen by this linked device).
	if env.Envelope.DataMessage != nil {
		c.handleDataMessage(&env, env.Envelope.DataMessage)
		return
	}

	if env.Envelope.SyncMessage != nil && env.Envelope.SyncMessage.SentMessage != nil {
		c.handleSyncSentMessage(&env, env.Envelope.SyncMessage.SentMessage)
		return
	}
}

// handleDataMessage processes a direct dataMessage (from another user).
func (c *SignalChannel) handleDataMessage(env *sseEnvelope, dm *dataMessage) {
	if dm.Message == "" {
		return
	}

	// Loop prevention: skip messages from our own account
	if c.config.AccountUUID != "" && env.Envelope.SourceUUID == c.config.AccountUUID {
		return
	}

	senderID := env.Envelope.Source
	if senderID == "" {
		senderID = env.Envelope.SourceUUID
	}

	sender := bus.SenderInfo{
		Platform:    "signal",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("signal", senderID),
		DisplayName: env.Envelope.SourceName,
	}

	if !c.IsAllowedSender(sender) {
		return
	}

	var chatID string
	var peer bus.Peer
	content := dm.Message

	if dm.GroupInfo != nil && dm.GroupInfo.GroupID != "" {
		chatID = "group:" + dm.GroupInfo.GroupID
		peer = bus.Peer{Kind: "group", ID: dm.GroupInfo.GroupID}

		isMentioned := c.isBotMentioned(dm.Mentions)
		shouldRespond, stripped := c.ShouldRespondInGroup(isMentioned, content)
		if !shouldRespond {
			return
		}
		content = stripped
	} else {
		chatID = senderID
		peer = bus.Peer{Kind: "direct", ID: senderID}
	}

	messageID := fmt.Sprintf("%d", env.Envelope.Timestamp)

	logger.InfoCF("signal", "Received message", map[string]any{
		"sender":    senderID,
		"chat_id":   chatID,
		"peer_kind": peer.Kind,
	})

	c.HandleMessage(c.ctx,
		peer,
		messageID,
		senderID,
		chatID,
		content,
		nil,
		nil,
		sender,
	)
}

// handleSyncSentMessage processes a sync message — a message sent from the
// primary device that the linked device observes. This allows the bot to
// respond to messages sent from the user's own phone in groups.
func (c *SignalChannel) handleSyncSentMessage(env *sseEnvelope, sm *sentMessage) {
	if sm.Message == "" {
		return
	}

	// For sync messages, the source is the account itself (our own messages).
	// We only process group sync messages — responding to your own DMs
	// would create a loop.
	if sm.GroupInfo == nil || sm.GroupInfo.GroupID == "" {
		return
	}

	senderID := env.Envelope.Source
	if senderID == "" {
		senderID = env.Envelope.SourceUUID
	}

	sender := bus.SenderInfo{
		Platform:    "signal",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("signal", senderID),
		DisplayName: env.Envelope.SourceName,
	}

	chatID := "group:" + sm.GroupInfo.GroupID
	peer := bus.Peer{Kind: "group", ID: sm.GroupInfo.GroupID}
	content := sm.Message

	isMentioned := c.isBotMentioned(sm.Mentions)
	shouldRespond, stripped := c.ShouldRespondInGroup(isMentioned, content)
	if !shouldRespond {
		return
	}
	content = stripped

	messageID := fmt.Sprintf("%d", env.Envelope.Timestamp)

	logger.InfoCF("signal", "Received sync group message", map[string]any{
		"sender":    senderID,
		"chat_id":   chatID,
		"peer_kind": peer.Kind,
	})

	c.HandleMessage(c.ctx,
		peer,
		messageID,
		senderID,
		chatID,
		content,
		nil,
		nil,
		sender,
	)
}

// isBotMentioned checks if any mention in the message refers to our account UUID.
func (c *SignalChannel) isBotMentioned(mentions []mention) bool {
	if c.config.AccountUUID == "" {
		return false
	}
	for _, m := range mentions {
		if m.UUID == c.config.AccountUUID {
			return true
		}
	}
	return false
}
