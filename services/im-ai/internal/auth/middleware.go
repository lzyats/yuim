package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const CtxUID contextKey = "uid"

type Config struct {
	Enabled bool
	Mode    string

	Header       string
	BearerPrefix string
	QueryKey     string

	PublicPaths    []string
	ProtectedPaths []string
}

func (c Config) isPublic(path string) bool {
	for _, p := range c.PublicPaths {
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "/") {
			if strings.HasPrefix(path, p) {
				return true
			}
			continue
		}
		if path == p {
			return true
		}
		// prefix match helper for convenience
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (c Config) isProtected(path string) bool {
	for _, p := range c.ProtectedPaths {
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "/") {
			if strings.HasPrefix(path, p) {
				return true
			}
			continue
		}
		if path == p || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func ExtractToken(r *http.Request, header string, bearerPrefix string, queryKey string) string {
	// Header first
	if header != "" {
		v := r.Header.Get(header)
		if v != "" {
			if bearerPrefix != "" && strings.HasPrefix(v, bearerPrefix) {
				return strings.TrimSpace(strings.TrimPrefix(v, bearerPrefix))
			}
			return strings.TrimSpace(v)
		}
	}
	// Query fallback
	if queryKey != "" {
		if v := r.URL.Query().Get(queryKey); v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Wrap returns a handler that enforces auth based on the configured mode and path lists.
func Wrap(cfg Config, sessions *SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		needAuth := true
		if strings.EqualFold(cfg.Mode, "default_public") {
			needAuth = cfg.isProtected(path)
		} else { // default_protect
			needAuth = !cfg.isPublic(path)
		}
		if !needAuth {
			next.ServeHTTP(w, r)
			return
		}

		tok := ExtractToken(r, cfg.Header, cfg.BearerPrefix, cfg.QueryKey)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		info, ok, err := sessions.Get(r.Context(), tok)
		if err != nil {
			http.Error(w, "auth error", http.StatusUnauthorized)
			return
		}
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		uid := info["userId"]
		if uid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), CtxUID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UIDFromContext(ctx context.Context) string {
	v := ctx.Value(CtxUID)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
