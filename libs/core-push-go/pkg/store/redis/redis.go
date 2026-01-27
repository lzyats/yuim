package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lzyats/core-push-go/pkg/push"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	cfg push.RedisSettings
	cli *redis.Client
}

func New(cfg push.RedisSettings) (*Store, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("redis: missing host")
	}
	if cfg.Port == 0 {
		cfg.Port = 6379
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	opts := &redis.Options{
		Addr:         addr,
		Password:     cfg.Password,
		DB:           cfg.Database,
		DialTimeout:  cfg.Timeout,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}
	// Map lettuce pool fields (best-effort).
	if cfg.Lettuce.Pool.MaxActive > 0 {
		opts.PoolSize = cfg.Lettuce.Pool.MaxActive
	}
	if cfg.Lettuce.Pool.MaxIdle > 0 {
		opts.MinIdleConns = cfg.Lettuce.Pool.MaxIdle
	}

	cli := redis.NewClient(opts)
	return &Store{cfg: cfg, cli: cli}, nil
}

func (s *Store) Close() error { return s.cli.Close() }

// Pop blocks for up to block to pop a single raw payload from a LIST key.
// It uses BRPOP so that multiple workers can share the same queue.
func (s *Store) Pop(ctx context.Context, key string, block time.Duration) (string, error) {
	if key == "" {
		key = s.cfg.QueueKey
	}
	if key == "" {
		return "", push.ErrInvalidArgument
	}
	if block <= 0 {
		block = 5 * time.Second
	}

	res, err := s.cli.BRPop(ctx, block, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// BRPOP returns [key, value]
	if len(res) != 2 {
		return "", nil
	}
	return res[1], nil
}

// DecodeMessage decodes JSON payload into push.Message.
// If payload is not JSON, it will be treated as a single target CID with empty title/body.
func DecodeMessage(payload string) (push.Message, error) {
	var m push.Message
	if payload == "" {
		return m, push.ErrInvalidArgument
	}
	if payload[0] != '{' {
		m.Targets = []string{payload}
		return m, nil
	}
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return m, err
	}
	return m, nil
}



/*
DeliveryStore implementation (route/offline/idem)

Keys:
  - im:route:uid:{uid}
  - im:offline:{uid}:{conv_id} (ZSET score=seq)
  - im:idem:{from_uid}:{client_msg_id}
*/
func (s *Store) routeKey(uid int64) string {
	return fmt.Sprintf("im:route:uid:%d", uid)
}
func (s *Store) offlineKey(uid int64, convID string) string {
	return fmt.Sprintf("im:offline:%d:%s", uid, convID)
}
func (s *Store) idemKey(fromUID int64, clientMsgID string) string {
	return fmt.Sprintf("im:idem:%d:%s", fromUID, clientMsgID)
}

func (s *Store) SetRoute(ctx context.Context, uid int64, cometAddr string, ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 60
	}
	return s.cli.Set(ctx, s.routeKey(uid), cometAddr, time.Duration(ttlSeconds)*time.Second).Err()
}

func (s *Store) GetRoute(ctx context.Context, uid int64) (string, error) {
	v, err := s.cli.Get(ctx, s.routeKey(uid)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (s *Store) EnqueueOffline(ctx context.Context, uid int64, convID string, seq int64, packetJSON string, maxKeep int64) error {
	if seq <= 0 {
		seq = time.Now().UnixMilli()
	}
	key := s.offlineKey(uid, convID)
	if err := s.cli.ZAdd(ctx, key, redis.Z{Score: float64(seq), Member: packetJSON}).Err(); err != nil {
		return err
	}
	if maxKeep > 0 {
		card, err := s.cli.ZCard(ctx, key).Result()
		if err == nil {
			trim := card - maxKeep
			if trim > 0 {
				_ = s.cli.ZRemRangeByRank(ctx, key, 0, trim-1).Err()
			}
		}
	}
	_ = s.cli.Expire(ctx, key, 7*24*time.Hour).Err()
	return nil
}

func (s *Store) PullOffline(ctx context.Context, uid int64, convID string, afterSeq int64, limit int64) ([]string, int64, error) {
	if limit <= 0 {
		limit = 200
	}
	key := s.offlineKey(uid, convID)
	min := fmt.Sprintf("(%d", afterSeq)
	max := "+inf"
	res, err := s.cli.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:    min,
		Max:    max,
		Offset: 0,
		Count:  limit,
	}).Result()
	if err != nil {
		return nil, afterSeq, err
	}
	last := afterSeq
	if len(res) > 0 {
		ws, err2 := s.cli.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Min:    min,
			Max:    max,
			Offset: int64(len(res) - 1),
			Count:  1,
		}).Result()
		if err2 == nil && len(ws) == 1 {
			last = int64(ws[0].Score)
		}
	}
	return res, last, nil
}

func (s *Store) Ack(ctx context.Context, uid int64, convID string, seq int64) error {
	key := s.offlineKey(uid, convID)
	return s.cli.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", seq)).Err()
}

func (s *Store) GetIdem(ctx context.Context, fromUID int64, clientMsgID string) (int64, bool, error) {
	v, err := s.cli.Get(ctx, s.idemKey(fromUID, clientMsgID)).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var n int64
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int64(ch-'0')
	}
	if n == 0 {
		return 0, false, nil
	}
	return n, true, nil
}

func (s *Store) SetIdem(ctx context.Context, fromUID int64, clientMsgID string, msgID int64, ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 7 * 24 * 3600
	}
	return s.cli.Set(ctx, s.idemKey(fromUID, clientMsgID), fmt.Sprintf("%d", msgID), time.Duration(ttlSeconds)*time.Second).Err()
}


// DedupeMsg returns true if msgID is seen for the first time within ttlSeconds.
// It uses SET NX to provide consumer-side idempotency (dedup by msg_id).
func (s *Store) DedupeMsg(ctx context.Context, msgID int64, ttlSeconds int64) (bool, error) {
	if msgID <= 0 {
		return false, push.ErrInvalidArgument
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 24 * 3600
	}
	key := fmt.Sprintf("im:dedupe:msg:%d", msgID)
	ok, err := s.cli.SetNX(ctx, key, "1", time.Duration(ttlSeconds)*time.Second).Result()
	return ok, err
}
