package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
	"github.com/imtaco/audio-rtc-exp/mixers"
)

// RoomWatcher watches etcd for room changes and manages FFmpeg lifecycle
type RoomWatcher struct {
	etcdwatcher.RoomWatcher
	etcdClient    etcd.Client
	id            string
	mixerIP       string
	portManager   mixers.PortManager
	ffmpegManager mixers.FFmpegManager
	prefixRooms   string
	activeRooms   sync.Map
	logger        *log.Logger
	tracer        trace.Tracer
}

// ActiveRoom represents an active room being processed
type ActiveRoom struct {
	Port   int    `json:"port"`
	Status string `json:"status"`
}

// NewRoomWatcher creates a new RoomWatcher
func NewRoomWatcher(
	etcdClient *clientv3.Client,
	id, mixerIP string,
	portManager mixers.PortManager,
	ffmpegManager mixers.FFmpegManager,
	prefixRooms, _ string,
	logger *log.Logger,
) *RoomWatcher {
	w := &RoomWatcher{
		id:            id,
		mixerIP:       mixerIP,
		portManager:   portManager,
		ffmpegManager: ffmpegManager,
		prefixRooms:   prefixRooms,
		etcdClient:    etcdClient,
		logger:        logger,
		tracer:        otel.Tracer("mixer.watcher"),
	}

	w.RoomWatcher = etcdwatcher.NewRoomWatcher(
		etcdClient,
		prefixRooms,
		[]string{constants.RoomKeyLiveMeta, constants.RoomKeyMixer},
		w.processChange,
		logger,
	)
	return w
}

// updateMixer writes mixer data to etcd
func (w *RoomWatcher) updateMixer(ctx context.Context, roomID string, port *int) error {
	key := fmt.Sprintf("%s%s/mixer", w.prefixRooms, roomID)

	if port != nil {
		data := etcdstate.Mixer{
			ID:   w.id,
			IP:   w.mixerIP,
			Port: *port,
		}
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal mixer data: %w", err)
		}
		_, err = w.etcdClient.Put(ctx, key, string(jsonData))
		return err
	}

	// Delete mixer data
	_, err := w.etcdClient.Delete(ctx, key)
	return err
}

// startRoomFFmpeg starts FFmpeg for a room
func (w *RoomWatcher) startRoomFFmpeg(ctx context.Context, roomID string, livemeta *etcdstate.LiveMeta) error {
	ctx, span := w.tracer.Start(ctx, "watcher.startRoomFFmpeg",
		trace.WithAttributes(
			attribute.String("room.id", roomID),
			attribute.String("mixer.id", w.id),
		))
	defer span.End()

	// Use only low-cardinality labels for metrics (mixer.id)
	// High-cardinality labels like room.id cause cardinality explosion
	attrs := metric.WithAttributes(
		attribute.String("mixer.id", w.id),
	)

	port, err := w.portManager.GetFreeRTPPort()
	if err != nil {
		span.RecordError(err)
		roomsFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to allocate RTP port: %w", err)
	}
	span.SetAttributes(attribute.Int("rtp.port", port))

	w.logger.Info("Allocated RTP port for room",
		log.String("roomId", roomID),
		log.Int("port", port))

	if err := w.ffmpegManager.StartFFmpeg(roomID, port, livemeta.CreatedAt, livemeta.Nonce); err != nil {
		span.RecordError(err)
		roomsFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	if err := w.updateMixer(ctx, roomID, &port); err != nil {
		span.RecordError(err)
		roomsFailed.Add(ctx, 1, attrs)
		return fmt.Errorf("failed to update mixer data: %w", err)
	}

	w.activeRooms.Store(roomID, &ActiveRoom{Port: port, Status: "running"})

	// Record metrics
	roomsStarted.Add(ctx, 1, attrs)
	activeRoomsGauge.Add(ctx, 1, attrs)

	return nil
}

// stopRoomFFmpeg stops FFmpeg for a room
func (w *RoomWatcher) stopRoomFFmpeg(ctx context.Context, roomID string, isStateRunner bool) error {
	ctx, span := w.tracer.Start(ctx, "watcher.stopRoomFFmpeg",
		trace.WithAttributes(
			attribute.String("room.id", roomID),
			attribute.String("mixer.id", w.id),
			attribute.Bool("is_state_runner", isStateRunner),
		))
	defer span.End()

	// Use only low-cardinality labels for metrics (mixer.id)
	attrs := metric.WithAttributes(
		attribute.String("mixer.id", w.id),
	)

	w.logger.Info("Stopping FFmpeg", log.String("roomId", roomID))

	if err := w.ffmpegManager.StopFFmpeg(roomID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to stop FFmpeg: %w", err)
	}

	w.activeRooms.Delete(roomID)

	// Record metrics
	roomsStopped.Add(ctx, 1, attrs)
	activeRoomsGauge.Add(ctx, -1, attrs)

	// If someone else took ownership, don't modify data
	if isStateRunner {
		w.logger.Info("Remove port for room", log.String("roomId", roomID))
		if err := w.updateMixer(ctx, roomID, nil); err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to remove mixer data: %w", err)
		}
	} else {
		w.logger.Info("Someone else holds state, not removing port for room",
			log.String("roomId", roomID))
	}

	return nil
}

