package main

import (
	"context"
	"net/http"

	"github.com/spf13/viper"

	"github.com/imtaco/audio-rtc-exp/internal/config"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/httputil"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/otel"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
	"github.com/imtaco/audio-rtc-exp/rooms/service"
	"github.com/imtaco/audio-rtc-exp/rooms/store"
	"github.com/imtaco/audio-rtc-exp/rooms/transport"
)

type Config struct {
	App                  config.App      `mapstructure:"app"`
	HTTP                 httputil.Config `mapstructure:"http"`
	Etcd                 etcd.Config     `mapstructure:"etcd"`
	Otel                 otel.Config     `mapstructure:"otel"`
	HLSAdvURL            string          `mapstructure:"hls_adv_url"`
	EtcdPrefixRoomStore  string          `mapstructure:"etcd_prefix_room_store"`
	EtcdPrefixJanusStore string          `mapstructure:"etcd_prefix_janus_store"`
	EtcdPrefixMixerStore string          `mapstructure:"etcd_prefix_mixer_store"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("hls_adv_url", "http://localhost:8080/hls/")
		v.SetDefault("etcd_prefix_room_store", "/rooms/")
		v.SetDefault("etcd_prefix_janus_store", "/januses/")
		v.SetDefault("etcd_prefix_mixer_store", "/mixers/")

		config.Setup(v, "app")
		etcd.Setup(v, "etcd")
		otel.Setup(v, "otel")
		httputil.Setup(v, "http")

		// override default addrs to ease testing
		v.SetDefault("http.addr", "0.0.0.0:3000")
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

	// global background context
	ctx := context.Background()

	// Initialize OpenTelemetry
	otelShutdown, err := otel.Init(ctx, &config.Otel, logger)
	if err != nil {
		logger.Fatal("Failed to initialize OTEL provider", log.Error(err))
	}

	logger.Info("Starting Room Manager service",
		log.String("addr", config.HTTP.Addr),
		log.Any("etcdUrl", config.Etcd.Endpoints),
		log.String("hlsAdvUrl", config.HLSAdvURL))

	// Create etcd client
	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}
	defer etcdClient.Close()

	// Create components
	roomStore := store.NewRoomStore(
		etcdClient,
		config.EtcdPrefixRoomStore,
		logger.Module("RoomStore"),
	)

	resManager := service.NewResourceManager(
		etcdClient,
		roomStore,
		config.EtcdPrefixRoomStore,
		config.EtcdPrefixJanusStore,
		config.EtcdPrefixMixerStore,
		logger.Module("ResMgr"),
	)

	roomService := service.NewRoomService(
		roomStore,
		resManager,
		config.HLSAdvURL,
		logger.Module("RoomSvc"),
	)

	// Initialize resource manager
	if err = resManager.Start(ctx); err != nil {
		logger.Fatal("Failed to start resource manager", log.Error(err))
	}

	// Setup router
	router := transport.NewRouter(roomService, roomStore, logger.Module("Router"))
	server := httputil.NewServer(&config.HTTP, router.Handler())

	// Start HTTP server
	go func() {
		logger.Info("Starting HTTP server", log.String("addr", config.HTTP.Addr))
		if err := server.Listen(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start HTTP server", log.Error(err))
		}
	}()

	logger.Info("Room Manager started")

	// Setup graceful shutdown
	cleanup := func(ctx context.Context) {
		_ = server.Shutdown(ctx)

		if err := resManager.Stop(); err != nil {
			logger.Error("Error cleaning up resource manager", log.Error(err))
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
