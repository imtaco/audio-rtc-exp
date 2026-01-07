package watcher

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	etcdstate "github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
)

const (
	maxRoomCreationAttempts = 5
)

// ActiveRoom tracks the Janus room state
type ActiveRoom struct {
	JanusRoomID int64
	StreamID    int64
	FwIP        string
	FwPort      int
}

// RoomWatcher watches mixer data and manages Janus RTP forwarders
type RoomWatcher struct {
	etcdwatcher.RoomWatcher
	etcdClient    etcd.KV
	janusAdmin    janus.Admin
	janusID       string
	janusAdvHost  string
	prefixRooms   string
	prefixJanuses string
	canaryRoomID  int64
	activeRooms   sync.Map
	logger        *log.Logger
}

// NewRoomWatcher creates a new RoomWatcher
func NewRoomWatcher(
	etcdClient etcd.Client,
	janusID string,
	janusAdvHost string,
	janusAdmin janus.Admin,
	prefixRooms string,
	prefixJanuses string,
	canaryRoomID int64,
	logger *log.Logger,
) *RoomWatcher {
	w := &RoomWatcher{
		janusID:       janusID,
		janusAdvHost:  janusAdvHost,
		janusAdmin:    janusAdmin,
		prefixRooms:   prefixRooms,
		prefixJanuses: prefixJanuses,
		canaryRoomID:  canaryRoomID,
		logger:        logger,
		etcdClient:    etcdClient,
	}

	w.RoomWatcher = etcdwatcher.NewRoomWatcher(
		etcdClient,
		prefixRooms,
		[]string{constants.RoomKeyMeta, constants.RoomKeyLiveMeta, constants.RoomKeyMixer},
		w.processChange,
		logger,
	)
	return w
}

// updateJanusStatus writes janus status data to etcd for a room
func (w *RoomWatcher) updateJanusStatus(ctx context.Context, roomID string, janusRoomID int64, status string) error {
	key := fmt.Sprintf("%s%s/janus", w.prefixRooms, roomID)

	if status != "" {
		data := etcdstate.Janus{
			JanusID:     w.janusID,
			Status:      status,
			Timestamp:   time.Now(),
			JanusRoomID: janusRoomID,
		}
		jsonData, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = w.etcdClient.Put(ctx, key, string(jsonData))
		if err != nil {
			return err
		}
		w.logger.Info("Updated status for room", log.String("roomId", roomID), log.String("status", status))
	} else {
		_, err := w.etcdClient.Delete(ctx, key)
		if err != nil {
			return err
		}
		w.logger.Info("Cleared status for room", log.String("roomId", roomID))
	}

	return nil
}

// createRoom creates a Janus room with random ID to avoid collisions
func (w *RoomWatcher) createRoom(ctx context.Context, roomID, pin string) (int64, error) {
	for attempt := 1; attempt <= maxRoomCreationAttempts; attempt++ {
		// Generate 6-digit room ID using crypto/rand
		randNum, err := cryptoRandInt(900000)
		if err != nil {
			return 0, fmt.Errorf("failed to generate random room ID: %w", err)
		}
		janusRoomID := 100000 + randNum

		err = w.janusAdmin.CreateRoom(ctx, janusRoomID, roomID, pin)
		if err == nil {
			return janusRoomID, nil
		}
		if !errors.Is(err, janus.ErrAlreadyExisted) {
			return 0, err
		}
		w.logger.Info("Room ID already exists, retrying...", log.Int64("janusRoomId", janusRoomID))
		continue
	}

	return 0, fmt.Errorf("failed to create room after %d attempts", maxRoomCreationAttempts)
}

// destroyRoom destroys a Janus room
func (w *RoomWatcher) destroyRoom(ctx context.Context, janusRoomID int64) error {
	err := w.janusAdmin.DestroyRoom(ctx, janusRoomID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, janus.ErrNotFound) {
		return err
	}
	w.logger.Info("Room not found in Janus, assuming already destroyed", log.Int64("janusRoomId", janusRoomID))
	return nil
}

