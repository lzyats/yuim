package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sony/sonyflake"

	"yuim/services/im-ai/internal/auth"
)

func mustID(sf *sonyflake.Sonyflake) int64 {
	id, err := sf.NextID()
	if err != nil {
		// in practice sonyflake NextID rarely fails; panic is fine for server startup usage.
		panic(err)
	}
	return int64(id)
}

func mustRand(n int) string {
	s, err := auth.RandomAlphaNum(n)
	if err != nil {
		panic(err)
	}
	return s
}

// hashPassword returns a deterministic hash suitable for MySQL storage.
// We keep it stdlib-only to avoid extra module downloads in offline envs.
// Format: sha256_hex(password)
func hashPassword(pw string) (string, error) {
	if pw == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(sum[:]), nil
}

// javaCompatibleToken builds the same payload used by ShiroUserVo in alpaca-api and encrypts it.
// Payload: {"timestamp": <random 14 chars>, "userId": <uid>}
func javaCompatibleToken(uid int64, secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("auth.token.secret is required")
	}
	timestamp := mustRand(14)
	payload := map[string]any{"timestamp": timestamp, "userId": uid}
	b, _ := json.Marshal(payload)
	return auth.Encrypt(string(b), secret)
}

// ParseUIDFromSession is a helper to convert string userId to int64.
func ParseUIDFromSession(uidStr string) int64 {
	uid, _ := strconv.ParseInt(uidStr, 10, 64)
	return uid
}
