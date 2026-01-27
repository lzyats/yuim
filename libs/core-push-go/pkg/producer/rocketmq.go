package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"github.com/lzyats/core-push-go/pkg/event"
	"github.com/lzyats/core-push-go/pkg/push"
)

type RocketMQProducer struct {
	cfg push.RocketMQSettings
	p   rmq.Producer
}

func NewRocketMQ(cfg push.RocketMQSettings) (*RocketMQProducer, error) {
	if cfg.NameServer == "" {
		return nil, fmt.Errorf("rocketmq: missing name-server")
	}
	if cfg.Producer.Group == "" {
		return nil, fmt.Errorf("rocketmq: missing producer.group")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("rocketmq: missing topic")
	}
	opts := []producer.Option{
		producer.WithNameServer([]string{cfg.NameServer}),
		producer.WithGroupName(cfg.Producer.Group),
		producer.WithRetry(2),
	}
	if cfg.Producer.AccessKey != "" || cfg.Producer.SecretKey != "" {
		opts = append(opts, producer.WithCredentials(primitive.Credentials{
			AccessKey: cfg.Producer.AccessKey,
			SecretKey: cfg.Producer.SecretKey,
		}))
	}
	prd, err := rmq.NewProducer(opts...)
	if err != nil {
		return nil, err
	}
	if err := prd.Start(); err != nil {
		return nil, err
	}
	return &RocketMQProducer{cfg: cfg, p: prd}, nil
}

func (r *RocketMQProducer) Publish(ctx context.Context, evt *event.ImEvent) error {
	if evt == nil {
		return fmt.Errorf("nil event")
	}
	if evt.TS == 0 {
		evt.TS = time.Now().Unix()
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	m := primitive.NewMessage(r.cfg.Topic, b)
	if r.cfg.Tag != "" {
		m.WithTag(r.cfg.Tag)
	}
	_, err = r.p.SendSync(ctx, m)
	return err
}

func (r *RocketMQProducer) Close() error {
	if r.p != nil {
		return r.p.Shutdown()
	}
	return nil
}
