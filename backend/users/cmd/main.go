package main

import (
	"context"
	"net/http"
	"time"

	"github.com/spf13/viper"

	"github.com/imtaco/audio-rtc-exp/internal/config"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/httputil"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/otel"
	"github.com/imtaco/audio-rtc-exp/internal/redis"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
	"github.com/imtaco/audio-rtc-exp/users/control"
	"github.com/imtaco/audio-rtc-exp/users/room"
	"github.com/imtaco/audio-rtc-exp/users/status"
	"github.com/imtaco/audio-rtc-exp/users/transport"
)

type Config struct {
	App                 config.App      `mapstructure:"app"`
	Http                httputil.Config `mapstructure:"http"`
	Redis               redis.Config    `mapstructure:"redis"`
	Etcd                etcd.Config     `mapstructure:"etcd"`
	Otel                otel.Config     `mapstructure:"otel"`
	RedisUserSvcPrefix  string          `mapstructure:"redis_user_svc_prefix"`
	EtcdRoomPrefix      string          `mapstructure:"etcd_room_prefix"`
	RedisReqStream      string          `mapstructure:"redis_req_stream"`
	RedisReplyStream    string          `mapstructure:"redis_reply_stream"`
	RedisWSNotifyStream string          `mapstructure:"redis_ws_notify_stream"`
	StreamTrimInterval  time.Duration   `mapstructure:"stream_trim_interval"`
	JWTSecret           string          `mapstructure:"jwt_secret"`
	JWTExpiresIn        string          `mapstructure:"jwt_expires_in"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("redis_user_svc_prefix", "rtcus")
		v.SetDefault("etcd_room_prefix", "/rooms/")
		v.SetDefault("redis_req_stream", "rtcus:user-status-req-stream")
		v.SetDefault("redis_reply_stream", "rtcus:user-status-reply-stream")
		v.SetDefault("redis_ws_notify_stream", "rtcus:user-status-ws-stream")
		v.SetDefault("jwt_secret", "MY-secret-key-change-in-production")
		v.SetDefault("jwt_expires_in", "1h")
		v.SetDefault("prefix_room_store", "/rooms/")
		v.SetDefault("stream_trim_interval", 30*time.Second)

		redis.Setup(v, "app")
		redis.Setup(v, "redis")
		etcd.Setup(v, "etcd")
		otel.Setup(v, "otel")
		httputil.Setup(v, "http")

		// override default addrs to ease testing
		v.SetDefault("http.addr", "0.0.0.0:8085")
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

	// global background context
	ctx := context.Background()

	// Initialize OpenTelemetry
	otelShutdown, err := otel.Init(ctx, &config.Otel, logger)
	if err != nil {
		logger.Fatal("Failed to initialize OTEL provider", log.Error(err))
	}

	logger.Info("Starting User Service...")

	// Initialize Redis client
	redisClient := redis.NewClient(&config.Redis)
	// check Redis
	if err := redis.Ping(redisClient); err != nil {
		logger.Fatal("Failed to connect to Redis", log.Error(err))
	}
	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}

	// Initialize JWT Auth (expiresIn handled in JWT library if needed)
	jwtAuth := jwt.NewJWTAuth(config.JWTSecret)

	// Initialize User Status Service
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

	// Initialize User Status Consumer
	roomUserState := room.New(redisClient, config.RedisUserSvcPrefix, logger.Module("RoomState"))
	userCtrl, err := control.NewUserStatusControl(
		redisClient,
		etcdClient,
		roomUserState,
		config.EtcdRoomPrefix,
		config.RedisReqStream,
		config.RedisReplyStream,
		config.RedisWSNotifyStream,
		logger.Module("UserCtrl"),
	)
	if err != nil {
		logger.Fatal("Failed to create User Control", log.Error(err))
	}

	// Initialize Trimer to clean up old messages
	trimer, err := control.NewTrimer(
		redisClient,
		config.RedisReqStream,
		config.RedisReplyStream,
		config.RedisWSNotifyStream,
		config.StreamTrimInterval,
		logger.Module("Trimer"),
	)
	if err != nil {
		logger.Fatal("Failed to create Trimer", log.Error(err))
	}

	// Initialize REST API router
	router := transport.NewRouter(userService, jwtAuth, logger.Module("Router"))
	server := httputil.NewServer(&config.Http, router.Handler())

	// Start components
	if err := trimer.Start(ctx); err != nil {
		logger.Fatal("Failed to start Trimer", log.Error(err))
	}
	if err := userCtrl.Start(ctx); err != nil {
		logger.Fatal("Failed to start User Control", log.Error(err))
	}
	if err := userService.Start(ctx); err != nil {
		logger.Fatal("Failed to start User Service", log.Error(err))
	}

	// Start HTTP server in goroutine
	go func() {
		logger.Info("Starting REST API server", log.String("addr", config.Http.Addr))
		if err := server.Listen(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start REST API server", log.Error(err))
		}
	}()

	// Graceful shutdown
	cleanup := func(ctx context.Context) {
		server.Shutdown(ctx)
		trimer.Stop()

		if err := userCtrl.Stop(); err != nil {
			logger.Error("Error closing user consumer", log.Error(err))
		}
		if err := redisClient.Close(); err != nil {
			logger.Error("Error closing Redis client", log.Error(err))
		}
		if err := etcdClient.Close(); err != nil {
			logger.Error("Failed to close etcd client", log.Error(err))
		}
		if err := otelShutdown(ctx); err != nil {
			logger.Error("Failed to shutdown OTEL", log.Error(err))
		}
	}
	workflow.WaitGracefulShutdown(ctx, logger.Module("CleanUp"), cleanup, config.App.ShutdownTimeout)
}
