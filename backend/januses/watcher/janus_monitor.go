package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// JanusHealthMonitor monitors Janus health by maintaining a canary room
// If the room disappears or its creation timestamp changes, it indicates a Janus restart
type JanusHealthMonitor struct {
	janusAdmin     janus.Admin
	canaryRoomID   int64
	interval       time.Duration
	restartHandler func(reason string)
	cancel         context.CancelFunc
	stopped        chan struct{}
	logger         *log.Logger
}

// NewJanusHealthMonitor creates a new JanusHealthMonitor
func NewJanusHealthMonitor(
	janusAdmin janus.Admin,
	canaryRoomID int64,
	interval time.Duration,
	logger *log.Logger,
) *JanusHealthMonitor {
	return &JanusHealthMonitor{
		janusAdmin:   janusAdmin,
		canaryRoomID: canaryRoomID,
		interval:     interval,
		logger:       logger,
		stopped:      make(chan struct{}),
	}
}

// SetRestartHandler sets the callback function to be called when Janus restart is detected
func (m *JanusHealthMonitor) SetRestartHandler(handler func(reason string)) {
	m.restartHandler = handler
}

// Start sets up the canary room and starts monitoring
func (m *JanusHealthMonitor) Start(ctx context.Context) error {
	m.logger.Info("Initializing Janus health monitor...")

	ctx, m.cancel = context.WithCancel(ctx)
	// Try to get existing canary room
	existed, err := m.janusAdmin.GetRoom(ctx, m.canaryRoomID)
	if err != nil {
		m.logger.Error("Failed to check canary room", log.Error(err))
		return err
	}

	if existed {
		m.logger.Info("Found existing canary room")
	} else {
		// Create new canary room
		if err := m.createCanaryRoom(ctx); err != nil {
			return err
		}
	}

	// Start monitoring
	go m.monitorLoop(ctx)

	m.logger.Info("Janus health monitor initialized")
	return nil
}

// createCanaryRoom creates a canary room with a timestamp as description
func (m *JanusHealthMonitor) createCanaryRoom(ctx context.Context) error {
	description := fmt.Sprintf("canary %d", time.Now().UnixMilli())

	err := m.janusAdmin.CreateRoom(ctx, m.canaryRoomID, description, "111111")
	if err != nil {
		m.logger.Error("Failed to create canary room", log.Error(err))
		return err
	}

	m.logger.Info("Created canary room", log.Int64("canaryRoomId", m.canaryRoomID))
	return nil
}

// monitorLoop periodically checks the canary room
func (m *JanusHealthMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer close(m.stopped)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkCanaryRoom()
		}
	}
}

// checkCanaryRoom checks if the canary room still exists
func (m *JanusHealthMonitor) checkCanaryRoom() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	existed, err := m.janusAdmin.GetRoom(ctx, m.canaryRoomID)
	if err != nil {
		m.logger.Error("Failed to check canary room", log.Error(err))
		return
	}

	if !existed {
		// Canary room disappeared - Janus likely restarted
		m.logger.Warn("Canary room disappeared - Janus restart detected!")
		m.handleJanusRestart("canary_room_disappeared")
		return
	}

	m.logger.Debug("Canary room check passed")
}

// handleJanusRestart handles Janus restart event
func (m *JanusHealthMonitor) handleJanusRestart(reason string) {
	m.logger.Info("Handling Janus restart", log.String("reason", reason))

	// Recreate canary room
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.createCanaryRoom(ctx); err != nil {
		m.logger.Error("Failed to recreate canary room", log.Error(err))
	}

	// Call restart handler if set
	if m.restartHandler != nil {
		m.restartHandler(reason)
	}
}

// Stop stops the health monitor
func (m *JanusHealthMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	<-m.stopped
	m.logger.Info("Stopped canary room monitoring")
}
