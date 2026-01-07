package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	watchermocks "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd/mocks"
	roomsmocks "github.com/imtaco/audio-rtc-exp/rooms/mocks"
	servicemocks "github.com/imtaco/audio-rtc-exp/rooms/service/mocks"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ResourceManagerTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockRoomStore    *roomsmocks.MockRoomStore
	mockRoomWatcher  *servicemocks.MockRoomWatcherWithStats
	mockJanusWatcher *watchermocks.MockHealthyModuleWatcher
	mockMixerWatcher *watchermocks.MockHealthyModuleWatcher
	rm               *resourceMgrImpl
	ctx              context.Context
	cancel           context.CancelFunc
}

func TestResourceManagerSuite(t *testing.T) {
	suite.Run(t, new(ResourceManagerTestSuite))
}

func (s *ResourceManagerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockRoomStore = roomsmocks.NewMockRoomStore(s.ctrl)
	s.mockRoomWatcher = servicemocks.NewMockRoomWatcherWithStats(s.ctrl)
	s.mockJanusWatcher = watchermocks.NewMockHealthyModuleWatcher(s.ctrl)
	s.mockMixerWatcher = watchermocks.NewMockHealthyModuleWatcher(s.ctrl)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	logger := log.NewTest(s.T())

	s.rm = &resourceMgrImpl{
		roomStore:    s.mockRoomStore,
		roomWatcher:  s.mockRoomWatcher,
		janusWatcher: s.mockJanusWatcher,
		mixerWatcher: s.mockMixerWatcher,
		stopCh:       make(chan struct{}),
		logger:       logger,
	}
}

func (s *ResourceManagerTestSuite) TearDownTest() {
	s.cancel()
	s.ctrl.Finish()
}

// Start Tests

func (s *ResourceManagerTestSuite) TestStart_Success() {
	s.mockRoomWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	s.mockJanusWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	s.mockMixerWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	err := s.rm.Start(s.ctx)
	s.Require().NoError(err)
}

func (s *ResourceManagerTestSuite) TestStart_RoomWatcherError() {
	s.mockRoomWatcher.EXPECT().
		Start(gomock.Any()).
		Return(errors.New("room watcher init error"))

	err := s.rm.Start(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to start room watcher")
}

func (s *ResourceManagerTestSuite) TestStart_JanusWatcherError() {
	s.mockRoomWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	s.mockJanusWatcher.EXPECT().
		Start(gomock.Any()).
		Return(errors.New("janus watcher init error"))

	err := s.rm.Start(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to start janus watcher")
}

func (s *ResourceManagerTestSuite) TestStart_MixerWatcherError() {
	s.mockRoomWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	s.mockJanusWatcher.EXPECT().
		Start(gomock.Any()).
		Return(nil)

	s.mockMixerWatcher.EXPECT().
		Start(gomock.Any()).
		Return(errors.New("mixer watcher init error"))

	err := s.rm.Start(s.ctx)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to start mixer watcher")
}

// Close Tests

func (s *ResourceManagerTestSuite) TestStop_Success() {
	s.mockRoomWatcher.EXPECT().
		Stop().
		Return(nil)

	s.mockJanusWatcher.EXPECT().
		Stop().
		Return(nil)

	s.mockMixerWatcher.EXPECT().
		Stop().
		Return(nil)

	err := s.rm.Stop()
	s.Require().NoError(err)

	// Verify stopCh was closed
	select {
	case <-s.rm.stopCh:
		// Channel closed as expected
	case <-time.After(100 * time.Millisecond):
		s.Fail("stopCh should be closed")
	}
}

func (s *ResourceManagerTestSuite) TestStop_WatcherStopErrors() {
	s.mockRoomWatcher.EXPECT().
		Stop().
		Return(errors.New("room watcher stop error"))

	s.mockJanusWatcher.EXPECT().
		Stop().
		Return(errors.New("janus watcher stop error"))

	s.mockMixerWatcher.EXPECT().
		Stop().
		Return(errors.New("mixer watcher stop error"))
	// Close should not return error, just log them
	err := s.rm.Stop()
	s.Require().NoError(err)
}

// PickJanus Tests

func (s *ResourceManagerTestSuite) TestPickJanus_Success() {
	pickableModule := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 10,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"janus-1", "janus-2"})

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(pickableModule, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-2").
		Return(pickableModule, true)

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-1").
		Return(0)

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-2").
		Return(0)

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.NotEmpty(janusID)
	s.Contains([]string{"janus-1", "janus-2"}, janusID)
}

func (s *ResourceManagerTestSuite) TestPickJanus_NoHealthyModules() {
	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{})

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.Empty(janusID)
}

func (s *ResourceManagerTestSuite) TestPickJanus_NoPickableModules() {
	unpickableModule := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status: "unhealthy", // Not healthy, so not pickable
		},
	}

	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"janus-1"})

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(unpickableModule, true)

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.Empty(janusID)
}

// PickMixer Tests

func (s *ResourceManagerTestSuite) TestPickMixer_Success() {
	pickableModule := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 10,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockMixerWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"mixer-1", "mixer-2"})

	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(pickableModule, true)

	s.mockMixerWatcher.EXPECT().
		Get("mixer-2").
		Return(pickableModule, true)

	s.mockRoomWatcher.EXPECT().
		GetMixerStreamCount("mixer-1").
		Return(0)

	s.mockRoomWatcher.EXPECT().
		GetMixerStreamCount("mixer-2").
		Return(0)

	mixerID, err := s.rm.PickMixer()
	s.Require().NoError(err)
	s.NotEmpty(mixerID)
	s.Contains([]string{"mixer-1", "mixer-2"}, mixerID)
}

