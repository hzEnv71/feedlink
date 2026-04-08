package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Feed     FeedConfig     `mapstructure:"feed"`
	Upload   UploadConfig   `mapstructure:"upload"`
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
	Host      string `mapstructure:"host"`
	Port      int    `mapstructure:"port"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	VHost     string `mapstructure:"vhost"`
	Queue     string `mapstructure:"queue"`
	Prefetch  int    `mapstructure:"prefetch"`
	Consumers int    `mapstructure:"consumers"`
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

var AppConfig *Config

func InitConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	return nil
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		d.Username, d.Password, d.Host, d.Port, d.DBName, d.Charset)
}

func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

func (r *RabbitMQConfig) URL() string {
	vhost := r.VHost
	if vhost == "" {
		vhost = "/"
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", r.Username, r.Password, r.Host, r.Port, vhost)
}
