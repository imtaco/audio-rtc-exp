package watcher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	etcdmocks "github.com/imtaco/audio-rtc-exp/internal/etcd/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/mixers/mocks"
)

type RoomWatcherTestSuite struct {
	suite.Suite
	ctrl           *gomock.Controller
	mockEtcdClient *etcdmocks.MockClient
	mockPortMgr    *mocks.MockPortManager
	mockFFmpegMgr  *mocks.MockFFmpegManager
	watcher        *RoomWatcher
	ctx            context.Context
}

func TestRoomWatcherSuite(t *testing.T) {
	suite.Run(t, new(RoomWatcherTestSuite))
}

func (s *RoomWatcherTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockEtcdClient = etcdmocks.NewMockClient(s.ctrl)
	s.mockPortMgr = mocks.NewMockPortManager(s.ctrl)
	s.mockFFmpegMgr = mocks.NewMockFFmpegManager(s.ctrl)
	s.ctx = context.Background()

	s.watcher = &RoomWatcher{
		id:            "mixer-1",
		mixerIP:       "192.168.1.100",
		portManager:   s.mockPortMgr,
		ffmpegManager: s.mockFFmpegMgr,
		prefixRooms:   "/rooms/",
		etcdClient:    s.mockEtcdClient,
		logger:        log.NewNop(),
		tracer:        otel.Tracer("test"),
	}
}

func (s *RoomWatcherTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RoomWatcherTestSuite) TestUpdateMixer() {
	s.Run("update mixer with port", func() {
		roomID := "room1"
		port := 5004

		expectedKey := "/rooms/room1/mixer"
		expectedData := etcdstate.Mixer{
			ID:   "mixer-1",
			IP:   "192.168.1.100",
			Port: port,
		}
		expectedJSON, _ := json.Marshal(expectedData)

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), expectedKey, string(expectedJSON)).
			Return(nil, nil)

		err := s.watcher.updateMixer(s.ctx, roomID, &port)

		s.Require().NoError(err)
	})

	s.Run("delete mixer data when port is nil", func() {
		roomID := "room1"
		expectedKey := "/rooms/room1/mixer"

		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), expectedKey).
			Return(nil, nil)

		err := s.watcher.updateMixer(s.ctx, roomID, nil)

		s.Require().NoError(err)
	})

	s.Run("put fails", func() {
		roomID := "room1"
		port := 5004

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("etcd error"))

		err := s.watcher.updateMixer(s.ctx, roomID, &port)

		s.Require().Error(err)
	})

	s.Run("delete fails", func() {
		roomID := "room1"

		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("etcd error"))

		err := s.watcher.updateMixer(s.ctx, roomID, nil)

		s.Require().Error(err)
	})
}

func (s *RoomWatcherTestSuite) TestStartRoomFFmpeg() {
	s.Run("start ffmpeg successfully", func() {
		roomID := "room1"
		port := 5004
		livemeta := &etcdstate.LiveMeta{
			Status:    constants.RoomStatusOnAir,
			MixerID:   "mixer-1",
			CreatedAt: time.Now(),
			Nonce:     "abc123",
		}

		s.mockPortMgr.EXPECT().
			GetFreeRTPPort().
			Return(port, nil)

		s.mockFFmpegMgr.EXPECT().
			StartFFmpeg(roomID, port, livemeta.CreatedAt, livemeta.Nonce).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.startRoomFFmpeg(s.ctx, roomID, livemeta)

		s.Require().NoError(err)

		activeRooms := s.watcher.GetActiveRooms()
		s.Contains(activeRooms, roomID)
		s.Equal(port, activeRooms[roomID].Port)
		s.Equal("running", activeRooms[roomID].Status)
	})

	s.Run("port allocation fails", func() {
		roomID := "room1"
		livemeta := &etcdstate.LiveMeta{
			Status:  constants.RoomStatusOnAir,
			MixerID: "mixer-1",
		}

		s.mockPortMgr.EXPECT().
			GetFreeRTPPort().
			Return(0, errors.New("no free ports"))

		err := s.watcher.startRoomFFmpeg(s.ctx, roomID, livemeta)

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to allocate RTP port")
	})

	s.Run("ffmpeg start fails", func() {
		roomID := "room1"
		port := 5004
		livemeta := &etcdstate.LiveMeta{
			Status:    constants.RoomStatusOnAir,
			MixerID:   "mixer-1",
			CreatedAt: time.Now(),
			Nonce:     "abc123",
		}

		s.mockPortMgr.EXPECT().
			GetFreeRTPPort().
			Return(port, nil)

		s.mockFFmpegMgr.EXPECT().
			StartFFmpeg(roomID, port, livemeta.CreatedAt, livemeta.Nonce).
			Return(errors.New("ffmpeg error"))

		err := s.watcher.startRoomFFmpeg(s.ctx, roomID, livemeta)

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to start FFmpeg")
	})

	s.Run("update mixer fails", func() {
		roomID := "room1"
		port := 5004
		livemeta := &etcdstate.LiveMeta{
			Status:    constants.RoomStatusOnAir,
			MixerID:   "mixer-1",
			CreatedAt: time.Now(),
			Nonce:     "abc123",
		}

		s.mockPortMgr.EXPECT().
			GetFreeRTPPort().
			Return(port, nil)

		s.mockFFmpegMgr.EXPECT().
			StartFFmpeg(roomID, port, livemeta.CreatedAt, livemeta.Nonce).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("etcd error"))

		err := s.watcher.startRoomFFmpeg(s.ctx, roomID, livemeta)

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to update mixer data")
	})
}

