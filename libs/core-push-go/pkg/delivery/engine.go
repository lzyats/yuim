package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/lzyats/core-push-go/pkg/event"
	"github.com/lzyats/core-push-go/pkg/store/storeiface"
)

var ErrQueueFull = errors.New("delivery: vendor queue full")

// Producer publishes internal events to MQ. It does NOT consume MQ.
type Producer interface {
	Publish(ctx context.Context, evt *event.ImEvent) error
	Close() error
}

// VendorPush is optional offline notify provider (e.g. GeTui/APNs/FCM).
type VendorPush interface {
	PushNotify(ctx context.Context, uid int64, title, body string, data map[string]string) error
}

// CometSender sends a push packet to a specific comet node (gRPC/HTTP).
type CometSender interface {
	SendToComet(ctx context.Context, cometAddr string, uid int64, packetJSON string) error
}

type Options struct {
	QueueSize       int
	WorkerCount     int
	OfflineMaxKeep  int64
	RouteTTLSeconds int64
	OpTimeout       time.Duration
}

func (o Options) withDefaults() Options {
	if o.QueueSize <= 0 {
		o.QueueSize = 4096
	}
	if o.WorkerCount <= 0 {
		o.WorkerCount = 8
	}
	if o.OfflineMaxKeep <= 0 {
		o.OfflineMaxKeep = 2000
	}
	if o.RouteTTLSeconds <= 0 {
		o.RouteTTLSeconds = 60
	}
	if o.OpTimeout <= 0 {
		o.OpTimeout = 3 * time.Second
	}
	return o
}

type Engine struct {
	store  storeiface.DeliveryStore
	prod   Producer
	sender CometSender
	vendor VendorPush
	opts   Options

	vq     chan vendorTask
	stopCh chan struct{}
}

type vendorTask struct {
	uid   int64
	title string
	body  string
	data  map[string]string
}

func New(store storeiface.DeliveryStore, prod Producer, sender CometSender, vendor VendorPush, opts Options) *Engine {
	opts = opts.withDefaults()
	e := &Engine{
		store:  store,
		prod:   prod,
		sender: sender,
		vendor: vendor,
		opts:   opts,
		vq:     make(chan vendorTask, opts.QueueSize),
		stopCh: make(chan struct{}),
	}
	if vendor != nil {
		for i := 0; i < opts.WorkerCount; i++ {
			go e.vendorWorker()
		}
	}
	return e
}

func (e *Engine) Close() error {
	select {
	case <-e.stopCh:
	default:
		close(e.stopCh)
	}
	if e.prod != nil {
		return e.prod.Close()
	}
	return nil
}

func (e *Engine) vendorWorker() {
	for {
		select {
		case <-e.stopCh:
			return
		case t := <-e.vq:
			ctx, cancel := context.WithTimeout(context.Background(), e.opts.OpTimeout)
			_ = e.vendor.PushNotify(ctx, t.uid, t.title, t.body, t.data)
			cancel()
		}
	}
}

func (e *Engine) Publish(ctx context.Context, evt *event.ImEvent) error {
	if e.prod == nil {
		return errors.New("delivery: producer not configured")
	}
	if evt != nil && evt.TS == 0 {
		evt.TS = time.Now().Unix()
	}
	return e.prod.Publish(ctx, evt)
}

// DeliverOrOffline tries online delivery via comet route; if offline or fail => enqueue offline and async vendor notify.
// packet is marshaled to JSON for storage/transport (can be upgraded to protobuf later).
func (e *Engine) DeliverOrOffline(ctx context.Context, uid int64, convID string, seq int64, packet any, notifyTitle, notifyBody string, notifyData map[string]string) (bool, error) {
	b, err := json.Marshal(packet)
	if err != nil {
		return false, err
	}
	packetJSON := string(b)

	// 1) route lookup
	rctx, cancel := context.WithTimeout(ctx, e.opts.OpTimeout)
	cometAddr, rerr := e.store.GetRoute(rctx, uid)
	cancel()

	// 2) online send if possible
	if rerr == nil && cometAddr != "" && e.sender != nil {
		sctx, cancel := context.WithTimeout(ctx, e.opts.OpTimeout)
		serr := e.sender.SendToComet(sctx, cometAddr, uid, packetJSON)
		cancel()
		if serr == nil {
			return true, nil
		}
		// fallthrough to offline on send fail
	}

	// 3) offline enqueue (best-effort)
	octx, cancel := context.WithTimeout(ctx, e.opts.OpTimeout)
	oerr := e.store.EnqueueOffline(octx, uid, convID, seq, packetJSON, e.opts.OfflineMaxKeep)
	cancel()

	// 4) vendor notify (bounded, drop on full)
	if e.vendor != nil && (notifyTitle != "" || notifyBody != "") {
		select {
		case e.vq <- vendorTask{uid: uid, title: notifyTitle, body: notifyBody, data: notifyData}:
		default:
			// drop
		}
	}

	if rerr != nil {
		return false, rerr
	}
	return false, oerr
}

func (e *Engine) PullOffline(ctx context.Context, uid int64, convID string, afterSeq int64, limit int64) ([]string, int64, error) {
	pctx, cancel := context.WithTimeout(ctx, e.opts.OpTimeout)
	defer cancel()
	return e.store.PullOffline(pctx, uid, convID, afterSeq, limit)
}

func (e *Engine) Ack(ctx context.Context, uid int64, convID string, seq int64) error {
	actx, cancel := context.WithTimeout(ctx, e.opts.OpTimeout)
	defer cancel()
	return e.store.Ack(actx, uid, convID, seq)
}