// createRtpForwarder creates an RTP forwarder for a room
func (w *RoomWatcher) createRtpForwarder(ctx context.Context, roomID string, activeRoom *ActiveRoom, fwip string, fwport int) error {
	if activeRoom.JanusRoomID == 0 {
		w.logger.Info("Room meta not found or no janusRoomId, skipping forwarder setup", log.String("roomId", roomID))
		return nil
	}

	w.logger.Info("Creating RTP forwarder for room",
		log.String("roomId", roomID),
		log.Int64("janusRoomId", activeRoom.JanusRoomID),
		log.String("fwip", fwip),
		log.Int("fwport", fwport))

	streamID, err := w.janusAdmin.CreateRTPForwarder(ctx, activeRoom.JanusRoomID, fwip, fwport)
	if err != nil {
		return err
	}

	activeRoom.StreamID = streamID
	activeRoom.FwIP = fwip
	activeRoom.FwPort = fwport

	return nil
}

// stopRtpForwarder stops an RTP forwarder for a room
func (w *RoomWatcher) stopRtpForwarder(ctx context.Context, roomID string, activeRoom *ActiveRoom) error {
	w.logger.Info("Stopping RTP forwarder for room", log.String("roomId", roomID))

	err := w.janusAdmin.StopRTPForwarder(ctx, activeRoom.JanusRoomID, activeRoom.StreamID)
	switch {
	case err == nil:
		w.logger.Info("Stopped RTP forwarder for room", log.String("roomId", roomID))
	case errors.Is(err, janus.ErrNotFound):
		w.logger.Info("RTP forwarder not found in Janus, assuming already stopped", log.String("roomId", roomID))
	default:
		w.logger.Error("Failed to stop RTP forwarder for room", log.String("roomId", roomID), log.Error(err))
		return err
	}

	activeRoom.StreamID = 0
	activeRoom.FwIP = ""
	activeRoom.FwPort = 0

	return nil
}

//nolint:gocyclo
func (w *RoomWatcher) processChange(_ context.Context, roomID string, state *etcdstate.RoomState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mixer := state.GetMixer()
	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()

	// Load active room
	var activeRoom *ActiveRoom
	if val, ok := w.activeRooms.Load(roomID); ok {
		activeRoom = val.(*ActiveRoom)
	}

	hasJanusRoom := activeRoom != nil
	hasRTPForwarder := activeRoom != nil && activeRoom.StreamID != 0
	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == w.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	// Should have forwarder if: assigned to us, mixer data exists with port
	shouldHaveForwarder := isAssignedToUs && mixer != nil && mixer.Port != 0

	// Handle room creation/removal
	switch {
	case isAssignedToUs && !hasJanusRoom:
		// Ensure Janus room exists
		janusRoomID, err := w.createRoom(ctx, roomID, meta.Pin)
		if err != nil {
			return err
		}
		if err := w.updateJanusStatus(ctx, roomID, janusRoomID, "room_created"); err != nil {
			return err
		}
		activeRoom = &ActiveRoom{JanusRoomID: janusRoomID}
		w.activeRooms.Store(roomID, activeRoom)

	case !isAssignedToUs && hasJanusRoom:
		// No longer assigned to us, remove from active rooms
		if err := w.destroyRoom(ctx, activeRoom.JanusRoomID); err != nil {
			return err
		}
		if err := w.updateJanusStatus(ctx, roomID, 0, ""); err != nil {
			return err
		}
		w.activeRooms.Delete(roomID)
		return nil
	case !isAssignedToUs && !hasJanusRoom:
		// not our business
		return nil
	}

	if activeRoom == nil {
		w.logger.Debug("No active room, skipping processing for RTP forwarder",
			log.String("roomId", roomID),
			log.Any("state", state))
		return nil
	}

	// Handle forwarder creation/removal/update
	switch {
	case shouldHaveForwarder && !hasRTPForwarder:
		// Create RTP forwarder
		if err := w.createRtpForwarder(ctx, roomID, activeRoom, mixer.IP, mixer.Port); err != nil {
			return err
		}
		if err := w.updateJanusStatus(ctx, roomID, activeRoom.JanusRoomID, "forwarding"); err != nil {
			return err
		}

	case !shouldHaveForwarder && hasRTPForwarder:
		if err := w.stopRtpForwarder(ctx, roomID, activeRoom); err != nil {
			return err
		}
		if err := w.updateJanusStatus(ctx, roomID, activeRoom.JanusRoomID, "not_forwarding"); err != nil {
			return err
		}

	case shouldHaveForwarder && hasRTPForwarder:
		// Check if mixer endpoint changed
		if activeRoom.FwIP != mixer.IP || activeRoom.FwPort != mixer.Port {
			w.logger.Info("Mixer endpoint changed, recreating forwarder", log.String("roomId", roomID))

			if err := w.stopRtpForwarder(ctx, roomID, activeRoom); err != nil {
				return err
			}
			if err := w.createRtpForwarder(ctx, roomID, activeRoom, mixer.IP, mixer.Port); err != nil {
				return err
			}
			if err := w.updateJanusStatus(ctx, roomID, activeRoom.JanusRoomID, "forwarding"); err != nil {
				return err
			}
		}
	}

	return nil
}

