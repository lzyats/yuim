package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/sony/sonyflake"

	"github.com/lzyats/core-push-go/pkg/event"
	"github.com/lzyats/core-push-go/pkg/producer"
	"github.com/lzyats/core-push-go/pkg/push"
	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"

	"yuim/services/im-ai/internal/config"
	"yuim/services/im-ai/internal/db"
	"yuim/services/im-ai/internal/outbox"
	"yuim/services/im-ai/internal/repo"
)

var (
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
	log.Info("im-ai starting", zap.String("version", Version), zap.String("addr", cfg.HTTP.Addr))

	// MySQL
	mysql, err := db.Open(db.Options{
		DSN:          cfg.MySQL.DSN,
		MaxOpenConns: cfg.MySQL.MaxOpenConns,
		MaxIdleConns: cfg.MySQL.MaxIdleConns,
		ConnMaxLife:  cfg.MySQL.ConnMaxLife,
		ConnMaxIdle:  cfg.MySQL.ConnMaxIdle,
	})
	if err != nil {
		log.Fatal("mysql init failed", zap.Error(err))
	}
	defer mysql.Close()

	msgRepo := repo.NewChatMsgRepo(mysql.DB)
	seqRepo := repo.NewSeqRepo(mysql.DB)
	outRepo := outbox.NewRepo(mysql.DB)

	// Redis store (idempotency)
	store, err := redisstore.New(parseRedisSettings(cfg))
	if err != nil {
		log.Fatal("redis init failed", zap.Error(err))
	}
	defer store.Close()

	// RocketMQ producer
	rmqCfg := push.RocketMQSettings{
		NameServer: cfg.RocketMQ.NameServer,
		Topic:      cfg.RocketMQ.Topic,
		Tag:        cfg.RocketMQ.Tag,
	}
	rmqCfg.Producer.Group = cfg.RocketMQ.ProducerGroup
	prod, err := producer.NewRocketMQ(rmqCfg)
	if err != nil {
		log.Fatal("rocketmq producer init failed", zap.Error(err))
	}
	defer prod.Close()

	// Start outbox worker (retry publish)
	obw := outbox.NewWorker(outRepo, prod, log, outbox.Options{Tick: cfg.Outbox.Tick, Batch: cfg.Outbox.Batch})
	obw.Start()
	defer obw.Stop()

	// MsgID generator
	sf := sonyflake.NewSonyflake(sonyflake.Settings{})
	if sf == nil {
		log.Fatal("sonyflake init failed")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})

	// Offline sync: GET /sync?uid=1001&conv_id=...&after_sync_id=0&limit=50
	mux.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		uid, _ := strconv.ParseInt(r.URL.Query().Get("uid"), 10, 64)
		convID := r.URL.Query().Get("conv_id")
		afterSync, _ := strconv.ParseInt(r.URL.Query().Get("after_sync_id"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = cfg.Sync.DefaultLimit
		}
		if limit > cfg.Sync.MaxLimit {
			limit = cfg.Sync.MaxLimit
		}
		if uid <= 0 || convID == "" {
			http.Error(w, "missing uid/conv_id", 400)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		list, err := msgRepo.ListByConvAfterSync(ctx, uid, convID, afterSync, limit)
		cancel()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		type msg struct {
			MsgID       int64           `json:"msg_id"`
			SyncID      int64           `json:"sync_id"`
			UserID      int64           `json:"user_id"`
			ReceiveID   int64           `json:"receive_id"`
			GroupID     *int64          `json:"group_id,omitempty"`
			TalkType    string          `json:"talk_type"`
			MsgType     string          `json:"msg_type"`
			Content     json.RawMessage `json:"content"`
			CreateTime  int64           `json:"create_time"`
			ConvID      string          `json:"conv_id"`
			ClientMsgID string          `json:"client_msg_id,omitempty"`
		}

		out := make([]msg, 0, len(list))
		for _, m := range list {
			var gid *int64
			if m.GroupID.Valid {
				v := m.GroupID.Int64
				gid = &v
			}
			out = append(out, msg{
				MsgID:       m.MsgID,
				SyncID:      m.SyncID,
				UserID:      m.UserID,
				ReceiveID:   m.ReceiveID,
				GroupID:     gid,
				TalkType:    m.TalkType,
				MsgType:     m.MsgType,
				Content:     json.RawMessage(m.Content),
				CreateTime:  m.CreateTime.Unix(),
				ConvID:      m.ConvID,
				ClientMsgID: m.ClientMsgID,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "items": out})
	})

	// Send P2P message: POST /send
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			FromUID     int64          `json:"from_uid"`
			ToUID       int64          `json:"to_uid"`
			ClientMsgID string         `json:"client_msg_id"`
			MsgType     string         `json:"msg_type"`
			Content     map[string]any `json:"content"`
		}
		type resp struct {
			OK          bool   `json:"ok"`
			MsgID       int64  `json:"msg_id"`
			SyncID      int64  `json:"sync_id"`
			ConvID      string `json:"conv_id"`
			ClientMsgID string `json:"client_msg_id,omitempty"`
			Published   bool   `json:"published"`
		}

		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if q.FromUID <= 0 || q.ToUID <= 0 {
			http.Error(w, "missing from_uid/to_uid", 400)
			return
		}
		if q.MsgType == "" {
			q.MsgType = "TEXT"
		}
		if q.Content == nil {
			q.Content = map[string]any{}
		}

		convID := buildP2PConvID(q.FromUID, q.ToUID)

		// idempotency fast-path (Redis)
		if q.ClientMsgID != "" {
			ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
			msgID, ok, err := store.GetIdem(ctx, q.FromUID, q.ClientMsgID)
			cancel()
			if err == nil && ok && msgID > 0 {
				writeJSON(w, resp{OK: true, MsgID: msgID, SyncID: 0, ConvID: convID, ClientMsgID: q.ClientMsgID, Published: true})
				return
			}
		}

		// msg_id
		id, err := sf.NextID()
		if err != nil {
			http.Error(w, "idgen failed", 500)
			return
		}
		msgID := int64(id)

		// tx: sync_id + chat_msg + outbox (return outbox_id)
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		tx, err := mysql.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}

		syncID, err := seqRepo.NextConvSeq(ctx, tx, convID)
		if err != nil {
			_ = tx.Rollback()
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}

		evt := event.ImEvent{
			Event:   "msg_send",
			TraceID: "",
			TS:      time.Now().Unix(),
			FromUID: q.FromUID,
			ToUIDs:  []int64{q.ToUID},
			ConvID:  convID,
			Msg: event.Message{
				MsgID:       msgID,
				ClientMsgID: q.ClientMsgID,
				Seq:         syncID,
				MsgType:     q.MsgType,
				Content:     q.Content,
			},
		}
		evtJSON, _ := json.Marshal(evt)

		cm := &repo.ChatMsg{
			MsgID:       msgID,
			SyncID:      syncID,
			UserID:      q.FromUID,
			ReceiveID:   q.ToUID,
			GroupID:     sql.NullInt64{Valid: false},
			TalkType:    "1",
			MsgType:     q.MsgType,
			Content:     repo.MustJSON(q.Content),
			ConvID:      convID,
			ClientMsgID: q.ClientMsgID,
		}
		if err := msgRepo.InsertTx(ctx, tx, cm); err != nil {
			_ = tx.Rollback()
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}

		outboxID, err := outRepo.EnqueueTx(ctx, tx, cfg.RocketMQ.Topic, cfg.RocketMQ.Tag, string(evtJSON))
		if err != nil {
			_ = tx.Rollback()
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}

		if err := tx.Commit(); err != nil {
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}
		cancel()

		// set idempotency record (best-effort)
		if q.ClientMsgID != "" {
			ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
			_ = store.SetIdem(ctx, q.FromUID, q.ClientMsgID, msgID, int64(cfg.Idempotency.TTL.Seconds()))
			cancel()
		}

		// inline publish, success then MarkSent(outboxID)
		published := false
		ctx, cancel = context.WithTimeout(r.Context(), cfg.Timeout)
		if err := prod.Publish(ctx, &evt); err == nil {
			published = true
			_ = outRepo.MarkSent(context.Background(), outboxID)
		}
		cancel()

		writeJSON(w, resp{OK: true, MsgID: msgID, SyncID: syncID, ConvID: convID, ClientMsgID: q.ClientMsgID, Published: published})
	})

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}

func buildP2PConvID(a, b int64) string {
	if a < b {
		return "p2p:" + itoa(a) + ":" + itoa(b)
	}
	return "p2p:" + itoa(b) + ":" + itoa(a)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	x := n
	for x > 0 {
		i--
		buf[i] = byte('0' + (x % 10))
		x /= 10
	}
	return string(buf[i:])
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
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
