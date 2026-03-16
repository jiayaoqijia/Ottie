package signal

import (
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func init() {
	channels.RegisterFactory("signal", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewSignalChannel(cfg.Channels.Signal, b)
	})
}
