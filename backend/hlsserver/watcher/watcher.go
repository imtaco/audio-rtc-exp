package watcher

import (
	"net/http"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/imtaco/audio-rtc-exp/hlsserver"
	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/singleflight"
)

type roomWatcherImpl struct {
	etcdwatcher.RoomWatcher
	handlerCache *lru.Cache[string, http.Handler]
	sfMixer      singleflight.Group
}

func NewRoomWatcher(
	etcdClient *clientv3.Client,
	prefixRooms string,
	logger *log.Logger,
) hlsserver.RoomWatcher {
	// cache, _ := lru.New[string, http.Handler](123)

	return &roomWatcherImpl{
		RoomWatcher: etcdwatcher.NewRoomWatcher(
			etcdClient,
			prefixRooms,
			[]string{constants.RoomKeyLiveMeta, constants.RoomKeyMixer},
			nil, // use default processChange (do nothing)
			logger,
		),
		// handlerCache: cache,
	}
}

func (w *roomWatcherImpl) GetActiveLiveMeta(roomID string) *etcdstate.LiveMeta {
	state, ok := w.GetCachedState(roomID)
	if !ok || state == nil {
		return nil
	}

	livemeta := state.LiveMeta
	if livemeta != nil && livemeta.Status == constants.RoomStatusOnAir {
		return livemeta
	}

	return nil
}

// func (w *roomWatcherImpl) GetMixer(roomID string) http.Handler {
// 	state, ok := w.GetCachedState(roomID)
// 	if !ok || state == nil {
// 		return nil
// 	}

// 	// no longer existed
// 	mixer := state.GetMixer()
// 	if mixer != nil || mixer.GetIP() == "" {
// 		return nil
// 	}

// 	result, _, _ := w.sfMixer.Do(roomID, func() (interface{}, error) {
// 		handler, ok := w.handlerMap.Load(roomID)
// 		if ok {
// 			return handler, nil
// 		}

// 		// TODO: fix port
// 		addr := url.URL{
// 			Scheme: "http",
// 			Host:   fmt.Sprintf("%s:%d", mixer.GetIP(), mixer.GetPort()),
// 		}
// 		handler = httputil.NewSingleHostReverseProxy(&addr)
// 		w.handlerMap.Store(roomID, handler)
// 		return handler, nil
// 	})

// 	return result.(http.Handler)
// }
