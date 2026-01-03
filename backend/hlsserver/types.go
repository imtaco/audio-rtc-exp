package hlsserver

import (
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
)

type RoomWatcher interface {
	watcher.Watcher[etcdstate.RoomState]
	GetActiveLiveMeta(roomID string) *etcdstate.LiveMeta
}
