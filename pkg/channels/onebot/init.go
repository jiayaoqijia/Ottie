package onebot

import (
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func init() {
	channels.RegisterFactory("onebot", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewOneBotChannel(cfg.Channels.OneBot, b)
	})
}
