package etcd

import (
	"context"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/watcher"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/watcher/etcd"
)

type roomWatcherImpl struct {
	watcher.Watcher[etcdstate.RoomState]
}

func NewRoomWatcher(
	etcdClient etcd.Watcher,
	prefixRooms string,
	allowedTypes []string,
	processChange watcher.ProcessChangeFunc[etcdstate.RoomState],
	logger *log.Logger,
) RoomWatcher {
	watcher := &roomWatcherImpl{}
	if processChange == nil {
		processChange = watcher.processChange
	}

	cfg := etcdwatcher.Config[etcdstate.RoomState]{
		Client:           etcdClient,
		PrefixToWatch:    prefixRooms,
		AllowedKeyTypes:  allowedTypes,
		Logger:           logger,
		ProcessChange:    processChange,
		StateTransformer: watcher,
	}
	watcher.Watcher = etcdwatcher.New(cfg)

	return watcher
}

func (*roomWatcherImpl) processChange(_ context.Context, _ string, _ *etcdstate.RoomState) error {
	return nil
}

func (*roomWatcherImpl) RebuildStart(_ context.Context) error {
	return nil
}

func (*roomWatcherImpl) RebuildState(_ context.Context, _ string, _ *etcdstate.RoomState) error {
	return nil
}

func (*roomWatcherImpl) RebuildEnd(_ context.Context) error {
	return nil
}

func (*roomWatcherImpl) NewState(
	_ string,
	keyType string,
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
		//nolint:nilnil
		return nil, nil
	}

	return curState, nil
}
