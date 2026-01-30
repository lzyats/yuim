package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	rmq "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/lzyats/core-push-go/pkg/event"
	"github.com/lzyats/core-push-go/pkg/push"
	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"

	"yuim/services/im-job/internal/breaker"
	"yuim/services/im-job/internal/comet"
	"yuim/services/im-job/internal/config"
	"yuim/services/im-job/internal/metrics"
	"yuim/services/im-job/internal/repo"
)

// Packet is the payload sent to Comet (can be protobuf later).
type Packet struct {
	Event   string        `json:"event"`
	TraceID string        `json:"trace_id,omitempty"`
	TS      int64         `json:"ts"`
	ConvID  string        `json:"conv_id"`
	FromUID int64         `json:"from_uid"`
	Msg     event.Message `json:"msg"`
}

type batchItem struct {
	uid   int64
	conv  string
	seq   int64
	pjson string
}

func main() {
	var cfgPaths string
	flag.StringVar(&cfgPaths, "c", "./config.yml", "config file path (supports: a.yml,b.yml)")
	flag.Parse()

	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load(cfgPaths)
	if err != nil {
		log.Fatal("load config failed", zap.Error(err))
	}

	metrics.Register()
	go serveMetrics(cfg.Metrics.Addr, log)

	// Redis store (core-push-go)
	store, err := redisstore.New(parseRedisSettings(cfg))
	if err != nil {
		log.Fatal("redis init failed", zap.Error(err))
	}
	defer store.Close()

	// Comet sender
	sender := comet.NewHTTPSender(cfg.Comet.Timeout, cfg.Comet.PushPath)
	sender.PushBatchPath = cfg.Comet.PushBatchPath

	// Circuit breaker (optional)
	var brk *breaker.Breaker
	if cfg.Breaker.Enabled {
		brk = breaker.New(breaker.Options{
			Threshold: cfg.Breaker.Threshold,
			Window:    cfg.Breaker.Window,
			OpenFor:   cfg.Breaker.OpenFor,
		})
	}

	// Batcher: group by cometAddr and flush periodically
	b := newBatcher(store, sender, brk, cfg, log)
	b.start()
	defer b.stop()

	// RocketMQ consumer (CLUSTERING)
	c, err := rmq.NewPushConsumer(
		consumer.WithNameServer([]string{cfg.RocketMQ.NameServer}),
		consumer.WithGroupName(cfg.RocketMQ.Group),
		consumer.WithConsumerModel(consumer.Clustering),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromFirstOffset),
	)
	if err != nil {
		log.Fatal("rocketmq consumer init failed", zap.Error(err))
	}

	selector := consumer.MessageSelector{Type: consumer.TAG, Expression: "*"}
	if cfg.RocketMQ.Tag != "" {
		selector.Expression = cfg.RocketMQ.Tag
	}

	err = c.Subscribe(cfg.RocketMQ.Topic, selector, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, m := range msgs {
			metrics.Consumed.Inc()

			var evt event.ImEvent
			if err := json.Unmarshal(m.Body, &evt); err != nil {
				metrics.EventDecodeFail.Inc()
				log.Warn("event decode failed", zap.Error(err))
				continue // drop bad message
			}

			// Consumer idempotency: dedupe by msg_id (production建议保留)
			if evt.Msg.MsgID > 0 {
				ctx2, cancel2 := context.WithTimeout(ctx, cfg.Delivery.OpTimeout)
				first, err := store.DedupeMsg(ctx2, evt.Msg.MsgID, int64(cfg.Dedupe.TTL.Seconds()))
				cancel2()
				if err != nil {
					// Redis error: do not drop; proceed (at-least-once)
				} else if !first {
					metrics.Duplicates.Inc()
					continue
				}
			}

			// Fan-out: build packet json per receiver and push into batcher
			for _, uid := range evt.ToUIDs {
				pkt := Packet{
					Event:   evt.Event,
					TraceID: evt.TraceID,
					TS:      evt.TS,
					ConvID:  evt.ConvID,
					FromUID: evt.FromUID,
					Msg:     evt.Msg,
				}
				pb, err := json.Marshal(pkt)
				if err != nil {
					continue
				}
				b.enqueue(uid, evt.ConvID, evt.Msg.Seq, string(pb))
			}
		}
		return consumer.ConsumeSuccess, nil
	})
	if err != nil {
		log.Fatal("rocketmq subscribe failed", zap.Error(err))
	}

	if err := c.Start(); err != nil {
		log.Fatal("rocketmq consumer start failed", zap.Error(err))
	}
	log.Info("im-job started",
		zap.String("topic", cfg.RocketMQ.Topic),
		zap.String("group", cfg.RocketMQ.Group),
		zap.String("metrics", cfg.Metrics.Addr),
	)

	// Graceful shutdown
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutdown signal received")
	_ = c.Shutdown()
	log.Info("im-job stopped")
}

