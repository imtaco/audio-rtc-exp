package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/viper"

	"github.com/imtaco/audio-rtc-exp/internal/config"
	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	etcdheartbeat "github.com/imtaco/audio-rtc-exp/internal/heartbeat/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/httputil"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/network"
	"github.com/imtaco/audio-rtc-exp/internal/otel"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
	"github.com/imtaco/audio-rtc-exp/januses/transport"
	"github.com/imtaco/audio-rtc-exp/januses/watcher"
)

const (
	monitorInterval = 5 * time.Second
)

type Config struct {
	App               config.App      `mapstructure:"app"`
	Etcd              etcd.Config     `mapstructure:"etcd"`
	Otel              otel.Config     `mapstructure:"otel"`
	Http              httputil.Config `mapstructure:"http"`
	JanusID           string          `mapstructure:"janus_id"`
	JanusAdvHost      string          `mapstructure:"janus_adv_host"`
	JanusBaseURL      string          `mapstructure:"janus_base_url"`
	JanusCapacity     int             `mapstructure:"janus_capacity"`
	AdminSecret       string          `mapstructure:"admin_secret"`
	EtcdPrefixRooms   string          `mapstructure:"etcd_prefix_rooms"`
	EtcdPrefixJanuses string          `mapstructure:"etcd_prefix_januses"`
	CanaryRoomID      int64           `mapstructure:"canary_room_id"`
	LeaseTTL          time.Duration   `mapstructure:"lease_ttl"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("janus_id", "janus1")
		v.SetDefault("janus_adv_host", "janus")
		v.SetDefault("janus_base_url", "http://janus:8088")
		v.SetDefault("janus_capacity", 10)
		v.SetDefault("admin_secret", "supersecret")
		v.SetDefault("etcd_prefix_rooms", "/rooms/")
		v.SetDefault("etcd_prefix_januses", "/januses/")
		v.SetDefault("canary_room_id", 999999)
		v.SetDefault("lease_ttl", 10*time.Second)

		config.Setup(v, "app")
		etcd.Setup(v, "etcd")
		otel.Setup(v, "otel")
		httputil.Setup(v, "http")
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

	if config.JanusAdvHost == "" {
		config.JanusAdvHost = network.HostIP().String()
		logger.Info("Janus advertisement host not set, detecting automatically",
			log.String("host", config.JanusAdvHost))
	}

	logger.Info("Janus Manager starting", log.String("janusId", config.JanusID))

	// Create etcd client
	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}

	// Create Janus API
	logger.Info("baseURL", log.String("url", config.JanusBaseURL))
	janusAPI := janus.New(config.JanusBaseURL, logger.Module("JanusAPI"))
	janusAdminInst, err := janusAPI.CreateAdminInstance(ctx, config.AdminSecret)
	if err != nil {
		logger.Fatal("Failed to create Janus admin instance", log.Error(err))
	}
	logger.Info("Janus admin instance created")

	// Start keepalive for admin instance
	janusAdminInst.StartKeepalive()

	// Create Janus monitor
	janusMonitor := watcher.NewJanusHealthMonitor(
		janusAdminInst,
		config.CanaryRoomID,
		monitorInterval,
		logger.Module("Monitor"),
	)

	// Create room watcher
	roomWatcher := watcher.NewRoomWatcher(
		etcdClient,
		config.JanusID,
		config.JanusAdvHost,
		janusAdminInst,
		config.EtcdPrefixRooms,
		config.EtcdPrefixJanuses,
		config.CanaryRoomID,
		logger.Module("RoomWatcher"),
	)

	// Connect restart event from monitor to watcher
	janusMonitor.SetRestartHandler(func(reason string) {
		logger.Warn("Janus server restarted, cleaning up etcd entries", log.String("reason", reason))
		if err := roomWatcher.JanusRestartDetected(); err != nil {
			logger.Error("Failed to handle Janus restart", log.Error(err))
		}
	})

	// Start Janus heartbeat
	hbKey := fmt.Sprintf("%s%s/heartbeat", config.EtcdPrefixJanuses, config.JanusID)
	hbData := etcdstate.HeartbeatData{
		Status:    constants.ModuleStatusHealthy,
		Host:      config.JanusAdvHost,
		Capacity:  config.JanusCapacity,
		StartedAt: time.Now().UTC(),
	}
	heartbeat := etcdheartbeat.New(
		etcdClient,
		hbKey,
		hbData,
		config.LeaseTTL,
		logger.Module("Heartbeat"),
	)

	// Start all components
	if err := janusMonitor.Start(ctx); err != nil {
		logger.Fatal("Failed to start Janus monitor", log.Error(err))
	}

	if err := roomWatcher.Start(ctx); err != nil {
		logger.Fatal("Failed to start room watcher", log.Error(err))
	}

	if err := heartbeat.Start(ctx); err != nil {
		logger.Fatal("Failed to start heartbeat", log.Error(err))
	}

	// Setup Gin router
	router := transport.NewRouter(config.JanusID, logger.Module("Router"))
	server := httputil.NewServer(&config.Http, router.Handler())

	go func() {
		logger.Info("Starting HTTP server", log.String("addr", config.Http.Addr))
		if err := server.Listen(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start HTTP server", log.Error(err))
		}
	}()
	logger.Info("Janus Manager started")

	// Setup graceful shutdown
	cleanup := func(ctx context.Context) {
		server.Shutdown(ctx)

		if err := heartbeat.Stop(ctx); err != nil {
			logger.Error("Failed to cleanup heartbeat", log.Error(err))
		}

		if err := roomWatcher.Stop(); err != nil {
			logger.Error("Failed to cleanup room watcher", log.Error(err))
		}
		janusMonitor.Stop()
		if err := etcdClient.Close(); err != nil {
			logger.Error("Failed to close etcd client", log.Error(err))
		}
		if err := otelShutdown(ctx); err != nil {
			logger.Error("Failed to shutdown OTEL", log.Error(err))
		}
	}
	workflow.WaitGracefulShutdown(ctx, logger.Module("CleanUp"), cleanup, config.App.ShutdownTimeout)
}
