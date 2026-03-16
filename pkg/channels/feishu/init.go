package feishu

import (
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func init() {
	channels.RegisterFactory("feishu", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewFeishuChannel(cfg.Channels.Feishu, b)
	})
}
