package auth

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// ValidateSession checks whether token session exists in Redis.
// It is compatible with Java TokenServiceImpl.convert(): EXISTS prefix+token.
func ValidateSession(ctx context.Context, rdb *redis.Client, redisPrefix, token string) (bool, error) {
	if rdb == nil {
		return false, nil
	}
	key := redisPrefix + token
	n, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
