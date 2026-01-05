package ffmpeg

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/mixers"
)

// ffmpegMgrImpl manages FFmpeg processes for multiple rooms
type ffmpegMgrImpl struct {
	hlsDir           string
	encGen           *EncryptionGenerator
	sdpGen           *SDPGenerator
	retryDelay       time.Duration
	forceKillTimeout time.Duration
	processes        sync.Map // map[string]*ProcessInfo
	logger           *log.Logger
	tracer           trace.Tracer
}

// NewFFmpegManager creates a new FFmpegManager
func NewFFmpegManager(
	hlsDir string,
	encGen *EncryptionGenerator,
	sdpGen *SDPGenerator,
	retryDelay, forceKillTimeout time.Duration,
	logger *log.Logger,
) mixers.FFmpegManager {
	if retryDelay == 0 {
		retryDelay = 1 * time.Second
	}
	if forceKillTimeout == 0 {
		forceKillTimeout = 5 * time.Second
	}

	hlsDir = filepath.Clean(hlsDir)

	activeProcesses.Add(context.Background(), 3)

	return &ffmpegMgrImpl{
		hlsDir:           hlsDir,
		encGen:           encGen,
		sdpGen:           sdpGen,
		retryDelay:       retryDelay,
		forceKillTimeout: forceKillTimeout,
		logger:           logger,
		tracer:           otel.Tracer("mixer.ffmpeg"),
	}
}

// StartFFmpeg starts an FFmpeg process for a room
func (fm *ffmpegMgrImpl) StartFFmpeg(roomID string, rtpPort int, createdAt time.Time, nonce string) error {
	startTime := time.Now()
	ctx, span := fm.tracer.Start(context.Background(), "ffmpeg.StartFFmpeg",
		trace.WithAttributes(
			attribute.String("room.id", roomID),
			attribute.Int("rtp.port", rtpPort),
		))
	defer span.End()

	// Use no labels for metrics to avoid cardinality explosion
	// room.id is high-cardinality and would create too many time series
	attrs := metric.WithAttributes()

	if _, exists := fm.processes.Load(roomID); exists {
		err := fmt.Errorf("FFmpeg already running for room %s", roomID)
		span.RecordError(err)
		processesFailed.Add(ctx, 1, attrs)
		return err
	}

	// Calculate initial sequence number based on createdAt
	initSeq := fm.calculateSeqNo(roomID, createdAt)
	span.SetAttributes(attribute.Int("hls.init_seq", initSeq))

	sdpPath, err := fm.sdpGen.Generate(roomID, rtpPort)
	if err != nil {
		span.RecordError(err)
		processesFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to generate SDP: %w", err)
	}

	// Create HLS output directory
	hlsDir := filepath.Join(fm.hlsDir, roomID)
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		span.RecordError(err)
		processesFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to create HLS directory: %w", err)
	}

	// Create AES encryption key info file
	keyInfoPath, err := fm.encGen.Generate(roomID, nonce, hlsDir)
	if err != nil {
		span.RecordError(err)
		processesFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	fm.logger.Info("Starting FFmpeg with AES encryption",
		log.String("roomId", roomID),
		log.Int("rtpPort", rtpPort),
		log.Int("initSeq", initSeq))

	processInfo := NewProcessInfo(
		roomID,
		rtpPort,
		sdpPath,
		hlsDir,
		keyInfoPath,
		initSeq,
		fm.logger,
	)

	fm.processes.Store(roomID, processInfo)

	// Start first attempt
	processInfo.Start()

	// Record metrics
	processesStarted.Add(ctx, 1, attrs)
	activeProcesses.Add(ctx, 1, attrs)
	startDuration.Record(ctx, time.Since(startTime).Milliseconds(), attrs)

	return nil
}

// StopFFmpeg stops the FFmpeg process for a room
func (fm *ffmpegMgrImpl) StopFFmpeg(roomID string) error {
	ctx, span := fm.tracer.Start(context.Background(), "ffmpeg.StopFFmpeg",
		trace.WithAttributes(
			attribute.String("room.id", roomID),
		))
	defer span.End()

	// Use no labels for metrics to avoid cardinality explosion
	attrs := metric.WithAttributes()

	val, exists := fm.processes.Load(roomID)
	if !exists {
		err := fmt.Errorf("no FFmpeg process found for room %s", roomID)
		span.RecordError(err)
		return err
	}

	processInfo := val.(*ProcessInfo)
	processInfo.Stop()

	// Clean up resources
	fm.sdpGen.Delete(roomID)
	fm.encGen.Delete(roomID)

	// Record metrics
	processesStopped.Add(ctx, 1, attrs)
	activeProcesses.Add(ctx, -1, attrs)

	// Remove from processes map after cleanup
	time.AfterFunc(fm.forceKillTimeout+1*time.Second, func() {
		fm.processes.Delete(roomID)
	})

	return nil
}

// Stop stops all FFmpeg processes
func (fm *ffmpegMgrImpl) Stop() error {
	fm.logger.Info("Stopping all FFmpeg processes")

	var roomIDs []string
	fm.processes.Range(func(key, value interface{}) bool {
		roomIDs = append(roomIDs, key.(string))
		return true
	})

	for _, roomID := range roomIDs {
		if err := fm.StopFFmpeg(roomID); err != nil {
			fm.logger.Error("Error stopping FFmpeg process",
				log.String("roomId", roomID),
				log.Error(err))
		}
	}

	fm.logger.Info("Stopped FFmpeg processes", log.Int("count", len(roomIDs)))
	return nil
}

func (fm *ffmpegMgrImpl) calculateSeqNo(roomID string, createdAt time.Time) int {
	if createdAt.IsZero() {
		return 0
	}

	elapsedSeconds := time.Since(createdAt).Seconds()
	// 2s per segment * 1.1 safety margin
	initSeq := int(math.Ceil((elapsedSeconds / 2) * 1.1))
	fm.logger.Info("Calculated initial sequence",
		log.String("roomId", roomID),
		log.Time("createdAt", createdAt),
		log.Int("initSeq", initSeq))

	return initSeq
}
