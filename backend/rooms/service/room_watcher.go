package service

import (
	"context"
	"sync"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/watcher/etcd"
)

// roomWatcherWithStats extends the base RoomWatcher with module usage statistics
type roomWatcherWithStats struct {
	watcher.Watcher[etcdstate.RoomState]
	// Track module usage: moduleID -> count of rooms using it
	rwLock     sync.RWMutex
	janusUsage *moduleUsage
	mixerUsage *moduleUsage
	logger     *log.Logger
}

// NewRoomWatcherWithStats creates a new room watcher that tracks module usage statistics
func NewRoomWatcherWithStats(
	etcdClient etcd.Watcher,
	prefixRooms string,
	logger *log.Logger,
) RoomWatcherWithStats {
	w := &roomWatcherWithStats{
		logger: logger,
	}

	allowedTypes := []string{constants.RoomKeyMeta, constants.RoomKeyLiveMeta, constants.RoomKeyJanus, constants.RoomKeyMixer}

	cfg := etcdwatcher.Config[etcdstate.RoomState]{
		Client:           etcdClient,
		PrefixToWatch:    prefixRooms,
		AllowedKeyTypes:  allowedTypes,
		Logger:           logger,
		ProcessChange:    w.processChange,
		StateTransformer: w,
	}
	w.Watcher = etcdwatcher.New(cfg)

	return w
}

func (w *roomWatcherWithStats) processChange(_ context.Context, roomID string, state *etcdstate.RoomState) error {
	// Get the previous state to track changes
	// Get old module IDs
	newJanusID := state.GetLiveMeta().GetJanusID()
	newMixerID := state.GetLiveMeta().GetMixerID()

	w.rwLock.Lock()
	defer w.rwLock.Unlock()

	// Update Janus usage
	w.janusUsage.set(roomID, newJanusID)
	w.mixerUsage.set(roomID, newMixerID)

	return nil
}

func (w *roomWatcherWithStats) RebuildStart(_ context.Context) error {
	w.rwLock.Lock()

	// Clear usage maps before rebuilding
	w.janusUsage = newModuleUsage("janus", w.logger)
	w.mixerUsage = newModuleUsage("mixer", w.logger)
	return nil
}

func (w *roomWatcherWithStats) RebuildState(_ context.Context, id string, etcdData *etcdstate.RoomState) error {
	// During rebuild, count all active rooms
	liveMeta := etcdData.GetLiveMeta()
	if liveMeta == nil {
		return nil
	}
	janusID := liveMeta.GetJanusID()
	mixerID := liveMeta.GetMixerID()

	if janusID != "" {
		w.janusUsage.set(id, janusID)
	}
	if mixerID != "" {
		w.mixerUsage.set(id, mixerID)
	}
	return nil
}

func (w *roomWatcherWithStats) RebuildEnd(_ context.Context) error {
	w.rwLock.Unlock()

	return nil
}

// GetJanusStreamCount returns the number of active streams for a given Janus instance
func (w *roomWatcherWithStats) GetJanusStreamCount(janusID string) int {
	w.rwLock.RLock()
	defer w.rwLock.RUnlock()
	return w.janusUsage.count(janusID)
}

// GetMixerStreamCount returns the number of active streams for a given Mixer instance
func (w *roomWatcherWithStats) GetMixerStreamCount(mixerID string) int {
	w.rwLock.RLock()
	defer w.rwLock.RUnlock()
	return w.mixerUsage.count(mixerID)
}

func (w *roomWatcherWithStats) NewState(
	_, keyType string,
	data []byte,
	curState *etcdstate.RoomState,
) (*etcdstate.RoomState, error) {
	if len(data) > 0 && curState == nil {
		curState = &etcdstate.RoomState{}
	}

	switch keyType {
	case constants.RoomKeyMeta:
		curState.SetMeta(etcdwatcher.ParseValue[etcdstate.Meta](data))
	case constants.RoomKeyLiveMeta:
		curState.SetLiveMeta(etcdwatcher.ParseValue[etcdstate.LiveMeta](data))
	case constants.RoomKeyJanus:
		curState.SetJanus(etcdwatcher.ParseValue[etcdstate.Janus](data))
	case constants.RoomKeyMixer:
		curState.SetMixer(etcdwatcher.ParseValue[etcdstate.Mixer](data))
	}

	if curState.IsEmpty() {
		return nil, nil
	}

	return curState, nil
}
