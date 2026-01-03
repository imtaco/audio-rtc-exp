package etcd

import (
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
)

type HealthyModuleWatcher interface {
	watcher.Watcher[etcdstate.ModuleState]
	Has(id string) bool
	Get(id string) (etcdstate.ModuleState, bool)
	GetAllHealthy() []string
}

type RoomWatcher interface {
	watcher.Watcher[etcdstate.RoomState]
}
