package line

import (
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func init() {
	channels.RegisterFactory("line", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewLINEChannel(cfg.Channels.LINE, b)
	})
}
