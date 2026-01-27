package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/lzyats/core-push-go/pkg/push"
	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"

	"yuim/services/im-push/internal/config"
	"yuim/services/im-push/internal/hub"
	"yuim/services/im-push/internal/metrics"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	// Version is injected via -ldflags "-X main.Version=..."
	Version = "dev"
)

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
	log.Info("im-push starting", zap.String("version", Version), zap.String("addr", cfg.HTTP.Addr), zap.String("comet_addr", cfg.CometAddr))

	metrics.Register()

	// Redis store (route/offline/idem live in libs/core-push-go)
	store, err := redisstore.New(parseRedisSettings(cfg))
	if err != nil {
		log.Fatal("redis init failed", zap.Error(err))
	}
	defer store.Close()

	h := hub.New()

	mux := http.NewServeMux()

	// Metrics
	mux.Handle("/metrics", promhttp.Handler())

	// WS: /ws?uid=1001 (demo). 实际要用 token 鉴权。
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		uidStr := r.URL.Query().Get("uid")
		uid, _ := strconv.ParseInt(uidStr, 10, 64)
		if uid <= 0 {
			http.Error(w, "missing uid", 400)
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		c := &hub.Conn{UID: uid, WS: ws, Out: make(chan []byte, 256)}
		h.Set(uid, c)
		metrics.OnlineConns.Set(float64(h.Len()))

		// write route
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = store.SetRoute(ctx, uid, cfg.CometAddr, cfg.RouteTTL)
		cancel()

		go writeLoop(c, cfg.WriteTimeout, func() {
			h.Del(uid)
			metrics.OnlineConns.Set(float64(h.Len()))
		})

		_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"ok":true}`))
	})

	// Internal single push: POST /internal/push {uid, packet_json}
	mux.HandleFunc("/internal/push", func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			UID        int64  `json:"uid"`
			PacketJSON string `json:"packet_json"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if c, ok := h.Get(q.UID); ok {
			select {
			case c.Out <- []byte(q.PacketJSON):
				metrics.WSPushOK.Inc()
				w.WriteHeader(204)
			default:
				metrics.WSPushBackpressure.Inc()
				http.Error(w, "backpressure", 429)
			}
			return
		}
		metrics.WSPushOffline.Inc()
		http.Error(w, "offline", 404)
	})

	// Internal batch push: POST /internal/push/batch {items:[{uid,packet_json},...]}
	mux.HandleFunc("/internal/push/batch", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			UID        int64  `json:"uid"`
			PacketJSON string `json:"packet_json"`
		}
		type req struct {
			Items []item `json:"items"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		metrics.BatchPushReq.Inc()
		metrics.BatchItems.Add(float64(len(q.Items)))

		for _, it := range q.Items {
			if c, ok := h.Get(it.UID); ok {
				select {
				case c.Out <- []byte(it.PacketJSON):
					metrics.WSPushOK.Inc()
				default:
					metrics.WSPushBackpressure.Inc()
					http.Error(w, "backpressure", 429)
					return
				}
			} else {
				metrics.WSPushOffline.Inc()
				http.Error(w, "offline", 404)
				return
			}
		}
		w.WriteHeader(204)
	})

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	log.Info("im-push listening", zap.String("addr", cfg.HTTP.Addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}

func writeLoop(c *hub.Conn, wt time.Duration, onClose func()) {
	defer func() {
		_ = c.WS.Close()
		if onClose != nil {
			onClose()
		}
	}()
	for b := range c.Out {
		_ = c.WS.SetWriteDeadline(time.Now().Add(wt))
		if err := c.WS.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}

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
		Database: cfg.Redis.DB,
	}
}
