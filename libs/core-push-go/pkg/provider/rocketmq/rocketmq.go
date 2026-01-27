package rocketmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"github.com/lzyats/core-push-go/pkg/push"
)

type Publisher struct {
	cfg push.RocketMQSettings

	once sync.Once
	p    rmq.Producer
	err  error
}

func New(cfg push.RocketMQSettings) *Publisher {
	return &Publisher{cfg: cfg}
}

func (p *Publisher) Type() string { return "rocketmq" }

func (p *Publisher) init() {
	p.once.Do(func() {
		if p.cfg.NameServer == "" {
			p.err = fmt.Errorf("rocketmq: missing name-server")
			return
		}
		if p.cfg.Producer.Group == "" {
			p.err = fmt.Errorf("rocketmq: missing producer.group")
			return
		}
		if p.cfg.Topic == "" {
			p.err = fmt.Errorf("rocketmq: missing topic")
			return
		}

		opts := []producer.Option{
			producer.WithNameServer([]string{p.cfg.NameServer}),
			producer.WithGroupName(p.cfg.Producer.Group),
			producer.WithRetry(2),
		}

		// ACL: when access/secret provided.
		if p.cfg.Producer.AccessKey != "" || p.cfg.Producer.SecretKey != "" {
			opts = append(opts, producer.WithCredentials(primitive.Credentials{
				AccessKey: p.cfg.Producer.AccessKey,
				SecretKey: p.cfg.Producer.SecretKey,
			}))
		}

		prd, err := rmq.NewProducer(opts...)
		if err != nil {
			p.err = err
			return
		}
		if err := prd.Start(); err != nil {
			p.err = err
			return
		}
		p.p = prd
	})
}

// Publish sends msg as JSON body to RocketMQ topic.
func (p *Publisher) Publish(ctx context.Context, msg push.Message) (push.Result, error) {
	p.init()
	if p.err != nil {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: p.err.Error()}, p.err
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: err.Error()}, err
	}

	m := primitive.NewMessage(p.cfg.Topic, b)
	if p.cfg.Tag != "" {
		m.WithTag(p.cfg.Tag)
	}

	res, err := p.p.SendSync(ctx, m)
	if err != nil {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: err.Error()}, err
	}
	return push.Result{OK: true, Provider: p.Type(), At: time.Now(), Body: res.String()}, nil
}

func (p *Publisher) Close() error {
	if p.p != nil {
		return p.p.Shutdown()
	}
	return nil
}
