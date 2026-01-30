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
		Addr string `yaml:"addr"` // ":7001"
	} `yaml:"http"`

	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		Database int    `yaml:"database"`
	} `yaml:"redis"`

	CometAddr    string        `yaml:"comet_addr"` // value written to route, e.g. "10.0.0.12:7001"
	RouteTTL     int64         `yaml:"route_ttl"`  // seconds
	WriteTimeout time.Duration `yaml:"write_timeout"`

	Auth struct {
		Enabled bool `yaml:"enabled"`
		Token   struct {
			Header       string `yaml:"header"`
			BearerPrefix string `yaml:"bearer_prefix"`
			QueryKey     string `yaml:"query_key"`
			RedisPrefix  string `yaml:"redis_prefix"`
			Secret       string `yaml:"secret"`
		} `yaml:"token"`
	} `yaml:"auth"`
}

// Load supports comma-separated config files: "-c common.yml,im-push.yml"
func Load(pathList string) (*Config, error) {
	if strings.TrimSpace(pathList) == "" {
		return nil, errors.New("config path required (e.g. -c ./config.yml or -c common.yml,im-push.yml)")
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

	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":7001"
	}
	if c.RouteTTL <= 0 {
		c.RouteTTL = 60
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 5 * time.Second
	}
	if c.CometAddr == "" {
		c.CometAddr = "127.0.0.1" + c.HTTP.Addr
	}
	// auth defaults (compatible with Java token + redis session)
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
		// Java example: token:app:<token>
		c.Auth.Token.RedisPrefix = "token:app:"
	}
	return &c, nil
}
