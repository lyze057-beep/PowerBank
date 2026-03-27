package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"

	_ "github.com/go-sql-driver/mysql"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewGreeterRepo,
	NewUserRepo,
	NewPaymentRepo,
	NewWxPayGateway,
	NewAlipayGateway,
	NewWalletRepo,
	NewNotifyRepo,
	NewMQTTNotifier,
	NewSupportRepo,
	NewOpenAIChatClient,
)

// Data .
type Data struct {
	db   *sql.DB
	rdb  *redis.Client
	mqtt *MQTTClient
}

// NewData .
func NewData(c *conf.Data) (*Data, func(), error) {
	if c == nil || c.Database == nil || c.Redis == nil {
		return nil, nil, fmt.Errorf("invalid data config")
	}
	db, err := sql.Open(c.Database.Driver, c.Database.Source)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	redisOpts := &redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       int(c.Redis.Db),
	}
	if c.Redis.ReadTimeout != nil {
		redisOpts.ReadTimeout = c.Redis.ReadTimeout.AsDuration()
	}
	if c.Redis.WriteTimeout != nil {
		redisOpts.WriteTimeout = c.Redis.WriteTimeout.AsDuration()
	}
	rdb := redis.NewClient(redisOpts)
	if err = rdb.Ping(ctx).Err(); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, nil, err
	}

	mqttClient, err := newMQTTClient(c.Emqx, log.NewStdLogger(os.Stdout))
	if err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, nil, err
	}

	data := &Data{
		db:   db,
		rdb:  rdb,
		mqtt: mqttClient,
	}
	cleanup := func() {
		log.Info("closing the data resources")
		if err := db.Close(); err != nil {
			log.Errorf("close mysql failed: %v", err)
		}
		if err := rdb.Close(); err != nil {
			log.Errorf("close redis failed: %v", err)
		}
		if mqttClient != nil {
			if err := mqttClient.Close(); err != nil {
				log.Errorf("close mqtt failed: %v", err)
			}
		}
	}
	return data, cleanup, nil
}

// DB returns mysql client.
func (d *Data) DB() *sql.DB {
	return d.db
}

// Redis returns redis client.
func (d *Data) Redis() *redis.Client {
	return d.rdb
}

// MQTT returns emqx mqtt client.
func (d *Data) MQTT() *MQTTClient {
	return d.mqtt
}