func (s *RoomWatcherTestSuite) TestStopRoomFFmpeg() {
	s.Run("stop ffmpeg successfully as state runner", func() {
		roomID := "room1"
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: 5004, Status: "running"})

		s.mockFFmpegMgr.EXPECT().
			StopFFmpeg(roomID).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.stopRoomFFmpeg(s.ctx, roomID, true)

		s.Require().NoError(err)

		activeRooms := s.watcher.GetActiveRooms()
		s.NotContains(activeRooms, roomID)
	})

	s.Run("stop ffmpeg successfully not state runner", func() {
		roomID := "room1"
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: 5004, Status: "running"})

		s.mockFFmpegMgr.EXPECT().
			StopFFmpeg(roomID).
			Return(nil)

		err := s.watcher.stopRoomFFmpeg(s.ctx, roomID, false)

		s.Require().NoError(err)

		activeRooms := s.watcher.GetActiveRooms()
		s.NotContains(activeRooms, roomID)
	})

	s.Run("ffmpeg stop fails", func() {
		roomID := "room1"
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: 5004, Status: "running"})

		s.mockFFmpegMgr.EXPECT().
			StopFFmpeg(roomID).
			Return(errors.New("stop error"))

		err := s.watcher.stopRoomFFmpeg(s.ctx, roomID, true)

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to stop FFmpeg")
	})

	s.Run("remove mixer data fails", func() {
		roomID := "room1"
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: 5004, Status: "running"})

		s.mockFFmpegMgr.EXPECT().
			StopFFmpeg(roomID).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("etcd error"))

		err := s.watcher.stopRoomFFmpeg(s.ctx, roomID, true)

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to remove mixer data")
	})
}

func (s *RoomWatcherTestSuite) TestSyncMixerData() {
	s.Run("sync mixer data successfully", func() {
		roomID := "room1"
		port := 5004
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: port, Status: "running"})

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.syncMixerData(s.ctx, roomID)

		s.Require().NoError(err)
	})

	s.Run("room not found in active rooms", func() {
		roomID := "nonexistent"

		err := s.watcher.syncMixerData(s.ctx, roomID)

		s.Require().Error(err)
		s.Contains(err.Error(), "room not found in active rooms")
	})
}

func (s *RoomWatcherTestSuite) TestProcessChange() {
	s.Run("start room when should be running but not running", func() {
		roomID := "room1"
		port := 5004
		state := &etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:    constants.RoomStatusOnAir,
				MixerID:   "mixer-1",
				CreatedAt: time.Now(),
				Nonce:     "abc123",
			},
		}

		s.mockPortMgr.EXPECT().
			GetFreeRTPPort().
			Return(port, nil)

		s.mockFFmpegMgr.EXPECT().
			StartFFmpeg(roomID, port, state.LiveMeta.CreatedAt, state.LiveMeta.Nonce).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.processChange(s.ctx, roomID, state)

		s.Require().NoError(err)
		s.Contains(s.watcher.GetActiveRooms(), roomID)
	})

	s.Run("sync mixer data when running but not state runner", func() {
		roomID := "room1"
		port := 5004
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: port, Status: "running"})

		state := &etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-1",
			},
			Mixer: &etcdstate.Mixer{
				ID:   "mixer-2",
				Port: 5006,
			},
		}

		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.processChange(s.ctx, roomID, state)

		s.Require().NoError(err)
	})

	s.Run("stop room when should not be running but is running", func() {
		roomID := "room1"
		s.watcher.activeRooms.Store(roomID, &ActiveRoom{Port: 5004, Status: "running"})

		state := &etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusRemoving,
				MixerID: "mixer-1",
			},
			Mixer: &etcdstate.Mixer{
				ID: "mixer-1",
			},
		}

		s.mockFFmpegMgr.EXPECT().
			StopFFmpeg(roomID).
			Return(nil)

		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := s.watcher.processChange(s.ctx, roomID, state)

		s.Require().NoError(err)
		s.NotContains(s.watcher.GetActiveRooms(), roomID)
	})

	s.Run("do nothing when already in correct state", func() {
		roomID := "room1"
		state := &etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusRemoving,
				MixerID: "mixer-1",
			},
		}

		err := s.watcher.processChange(s.ctx, roomID, state)

		s.Require().NoError(err)
	})

	s.Run("handle nil state", func() {
		roomID := "room1"

		err := s.watcher.processChange(s.ctx, roomID, nil)

		s.Require().NoError(err)
	})

	s.Run("different mixer ID should not start", func() {
		roomID := "room1"
		state := &etcdstate.RoomState{
			LiveMeta: &etcdstate.LiveMeta{
				Status:  constants.RoomStatusOnAir,
				MixerID: "mixer-2",
			},
		}

		err := s.watcher.processChange(s.ctx, roomID, state)

		s.Require().NoError(err)
		s.NotContains(s.watcher.GetActiveRooms(), roomID)
	})
}

func (s *RoomWatcherTestSuite) TestGetActiveRooms() {
	s.Run("get empty active rooms", func() {
		rooms := s.watcher.GetActiveRooms()
		s.Empty(rooms)
	})

	s.Run("get active rooms with data", func() {
		s.watcher.activeRooms.Store("room1", &ActiveRoom{Port: 5004, Status: "running"})
		s.watcher.activeRooms.Store("room2", &ActiveRoom{Port: 5006, Status: "running"})

		rooms := s.watcher.GetActiveRooms()

		s.Len(rooms, 2)
		s.Contains(rooms, "room1")
		s.Contains(rooms, "room2")
		s.Equal(5004, rooms["room1"].Port)
		s.Equal(5006, rooms["room2"].Port)
	})
}
