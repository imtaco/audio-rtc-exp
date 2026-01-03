package config

import (
	"strings"

	"github.com/spf13/viper"
)

func NewViper() *viper.Viper {
	v := viper.New()

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("")
	v.AutomaticEnv()

	return v
}

func Load[T any](c *T, configure func(v *viper.Viper)) (*T, error) {
	v := NewViper()

	configure(v)
	return c, v.Unmarshal(c)
}
