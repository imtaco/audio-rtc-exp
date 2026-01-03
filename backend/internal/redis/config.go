package redis

import (
	"crypto/tls"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type Config struct {
	Addr     string `mapstructure:"addr"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	TLS      bool   `mapstructure:"tls"`
	// DialTimeout time.Duration `mapstructure:"dial_timeout"`
	// ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	// WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// PoolSize int `mapstructure:"pool_size"`
	// MinIdleConns int           `mapstructure:"min_idle_conns"`
	// MaxRetries   int           `mapstructure:"max_retries"`
}

func NewClient(cfg *Config) *redis.Client {
	opt := &redis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
		// DialTimeout: cfg.DialTimeout,
		// ReadTimeout:  cfg.ReadTimeout,
		// WriteTimeout: cfg.WriteTimeout,
		// PoolSize: cfg.PoolSize,
		// MinIdleConns: cfg.MinIdleConns,
		// MaxRetries:   cfg.MaxRetries,
	}

	if cfg.TLS {
		opt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	return redis.NewClient(opt)
}

func Setup(v *viper.Viper, prefix string) {
	p := func(key string) string { return prefix + "." + key }

	// Defaults
	v.SetDefault(p("addr"), "redis:6379")
	v.SetDefault(p("username"), "")
	v.SetDefault(p("password"), "")
	v.SetDefault(p("db"), 0)
	v.SetDefault(p("tls"), false)
	// TODO: check default value
	// v.SetDefault("redis.dial_timeout", "5s")
	// v.SetDefault("redis.read_timeout", "3s")
	// v.SetDefault("redis.write_timeout", "3s")
	// v.SetDefault("redis.pool_size", 50)
	// v.SetDefault("redis.min_idle_conns", 10)
	// v.SetDefault("redis.max_retries", 3)

	// Optional: read config file if present
	// v.SetConfigName("config")
	// v.SetConfigType("yaml")
	// v.AddConfigPath(".")
}
