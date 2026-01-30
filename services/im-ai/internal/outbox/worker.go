package outbox

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/lzyats/core-push-go/pkg/delivery"
	"github.com/lzyats/core-push-go/pkg/event"
)

type Worker struct {
	repo *Repo
	prod delivery.Producer
	log  *zap.Logger

	tick  time.Duration
	batch int
	stop  chan struct{}
}

type Options struct {
	Tick  time.Duration
	Batch int
}

func NewWorker(repo *Repo, prod delivery.Producer, log *zap.Logger, opt Options) *Worker {
	if opt.Tick <= 0 {
		opt.Tick = 1 * time.Second
	}
	if opt.Batch <= 0 {
		opt.Batch = 200
	}
	return &Worker{
		repo:  repo,
		prod:  prod,
		log:   log,
		tick:  opt.Tick,
		batch: opt.Batch,
		stop:  make(chan struct{}),
	}
}

func (w *Worker) Start() {
	go func() {
		t := time.NewTicker(w.tick)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
				w.runOnce()
			}
		}
	}()
}

func (w *Worker) Stop() { close(w.stop) }

func (w *Worker) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	recs, err := w.repo.FetchDue(ctx, w.batch)
	cancel()
	if err != nil || len(recs) == 0 {
		return
	}

	for _, r := range recs {
		// payload_json is ImEvent
		var evt event.ImEvent
		if err := json.Unmarshal([]byte(r.PayloadJSON), &evt); err != nil {
			_ = w.repo.MarkFailed(context.Background(), r.ID, r.RetryCount+1, "decode:"+err.Error(), 10*time.Second)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := w.prod.Publish(ctx, &evt)
		cancel()
		if err == nil {
			_ = w.repo.MarkSent(context.Background(), r.ID)
			continue
		}

		rc := r.RetryCount + 1
		backoff := calcBackoff(rc)
		_ = w.repo.MarkFailed(context.Background(), r.ID, rc, err.Error(), backoff)
		if rc == 1 || rc%10 == 0 {
			w.log.Warn("outbox publish retry", zap.Int64("id", r.ID), zap.Int("retry", rc), zap.Duration("backoff", backoff), zap.Error(err))
		}
	}
}

func calcBackoff(retry int) time.Duration {
	// exponential backoff with cap
	if retry <= 0 {
		return 1 * time.Second
	}
	d := time.Duration(1<<min(retry, 8)) * time.Second // 2s..256s
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