func (s *ResourceManagerTestSuite) TestPickMixer_NoHealthyModules() {
	s.mockMixerWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{})

	mixerID, err := s.rm.PickMixer()
	s.Require().NoError(err)
	s.Empty(mixerID)
}

func (s *ResourceManagerTestSuite) TestPickMixer_NoPickableModules() {
	unpickableModule := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status: "unhealthy", // Not healthy, so not pickable
		},
	}

	s.mockMixerWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"mixer-1"})

	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(unpickableModule, true)

	mixerID, err := s.rm.PickMixer()
	s.Require().NoError(err)
	s.Empty(mixerID)
}

// randomPickModule Tests

func (s *ResourceManagerTestSuite) TestRandomPickModule_MultipleCalls() {
	pickableModule := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 10,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	// Call PickJanus multiple times to verify randomness (or at least that it works)
	for i := 0; i < 5; i++ {
		s.mockJanusWatcher.EXPECT().
			GetAllHealthy().
			Return([]string{"janus-1", "janus-2", "janus-3"})

		s.mockJanusWatcher.EXPECT().
			Get("janus-1").
			Return(pickableModule, true)

		s.mockJanusWatcher.EXPECT().
			Get("janus-2").
			Return(pickableModule, true)

		s.mockJanusWatcher.EXPECT().
			Get("janus-3").
			Return(pickableModule, true)

		s.mockRoomWatcher.EXPECT().
			GetJanusStreamCount("janus-1").
			Return(0)

		s.mockRoomWatcher.EXPECT().
			GetJanusStreamCount("janus-2").
			Return(0)

		s.mockRoomWatcher.EXPECT().
			GetJanusStreamCount("janus-3").
			Return(0)

		janusID, err := s.rm.PickJanus()
		s.Require().NoError(err)
		s.NotEmpty(janusID)
	}
}

// Capacity Tests

func (s *ResourceManagerTestSuite) TestPickJanus_WithCapacityLimit() {
	// janus-1 has capacity 5 and currently serving 3 streams (under capacity)
	// janus-2 has capacity 5 and currently serving 5 streams (at capacity)
	// janus-3 has capacity 5 and currently serving 6 streams (over capacity, should not happen but test it)
	moduleWithCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 5,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"janus-1", "janus-2", "janus-3"})

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(moduleWithCapacity, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-2").
		Return(moduleWithCapacity, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-3").
		Return(moduleWithCapacity, true)

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-1").
		Return(3) // Under capacity

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-2").
		Return(5) // At capacity

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-3").
		Return(6) // Over capacity

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.Equal("janus-1", janusID) // Only janus-1 should be picked
}

func (s *ResourceManagerTestSuite) TestPickJanus_AllAtCapacity() {
	moduleAtCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 3,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"janus-1", "janus-2"})

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(moduleAtCapacity, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-2").
		Return(moduleAtCapacity, true)

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-1").
		Return(3) // At capacity

	s.mockRoomWatcher.EXPECT().
		GetJanusStreamCount("janus-2").
		Return(4) // Over capacity

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.Empty(janusID) // No module available
}

func (s *ResourceManagerTestSuite) TestPickJanus_NoCapacitySet() {
	// When capacity is 0 or not set, module should be skipped
	moduleNoCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 0, // No capacity set
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockJanusWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"janus-1"})

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(moduleNoCapacity, true)

	janusID, err := s.rm.PickJanus()
	s.Require().NoError(err)
	s.Empty(janusID)
}

func (s *ResourceManagerTestSuite) TestPickMixer_WithCapacityLimit() {
	// mixer-1 has capacity 10 and currently serving 5 streams
	// mixer-2 has capacity 10 and currently serving 10 streams (at capacity)
	moduleWithCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 10,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockMixerWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"mixer-1", "mixer-2"})

	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(moduleWithCapacity, true)

	s.mockMixerWatcher.EXPECT().
		Get("mixer-2").
		Return(moduleWithCapacity, true)

	s.mockRoomWatcher.EXPECT().
		GetMixerStreamCount("mixer-1").
		Return(5) // Under capacity

	s.mockRoomWatcher.EXPECT().
		GetMixerStreamCount("mixer-2").
		Return(10) // At capacity

	mixerID, err := s.rm.PickMixer()
	s.Require().NoError(err)
	s.Equal("mixer-1", mixerID) // Only mixer-1 should be picked
}

func (s *ResourceManagerTestSuite) TestPickMixer_MixedCapacityAndNoCapacity() {
	// mixer-1 has capacity set and under limit
	// mixer-2 has no capacity set (should be skipped)
	mixerWithCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 5,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	mixerNoCapacity := etcdstate.ModuleState{
		Heartbeat: &etcdstate.HeartbeatData{
			Status:   constants.ModuleStatusHealthy,
			Capacity: 0,
		},
		Mark: &etcdstate.MarkData{
			Label: constants.MarkLabelReady,
		},
	}

	s.mockMixerWatcher.EXPECT().
		GetAllHealthy().
		Return([]string{"mixer-1", "mixer-2"})

	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(mixerWithCapacity, true)

	s.mockMixerWatcher.EXPECT().
		Get("mixer-2").
		Return(mixerNoCapacity, true)

	s.mockRoomWatcher.EXPECT().
		GetMixerStreamCount("mixer-1").
		Return(2) // Under capacity

	mixerID, err := s.rm.PickMixer()
	s.Require().NoError(err)
	s.Equal("mixer-1", mixerID) // Only mixer-1 should be picked
}
