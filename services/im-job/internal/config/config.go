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

	Metrics struct {
		Addr string `yaml:"addr"`
	} `yaml:"metrics"`

	RocketMQ struct {
		NameServer string `yaml:"name_server"`
		Group      string `yaml:"group"`
		Topic      string `yaml:"topic"`
		Tag        string `yaml:"tag,omitempty"`
	} `yaml:"rocketmq"`

	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`

	Delivery struct {
		OpTimeout       time.Duration `yaml:"op_timeout"`
		OfflineMaxKeep  int64         `yaml:"offline_max_keep"`
		VendorQueueSize int           `yaml:"vendor_queue_size"`
		VendorWorkers   int           `yaml:"vendor_workers"`
	} `yaml:"delivery"`

	Comet struct {
		PushPath      string        `yaml:"push_path"`       // single push endpoint
		PushBatchPath string        `yaml:"push_batch_path"` // batch push endpoint
		Timeout       time.Duration `yaml:"timeout"`
		MaxBatch      int           `yaml:"max_batch"`   // max items per batch
		FlushEvery    time.Duration `yaml:"flush_every"` // batching window
	} `yaml:"comet"`

	Breaker struct {
		Enabled   bool          `yaml:"enabled"`
		Threshold int           `yaml:"threshold"`
		Window    time.Duration `yaml:"window"`
		OpenFor   time.Duration `yaml:"open_for"`
	} `yaml:"breaker"`

	Dedupe struct {
		TTL time.Duration `yaml:"ttl"` // msg_id 去重 TTL
	} `yaml:"dedupe"`
}

// Load supports comma-separated config files: "-c common.yml,im-job.yml"
func Load(pathList string) (*Config, error) {
	if strings.TrimSpace(pathList) == "" {
		return nil, errors.New("config path required (e.g. -c ./config.yml or -c common.yml,im-job.yml)")
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
	if c.Metrics.Addr == "" {
		c.Metrics.Addr = ":2112"
	}
	if c.Delivery.OpTimeout == 0 {
		c.Delivery.OpTimeout = 3 * time.Second
	}
	if c.Delivery.OfflineMaxKeep <= 0 {
		c.Delivery.OfflineMaxKeep = 2000
	}
	if c.Delivery.VendorQueueSize <= 0 {
		c.Delivery.VendorQueueSize = 4096
	}
	if c.Delivery.VendorWorkers <= 0 {
		c.Delivery.VendorWorkers = 8
	}
	if c.Comet.Timeout == 0 {
		c.Comet.Timeout = 2 * time.Second
	}
	if c.Comet.PushPath == "" {
		c.Comet.PushPath = "/internal/push"
	}
	if c.Comet.PushBatchPath == "" {
		c.Comet.PushBatchPath = "/internal/push/batch"
	}
	if c.Comet.MaxBatch <= 0 {
		c.Comet.MaxBatch = 200
	}
	if c.Comet.FlushEvery == 0 {
		c.Comet.FlushEvery = 5 * time.Millisecond
	}
	if c.Breaker.Threshold <= 0 {
		c.Breaker.Threshold = 5
	}
	if c.Breaker.Window == 0 {
		c.Breaker.Window = 10 * time.Second
	}
	if c.Breaker.OpenFor == 0 {
		c.Breaker.OpenFor = 5 * time.Second
	}
	if c.Dedupe.TTL == 0 {
		c.Dedupe.TTL = 7 * 24 * time.Hour
	}
	return &c, nil
}
