package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-kratos/kratos/v2/log"
)

// MQTTClient wraps MQTT protocol operations for EMQX.
type MQTTClient struct {
	client      mqtt.Client
	log         *log.Helper
	topicPrefix string
	defaultQos  byte
}

func newMQTTClient(c *conf.Data_EMQX, logger log.Logger) (*MQTTClient, error) {
	helper := log.NewHelper(logger)
	if c == nil || !c.Enabled {
		helper.Info("emqx mqtt disabled in config")
		return nil, nil
	}
	broker := strings.TrimSpace(c.Broker)
	if broker == "" {
		return nil, fmt.Errorf("emqx broker is empty")
	}
	clientID := strings.TrimSpace(c.ClientId)
	if clientID == "" {
		clientID = fmt.Sprintf("powerbank-%d", time.Now().UnixNano())
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	if wsBroker := strings.TrimSpace(c.WsBroker); wsBroker != "" {
		opts.AddBroker(wsBroker)
	}
	opts.SetClientID(clientID)
	opts.SetUsername(c.Username)
	opts.SetPassword(c.Password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(30 * time.Second)
	connectTimeout := 5 * time.Second
	if c.ConnectTimeout != nil {
		connectTimeout = c.ConnectTimeout.AsDuration()
	}
	opts.SetConnectTimeout(connectTimeout)
	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		helper.Errorf("mqtt connection lost: %v", err)
	}
	opts.OnConnect = func(client mqtt.Client) {
		helper.Infof("mqtt connected, client_id=%s", clientID)
		defaultSubTopic := strings.TrimSpace(c.DefaultSubTopic)
		if defaultSubTopic == "" {
			return
		}
		subTopic := normalizeTopic(c.TopicPrefix, defaultSubTopic)
		token := client.Subscribe(subTopic, byte(c.Qos), func(_ mqtt.Client, msg mqtt.Message) {
			helper.Infof("mqtt received topic=%s payload=%s", msg.Topic(), string(msg.Payload()))
		})
		if !token.WaitTimeout(connectTimeout) {
			helper.Warnf("mqtt subscribe timeout, topic=%s", subTopic)
			return
		}
		if err := token.Error(); err != nil {
			helper.Errorf("mqtt subscribe failed, topic=%s err=%v", subTopic, err)
			return
		}
		helper.Infof("mqtt subscribed default topic=%s", subTopic)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(connectTimeout) {
		return nil, fmt.Errorf("mqtt connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, err
	}

	return &MQTTClient{
		client:      client,
		log:         helper,
		topicPrefix: strings.TrimSpace(c.TopicPrefix),
		defaultQos:  byte(c.Qos),
	}, nil
}

// Publish publishes message to topic with context control.
func (m *MQTTClient) Publish(ctx context.Context, topic string, payload []byte, qos byte, retained bool) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("mqtt client not initialized")
	}
	fullTopic := normalizeTopic(m.topicPrefix, topic)
	if qos > 2 {
		qos = m.defaultQos
	}
	token := m.client.Publish(fullTopic, qos, retained, payload)
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()
	select {
	case <-done:
		return token.Error()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close disconnects mqtt client.
func (m *MQTTClient) Close() error {
	if m == nil || m.client == nil {
		return nil
	}
	m.client.Disconnect(250)
	return nil
}

func normalizeTopic(prefix, topic string) string {
	prefix = strings.Trim(prefix, "/")
	topic = strings.Trim(topic, "/")
	if prefix == "" {
		return topic
	}
	if topic == "" {
		return prefix
	}
	return prefix + "/" + topic
}
