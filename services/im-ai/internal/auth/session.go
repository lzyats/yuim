package auth

import (
	"context"
	"fmt"
	"time"

	redisstore "github.com/lzyats/core-push-go/pkg/store/redis"
)

type SessionStore struct {
	RedisPrefix string
	TTLDays     int
	Store       *redisstore.Store
}

func (s *SessionStore) key(token string) string {
	return s.RedisPrefix + token
}

// Put stores a session payload as Redis hash + TTL.
func (s *SessionStore) Put(ctx context.Context, token string, fields map[string]string) error {
	if token == "" {
		return fmt.Errorf("token empty")
	}
	cli := s.Store.Client()
	key := s.key(token)
	if err := cli.HSet(ctx, key, fields).Err(); err != nil {
		return err
	}
	ttl := time.Duration(s.TTLDays) * 24 * time.Hour
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return cli.Expire(ctx, key, ttl).Err()
}

func (s *SessionStore) Get(ctx context.Context, token string) (map[string]string, bool, error) {
	if token == "" {
		return nil, false, nil
	}
	cli := s.Store.Client()
	key := s.key(token)
	exists, err := cli.Exists(ctx, key).Result()
	if err != nil {
		return nil, false, err
	}
	if exists == 0 {
		return nil, false, nil
	}
	m, err := cli.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, false, err
	}
	return m, true, nil
}

func (s *SessionStore) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.Store.Client().Del(ctx, s.key(token)).Err()
}
