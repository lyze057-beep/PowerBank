package data

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type mqttNotifier struct {
	data *Data
	log  *log.Helper
}

// NewMQTTNotifier creates MQTT notifier based on EMQX client.
func NewMQTTNotifier(data *Data, logger log.Logger) biz.Notifier {
	return &mqttNotifier{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/notifier")),
	}
}

func (n *mqttNotifier) Publish(ctx context.Context, topic string, payload []byte) error {
	client := n.data.MQTT()
	if client == nil {
		return fmt.Errorf("mqtt client is disabled")
	}
	return client.Publish(ctx, topic, payload, 1, false)
}
