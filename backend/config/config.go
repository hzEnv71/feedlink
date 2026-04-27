package config

import (
	"fmt"

	"github.com/spf13/viper"
)

const (
	defaultConfigName = "config"
	defaultConfigType = "yaml"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	RabbitMQ  RabbitMQConfig  `mapstructure:"rabbitmq"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Feed      FeedConfig      `mapstructure:"feed"`
	Upload    UploadConfig    `mapstructure:"upload"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	WS        WSConfig        `mapstructure:"ws"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	Charset      string `mapstructure:"charset"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type RabbitMQConfig struct {
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	VHost            string `mapstructure:"vhost"`
	Queue            string `mapstructure:"queue"`
	Prefetch         int    `mapstructure:"prefetch"`
	Consumers        int    `mapstructure:"consumers"`
	DeadLetterQueue  string `mapstructure:"dead_letter_queue"`
	RetryQueue       string `mapstructure:"retry_queue"`
	MaxRetries       int    `mapstructure:"max_retries"`
	RetryDelayMS     int    `mapstructure:"retry_delay_ms"`
	IdempotentTTLMin int    `mapstructure:"idempotent_ttl_min"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expire int    `mapstructure:"expire"`
}

type FeedConfig struct {
	BigVThreshold int `mapstructure:"big_v_threshold"`
	PushFanLimit  int `mapstructure:"push_fan_limit"`
	InboxMaxSize  int `mapstructure:"inbox_max_size"`
	OutboxMaxSize int `mapstructure:"outbox_max_size"`
	PageSize      int `mapstructure:"page_size"`
}

type UploadConfig struct {
	Path         string   `mapstructure:"path"`
	ImageExts    []string `mapstructure:"image_exts"`
	VideoExts    []string `mapstructure:"video_exts"`
	ImageMaxSize int      `mapstructure:"image_max_size"`
	VideoMaxSize int      `mapstructure:"video_max_size"`
}

// RateLimitConfig 令牌桶限流配置。
type RateLimitConfig struct {
	LoginIP      TokenBucketConfig `mapstructure:"login_ip"`
	RegisterIP   TokenBucketConfig `mapstructure:"register_ip"`
	PublishFeed  TokenBucketConfig `mapstructure:"publish_feed"`
	RepostFeed   TokenBucketConfig `mapstructure:"repost_feed"`
	LikeFeed     TokenBucketConfig `mapstructure:"like_feed"`
	CommentFeed  TokenBucketConfig `mapstructure:"comment_feed"`
	SendMessage  TokenBucketConfig `mapstructure:"send_message"`
}

// WSConfig WebSocket 限流配置。
type WSConfig struct {
	SendMessage TokenBucketConfig `mapstructure:"send_message"`
}

// TokenBucketConfig 配置单个接口的 rate/burst。
type TokenBucketConfig struct {
	Rate  float64 `mapstructure:"rate"`
	Burst int     `mapstructure:"burst"`
}

var AppConfig *Config

func InitConfig() error {
	viper.SetConfigName(defaultConfigName)
	viper.SetConfigType(defaultConfigType)
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	setRateLimitDefaults(&cfg.RateLimit)
	AppConfig = cfg
	return nil
}

func setRateLimitDefaults(c *RateLimitConfig) {
	setTokenBucketDefault(&c.LoginIP, 0.3, 15)
	setTokenBucketDefault(&c.RegisterIP, 0.2, 10)
	setTokenBucketDefault(&c.PublishFeed, 0.2, 10)
	setTokenBucketDefault(&c.RepostFeed, 0.4, 20)
	setTokenBucketDefault(&c.LikeFeed, 2, 60)
	setTokenBucketDefault(&c.CommentFeed, 0.6, 30)
	setTokenBucketDefault(&c.SendMessage, 0.8, 40)
}

func setTokenBucketDefault(c *TokenBucketConfig, rate float64, burst int) {
	if c.Rate <= 0 {
		c.Rate = rate
	}
	if c.Burst <= 0 {
		c.Burst = burst
	}
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		d.Username,
		d.Password,
		d.Host,
		d.Port,
		d.DBName,
		d.Charset,
	)
}

func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

func (r *RabbitMQConfig) QueueName() string {
	if r.Queue == "" {
		return "feed_queue"
	}
	return r.Queue
}

func (r *RabbitMQConfig) DeadLetterQueueName() string {
	if r.DeadLetterQueue == "" {
		return r.QueueName() + ".dlq"
	}
	return r.DeadLetterQueue
}

func (r *RabbitMQConfig) RetryQueueName() string {
	if r.RetryQueue == "" {
		return r.QueueName() + ".retry"
	}
	return r.RetryQueue
}

func (r *RabbitMQConfig) URL() string {
	vhost := r.VHost
	if vhost == "" {
		vhost = "/"
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", r.Username, r.Password, r.Host, r.Port, vhost)
}
