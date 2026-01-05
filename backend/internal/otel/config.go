package otel

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	// Tracing configuration
	TracingEnabled bool    `mapstructure:"tracing_enabled"`
	SamplingRate   float64 `mapstructure:"sampling_rate"`

	// Metrics configuration
	MetricsEnabled        bool          `mapstructure:"metrics_enabled"`
	MetricsExportInterval time.Duration `mapstructure:"metrics_export_interval"`
	RuntimeMetricsEnabled bool          `mapstructure:"go_metrics_enabled"`

	// Common configuration
	ServiceName string        `mapstructure:"service_name"`
	Endpoint    string        `mapstructure:"endpoint"`
	Insecure    bool          `mapstructure:"insecure"`
	Timeout     time.Duration `mapstructure:"timeout"`
}

func Setup(v *viper.Viper, prefix string) {
	p := func(key string) string { return prefix + "." + key }

	// Tracing defaults
	v.SetDefault(p("tracing_enabled"), false)
	v.SetDefault(p("sampling_rate"), 1.0)

	// Metrics defaults
	v.SetDefault(p("metrics_enabled"), false)
	v.SetDefault(p("metrics_export_interval"), "30s")
	v.SetDefault(p("go_metrics_enabled"), false)

	// Common defaults
	v.SetDefault(p("service_name"), "mixer-service")
	v.SetDefault(p("endpoint"), "localhost:4317")
	v.SetDefault(p("insecure"), true)
	v.SetDefault(p("timeout"), "10s")
}
