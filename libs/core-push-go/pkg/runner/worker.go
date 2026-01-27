package runner

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/lzyats/core-push-go/pkg/provider/getui"
	"github.com/lzyats/core-push-go/pkg/provider/rocketmq"
	"github.com/lzyats/core-push-go/pkg/push"
	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"
)

type Worker struct {
	st    push.Settings
	store *redisstore.Store
	getui *getui.Provider
	rmq   *rocketmq.Publisher
}

func NewWorker(st push.Settings) (*Worker, error) {
	st = st.WithDefaults()
	var store *redisstore.Store
	if st.Redis.Enabled == "Y" {
		var err error
		store, err = redisstore.New(st.Redis)
		if err != nil {
			return nil, err
		}
	}
	w := &Worker{
		st:    st,
		store: store,
	}
	if st.Push.Enabled == "Y" {
		w.getui = getui.New(st.Push)
	}
	if st.RocketMQ.Enabled == "Y" {
		w.rmq = rocketmq.New(st.RocketMQ)
	}
	return w, nil
}

func (w *Worker) Close() {
	if w.rmq != nil {
		_ = w.rmq.Close()
	}
	if w.store != nil {
		_ = w.store.Close()
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w.store == nil {
		return push.ErrNotConfigured
	}
	queueKey := strings.TrimSpace(w.st.Redis.QueueKey)
	if queueKey == "" {
		queueKey = "push:queue"
	}

	log.Printf("core-push-go worker started, queueKey=%s getui=%s rocketmq=%s", queueKey, w.st.Push.Enabled, w.st.RocketMQ.Enabled)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := w.store.Pop(ctx, queueKey, 5*time.Second)
		if err != nil {
			log.Printf("redis pop error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if payload == "" {
			continue
		}

		msg, err := redisstore.DecodeMessage(payload)
		if err != nil {
			log.Printf("decode error: %v payload=%q", err, payload)
			continue
		}

		// 1) GeTui push (optional)
		if w.getui != nil {
			if res, err := w.getui.Push(ctx, msg); err != nil {
				log.Printf("getui push error: %v body=%s", err, res.Body)
			} else {
				log.Printf("getui push ok: %s", res.Body)
			}
		}

		// 2) RocketMQ publish (optional)
		if w.rmq != nil {
			if res, err := w.rmq.Publish(ctx, msg); err != nil {
				log.Printf("rocketmq publish error: %v", err)
			} else {
				log.Printf("rocketmq publish ok: %s", res.Body)
			}
		}
	}
}
