package service

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
	"github.com/imtaco/audio-rtc-exp/rooms"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type resourceMgrImpl struct {
	roomStore    rooms.RoomStore
	roomWatcher  RoomWatcherWithStats
	janusWatcher etcdwatcher.HealthyModuleWatcher
	mixerWatcher etcdwatcher.HealthyModuleWatcher
	stopCh       chan struct{}
	logger       *log.Logger
}

const (
	housekeepInterval = 30 * time.Second
)

func NewResourceManager(
	etcdClient *clientv3.Client,
	roomStore rooms.RoomStore,
	prefixRoom string,
	prefixJanus string,
	prefixMixer string,
	logger *log.Logger,
) rooms.ResourceManager {
	// Use custom room watcher with statistics
	roomWatcher := NewRoomWatcherWithStats(
		etcdClient,
		prefixRoom,
		logger.Module("Room"),
	)
	janusWatcher := etcdwatcher.NewHealthyModuleWatcher(etcdClient, prefixJanus, logger.Module("Janus"))
	mixerWatcher := etcdwatcher.NewHealthyModuleWatcher(etcdClient, prefixMixer, logger.Module("Mixer"))

	return &resourceMgrImpl{
		roomStore:    roomStore,
		roomWatcher:  roomWatcher,
		janusWatcher: janusWatcher,
		mixerWatcher: mixerWatcher,
		stopCh:       make(chan struct{}),
		logger:       logger,
	}
}

func (rm *resourceMgrImpl) Start(ctx context.Context) error {
	if err := rm.roomWatcher.Start(ctx); err != nil {
		watcherErrors.Add(ctx, 1)
		return fmt.Errorf("failed to start room watcher: %w", err)
	}
	watcherStarted.Add(ctx, 1)

	if err := rm.janusWatcher.Start(ctx); err != nil {
		watcherErrors.Add(ctx, 1)
		return fmt.Errorf("failed to start janus watcher: %w", err)
	}
	watcherStarted.Add(ctx, 1)

	if err := rm.mixerWatcher.Start(ctx); err != nil {
		watcherErrors.Add(ctx, 1)
		return fmt.Errorf("failed to start mixer watcher: %w", err)
	}
	watcherStarted.Add(ctx, 1)

	// Start housekeeping in background
	go rm.housekeepLoop()

	return nil
}

func (rm *resourceMgrImpl) housekeepLoop() {
	ticker := time.NewTicker(housekeepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopCh:
			rm.logger.Info("Stopping resourceMgrImpl housekeeping loop")
			return
		case <-ticker.C:
			rm.housekeepOnce()
		}
	}
}

func (rm *resourceMgrImpl) housekeepOnce() {
	rm.logger.Info("Starting housekeeping cycle")

	ctx := context.Background()
	startTime := time.Now()

	housekeepingRuns.Add(ctx, 1)

	if err := rm.checkStaleRooms(ctx); err != nil {
		rm.logger.Error("Error during housekeeping rooms", log.Error(err))
	}
	if err := rm.checkRoomModules(ctx); err != nil {
		rm.logger.Error("Error during checking room modules", log.Error(err))
	}

	duration := time.Since(startTime).Seconds()
	housekeepingDuration.Record(ctx, duration)
}

func (rm *resourceMgrImpl) Stop() error {
	ctx := context.Background()
	close(rm.stopCh)

	if err := rm.roomWatcher.Stop(); err != nil {
		rm.logger.Error("Error stopping room watcher", log.Error(err))
		watcherErrors.Add(ctx, 1)
	}
	watcherStopped.Add(ctx, 1)

	if err := rm.janusWatcher.Stop(); err != nil {
		rm.logger.Error("Error stopping janus watcher", log.Error(err))
		watcherErrors.Add(ctx, 1)
	}
	watcherStopped.Add(ctx, 1)

	if err := rm.mixerWatcher.Stop(); err != nil {
		rm.logger.Error("Error stopping mixer watcher", log.Error(err))
		watcherErrors.Add(ctx, 1)
	}
	watcherStopped.Add(ctx, 1)

	return nil
}

func (rm *resourceMgrImpl) PickJanus() (string, error) {
	ctx := context.Background()
	rm.logger.Debug("Picking Janus for room")

	janusPickAttempts.Add(ctx, 1)
	janusID := rm.randomPickModule(rm.janusWatcher, "janus")

	if janusID == "" {
		janusPickFailed.Add(ctx, 1)
	} else {
		janusPickSuccess.Add(ctx, 1)
	}

	return janusID, nil
}

func (rm *resourceMgrImpl) PickMixer() (string, error) {
	ctx := context.Background()
	rm.logger.Debug("Picking mixer for room")

	mixerPickAttempts.Add(ctx, 1)
	mixerID := rm.randomPickModule(rm.mixerWatcher, "mixer")

	if mixerID == "" {
		mixerPickFailed.Add(ctx, 1)
	} else {
		mixerPickSuccess.Add(ctx, 1)
	}

	return mixerID, nil
}

func (rm *resourceMgrImpl) randomPickModule(watcher etcdwatcher.HealthyModuleWatcher, moduleType string) string {
	var pickableKeys []string

	// Note that GetStreamCount might be delayed due to eventual consistency
	// It's hard to precisely track real-time usage
	// Iterate through healths map to find pickable modules
	healthyIDs := watcher.GetAllHealthy()
	for _, id := range healthyIDs {
		data, ok := watcher.Get(id)
		if !ok || !data.IsPickable() {
			continue
		}

		// Check capacity limit from heartbeat
		capacity := data.GetHeartbeat().GetCapacity()
		if capacity <= 0 {
			continue
		}

		// Get current streams count from room watcher
		var currentStreams int
		switch moduleType {
		case "janus":
			currentStreams = rm.roomWatcher.GetJanusStreamCount(id)
		case "mixer":
			currentStreams = rm.roomWatcher.GetMixerStreamCount(id)
		}

		rm.logger.Debug("Module at capacity",
			log.String("moduleType", moduleType),
			log.String("moduleID", id),
			log.Int("streams", currentStreams),
			log.Int("capacity", capacity),
		)

		if currentStreams < capacity {
			pickableKeys = append(pickableKeys, id)
			continue
		}
	}

	if len(pickableKeys) == 0 {
		return ""
	}

	// Randomly pick one
	return pickableKeys[rand.IntN(len(pickableKeys))] // #nosec G404 -- weak random is acceptable for load balancing resource selection, no security impact
}
