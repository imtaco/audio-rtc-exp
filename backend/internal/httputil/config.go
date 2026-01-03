package httputil

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type Config struct {
	Addr string    `mapstructure:"addr"`
	TLS  TLSConfig `mapstructure:"tls"`
}

type Server struct {
	*http.Server
	cfg *Config
}

func Setup(v *viper.Viper, prefix string) {
	p := func(key string) string { return prefix + "." + key }

	v.SetDefault(p("addr"), ":8080")
	v.SetDefault(p("tls.enabled"), false)
	v.SetDefault(p("tls.cert_file"), "")
	v.SetDefault(p("tls.key_file"), "")
}

func NewServer(cfg *Config, handler http.Handler) *Server {
	return &Server{
		Server: &http.Server{
			Addr:    cfg.Addr,
			Handler: handler,
		},
		cfg: cfg,
	}
}

func (s *Server) Listen() error {
	cfg := s.cfg
	if !cfg.TLS.Enabled {
		return s.ListenAndServe()
	}

	if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
		return errors.New("TLS is enabled but cert_file or key_file is not set")
	}
	return s.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
}
