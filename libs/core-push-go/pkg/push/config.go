package push

import (
	"strings"
	"time"
)

type Settings struct {
	RocketMQ RocketMQSettings `yaml:"rocketmq" json:"rocketmq"`
	Push     PushSettings     `yaml:"push" json:"push"`
	Redis    RedisSettings    `yaml:"redis" json:"redis"`
}

type RocketMQSettings struct {
	Enabled    string           `yaml:"enabled" json:"enabled"`
	NameServer string           `yaml:"name-server" json:"nameServer"`
	Producer   RocketMQProducer `yaml:"producer" json:"producer"`
	Topic      string           `yaml:"topic" json:"topic"`
	Tag        string           `yaml:"tag" json:"tag"`
}

type RocketMQProducer struct {
	AccessKey string `yaml:"access-key" json:"accessKey"`
	SecretKey string `yaml:"secret-key" json:"secretKey"`
	Group     string `yaml:"group" json:"group"`
}

type PushSettings struct {
	Enabled      string `yaml:"enabled" json:"enabled"`
	AppID        string `yaml:"appId" json:"appId"`
	AppKey       string `yaml:"appKey" json:"appKey"`
	AppSecret    string `yaml:"appSecret" json:"appSecret"`
	MasterSecret string `yaml:"masterSecret" json:"masterSecret"`
	BaseURL      string `yaml:"baseUrl" json:"baseUrl"`
	TTLMillis    int64  `yaml:"ttl" json:"ttl"`
}

type RedisSettings struct {
	Enabled  string        `yaml:"enabled" json:"enabled"`
	Host     string        `yaml:"host" json:"host"`
	Port     int           `yaml:"port" json:"port"`
	Database int           `yaml:"database" json:"database"`
	Password string        `yaml:"password" json:"password"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
	QueueKey string        `yaml:"queue-key" json:"queueKey"`
	Lettuce  RedisLettuce  `yaml:"lettuce" json:"lettuce"`
}

type RedisLettuce struct {
	Pool RedisPool `yaml:"pool" json:"pool"`
}

type RedisPool struct {
	MaxIdle   int           `yaml:"max-idle" json:"maxIdle"`
	MaxActive int           `yaml:"max-active" json:"maxActive"`
	MaxWait   time.Duration `yaml:"max-wait" json:"maxWait"`
}

func (s Settings) WithDefaults() Settings {
	o := s
	o.RocketMQ.Enabled = normalizeYN(o.RocketMQ.Enabled)
	o.Push.Enabled = normalizeYN(o.Push.Enabled)
	o.Redis.Enabled = normalizeYN(o.Redis.Enabled)

	if o.Push.BaseURL == "" {
		o.Push.BaseURL = "https://restapi.getui.com/v2"
	}
	if o.Push.TTLMillis <= 0 {
		o.Push.TTLMillis = 2 * 60 * 60 * 1000
	}
	if o.Redis.Port == 0 {
		o.Redis.Port = 6379
	}
	if o.Redis.Timeout == 0 {
		o.Redis.Timeout = 5 * time.Second
	}
	if o.Redis.QueueKey == "" {
		o.Redis.QueueKey = "push:queue"
	}
	return o
}

func normalizeYN(v string) string {
	v = strings.TrimSpace(strings.ToUpper(v))
	if v == "" {
		return "N"
	}
	if v == "TRUE" {
		return "Y"
	}
	if v == "FALSE" {
		return "N"
	}
	if v == "1" {
		return "Y"
	}
	if v == "0" {
		return "N"
	}
	if v != "Y" {
		return "N"
	}
	return "Y"
}