// JanusRestartDetected handles Janus restart event
func (w *RoomWatcher) JanusRestartDetected() error {
	w.logger.Warn("Janus restart detected, clearing active rooms")
	// Clear all active rooms since Janus was restarted
	// trigger rebuild to recreate rooms and forwarders
	w.Restart()
	return nil
}

// rebuildStart is called before rebuild
func (w *RoomWatcher) RebuildStart(ctx context.Context) error {
	w.logger.Info("Starting rebuild of RoomWatcher")
	w.activeRooms = sync.Map{}

	w.logger.Info("Building janusRoomId -> streamId mapping from Janus...")

	rooms, err := w.janusAdmin.ListRooms(context.Background())
	if err != nil {
		return err
	}

	w.logger.Info("Found rooms in Janus", log.Int("count", len(rooms)))

	for _, room := range rooms {
		roomID := room.Description // use description as our roomId
		janusRoomID := room.Room

		if janusRoomID == w.canaryRoomID {
			continue // skip canary room
		}

		// List forwarders for this room
		forwarders, err := w.janusAdmin.ListRTPForwarders(ctx, janusRoomID)
		if err != nil {
			w.logger.Error("Failed to list forwarders", log.Int64("janusRoomId", janusRoomID), log.Error(err))
			continue
		}

		activeRoom := &ActiveRoom{
			JanusRoomID: janusRoomID,
		}

		// Pick the first forwarder if exists
		if len(forwarders) > 0 {
			fw := forwarders[0]
			activeRoom.StreamID = fw.StreamID
			activeRoom.FwIP = fw.Host
			activeRoom.FwPort = fw.Port
		}

		w.activeRooms.Store(roomID, activeRoom)
		w.logger.Info("Mapped janusRoomId to info", log.String("roomId", roomID), log.Int64("janusRoomId", janusRoomID))
	}

	w.logger.Info("Built mapping for rooms", log.Int("count", len(rooms)))
	return nil
}

// RebuildEnd is called after rebuild
func (w *RoomWatcher) RebuildEnd(_ context.Context) error {
	w.logger.Info("Completed rebuild of RoomWatcher")
	return nil
}

// rebuildState is called for each room during rebuild
func (w *RoomWatcher) RebuildState(_ context.Context, roomID string, stateData *etcdstate.RoomState) error {
	val, ok := w.activeRooms.Load(roomID)
	if !ok {
		return nil // no active room, nothing to do
	}

	// Validate that the room exists in Janus
	activeRoom := val.(*ActiveRoom)
	mixerData := stateData.Mixer

	// Match forwarder with cached mixer data
	if mixerData != nil && activeRoom.FwIP == mixerData.IP && activeRoom.FwPort == mixerData.Port {
		w.logger.Debug("Room matched during rebuild", log.String("roomId", roomID))
		return nil
	}
	if activeRoom.StreamID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := w.stopRtpForwarder(ctx, roomID, activeRoom); err != nil {
			w.logger.Error("Failed to stop stale RTP forwarder", log.String("roomId", roomID), log.Error(err))
		}
	}

	return nil
}

// cryptoRandInt generates a cryptographically secure random integer in range [0, maxVal)
func cryptoRandInt(maxVal int64) (int64, error) {
	nBig, err := rand.Int(rand.Reader, big.NewInt(maxVal))
	if err != nil {
		return 0, err
	}
	return nBig.Int64(), nil
}