// syncMixerData syncs mixer data to etcd
func (w *RoomWatcher) syncMixerData(ctx context.Context, roomID string) error {
	w.logger.Info("Syncing mixer data to etcd", log.String("roomId", roomID))

	val, ok := w.activeRooms.Load(roomID)
	if !ok {
		return fmt.Errorf("room not found in active rooms")
	}

	activeRoom := val.(*ActiveRoom)
	return w.updateMixer(ctx, roomID, &activeRoom.Port)
}

// processChange processes a room state change
func (w *RoomWatcher) processChange(ctx context.Context, roomID string, state *etcdstate.RoomState) error {
	ctx, span := w.tracer.Start(ctx, "watcher.processChange",
		trace.WithAttributes(
			attribute.String("room.id", roomID),
			attribute.String("mixer.id", w.id),
		))
	defer span.End()

	// Record that we processed a change (use only low-cardinality labels)
	attrs := metric.WithAttributes(
		attribute.String("mixer.id", w.id),
	)
	roomsProcessed.Add(ctx, 1, attrs)

	w.logger.Info("Processing change for room", log.String("roomId", roomID))

	if state == nil {
		state = &etcdstate.RoomState{}
	}

	livemeta := state.LiveMeta
	mixer := state.Mixer

	shouldBeRunning := livemeta != nil &&
		livemeta.Status == constants.RoomStatusOnAir &&
		livemeta.MixerID == w.id

	_, isRunning := w.activeRooms.Load(roomID)
	isStateRunner := mixer != nil && mixer.ID == w.id

	span.SetAttributes(
		attribute.Bool("should_be_running", shouldBeRunning),
		attribute.Bool("is_running", isRunning),
		attribute.Bool("is_state_runner", isStateRunner),
	)

	switch {
	case shouldBeRunning && !isRunning:
		// Must have livemeta here
		return w.startRoomFFmpeg(ctx, roomID, livemeta)
	case shouldBeRunning && isRunning && !isStateRunner:
		return w.syncMixerData(ctx, roomID)
	case !shouldBeRunning && isRunning:
		return w.stopRoomFFmpeg(ctx, roomID, isStateRunner)
	default:
		return nil
	}
}

// GetActiveRooms returns the active rooms map
func (w *RoomWatcher) GetActiveRooms() map[string]*ActiveRoom {
	result := make(map[string]*ActiveRoom)
	w.activeRooms.Range(func(key, value any) bool {
		result[key.(string)] = value.(*ActiveRoom)
		return true
	})
	return result
}
