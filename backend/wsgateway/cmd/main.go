package main

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/spf13/viper"

	"github.com/imtaco/audio-rtc-exp/internal/config"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/httputil"
	wsrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/websocket"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/otel"
	"github.com/imtaco/audio-rtc-exp/internal/redis"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
	"github.com/imtaco/audio-rtc-exp/users/status"
	"github.com/imtaco/audio-rtc-exp/wsgateway/janusproxy"
	"github.com/imtaco/audio-rtc-exp/wsgateway/signal"
)

type Config struct {
	App    config.App      `mapstructure:"app"`
	WSHttp httputil.Config `mapstructure:"ws_http"`
	Redis  redis.Config    `mapstructure:"redis"`
	Etcd   etcd.Config     `mapstructure:"etcd"`
	Otel   otel.Config     `mapstructure:"otel"`

	RedisUserSvcPrefix   string `mapstructure:"redis_user_svc_prefix"`
	EtcdPrefixRoomStore  string `mapstructure:"etcd_prefix_room_store"`
	EtcdPrefixJanusStore string `mapstructure:"etcd_prefix_janus_store"`

	RedisReqStream      string `mapstructure:"redis_req_stream"`
	RedisReplyStream    string `mapstructure:"redis_reply_stream"`
	RedisWSNotifyStream string `mapstructure:"redis_ws_notify_stream"`

	JWTSecret    string `mapstructure:"jwt_secret"`
	JWTExpiresIn string `mapstructure:"jwt_expires_in"`

	JanusPort          string `mapstructure:"janus_port"`
	JanusTokenKey      string `mapstructure:"janus_token_key"`
	JanusInstCacheSize int    `mapstructure:"janus_inst_cache_size"`

	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("redis_user_svc_prefix", "rtcus")
		v.SetDefault("etcd_prefix_room_store", "/rooms/")
		v.SetDefault("etcd_prefix_janus_store", "/januses/")
		v.SetDefault("redis_req_stream", "rtcus:user-status-req-stream")
		v.SetDefault("redis_reply_stream", "rtcus:user-status-reply-stream")
		v.SetDefault("redis_ws_notify_stream", "rtcus:user-status-ws-stream")
		v.SetDefault("janus_port", "8088")
		v.SetDefault("jwt_secret", "MY-secret-key-change-in-production")
		v.SetDefault("jwt_expires_in", "1h")
		v.SetDefault("janus_token_key", "my-janus-token-key-32bytes!!!!!!")
		v.SetDefault("janus_inst_cache_size", 2000)
		v.SetDefault("allowed_origins", []string{"*"})

		config.Setup(v, "app")
		redis.Setup(v, "redis")
		etcd.Setup(v, "etcd")
		otel.Setup(v, "otel")
		httputil.Setup(v, "ws_http")

		// override default addrs to ease testing
		v.SetDefault("ws_http.addr", "0.0.0.0:8081")
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
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()

	// Initialize OpenTelemetry
	otelShutdown, err := otel.Init(ctx, &config.Otel, logger)
	if err != nil {
		logger.Fatal("Failed to initialize OTEL provider", log.Error(err))
	}

	logger.Info("Starting WebSocket Gateway...")

	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}

	redisClient := redis.NewClient(&config.Redis)
	if err := redis.Ping(redisClient); err != nil {
		logger.Fatal("Failed to connect to Redis", log.Error(err))
	}

	jwtAuth := jwt.NewAuth(config.JWTSecret)

	janusProxy, err := janusproxy.NewProxy(
		etcdClient,
		config.EtcdPrefixRoomStore,
		config.EtcdPrefixJanusStore,
		config.JanusInstCacheSize,
		config.JanusPort,
		logger.Module("JanusProxy"),
	)
	if err != nil {
		logger.Fatal("Failed to create Janus proxy", log.Error(err))
	}

	userService, err := status.NewUserService(
		redisClient,
		jwtAuth,
		config.RedisReqStream,
		config.RedisReplyStream,
		logger.Module("UserSvc"),
	)
	if err != nil {
		logger.Fatal("Failed to create User Service", log.Error(err))
	}

	connMgr, err := signal.NewWSConnMgr(
		redisClient,
		config.RedisWSNotifyStream,
		logger.Module("ConnMgr"),
	)
	if err != nil {
		logger.Fatal("Failed to create WS Client Manager", log.Error(err))
	}

	serverID := uuid.New().String()
	connGuard := signal.NewConnGuard(
		redisClient,
		config.RedisUserSvcPrefix,
		serverID,
		logger.Module("ConnLock"),
	)
	hook := signal.NewWSHook(
		connMgr,
		connGuard,
		jwtAuth,
		logger.Module("WSHook"),
	)
	janusTokenCodec, err := janusproxy.NewJanusTokenCodec([]byte(config.JanusTokenKey))
	if err != nil {
		logger.Fatal("Failed to create Janus token codec", log.Error(err))
	}
	wsRPCServer := wsrpc.NewServer(
		hook,
		config.AllowedOrigins,
		logger.Module("WSRPC"),
	)
	signalServer := signal.NewServer(
		wsRPCServer,
		janusProxy,
		janusTokenCodec,
		connMgr,
		userService,
		connGuard,
		jwtAuth,
		logger.Module("Signal"),
	)

	// Start components
	if err := janusProxy.Open(ctx); err != nil {
		logger.Fatal("Failed to initialize Janus proxy", log.Error(err))
	}
	if err := connMgr.Start(ctx); err != nil {
		logger.Fatal("Failed to start WS Client Manager", log.Error(err))
	}
	if err := signalServer.Open(ctx); err != nil {
		logger.Fatal("Failed to open Signal Server", log.Error(err))
	}

	wsMux := http.NewServeMux()
	wsMux.HandleFunc("/ws", wsRPCServer.HandleWebSocket)
	// TODO: health check endpoint?
	wsServer := httputil.NewServer(&config.WSHttp, wsMux)

	// Start WebSocket server in goroutine
	go func() {
		logger.Info("Starting WebSocket server", log.String("addr", config.WSHttp.Addr))
		if err := wsServer.Listen(); err != nil {
			logger.Fatal("Failed to start WebSocket server", log.Error(err))
		}
	}()

	// Graceful shutdown
	cleanup := func(ctx context.Context) {
		_ = wsServer.Shutdown(ctx)

		signalServer.Close()
		_ = connMgr.Stop(ctx)

		if err := janusProxy.Close(); err != nil {
			logger.Error("Error cleaning up Janus proxy", log.Error(err))
		}
		if err := redisClient.Close(); err != nil {
			logger.Error("Error closing Redis client", log.Error(err))
		}
		if err := etcdClient.Close(); err != nil {
			logger.Error("Error closing etcd client", log.Error(err))
		}
		if err := otelShutdown(ctx); err != nil {
			logger.Error("Failed to shutdown OTEL", log.Error(err))
		}
	}
	workflow.WaitGracefulShutdown(ctx, logger.Module("CleanUp"), cleanup, config.App.ShutdownTimeout)
}