func serveMetrics(addr string, log *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	log.Info("metrics listening", zap.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("metrics server error", zap.Error(err))
	}
}

// parseRedisSettings adapts YAML config to core-push-go's RedisSettings constructor (Host+Port).
func parseRedisSettings(cfg *config.Config) push.RedisSettings {
	host := cfg.Redis.Addr
	port := 6379

	for i := 0; i < len(cfg.Redis.Addr); i++ {
		if cfg.Redis.Addr[i] == ':' {
			host = cfg.Redis.Addr[:i]
			p := 0
			for j := i + 1; j < len(cfg.Redis.Addr); j++ {
				ch := cfg.Redis.Addr[j]
				if ch < '0' || ch > '9' {
					break
				}
				p = p*10 + int(ch-'0')
			}
			if p > 0 {
				port = p
			}
			break
		}
	}

	return push.RedisSettings{
		Enabled:  "Y",
		Host:     host,
		Port:     port,
		Password: cfg.Redis.Password,
		Database: cfg.Redis.Database,
	}
}

/* ---------------- batching + retry + breaker ---------------- */

type deliveryStore interface {
	GetRoute(ctx context.Context, uid int64) (string, error)
	EnqueueOffline(ctx context.Context, uid int64, convID string, seq int64, packetJSON string, maxKeep int64) error
}

type batcher struct {
	store  deliveryStore
	sender *comet.HTTPSender
	brk    *breaker.Breaker
	cfg    *config.Config
	log    *zap.Logger

	mu    sync.Mutex
	buf   map[string][]batchItem // cometAddr -> items
	tick  *time.Ticker
	stopC chan struct{}
}

func newBatcher(store deliveryStore, sender *comet.HTTPSender, brk *breaker.Breaker, cfg *config.Config, log *zap.Logger) *batcher {
	return &batcher{
		store:  store,
		sender: sender,
		brk:    brk,
		cfg:    cfg,
		log:    log,
		buf:    make(map[string][]batchItem),
		stopC:  make(chan struct{}),
	}
}

func (b *batcher) start() {
	b.tick = time.NewTicker(b.cfg.Comet.FlushEvery)
	go func() {
		for {
			select {
			case <-b.stopC:
				return
			case <-b.tick.C:
				b.flushAll()
			}
		}
	}()
}

func (b *batcher) stop() {
	close(b.stopC)
	if b.tick != nil {
		b.tick.Stop()
	}
	b.flushAll()
}

// enqueue resolves route and groups by cometAddr (or enqueues offline if no route / breaker open)
func (b *batcher) enqueue(uid int64, convID string, seq int64, packetJSON string) {
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Delivery.OpTimeout)
	cometAddr, err := b.store.GetRoute(ctx, uid)
	cancel()

	if err != nil || cometAddr == "" {
		b.enqueueOffline(uid, convID, seq, packetJSON)
		return
	}

	if b.brk != nil && !b.brk.Allow(cometAddr) {
		metrics.BreakerDrop.Inc()
		b.enqueueOffline(uid, convID, seq, packetJSON)
		return
	}

	b.mu.Lock()
	items := b.buf[cometAddr]
	items = append(items, batchItem{uid: uid, conv: convID, seq: seq, pjson: packetJSON})
	b.buf[cometAddr] = items
	shouldFlush := len(items) >= b.cfg.Comet.MaxBatch
	b.mu.Unlock()

	if shouldFlush {
		b.flush(cometAddr)
	}
}

func (b *batcher) enqueueOffline(uid int64, convID string, seq int64, packetJSON string) {
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Delivery.OpTimeout)
	_ = b.store.EnqueueOffline(ctx, uid, convID, seq, packetJSON, b.cfg.Delivery.OfflineMaxKeep)
	cancel()
	metrics.OfflineEnqueued.Inc()
}

func (b *batcher) flushAll() {
	b.mu.Lock()
	addrs := make([]string, 0, len(b.buf))
	for addr := range b.buf {
		addrs = append(addrs, addr)
	}
	b.mu.Unlock()

	for _, addr := range addrs {
		b.flush(addr)
	}
}

