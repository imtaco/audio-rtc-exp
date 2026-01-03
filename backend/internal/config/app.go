package config

import (
	"time"

	"github.com/spf13/viper"
)

type App struct {
	LogConfigFile   string        `mapstructure:"log_config_file"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

func Setup(v *viper.Viper, prefix string) {
	p := func(key string) string { return prefix + "." + key }

	v.SetDefault(p("log_config_file"), "") // empty means use default config
	v.SetDefault(p("shutdown_timeout"), "10s")
}
