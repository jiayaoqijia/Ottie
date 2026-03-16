package slack

import (
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func init() {
	channels.RegisterFactory("slack", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewSlackChannel(cfg.Channels.Slack, b)
	})
}
