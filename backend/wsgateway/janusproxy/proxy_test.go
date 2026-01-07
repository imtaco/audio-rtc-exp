package janusproxy

import (
	"context"
	"errors"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	mockwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd/mocks"
)

type ProxySuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	janusWatcher *mockwatcher.MockHealthyModuleWatcher
	roomWatcher  *mockwatcher.MockRoomWatcher
	proxy        *janusProxyImpl
	logger       *log.Logger
}

func TestProxySuite(t *testing.T) {
	suite.Run(t, new(ProxySuite))
}

func (s *ProxySuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.janusWatcher = mockwatcher.NewMockHealthyModuleWatcher(s.ctrl)
	s.roomWatcher = mockwatcher.NewMockRoomWatcher(s.ctrl)
	s.logger = log.NewNop()

	cache, err := lru.New[string, janus.API](10)
	s.Require().NoError(err)

	s.proxy = &janusProxyImpl{
		janusPort:    "8088",
		instCache:    cache,
		janusWatcher: s.janusWatcher,
		roomWatcher:  s.roomWatcher,
		logger:       s.logger,
	}
}

func (s *ProxySuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ProxySuite) TestNewProxy_Success() {
	p, err := NewProxy(nil, "room/", "janus/", 10, "8088", log.NewTest(s.T()))
	s.Require().NoError(err)
	s.NotNil(p)
}

func (s *ProxySuite) TestNewProxy_Error() {
	_, err := NewProxy(nil, "", "", 0, "", log.NewTest(s.T()))
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to create LRU cache")
}

func (s *ProxySuite) TestOpen() {
	ctx := context.Background()

	s.janusWatcher.EXPECT().Start(ctx).Return(nil)
	s.roomWatcher.EXPECT().Start(ctx).Return(nil)

	err := s.proxy.Open(ctx)
	s.Require().NoError(err)
}

func (s *ProxySuite) TestOpen_JanusWatcherError() {
	ctx := context.Background()
	expectedErr := errors.New("janus watcher error")

	s.janusWatcher.EXPECT().Start(ctx).Return(expectedErr)

	err := s.proxy.Open(ctx)
	s.Require().Error(err)
	s.Equal(expectedErr, err)
}

func (s *ProxySuite) TestOpen_RoomWatcherError() {
	ctx := context.Background()
	expectedErr := errors.New("room watcher error")

	s.janusWatcher.EXPECT().Start(ctx).Return(nil)
	s.roomWatcher.EXPECT().Start(ctx).Return(expectedErr)

	err := s.proxy.Open(ctx)
	s.Require().Error(err)
	s.Equal(expectedErr, err)
}

func (s *ProxySuite) TestGetRoomLiveMeta() {
	roomID := "room1"
	liveMeta := &etcdstate.LiveMeta{
		Status:  constants.RoomStatusOnAir,
		JanusID: "janus1",
		MixerID: "mixer1",
	}

	state := &etcdstate.RoomState{
		LiveMeta: liveMeta,
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(state, true)

	result := s.proxy.GetRoomLiveMeta(roomID)
	s.Equal(liveMeta, result)
}

func (s *ProxySuite) TestGetRoomMeta() {
	roomID := "room1"
	meta := &etcdstate.Meta{
		Pin: "123456", MaxAnchors: 5,
		CreatedAt: time.Now(),
	}

	state := &etcdstate.RoomState{
		Meta: meta,
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(state, true)

	result := s.proxy.GetRoomMeta(roomID)
	s.Equal(meta, result)
}

func (s *ProxySuite) TestGetJanusID() {
	roomID := "room1"
	janusID := "janus1"

	state := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: janusID,
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(state, true)

	result := s.proxy.getJanusID(roomID)
	s.Equal(janusID, result)
}

func (s *ProxySuite) TestGetJanusID_NotFound() {
	roomID := "non-existent-room"
	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(nil, false)

	result := s.proxy.getJanusID(roomID)
	s.Equal("", result)
}

func (s *ProxySuite) TestGetJanusRoomID() {
	roomID := "room1"
	janusRoomID := int64(12345)

	state := &etcdstate.RoomState{
		Janus: &etcdstate.Janus{
			JanusRoomID: janusRoomID,
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(state, true)

	result := s.proxy.GetJanusRoomID(roomID)
	s.Equal(janusRoomID, result)
}

func (s *ProxySuite) TestGetJanusRoomID_NotFound() {
	roomID := "non-existent-room"
	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(nil, false)

	result := s.proxy.GetJanusRoomID(roomID)
	s.Equal(int64(0), result)
}

func (s *ProxySuite) TestGetJanusAPI_Success() {
	roomID := "room1"
	janusID := "janus1"
	host := "192.168.1.10"

	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: janusID,
		},
	}

	moduleState := &etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Host:   host,
			Status: "healthy",
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(roomState, true)
	s.janusWatcher.EXPECT().Get(janusID).Return(*moduleState, true)

	api := s.proxy.GetJanusAPI(roomID)
	s.NotNil(api)

	cached, ok := s.proxy.instCache.Get(janusID)
	s.True(ok)
	s.Equal(api, cached)
}

func (s *ProxySuite) TestGetJanusAPI_EmptyJanusID() {
	roomID := "room1"

	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "",
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(roomState, true)

	api := s.proxy.GetJanusAPI(roomID)
	s.Nil(api)
}

func (s *ProxySuite) TestGetJanusAPI_NoHost() {
	roomID := "room1"
	janusID := "janus1"

	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: janusID,
		},
	}

	moduleState := &etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Host:   "",
			Status: "healthy",
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(roomState, true)
	s.janusWatcher.EXPECT().Get(janusID).Return(*moduleState, true)

	api := s.proxy.GetJanusAPI(roomID)
	s.Nil(api)
}

func (s *ProxySuite) TestGetJanusAPI_CacheHit() {
	roomID := "room1"
	janusID := "janus1"
	host := "192.168.1.10"

	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: janusID,
		},
	}

	moduleState := &etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Host:   host,
			Status: "healthy",
		},
	}

	s.roomWatcher.EXPECT().GetCachedState(roomID).Return(roomState, true).Times(2)
	s.janusWatcher.EXPECT().Get(janusID).Return(*moduleState, true).Times(2)

	api1 := s.proxy.GetJanusAPI(roomID)
	s.NotNil(api1)

	api2 := s.proxy.GetJanusAPI(roomID)
	s.NotNil(api2)
	s.Equal(api1, api2)
}

func (s *ProxySuite) TestClose() {
	s.janusWatcher.EXPECT().Stop().Return(nil)
	s.roomWatcher.EXPECT().Stop().Return(nil)

	err := s.proxy.Close()
	s.Require().NoError(err)
}

func (s *ProxySuite) TestClose_JanusWatcherError() {
	expectedErr := errors.New("janus watcher stop error")

	s.janusWatcher.EXPECT().Stop().Return(expectedErr)
	s.roomWatcher.EXPECT().Stop().Return(nil)

	err := s.proxy.Close()
	s.Require().NoError(err)
}

func (s *ProxySuite) TestClose_RoomWatcherError() {
	expectedErr := errors.New("room watcher stop error")

	s.janusWatcher.EXPECT().Stop().Return(nil)
	s.roomWatcher.EXPECT().Stop().Return(expectedErr)

	err := s.proxy.Close()
	s.Require().NoError(err)
}
