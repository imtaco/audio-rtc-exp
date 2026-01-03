package ffmpeg

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/imtaco/audio-rtc-exp/mixers"

	"github.com/imtaco/audio-rtc-exp/internal/log"
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

	return &ffmpegMgrImpl{
		hlsDir:           hlsDir,
		encGen:           encGen,
		sdpGen:           sdpGen,
		retryDelay:       retryDelay,
		forceKillTimeout: forceKillTimeout,
		logger:           logger,
	}
}

// StartFFmpeg starts an FFmpeg process for a room
func (fm *ffmpegMgrImpl) StartFFmpeg(roomID string, rtpPort int, createdAt time.Time, nonce string) error {
	if _, exists := fm.processes.Load(roomID); exists {
		return fmt.Errorf("FFmpeg already running for room %s", roomID)
	}

	// Calculate initial sequence number based on createdAt
	initSeq := fm.calculateSeqNo(roomID, createdAt)

	sdpPath, err := fm.sdpGen.Generate(roomID, rtpPort)
	if err != nil {
		return fmt.Errorf("failed to generate SDP: %w", err)
	}

	// Create HLS output directory
	hlsDir := filepath.Join(fm.hlsDir, roomID)
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		return fmt.Errorf("failed to create HLS directory: %w", err)
	}

	// Create AES encryption key info file
	keyInfoPath, err := fm.encGen.Generate(roomID, nonce, hlsDir)
	if err != nil {
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
	return nil
}

// StopFFmpeg stops the FFmpeg process for a room
func (fm *ffmpegMgrImpl) StopFFmpeg(roomID string) error {
	val, exists := fm.processes.Load(roomID)
	if !exists {
		return fmt.Errorf("no FFmpeg process found for room %s", roomID)
	}

	processInfo := val.(*ProcessInfo)
	processInfo.Stop()

	// Clean up resources
	fm.sdpGen.Delete(roomID)
	fm.encGen.Delete(roomID)

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
