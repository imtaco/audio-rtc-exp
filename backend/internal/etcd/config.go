package etcd

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CAFile   string `mapstructure:"ca_file"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	// Optional: set to true if you intentionally want to skip verification
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

type Config struct {
	Endpoints            []string      `mapstructure:"endpoints"`
	Username             string        `mapstructure:"username"`
	Password             string        `mapstructure:"password"`
	DialTimeout          time.Duration `mapstructure:"dial_timeout"`
	DialKeepAliveTime    time.Duration `mapstructure:"dial_keepalive_time"`
	DialKeepAliveTimeout time.Duration `mapstructure:"dial_keepalive_timeout"`

	AutoSyncInterval time.Duration `mapstructure:"auto_sync_interval"`

	// gRPC message size limits (bytes)
	MaxCallSendMsgSize int `mapstructure:"max_call_send_msg_size"`
	MaxCallRecvMsgSize int `mapstructure:"max_call_recv_msg_size"`

	TLS TLSConfig `mapstructure:"tls"`
}

func Setup(v *viper.Viper, prefix string) {
	p := func(key string) string { return prefix + "." + key }

	v.SetDefault(p("endpoints"), []string{"etcd:2379"})
	v.SetDefault(p("username"), "")
	v.SetDefault(p("password"), "")

	v.SetDefault(p("dial_timeout"), "5s")
	v.SetDefault(p("dial_keepalive_time"), "30s")
	v.SetDefault(p("dial_keepalive_timeout"), "10s")

	// 0 disables autosync
	v.SetDefault(p("auto_sync_interval"), "0s")

	// etcd client defaults are usually 0 (use gRPC defaults); set if you want explicit caps
	v.SetDefault(p("max_call_send_msg_size"), 0)
	v.SetDefault(p("max_call_recv_msg_size"), 0)

	v.SetDefault(p("tls.enabled"), false)
	v.SetDefault(p("tls.ca_file"), "")
	v.SetDefault(p("tls.cert_file"), "")
	v.SetDefault(p("tls.key_file"), "")
	v.SetDefault(p("tls.insecure_skip_verify"), false)
}

func (c Config) BuildClientConfig() (clientv3.Config, error) {
	cfg := clientv3.Config{
		Endpoints:            c.Endpoints,
		Username:             c.Username,
		Password:             c.Password,
		DialTimeout:          c.DialTimeout,
		DialKeepAliveTime:    c.DialKeepAliveTime,
		DialKeepAliveTimeout: c.DialKeepAliveTimeout,
		AutoSyncInterval:     c.AutoSyncInterval,
	}

	if c.MaxCallSendMsgSize > 0 {
		cfg.MaxCallSendMsgSize = c.MaxCallSendMsgSize
	}
	if c.MaxCallRecvMsgSize > 0 {
		cfg.MaxCallRecvMsgSize = c.MaxCallRecvMsgSize
	}

	if c.TLS.Enabled {
		tlsCfg, err := buildTLSConfig(c.TLS)
		if err != nil {
			return clientv3.Config{}, err
		}
		cfg.TLS = tlsCfg
	}

	return cfg, nil
}

func buildTLSConfig(t TLSConfig) (*tls.Config, error) {
	tc := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: t.InsecureSkipVerify,
	}

	if t.CAFile != "" {
		caPEM, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, errors.Wrapf(err, "fail to read etcd ca_file")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("parse etcd ca_file: no certs found")
		}
		tc.RootCAs = pool
	}

	// mTLS client cert (optional)
	if t.CertFile != "" || t.KeyFile != "" {
		if t.CertFile == "" || t.KeyFile == "" {
			return nil, errors.New("fail to etcd tls requires both cert_file and key_file")
		}
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "fail to load etcd client cert/key")
		}
		tc.Certificates = []tls.Certificate{cert}
	}

	return tc, nil
}

func NewClient(c *Config) (*clientv3.Client, error) {
	cfg, err := c.BuildClientConfig()
	if err != nil {
		return nil, err
	}
	return clientv3.New(cfg)
}
