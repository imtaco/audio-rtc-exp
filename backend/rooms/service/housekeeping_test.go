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
	"github.com/imtaco/audio-rtc-exp/rooms/utils"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type HouseKeeperTestSuite struct {
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

func TestHouseKeeperSuite(t *testing.T) {
	suite.Run(t, new(HouseKeeperTestSuite))
}

func (s *HouseKeeperTestSuite) SetupTest() {
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
		logger:       logger,
	}
}

func (s *HouseKeeperTestSuite) TearDownTest() {
	s.cancel()
	s.ctrl.Finish()
}

// CheckStaleRooms Tests

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_Success() {
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(map[string]*etcdstate.Meta{}, nil)

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_GetAllRoomsError() {
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(nil, errors.New("get all rooms error"))

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().Error(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_DeletesMalformedRoom() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	// Room has no meta - should be deleted
	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: nil, // Malformed - no meta
		}, true)

	s.mockRoomStore.EXPECT().
		DeleteRoom(gomock.Any(), "room-1").
		Return(true, nil)

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_DeletesInactiveRoomAfterTimeout() {
	now := time.Now()
	oldTime := now.Add(-(startTimeout + time.Minute)) // Exceeds start timeout

	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: oldTime,
			},
			LiveMeta: nil, // No livemeta - room failed to start
		}, true)

	s.mockRoomStore.EXPECT().
		DeleteRoom(gomock.Any(), "room-1").
		Return(true, nil)

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_KeepsRecentInactiveRoom() {
	now := time.Now()
	recentTime := now.Add(-1 * time.Minute) // Within start timeout

	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: recentTime,
			},
			LiveMeta: nil,
		}, true)

	// Should not delete - no mock expectation for DeleteRoom

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_DeletesRoomExceedingMaxAge() {
	now := time.Now()
	oldTime := now.Add(-(roomMaxAge + time.Minute)) // Exceeds max age

	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: oldTime,
			},
			LiveMeta: &etcdstate.LiveMeta{
				Status: constants.RoomStatusOnAir,
			},
		}, true)

	s.mockRoomStore.EXPECT().
		DeleteRoom(gomock.Any(), "room-1").
		Return(true, nil)

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_DeletesRoomAfterGracePeriod() {
	now := time.Now()
	discardTime := now.Add(-(inactiveGracefulPeriod + time.Minute)) // Exceeds grace period

	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: now.Add(-10 * time.Minute),
			},
			LiveMeta: &etcdstate.LiveMeta{
				Status:    constants.RoomStatusRemoving,
				DiscardAt: utils.Ptr(discardTime),
			},
		}, true)

	s.mockRoomStore.EXPECT().
		DeleteRoom(gomock.Any(), "room-1").
		Return(true, nil)

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckStaleRooms_RoomNotFoundInWatcher() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(nil, false) // Room not found in watcher

	// Should not delete - just skip

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err)
}

// CheckRoomModules Tests

func (s *HouseKeeperTestSuite) TestCheckRoomModules_Success() {
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(map[string]*etcdstate.Meta{}, nil)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_GetAllRoomsError() {
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(nil, errors.New("get all rooms error"))

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().Error(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_RoomNotFoundInWatcher() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(nil, false)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_NoLiveMeta() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: nil,
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_RoomNotOnAir() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status: constants.RoomStatusRemoving,
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_MixerUnhealthy() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true)

	// Mixer is unhealthy
	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: "unhealthy",
			},
		}, true)

	// Janus is healthy
	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_JanusUnhealthy() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true)

	// Mixer is healthy
	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	// Janus is unhealthy
	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: "unhealthy",
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_BothModulesHealthy() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true)

	// Mixer is healthy
	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	// Janus is healthy
	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

func (s *HouseKeeperTestSuite) TestCheckRoomModules_MixerNotFound() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": &etcdstate.Meta{},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true)

	// Mixer not found
	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{}, false)

	// Janus is healthy
	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err)
}

// Test housekeepOnce
func (s *HouseKeeperTestSuite) TestHousekeepOnce_Success() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": {},
	}

	// Expect two calls to GetAllRooms (one for checkStaleRooms, one for checkRoomModules)
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil).
		Times(2)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: time.Now(),
			},
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true).
		Times(2)

	// For checkRoomModules
	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	s.rm.housekeepOnce()
}

func (s *HouseKeeperTestSuite) TestHousekeepOnce_CheckStaleRoomsError() {
	// GetAllRooms fails for checkStaleRooms
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(nil, errors.New("etcd error"))

	// Even if checkStaleRooms fails, checkRoomModules should still run
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(map[string]*etcdstate.Meta{}, nil)

	s.rm.housekeepOnce()
}

func (s *HouseKeeperTestSuite) TestHousekeepOnce_CheckRoomModulesError() {
	// checkStaleRooms succeeds
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(map[string]*etcdstate.Meta{}, nil)

	// checkRoomModules fails
	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(nil, errors.New("etcd error"))

	s.rm.housekeepOnce()
}

// Test error handling in checkStaleRooms
func (s *HouseKeeperTestSuite) TestCheckStaleRooms_ErrorLogging() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": {},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: time.Now().Add(-15 * time.Minute), // Exceed timeout
			},
		}, true)

	// DeleteRoom will be called and will fail
	s.mockRoomStore.EXPECT().
		DeleteRoom(gomock.Any(), "room-1").
		Return(false, errors.New("delete failed"))

	err := s.rm.checkStaleRooms(s.ctx)
	s.Require().NoError(err) // checkStaleRooms doesn't propagate individual room errors
}

// Test error handling in checkRoomModules
func (s *HouseKeeperTestSuite) TestCheckRoomModules_ErrorLogging() {
	rooms := map[string]*etcdstate.Meta{
		"room-1": {},
		"room-2": {},
	}

	s.mockRoomStore.EXPECT().
		GetAllRooms(gomock.Any()).
		Return(rooms, nil)

	// First room - causes error (room state not found)
	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-1").
		Return(nil, false)

	// Second room - succeeds
	s.mockRoomWatcher.EXPECT().
		GetCachedState("room-2").
		Return(&etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				CreatedAt: time.Now(),
			},
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
				JanusID: "janus-1",
			},
		}, true)

	s.mockMixerWatcher.EXPECT().
		Get("mixer-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	s.mockJanusWatcher.EXPECT().
		Get("janus-1").
		Return(etcdstate.ModuleState{
			Heartbeat: &etcdstate.HeartbeatData{
				Status: constants.ModuleStatusHealthy,
			},
			Mark: &etcdstate.MarkData{
				Label: constants.MarkLabelReady,
			},
		}, true)

	err := s.rm.checkRoomModules(s.ctx)
	s.Require().NoError(err) // checkRoomModules doesn't propagate individual room errors
}
