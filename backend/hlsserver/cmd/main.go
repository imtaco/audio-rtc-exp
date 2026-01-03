package main

import (
	"context"
	"net/http"

	"github.com/spf13/viper"

	"github.com/imtaco/audio-rtc-exp/hlsserver/transport"
	"github.com/imtaco/audio-rtc-exp/hlsserver/watcher"
	"github.com/imtaco/audio-rtc-exp/internal/config"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/httputil"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
)

type Config struct {
	App               config.App      `mapstructure:"app"`
	Etcd              etcd.Config     `mapstructure:"etcd"`
	TokenServerHttp   httputil.Config `mapstructure:"token_server_http"`
	KeyServerHttp     httputil.Config `mapstructure:"key_server_http"`
	M3U8ServerHttp    httputil.Config `mapstructure:"m3u8_server_http"`
	EnableTokenServer bool            `mapstructure:"enable_token_server"`
	EnableKeyServer   bool            `mapstructure:"enable_key_server"`
	EnableM3U8Server  bool            `mapstructure:"enable_m3u8_server"`
	JWTSecret         string          `mapstructure:"jwt_secret"`
	EtcdPrefixRooms   string          `mapstructure:"etcd_prefix_rooms"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("enable_token_server", true)
		v.SetDefault("enable_key_server", true)
		v.SetDefault("enable_m3u8_server", false)
		v.SetDefault("jwt_secret", "your-secret-key-change-in-production")
		v.SetDefault("etcd_prefix_rooms", "/rooms/")

		config.Setup(v, "app")
		etcd.Setup(v, "etcd")
		httputil.Setup(v, "token_server_http")
		httputil.Setup(v, "key_server_http")
		httputil.Setup(v, "m3u8_server_http")

		// override default addrs to ease testing
		v.SetDefault("token_server_http.addr", "0.0.0.0:3100")
		v.SetDefault("key_server_http.addr", "0.0.0.0:3101")
		v.SetDefault("m3u8_server_http.addr", "0.0.0.0:3102")
	})
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	logger, err := log.NewLogger(config.App.LogConfigFile)
	if err != nil {
		log.Fatal("Failed to create logger", err)
	}
	defer logger.Sync()

	logger.Info("Starting HLS servers",
		log.Bool("tokenServerEnabled", config.EnableTokenServer),
		log.Bool("keyServerEnabled", config.EnableKeyServer),
		log.Bool("m3u8ServerEnabled", config.EnableM3U8Server),
		log.String("tokenServerAddr", config.TokenServerHttp.Addr),
		log.String("keyServerAddr", config.KeyServerHttp.Addr),
		log.String("m3u8ServerAddr", config.M3U8ServerHttp.Addr))

	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}
	defer etcdClient.Close()

	jwtAuth := jwt.NewJWTAuth(config.JWTSecret)

	roomWatcher := watcher.NewRoomWatcher(
		etcdClient,
		config.EtcdPrefixRooms,
		logger.Module("RoomWatcher"),
	)

	ctx := context.Background()
	if err := roomWatcher.Start(ctx); err != nil {
		logger.Fatal("Failed to start room watcher", log.Error(err))
	}

	tokenRouter := transport.NewTokenRouter(roomWatcher, jwtAuth, logger.Module("TokenRouter"))
	keyRouter := transport.NewKeyRouter(roomWatcher, jwtAuth, logger.Module("KeyRouter"))

	var tokenServer *httputil.Server
	var keyServer *httputil.Server

	// Start servers based on configuration
	if config.EnableTokenServer {
		tokenServer = httputil.NewServer(&config.TokenServerHttp, tokenRouter.Handler())
		go func() {
			logger.Info("Starting token server", log.String("addr", config.TokenServerHttp.Addr))
			if err := tokenServer.Listen(); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start token server", log.Error(err))
			}
		}()
	}

	if config.EnableKeyServer {
		keyServer = httputil.NewServer(&config.KeyServerHttp, keyRouter.Handler())
		go func() {
			logger.Info("Starting key server", log.String("addr", config.KeyServerHttp.Addr))
			if err := keyServer.Listen(); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Failed to start key server", log.Error(err))
			}
		}()
	}

	if config.EnableM3U8Server {
		logger.Info("M3U8 server enabled but not yet implemented",
			log.String("addr", config.M3U8ServerHttp.Addr))
	}

	cleanup := func(ctx context.Context) {
		if tokenServer != nil {
			tokenServer.Shutdown(ctx)
		}
		if keyServer != nil {
			keyServer.Shutdown(ctx)
		}

		if err := roomWatcher.Stop(); err != nil {
			logger.Error("Error stopping room watcher", log.Error(err))
		}
		if err := etcdClient.Close(); err != nil {
			logger.Error("Failed to close etcd client", log.Error(err))
		}
	}
	workflow.WaitGracefulShutdown(ctx, logger.Module("CleanUp"), cleanup, config.App.ShutdownTimeout)
}
