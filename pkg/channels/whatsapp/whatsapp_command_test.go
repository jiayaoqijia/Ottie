package whatsapp

import (
	"context"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func TestHandleIncomingMessage_DoesNotConsumeGenericCommandsLocally(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := &WhatsAppChannel{
		BaseChannel: channels.NewBaseChannel("whatsapp", config.WhatsAppConfig{}, messageBus, nil),
		ctx:         context.Background(),
	}

	ch.handleIncomingMessage(map[string]any{
		"type":    "message",
		"id":      "mid1",
		"from":    "user1",
		"chat":    "chat1",
		"content": "/help",
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	inbound, ok := messageBus.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected inbound message to be forwarded")
	}
	if inbound.Channel != "whatsapp" {
		t.Fatalf("channel=%q", inbound.Channel)
	}
	if inbound.Content != "/help" {
		t.Fatalf("content=%q", inbound.Content)
	}
}
