package main

import (
	"context"
	"errors"
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
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/network"
	"github.com/imtaco/audio-rtc-exp/internal/otel"
	"github.com/imtaco/audio-rtc-exp/internal/workflow"
	"github.com/imtaco/audio-rtc-exp/mixers/ffmpeg"
	"github.com/imtaco/audio-rtc-exp/mixers/transport"
	"github.com/imtaco/audio-rtc-exp/mixers/watcher"
)

type Config struct {
	App             config.App      `mapstructure:"app"`
	Etcd            etcd.Config     `mapstructure:"etcd"`
	HTTP            httputil.Config `mapstructure:"http"`
	Otel            otel.Config     `mapstructure:"otel"`
	MixerID         string          `mapstructure:"mixer_id"`
	MixerIP         string          `mapstructure:"mixer_ip"`
	MixerCapacity   int             `mapstructure:"mixer_capacity"`
	RTPPortStart    int             `mapstructure:"rtp_port_start"`
	RTPPortEnd      int             `mapstructure:"rtp_port_end"`
	EtcdPrefixRooms string          `mapstructure:"etcd_prefix_rooms"`
	EtcdPrefixMixer string          `mapstructure:"etcd_prefix_mixer"`
	KeyBaseURL      string          `mapstructure:"key_base_url"`
	HLSDir          string          `mapstructure:"hls_dir"`
	TempDir         string          `mapstructure:"temp_dir"`
	SDPDir          string          `mapstructure:"sdp_dir"`
	LeaseTTL        time.Duration   `mapstructure:"lease_ttl"`
}

func loadConfig() (*Config, error) {
	return config.Load(&Config{}, func(v *viper.Viper) {
		v.SetDefault("mixer_id", "mixer1")
		v.SetDefault("mixer_ip", "")
		v.SetDefault("mixer_capacity", 10)
		v.SetDefault("rtp_port_start", 10000)
		v.SetDefault("rtp_port_end", 20000)
		v.SetDefault("etcd_prefix_rooms", "/rooms/")
		v.SetDefault("etcd_prefix_mixer", "/mixers/")
		v.SetDefault("key_base_url", "http://localhost:3101/hls/rooms/")
		v.SetDefault("hls_dir", "/hls")
		v.SetDefault("temp_dir", "/tmp")
		v.SetDefault("sdp_dir", "/tmp/sdp")
		v.SetDefault("lease_ttl", 10*time.Second)

		config.Setup(v, "app")
		etcd.Setup(v, "etcd")
		httputil.Setup(v, "http")
		otel.Setup(v, "otel")

		// override default http.addr
		v.SetDefault("http.addr", "0.0.0.0:3001")
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

	if config.MixerIP == "" {
		config.MixerIP = network.HostIP().String()
		logger.Info("Mixer IP not set, detecting automatically", log.String("ip", config.MixerIP))
	}

	logger.Info("Starting Mixer service",
		log.String("mixerId", config.MixerID),
		log.String("mixerIp", config.MixerIP),
		log.String("rtpPortRange", fmt.Sprintf("%d-%d", config.RTPPortStart, config.RTPPortEnd)))

	// Initialize OpenTelemetry
	ctx := context.Background()
	otelShutdown, err := otel.Init(ctx, &config.Otel, logger)
	if err != nil {
		logger.Fatal("Failed to create OpenTelemetry provider", log.Error(err))
	}

	etcdClient, err := etcd.NewClient(&config.Etcd)
	if err != nil {
		logger.Fatal("Failed to create etcd client", log.Error(err))
	}
	defer etcdClient.Close()

	// Create components
	encGenerator := ffmpeg.NewEncryptionGenerator(config.KeyBaseURL, config.TempDir)
	sdpGenerator := ffmpeg.NewSDPGenerator(config.SDPDir)
	ffmpegManager := ffmpeg.NewFFmpegManager(
		config.HLSDir,
		encGenerator,
		sdpGenerator,
		1*time.Second, // retry delay
		5*time.Second, // force kill delay
		logger.Module("FFmpegMgr"),
	)

	// Create room watcher
	portManager := watcher.NewPortManager(
		config.RTPPortStart,
		config.RTPPortEnd,
		logger.Module("PortMgr"),
	)
	roomWatcher := watcher.NewRoomWatcher(
		etcdClient,
		config.MixerID,
		config.MixerIP,
		portManager,
		ffmpegManager,
		config.EtcdPrefixRooms,
		config.EtcdPrefixMixer,
		logger.Module("RoomWatcher"),
	)

	// Create heartbeat
	hbKey := fmt.Sprintf("%s%s/heartbeat", config.EtcdPrefixMixer, config.MixerID)
	hbData := etcdstate.HeartbeatData{
		Status:    constants.ModuleStatusHealthy,
		Host:      config.MixerIP,
		Capacity:  config.MixerCapacity,
		StartedAt: time.Now().UTC(),
	}
	heartbeat := etcdheartbeat.New(
		etcdClient,
		hbKey,
		hbData,
		config.LeaseTTL,
		logger.Module("Heartbeat"),
	)

	// initCtx := context.Background()
	// TODO: init with timeout ?!
	if err := roomWatcher.Start(ctx); err != nil {
		logger.Fatal("Failed to start room watcher", log.Error(err))
	}
	if err := heartbeat.Start(ctx); err != nil {
		logger.Fatal("Failed to start heartbeat", log.Error(err))
	}

	// Setup Gin router
	router := transport.NewRouter(config.MixerID, logger.Module("Router"))
	server := httputil.NewServer(&config.HTTP, router.Handler())

	go func() {
		logger.Info("Starting HTTP server", log.String("addr", config.HTTP.Addr))
		if err := server.Listen(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Failed to start HTTP server", log.Error(err))
		}
	}()
	logger.Info("Mixer started")

	// Setup graceful shutdown
	cleanup := func(ctx context.Context) {
		_ = server.Shutdown(ctx)

		if err := heartbeat.Stop(ctx); err != nil {
			logger.Error("Error cleaning up heartbeat", log.Error(err))
		}
		if err := roomWatcher.Stop(); err != nil {
			logger.Error("Error cleaning up room watcher", log.Error(err))
		}
		if err := ffmpegManager.Stop(); err != nil {
			logger.Error("Error cleaning up FFmpeg manager", log.Error(err))
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
