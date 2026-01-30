package config

import (
	"errors"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Env string `yaml:"env"`

	HTTP struct {
		Addr string `yaml:"addr"` // ":8080"
	} `yaml:"http"`

	MySQL struct {
		DSN          string        `yaml:"dsn"`
		MaxOpenConns int           `yaml:"max_open_conns"`
		MaxIdleConns int           `yaml:"max_idle_conns"`
		ConnMaxLife  time.Duration `yaml:"conn_max_life"`
		ConnMaxIdle  time.Duration `yaml:"conn_max_idle"`
	} `yaml:"mysql"`

	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		Database int    `yaml:"database"`
	} `yaml:"redis"`

	RocketMQ struct {
		NameServer    string `yaml:"name_server"`
		Topic         string `yaml:"topic"`
		Tag           string `yaml:"tag,omitempty"`
		ProducerGroup string `yaml:"producer_group"`
	} `yaml:"rocketmq"`

	Timeout time.Duration `yaml:"timeout"`

	Idempotency struct {
		TTL time.Duration `yaml:"ttl"` // default 7d
	} `yaml:"idempotency"`

	Outbox struct {
		Tick  time.Duration `yaml:"tick"`
		Batch int           `yaml:"batch"`
	} `yaml:"outbox"`

	Sync struct {
		DefaultLimit int `yaml:"default_limit"`
		MaxLimit     int `yaml:"max_limit"`
	} `yaml:"sync"`

	Auth struct {
		Enabled bool   `yaml:"enabled"`
		Mode    string `yaml:"mode"` // default_protect | default_public

		Token struct {
			Header       string `yaml:"header"`
			BearerPrefix string `yaml:"bearer_prefix"`
			QueryKey     string `yaml:"query_key"`
			RedisPrefix  string `yaml:"redis_prefix"`
			TTLDays      int    `yaml:"ttl_days"`
			Secret       string `yaml:"secret"`
		} `yaml:"token"`

		PublicPaths    []string `yaml:"public_paths"`
		ProtectedPaths []string `yaml:"protected_paths"`
	} `yaml:"auth"`
}

// Load supports comma-separated config files: "-c common.yml,im-ai.yml".
// Later files override earlier ones (shallow merge by YAML into struct using successive unmarshal).
func Load(pathList string) (*Config, error) {
	if strings.TrimSpace(pathList) == "" {
		return nil, errors.New("config path required (e.g. -c ./config.yml or -c common.yml,im-ai.yml)")
	}

	var c Config
	paths := strings.Split(pathList, ",")
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(b, &c); err != nil {
			return nil, err
		}
	}

	// defaults
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}
	if c.Timeout == 0 {
		c.Timeout = 3 * time.Second
	}
	if c.Idempotency.TTL == 0 {
		c.Idempotency.TTL = 7 * 24 * time.Hour
	}
	if c.MySQL.MaxOpenConns <= 0 {
		c.MySQL.MaxOpenConns = 50
	}
	if c.MySQL.MaxIdleConns <= 0 {
		c.MySQL.MaxIdleConns = 25
	}
	if c.MySQL.ConnMaxLife == 0 {
		c.MySQL.ConnMaxLife = 30 * time.Minute
	}
	if c.MySQL.ConnMaxIdle == 0 {
		c.MySQL.ConnMaxIdle = 5 * time.Minute
	}
	if c.Outbox.Tick == 0 {
		c.Outbox.Tick = 1 * time.Second
	}
	if c.Outbox.Batch <= 0 {
		c.Outbox.Batch = 200
	}
	if c.Sync.DefaultLimit <= 0 {
		c.Sync.DefaultLimit = 50
	}
	if c.Sync.MaxLimit <= 0 {
		c.Sync.MaxLimit = 500
	}

	// auth defaults
	if c.Auth.Mode == "" {
		c.Auth.Mode = "default_protect"
	}
	if c.Auth.Token.Header == "" {
		c.Auth.Token.Header = "Authorization"
	}
	if c.Auth.Token.BearerPrefix == "" {
		c.Auth.Token.BearerPrefix = "Bearer "
	}
	if c.Auth.Token.QueryKey == "" {
		c.Auth.Token.QueryKey = "token"
	}
	if c.Auth.Token.RedisPrefix == "" {
		c.Auth.Token.RedisPrefix = "app:token:"
	}
	if c.Auth.Token.TTLDays == 0 {
		c.Auth.Token.TTLDays = 30
	}
	if c.Auth.PublicPaths == nil {
		c.Auth.PublicPaths = []string{"/healthz"}
	}
	return &c, nil
}
