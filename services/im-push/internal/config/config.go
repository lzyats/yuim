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
		DB       int    `yaml:"db"`
	} `yaml:"redis"`

	CometAddr     string        `yaml:"comet_addr"` // value written to route, e.g. "10.0.0.12:7001"
	RouteTTL      int64         `yaml:"route_ttl"`  // seconds
	WriteTimeout  time.Duration `yaml:"write_timeout"`
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
	return &c, nil
}
