package janusproxy

import (
	"context"
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/singleflight"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
	"github.com/imtaco/audio-rtc-exp/wsgateway"
)

type janusProxyImpl struct {
	janusPort    string
	janusWatcher etcdwatcher.HealthyModuleWatcher
	roomWatcher  etcdwatcher.RoomWatcher
	instCache    *lru.Cache[string, janus.API]
	sfJanus      singleflight.Group
	logger       *log.Logger
}

func NewProxy(
	etcdClient *clientv3.Client,
	prefixRoom string,
	prefixJanus string,
	janusInstCacheSize int,
	janusPort string,
	logger *log.Logger,
) (wsgateway.JanusProxy, error) {
	instCache, err := lru.New[string, janus.API](janusInstCacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	janusWatcher := etcdwatcher.NewHealthyModuleWatcher(etcdClient, prefixJanus, logger.Module("JanusWatcher"))
	roomWatcher := etcdwatcher.NewRoomWatcher(
		etcdClient,
		prefixRoom,
		[]string{constants.RoomKeyMeta, constants.RoomKeyLiveMeta, constants.RoomKeyJanus},
		nil,
		logger.Module("RoomWatcher"),
	)

	return &janusProxyImpl{
		janusPort:    janusPort,
		instCache:    instCache,
		janusWatcher: janusWatcher,
		roomWatcher:  roomWatcher,
		logger:       logger,
	}, nil
}

func (jp *janusProxyImpl) Open(ctx context.Context) error {
	if err := jp.janusWatcher.Start(ctx); err != nil {
		return err
	}
	if err := jp.roomWatcher.Start(ctx); err != nil {
		return err
	}
	return nil
}

func (jp *janusProxyImpl) GetRoomLiveMeta(roomID string) *etcdstate.LiveMeta {
	state, _ := jp.roomWatcher.GetCachedState(roomID)
	return state.GetLiveMeta()
}

func (jp *janusProxyImpl) GetRoomMeta(roomID string) *etcdstate.Meta {
	state, _ := jp.roomWatcher.GetCachedState(roomID)
	return state.GetMeta()
}

func (jp *janusProxyImpl) getJanusID(roomID string) string {
	state, _ := jp.roomWatcher.GetCachedState(roomID)
	return state.GetLiveMeta().GetJanusID()
}

func (jp *janusProxyImpl) GetJanusRoomID(roomID string) int64 {
	state, _ := jp.roomWatcher.GetCachedState(roomID)
	return state.GetJanus().GetJanusRoomID()
}

func (jp *janusProxyImpl) GetJanusAPI(roomID string) janus.API {
	result, _, _ := jp.sfJanus.Do(roomID, func() (any, error) {
		janusID := jp.getJanusID(roomID)
		if janusID == "" {
			//nolint:nilnil
			return nil, nil
		}

		hb, _ := jp.janusWatcher.Get(janusID)
		host := hb.GetHeartbeat().GetHost()

		// unregister janus instance if host is not found or unhealthy
		if host == "" {
			jp.instCache.Remove(janusID)
			//nolint:nilnil
			return nil, nil
		}

		janusAPI, ok := jp.instCache.Get(janusID)
		if ok {
			return janusAPI, nil
		}

		url := fmt.Sprintf("http://%s:%s", host, jp.janusPort)
		janusAPI = janus.New(url, jp.logger)
		jp.instCache.Add(janusID, janusAPI)

		jp.logger.Info("Created new Janus API instance",
			log.String("janusId", janusID),
			log.String("url", url),
		)

		return janusAPI, nil
	})

	if result == nil {
		return nil
	}
	return result.(janus.API)
}

func (jp *janusProxyImpl) Close() error {
	if err := jp.janusWatcher.Stop(); err != nil {
		jp.logger.Error("Error stopping Janus watcher", log.Error(err))
	}
	if err := jp.roomWatcher.Stop(); err != nil {
		jp.logger.Error("Error stopping Room watcher", log.Error(err))
	}
	return nil
}
