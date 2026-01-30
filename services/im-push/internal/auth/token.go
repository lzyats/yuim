package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type TokenPayload struct {
	UserID    int64  `json:"userId"`
	Timestamp string `json:"timestamp"`
}

// ExtractToken gets token from Authorization header (Bearer) or query parameter.
func ExtractToken(r *http.Request, header, bearerPrefix, queryKey string) string {
	if header != "" {
		v := strings.TrimSpace(r.Header.Get(header))
		if v != "" {
			if bearerPrefix != "" && strings.HasPrefix(v, bearerPrefix) {
				return strings.TrimSpace(strings.TrimPrefix(v, bearerPrefix))
			}
			return v
		}
	}
	if queryKey != "" {
		q := strings.TrimSpace(r.URL.Query().Get(queryKey))
		if q != "" {
			return q
		}
	}
	return ""
}

// ParseToken decrypts the Java-compatible token and returns its payload.
func ParseToken(token, secret string) (*TokenPayload, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	plain, err := Decrypt(token, secret)
	if err != nil {
		return nil, err
	}
	var p TokenPayload
	if err := json.Unmarshal([]byte(plain), &p); err != nil {
		return nil, err
	}
	if p.UserID <= 0 || p.Timestamp == "" {
		return nil, errors.New("invalid token payload")
	}
	return &p, nil
}