func (b *batcher) flush(cometAddr string) {
	// take buffer
	b.mu.Lock()
	items := b.buf[cometAddr]
	if len(items) == 0 {
		b.mu.Unlock()
		return
	}
	delete(b.buf, cometAddr)
	b.mu.Unlock()

	// breaker check again before network
	if b.brk != nil && !b.brk.Allow(cometAddr) {
		metrics.BreakerDrop.Inc()
		for _, it := range items {
			b.enqueueOffline(it.uid, it.conv, it.seq, it.pjson)
		}
		return
	}

	// send in chunks
	for start := 0; start < len(items); start += b.cfg.Comet.MaxBatch {
		end := start + b.cfg.Comet.MaxBatch
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]

		reqItems := make([]cometPushReq, 0, len(chunk))
		for _, it := range chunk {
			reqItems = append(reqItems, cometPushReq{UID: it.uid, PacketJSON: it.pjson})
		}

		// retry once with small backoff
		err := b.sendBatchWithRetry(cometAddr, reqItems, 1)
		if err != nil {
			metrics.DeliverFail.Inc()
			opened := false
			if b.brk != nil {
				if b.brk.Failure(cometAddr) {
					opened = true
					metrics.BreakerOpen.Inc()
				}
			}
			b.log.Warn("comet batch send failed, fallback to offline",
				zap.String("comet", cometAddr),
				zap.Int("items", len(reqItems)),
				zap.Bool("breaker_opened", opened),
				zap.Error(err),
			)
			for _, it := range chunk {
				b.enqueueOffline(it.uid, it.conv, it.seq, it.pjson)
			}
			continue
		}

		// success
		if b.brk != nil {
			b.brk.Success(cometAddr)
		}
		metrics.BatchSent.Inc()
		metrics.BatchItems.Add(float64(len(reqItems)))
		metrics.DeliverOK.Add(float64(len(reqItems)))
	}
}

// Local alias to avoid exporting comet.pushReq
type cometPushReq struct {
	UID        int64
	PacketJSON string
}

func (b *batcher) sendBatchWithRetry(cometAddr string, items []cometPushReq, retry int) error {
	// convert
	req := make([]struct {
		UID        int64  `json:"uid"`
		PacketJSON string `json:"packet_json"`
	}, 0, len(items))
	for _, it := range items {
		req = append(req, struct {
			UID        int64  `json:"uid"`
			PacketJSON string `json:"packet_json"`
		}{UID: it.UID, PacketJSON: it.PacketJSON})
	}

	// We call sender.SendBatch which expects internal type; easiest: marshal ourselves and post using sender.Client.
	// We'll reuse sender.SendBatch by building its expected internal struct via JSON re-marshal.
	// For simplicity (and stability), call sender.SendBatch with []pushReq by re-marshaling:
	// Here we just use a small adapter: encode to bytes, then post to batch endpoint.
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Comet.Timeout)
	defer cancel()

	// manual post to avoid type export
	type pushReq struct {
		UID        int64  `json:"uid"`
		PacketJSON string `json:"packet_json"`
	}
	type batchReq struct {
		Items []pushReq `json:"items"`
	}
	br := batchReq{Items: make([]pushReq, 0, len(items))}
	for _, it := range items {
		br.Items = append(br.Items, pushReq{UID: it.UID, PacketJSON: it.PacketJSON})
	}

	// send
	body, _ := json.Marshal(br)
	err := b.senderRawPost(ctx, cometAddr, b.cfg.Comet.PushBatchPath, body)
	if err == nil {
		return nil
	}
	if retry > 0 {
		time.Sleep(20 * time.Millisecond)
		return b.sendBatchWithRetry(cometAddr, items, retry-1)
	}
	return err
}

func (b *batcher) senderRawPost(ctx context.Context, cometAddr, path string, body []byte) error {
	addr := cometAddr
	if len(addr) >= 7 && addr[:7] != "http://" && (len(addr) < 8 || addr[:8] != "https://") {
		addr = "http://" + addr
	}
	// trim slash
	for len(addr) > 0 && addr[len(addr)-1] == '/' {
		addr = addr[:len(addr)-1]
	}
	url := addr + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.sender.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("comet push status=%d", resp.StatusCode)
	}
	return nil
}

func isGroupConv(convID string) (int64, bool) {
	if strings.HasPrefix(convID, "g:") {
		gid, err := strconv.ParseInt(strings.TrimPrefix(convID, "g:"), 10, 64)
		if err == nil && gid > 0 {
			return gid, true
		}
	}
	return 0, false
}

type groupCache interface {
	Get(int64) ([]int64, bool)
	Set(int64, []int64)
}

func expandGroupRecipients(ctx context.Context, groupID int64, fromUID int64, limit int, gr *repo.GroupRepo, c groupCache) ([]int64, error) {
	if gr == nil || c == nil {
		return nil, nil
	}
	if uids, ok := c.Get(groupID); ok {
		return filterOut(uids, fromUID), nil
	}
	uids, err := gr.ListActiveMemberUIDs(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	c.Set(groupID, uids)
	return filterOut(uids, fromUID), nil
}

func filterOut(uids []int64, fromUID int64) []int64 {
	out := make([]int64, 0, len(uids))
	for _, u := range uids {
		if u > 0 && u != fromUID {
			out = append(out, u)
		}
	}
	return out
}
