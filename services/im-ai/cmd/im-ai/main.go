package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/sony/sonyflake"

	"github.com/lzyats/core-push-go/pkg/event"
	"github.com/lzyats/core-push-go/pkg/producer"
	"github.com/lzyats/core-push-go/pkg/push"
	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"

	"yuim/services/im-ai/internal/auth"
	"yuim/services/im-ai/internal/config"
	"yuim/services/im-ai/internal/db"
	"yuim/services/im-ai/internal/outbox"
	"yuim/services/im-ai/internal/repo"
)

var Version = "dev"

func main() {
	var cfgPaths string
	var outboxOnly bool
	flag.StringVar(&cfgPaths, "c", "./config.yml", "config file path (supports: a.yml,b.yml)")
	flag.BoolVar(&outboxOnly, "outbox-only", false, "run only outbox worker (no http server)")
	flag.Parse()

	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load(cfgPaths)
	if err != nil {
		log.Fatal("load config failed", zap.Error(err))
	}
	log.Info("im-ai starting", zap.String("version", Version), zap.String("addr", cfg.HTTP.Addr))

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

	chatRepo := repo.NewChatRepo(mysql.DB)
	seqRepo := repo.NewSeqRepo(mysql.DB)
	outRepo := outbox.NewRepo(mysql.DB)

	store, err := redisstore.New(parseRedisSettings(cfg))
	if err != nil {
		log.Fatal("redis init failed", zap.Error(err))
	}
	defer store.Close()

	sessions := &auth.SessionStore{
		RedisPrefix: cfg.Auth.Token.RedisPrefix,
		TTLDays:     cfg.Auth.Token.TTLDays,
		Store:       store,
	}

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

	obw := outbox.NewWorker(outRepo, prod, log, outbox.Options{Tick: cfg.Outbox.Tick, Batch: cfg.Outbox.Batch})
	obw.Start()
	defer obw.Stop()

	if outboxOnly {
		log.Info("outbox-only mode enabled")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Info("outbox-only shutting down")
		return
	}

	sf := sonyflake.NewSonyflake(sonyflake.Settings{})
	if sf == nil {
		log.Fatal("sonyflake init failed")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })

	// ============================
	// Auth (Java-token compatible)
	// ============================
	// POST /v1/auth/sms/send
	mux.HandleFunc("/v1/auth/sms/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type req struct {
			BizType     string `json:"biz_type"`
			CountryCode string `json:"country_code"`
			Mobile      string `json:"mobile"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		q.BizType = strings.TrimSpace(q.BizType)
		if q.BizType == "" {
			q.BizType = "login"
		}
		q.CountryCode = strings.TrimSpace(q.CountryCode)
		if q.CountryCode == "" {
			q.CountryCode = "86"
		}
		q.Mobile = strings.TrimSpace(q.Mobile)
		if q.Mobile == "" {
			http.Error(w, "missing mobile", 400)
			return
		}

		code := "123456" // TODO: integrate real SMS provider; keep stable for MVP联调
		smsKey := "im:sms:" + q.BizType + ":" + q.CountryCode + ":" + q.Mobile
		_ = store.Client().Set(r.Context(), smsKey, code, 5*time.Minute).Err()

		// audit record (best-effort)
		_, _ = mysql.DB.ExecContext(r.Context(),
			"INSERT INTO im_sms (sms_id,biz_type,country_code,mobile,content,status,create_time,deleted,update_time) VALUES (?,?,?,?,?,'S',NOW(),0,NOW())",
			mustID(sf), q.BizType, q.CountryCode, q.Mobile, "code sent")

		writeJSON(w, map[string]any{"ok": true})
	})

	// POST /v1/auth/register
	mux.HandleFunc("/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type req struct {
			CountryCode string `json:"country_code"`
			Mobile      string `json:"mobile"`
			Code        string `json:"code"`
			Password    string `json:"password"`
			Nickname    string `json:"nickname"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if strings.TrimSpace(q.CountryCode) == "" {
			q.CountryCode = "86"
		}
		q.Mobile = strings.TrimSpace(q.Mobile)
		if q.Mobile == "" {
			http.Error(w, "missing mobile", 400)
			return
		}
		// verify code
		smsKey := "im:sms:register:" + q.CountryCode + ":" + q.Mobile
		code, _ := store.Client().Get(r.Context(), smsKey).Result()
		if code == "" || code != strings.TrimSpace(q.Code) {
			http.Error(w, "invalid code", 400)
			return
		}

		// create user (minimal fields)
		uid := mustID(sf)
		userNo := "U" + strconv.FormatInt(uid, 10)
		nickname := strings.TrimSpace(q.Nickname)
		if nickname == "" {
			nickname = userNo
		}
		passHash, err := hashPassword(strings.TrimSpace(q.Password))
		if err != nil {
			http.Error(w, "server error", 500)
			return
		}
		_, err = mysql.DB.ExecContext(r.Context(),
			"INSERT INTO im_user (user_id,user_no,phone,nickname,password,create_time,deleted) VALUES (?,?,?,?,?,NOW(),0)",
			uid, userNo, q.Mobile, nickname, passHash)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		tok, err := javaCompatibleToken(uid, cfg.Auth.Token.Secret)
		if err != nil {
			http.Error(w, "token error", 500)
			return
		}
		fields := map[string]string{
			"token":        tok,
			"userId":       strconv.FormatInt(uid, 10),
			"nickname":     nickname,
			"portrait":     "",
			"sign":         mustRand(32),
			"phone":        q.Mobile,
			"userNo":       userNo,
			"banned":       "N",
			"lastId":       "0",
			"lastMomentId": "0",
		}
		if err := sessions.Put(r.Context(), tok, fields); err != nil {
			http.Error(w, "session error", 500)
			return
		}

		writeJSON(w, map[string]any{"ok": true, "token": tok, "user_id": uid})
	})

	// POST /v1/auth/login/sms
	mux.HandleFunc("/v1/auth/login/sms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type req struct {
			CountryCode string `json:"country_code"`
			Mobile      string `json:"mobile"`
			Code        string `json:"code"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if strings.TrimSpace(q.CountryCode) == "" {
			q.CountryCode = "86"
		}
		q.Mobile = strings.TrimSpace(q.Mobile)
		if q.Mobile == "" {
			http.Error(w, "missing mobile", 400)
			return
		}
		smsKey := "im:sms:login:" + q.CountryCode + ":" + q.Mobile
		code, _ := store.Client().Get(r.Context(), smsKey).Result()
		if code == "" || code != strings.TrimSpace(q.Code) {
			http.Error(w, "invalid code", 400)
			return
		}

		var uid int64
		var nickname, userNo string
		err := mysql.DB.QueryRowContext(r.Context(),
			"SELECT user_id,nickname,user_no FROM im_user WHERE phone=? AND deleted=0 LIMIT 1", q.Mobile).Scan(&uid, &nickname, &userNo)
		if err == sql.ErrNoRows {
			http.Error(w, "user not found", 404)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		tok, err2 := javaCompatibleToken(uid, cfg.Auth.Token.Secret)
		if err2 != nil {
			http.Error(w, "token error", 500)
			return
		}
		fields := map[string]string{
			"token":        tok,
			"userId":       strconv.FormatInt(uid, 10),
			"nickname":     nickname,
			"portrait":     "",
			"sign":         mustRand(32),
			"phone":        q.Mobile,
			"userNo":       userNo,
			"banned":       "N",
			"lastId":       "0",
			"lastMomentId": "0",
		}
		if err := sessions.Put(r.Context(), tok, fields); err != nil {
			http.Error(w, "session error", 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "token": tok, "user_id": uid})
	})

	// POST /v1/auth/password/forgot
	mux.HandleFunc("/v1/auth/password/forgot", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type req struct {
			CountryCode string `json:"country_code"`
			Mobile      string `json:"mobile"`
			Code        string `json:"code"`
			NewPassword string `json:"new_password"`
		}
		var q req
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if strings.TrimSpace(q.CountryCode) == "" {
			q.CountryCode = "86"
		}
		q.Mobile = strings.TrimSpace(q.Mobile)
		if q.Mobile == "" {
			http.Error(w, "missing mobile", 400)
			return
		}
		smsKey := "im:sms:reset:" + q.CountryCode + ":" + q.Mobile
		code, _ := store.Client().Get(r.Context(), smsKey).Result()
		if code == "" || code != strings.TrimSpace(q.Code) {
			http.Error(w, "invalid code", 400)
			return
		}
		passHash, err := hashPassword(strings.TrimSpace(q.NewPassword))
		if err != nil {
			http.Error(w, "server error", 500)
			return
		}
		_, err = mysql.DB.ExecContext(r.Context(), "UPDATE im_user SET password=? WHERE phone=? AND deleted=0", passHash, q.Mobile)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	// POST /v1/auth/logout (requires login)
	mux.HandleFunc("/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		tok := auth.ExtractToken(r, cfg.Auth.Token.Header, cfg.Auth.Token.BearerPrefix, cfg.Auth.Token.QueryKey)
		_ = sessions.Delete(r.Context(), tok)
		writeJSON(w, map[string]any{"ok": true})
	})

	// GET /sync?uid=1001&conv_id=...&after_sync_id=0&limit=50
	// GET /sync?uid=1001&talk_type=1&peer_id=2002&after_sync_id=0
	// GET /sync?uid=1001&talk_type=2&group_id=123&after_sync_id=0
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
		if uid <= 0 {
			http.Error(w, "missing uid", 400)
			return
		}
		if convID == "" {
			tt := r.URL.Query().Get("talk_type")
			if tt == "2" {
				gid, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
				if gid <= 0 {
					http.Error(w, "missing group_id", 400)
					return
				}
				convID = convIDForGroup(gid)
			} else {
				peer, _ := strconv.ParseInt(r.URL.Query().Get("peer_id"), 10, 64)
				if peer <= 0 {
					http.Error(w, "missing peer_id", 400)
					return
				}
				convID = convIDForP2P(uid, peer)
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		list, err := chatRepo.ListByConvAfterSync(ctx, uid, convID, afterSync, limit)
		cancel()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		type msg struct {
			MsgID       int64           `json:"msg_id"`
			SyncID      int64           `json:"sync_id"`
			UserID      int64           `json:"user_id"`
			ReceiveID   *int64          `json:"receive_id,omitempty"`
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
			var rid *int64
			if m.ReceiveID.Valid {
				v := m.ReceiveID.Int64
				rid = &v
			}
			var gid *int64
			if m.GroupID.Valid {
				v := m.GroupID.Int64
				gid = &v
			}
			cid := ""
			if m.ConvID.Valid {
				cid = m.ConvID.String
			}
			cmid := ""
			if m.ClientMsgID.Valid {
				cmid = m.ClientMsgID.String
			}
			out = append(out, msg{
				MsgID:       m.MsgID,
				SyncID:      m.SyncID,
				UserID:      m.UserID,
				ReceiveID:   rid,
				GroupID:     gid,
				TalkType:    m.TalkType,
				MsgType:     m.MsgType,
				Content:     json.RawMessage(m.Content),
				CreateTime:  m.CreateTime.Unix(),
				ConvID:      cid,
				ClientMsgID: cmid,
			})
		}
		writeJSON(w, map[string]any{"ok": true, "items": out})
	})

	// POST /send (兼容 chat_msg 字段)
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		type req struct {
			UserID      int64          `json:"user_id"`
			ReceiveID   int64          `json:"receive_id"`
			GroupID     int64          `json:"group_id"`
			TalkType    string         `json:"talk_type"`
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
		if q.UserID <= 0 {
			http.Error(w, "missing user_id", 400)
			return
		}
		if q.TalkType == "" {
			q.TalkType = "1"
		}
		if q.MsgType == "" {
			q.MsgType = "TEXT"
		}
		if q.Content == nil {
			q.Content = map[string]any{}
		}

		convID, toUIDs, recvID, groupID, err := normalizeRoute(q.UserID, q.ReceiveID, q.GroupID, q.TalkType)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		if q.ClientMsgID != "" {
			ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
			msgID, ok, err := store.GetIdem(ctx, q.UserID, q.ClientMsgID)
			cancel()
			if err == nil && ok && msgID > 0 {
				writeJSON(w, resp{OK: true, MsgID: msgID, SyncID: 0, ConvID: convID, ClientMsgID: q.ClientMsgID, Published: true})
				return
			}
		}

		id, err := sf.NextID()
		if err != nil {
			http.Error(w, "idgen failed", 500)
			return
		}
		msgID := int64(id)

		evt := event.ImEvent{
			Event:   "msg_send",
			TraceID: "",
			TS:      time.Now().Unix(),
			FromUID: q.UserID,
			ToUIDs:  toUIDs,
			ConvID:  convID,
			Msg: event.Message{
				MsgID:       msgID,
				ClientMsgID: q.ClientMsgID,
				Seq:         0, // sync_id
				MsgType:     q.MsgType,
				Content:     q.Content,
			},
		}

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
		evt.Msg.Seq = syncID
		evtJSON, _ := json.Marshal(evt)

		m := &repo.ChatMsg{
			MsgID:     msgID,
			SyncID:    syncID,
			UserID:    q.UserID,
			ReceiveID: recvID,
			GroupID:   groupID,
			TalkType:  q.TalkType,
			MsgType:   q.MsgType,
			Content:   mustJSON(q.Content),
			ConvID:    sql.NullString{String: convID, Valid: true},
			ClientMsgID: func() sql.NullString {
				if q.ClientMsgID == "" {
					return sql.NullString{Valid: false}
				}
				return sql.NullString{String: q.ClientMsgID, Valid: true}
			}(),
		}
		if err := chatRepo.InsertTx(ctx, tx, m); err != nil {
			_ = tx.Rollback()
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}

		outboxID, err := outRepo.EnqueueTx(ctx, tx, evt.Event, msgID, convID, syncID, cfg.RocketMQ.Topic, cfg.RocketMQ.Tag, string(evtJSON))
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

		if q.ClientMsgID != "" {
			ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
			_ = store.SetIdem(ctx, q.UserID, q.ClientMsgID, msgID, int64(cfg.Idempotency.TTL.Seconds()))
			cancel()
		}

		published := false
		ctx, cancel = context.WithTimeout(r.Context(), cfg.Timeout)
		if err := prod.Publish(ctx, &evt); err == nil {
			published = true
			_ = outRepo.MarkSent(context.Background(), outboxID)
		}
		cancel()

		writeJSON(w, resp{OK: true, MsgID: msgID, SyncID: syncID, ConvID: convID, ClientMsgID: q.ClientMsgID, Published: published})
	})

	authCfg := auth.Config{
		Enabled:        cfg.Auth.Enabled,
		Mode:           cfg.Auth.Mode,
		Header:         cfg.Auth.Token.Header,
		BearerPrefix:   cfg.Auth.Token.BearerPrefix,
		QueryKey:       cfg.Auth.Token.QueryKey,
		PublicPaths:    cfg.Auth.PublicPaths,
		ProtectedPaths: cfg.Auth.ProtectedPaths,
	}
	h := auth.Wrap(authCfg, sessions, mux)
	srv := &http.Server{Addr: cfg.HTTP.Addr, Handler: h, ReadHeaderTimeout: 2 * time.Second}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func convIDForP2P(a, b int64) string {
	if a < b {
		return "p2p:" + strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
	}
	return "p2p:" + strconv.FormatInt(b, 10) + ":" + strconv.FormatInt(a, 10)
}

func convIDForGroup(gid int64) string { return "g:" + strconv.FormatInt(gid, 10) }

func normalizeRoute(uid, receiveID, groupID int64, talkType string) (convID string, toUIDs []int64, recv sql.NullInt64, gid sql.NullInt64, err error) {
	if talkType == "2" {
		if groupID <= 0 {
			return "", nil, sql.NullInt64{}, sql.NullInt64{}, errBad("missing group_id for talk_type=2")
		}
		convID = convIDForGroup(groupID)
		toUIDs = nil // group fanout done by downstream (im-job)
		recv = sql.NullInt64{Valid: false}
		gid = sql.NullInt64{Int64: groupID, Valid: true}
		return
	}
	if receiveID <= 0 {
		return "", nil, sql.NullInt64{}, sql.NullInt64{}, errBad("missing receive_id for talk_type=1")
	}
	convID = convIDForP2P(uid, receiveID)
	toUIDs = []int64{receiveID}
	recv = sql.NullInt64{Int64: receiveID, Valid: true}
	gid = sql.NullInt64{Valid: false}
	return
}

type errBad string

func (e errBad) Error() string { return string(e) }

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
	return push.RedisSettings{Enabled: "Y", Host: host, Port: port, Password: cfg.Redis.Password, Database: cfg.Redis.Database}
}
