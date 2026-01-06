package etcd

import (
	"context"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/watcher/etcd"
)

// healthModuleWatcherImpl watches etcd for module health status and maintains a list of healthy modules.
// It extends BaseEtcdWatcher to track modules that have both valid heartbeats and marks. A module is
// considered healthy only when both conditions are met, and is automatically removed from the healthy
// list if either becomes invalid or the heartbeat expires.
//
// Example usage:
//
//	watcher := etcd.NewHealthyModuleWatcher(etcdClient, "/modules/", logger)
//	if err := watcher.Initialize(ctx); err != nil {
//		logger.Fatal(err)
//	}
//	defer watcher.Cleanup()
//
//	// Check if a specific module is healthy
//	if watcher.Has("server-1") {
//		data, _ := watcher.Get("server-1")
//		fmt.Printf("Module data: %+v\n", data)
//	}
//
//	// Get all healthy module IDs
//	healthyIDs := watcher.GetAllHealthy()
//	fmt.Printf("Healthy modules: %v\n", healthyIDs)
type healthModuleWatcherImpl struct {
	watcher.Watcher[etcdstate.ModuleState]
	healths sync.Map
	logger  *log.Logger
}

// NewHealthyModuleWatcher creates a new healthModuleWatcherImpl
func NewHealthyModuleWatcher(
	etcdClient *clientv3.Client,
	prefix string,
	logger *log.Logger,
) HealthyModuleWatcher {
	w := &healthModuleWatcherImpl{
		logger: logger,
	}

	cfg := etcdwatcher.Config[etcdstate.ModuleState]{
		Client:           etcdClient,
		PrefixToWatch:    prefix,
		AllowedKeyTypes:  []string{constants.ModuleKeyHeartbeat, constants.ModuleKeyMark},
		Logger:           logger,
		ProcessChange:    w.processChange,
		StateTransformer: w,
	}
	w.Watcher = etcdwatcher.New(cfg)
	return w
}

// Has checks if a module ID exists in the healthy modules
func (w *healthModuleWatcherImpl) Has(id string) bool {
	_, ok := w.healths.Load(id)
	return ok
}

// Get retrieves the state of a healthy module by ID
func (w *healthModuleWatcherImpl) Get(id string) (etcdstate.ModuleState, bool) {
	value, ok := w.healths.Load(id)
	if !ok {
		return etcdstate.ModuleState{}, false
	}
	ptr := value.(*etcdstate.ModuleState)
	return *ptr, true
}

// GetAllHealthy returns all healthy module IDs
func (w *healthModuleWatcherImpl) GetAllHealthy() []string {
	var healthyIDs []string
	w.healths.Range(func(key, _ interface{}) bool {
		healthyIDs = append(healthyIDs, key.(string))
		return true
	})
	return healthyIDs
}

func (w *healthModuleWatcherImpl) RebuildStart(_ context.Context) error {
	w.logger.Info("Starting rebuild of healthModuleWatcherImpl")
	w.healths = sync.Map{}
	return nil
}

func (w *healthModuleWatcherImpl) RebuildEnd(_ context.Context) error {
	w.logger.Info("Starting rebuild of healthModuleWatcherImpl")
	return nil
}

// rebuild is called after initial data fetch but before processing
func (w *healthModuleWatcherImpl) RebuildState(_ context.Context, id string, state *etcdstate.ModuleState) error {
	w.logger.Debug("found during rebuild", log.String("id", id))
	if state.IsHealthy() {
		w.logger.Debug("healthy during rebuild", log.String("id", id))
		w.healths.Store(id, state)
	} else {
		w.logger.Warn("unhealthy during rebuild", log.String("id", id))
	}
	return nil
}

// processChange is called when a module state changes
func (w *healthModuleWatcherImpl) processChange(_ context.Context, id string, state *etcdstate.ModuleState) error {
	if state.IsHealthy() {
		w.logger.Debug("healthy", log.String("id", id))
		w.healths.Store(id, state)
	} else {
		w.logger.Warn("unhealthy or removed", log.String("id", id))
		w.healths.Delete(id)
	}

	return nil
}

// processChange is called when a module state changes
func (w *healthModuleWatcherImpl) NewState(
	_ string,
	keyType string,
	data []byte,
	curState *etcdstate.ModuleState,
) (*etcdstate.ModuleState, error) {

	if len(data) > 0 && curState == nil {
		curState = &etcdstate.ModuleState{}
	}

	switch keyType {
	case constants.ModuleKeyHeartbeat:
		curState.SetHeartbeat(etcdwatcher.ParseValue[etcdstate.HeartbeatData](data))

	case constants.ModuleKeyMark:
		curState.SetMark(etcdwatcher.ParseValue[etcdstate.MarkData](data))
	}

	if curState.IsEmpty() {
		return nil, nil
	}
	return curState, nil
}
