package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/rooms"
	"github.com/imtaco/audio-rtc-exp/rooms/mocks"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type RoomServiceTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockStore  *mocks.MockRoomStore
	mockResMgr *mocks.MockResourceManager
	svc        *roomSvcImpl
	ctx        context.Context
}

func TestRoomServiceSuite(t *testing.T) {
	suite.Run(t, new(RoomServiceTestSuite))
}

func (s *RoomServiceTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = mocks.NewMockRoomStore(s.ctrl)
	s.mockResMgr = mocks.NewMockResourceManager(s.ctrl)
	s.ctx = context.Background()

	s.svc = NewRoomService(
		s.mockStore,
		s.mockResMgr,
		"https://example.com/hls/",
		log.NewNop(),
	).(*roomSvcImpl)
}

func (s *RoomServiceTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RoomServiceTestSuite) TestCreateRoom() {
	s.Run("create room successfully", func() {
		roomID := "room1"
		pin := "1234"
		maxAnchors := 5 // Test with non-default value
		now := time.Now().UTC()

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(false, nil)

		s.mockStore.EXPECT().
			CreateRoom(gomock.Any(), gomock.Eq(roomID), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, data *etcdstate.Meta) (*etcdstate.Meta, error) {
				s.Equal(pin, data.Pin)
				s.Equal("room1/stream.m3u8", data.HLSPath)
				s.Equal(maxAnchors, data.MaxAnchors)
				return &etcdstate.Meta{
					Pin:        pin,
					HLSPath:    "room1/stream.m3u8",
					MaxAnchors: maxAnchors,
					CreatedAt:  now,
				}, nil
			})

		resp, err := s.svc.CreateRoom(s.ctx, roomID, pin, maxAnchors)

		s.Require().NoError(err)
		s.Equal(roomID, resp.RoomID)
		s.Equal(pin, resp.Pin)
		s.Equal("https://example.com/hls/room1/stream.m3u8", resp.HLSURL)
		s.Equal(now, resp.CreatedAt)
	})

	s.Run("room already exists", func() {
		roomID := "existing-room"
		pin := "1234"
		maxAnchors := 1 // Test with minimum value

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(true, nil)

		resp, err := s.svc.CreateRoom(s.ctx, roomID, pin, maxAnchors)

		s.Require().Error(err)
		s.Nil(resp)
		var roomExistsErr *rooms.RoomExistsError
		s.Require().ErrorAs(err, &roomExistsErr)
		s.Equal(roomID, roomExistsErr.RoomID)
	})

	s.Run("exists check fails", func() {
		roomID := "room1"
		pin := "1234"
		maxAnchors := 2 // Test with different value

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(false, errors.New("database error"))

		resp, err := s.svc.CreateRoom(s.ctx, roomID, pin, maxAnchors)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to check room existence")
	})

	s.Run("create room fails", func() {
		roomID := "room1"
		pin := "1234"
		maxAnchors := 4 // Test with different value

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(false, nil)

		s.mockStore.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("storage error"))

		resp, err := s.svc.CreateRoom(s.ctx, roomID, pin, maxAnchors)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to create room")
	})
}

func (s *RoomServiceTestSuite) TestStartLive() {
	s.Run("start live successfully", func() {
		roomID := "room1"
		mixerID := "mixer1"
		janusID := "janus1"

		s.mockResMgr.EXPECT().
			PickMixer().
			Return(mixerID, nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return(janusID, nil)

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(true, nil)

		s.mockStore.EXPECT().
			CreateLiveMeta(gomock.Any(), roomID, mixerID, janusID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _, nonce string) error {
				s.NotEmpty(nonce)
				s.Len(nonce, 20) // 10 bytes hex encoded = 20 chars
				return nil
			})

		err := s.svc.StartLive(s.ctx, roomID)

		s.Require().NoError(err)
	})

	s.Run("no available mixer", func() {
		s.mockResMgr.EXPECT().
			PickMixer().
			Return("", errors.New("no mixer available"))

		err := s.svc.StartLive(s.ctx, "room1")

		s.Require().Error(err)
		s.Contains(err.Error(), "no available mixer")
	})

	s.Run("mixer returns empty string", func() {
		s.mockResMgr.EXPECT().
			PickMixer().
			Return("", nil)

		err := s.svc.StartLive(s.ctx, "room1")

		s.Require().Error(err)
		s.Contains(err.Error(), "no available mixer")
	})

	s.Run("no available janus", func() {
		s.mockResMgr.EXPECT().
			PickMixer().
			Return("mixer1", nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return("", errors.New("no janus available"))

		err := s.svc.StartLive(s.ctx, "room1")

		s.Require().Error(err)
		s.Contains(err.Error(), "no available Janus server")
	})

	s.Run("janus returns empty string", func() {
		s.mockResMgr.EXPECT().
			PickMixer().
			Return("mixer1", nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return("", nil)

		err := s.svc.StartLive(s.ctx, "room1")

		s.Require().Error(err)
		s.Contains(err.Error(), "no available Janus server")
	})

	s.Run("room not found", func() {
		roomID := "nonexistent"

		s.mockResMgr.EXPECT().
			PickMixer().
			Return("mixer1", nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return("janus1", nil)

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(false, nil)

		err := s.svc.StartLive(s.ctx, roomID)

		s.Require().Error(err)
		var roomNotFoundErr *rooms.RoomNotFoundError
		s.Require().ErrorAs(err, &roomNotFoundErr)
		s.Equal(roomID, roomNotFoundErr.RoomID)
	})

	s.Run("exists check fails", func() {
		s.mockResMgr.EXPECT().
			PickMixer().
			Return("mixer1", nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return("janus1", nil)

		s.mockStore.EXPECT().
			Exists(gomock.Any(), "room1").
			Return(false, errors.New("database error"))

		err := s.svc.StartLive(s.ctx, "room1")

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to check room existence")
	})

	s.Run("create live meta fails", func() {
		roomID := "room1"

		s.mockResMgr.EXPECT().
			PickMixer().
			Return("mixer1", nil)

		s.mockResMgr.EXPECT().
			PickJanus().
			Return("janus1", nil)

		s.mockStore.EXPECT().
			Exists(gomock.Any(), roomID).
			Return(true, nil)

		s.mockStore.EXPECT().
			CreateLiveMeta(gomock.Any(), roomID, "mixer1", "janus1", gomock.Any()).
			Return(errors.New("meta creation failed"))

		err := s.svc.StartLive(s.ctx, roomID)

		s.Require().Error(err)
		s.Contains(err.Error(), "meta creation failed")
	})
}

func (s *RoomServiceTestSuite) TestGetRoom() {
	s.Run("get room successfully without mixer data", func() {
		roomID := "room1"
		now := time.Now().UTC()
		roomData := &etcdstate.Meta{
			Pin:       "1234",
			HLSPath:   "room1/stream.m3u8",
			CreatedAt: now,
		}

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(roomData, nil)

		s.mockStore.EXPECT().
			GetMixerData(gomock.Any(), roomID).
			Return(nil, errors.New("no mixer data"))

		resp, err := s.svc.GetRoom(s.ctx, roomID)

		s.Require().NoError(err)
		s.Equal(roomID, resp.RoomID)
		s.Equal("https://example.com/hls/room1/stream.m3u8", resp.HLSURL)
		s.Equal(now, resp.CreatedAt)
		s.Nil(resp.RTPPort)
	})

	s.Run("get room successfully with mixer data", func() {
		roomID := "room1"
		port := 5004
		now := time.Now().UTC()
		roomData := &etcdstate.Meta{
			Pin:       "1234",
			HLSPath:   "room1/stream.m3u8",
			CreatedAt: now,
		}
		mixerData := &etcdstate.Mixer{
			Port: port,
		}

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(roomData, nil)

		s.mockStore.EXPECT().
			GetMixerData(gomock.Any(), roomID).
			Return(mixerData, nil)

		resp, err := s.svc.GetRoom(s.ctx, roomID)

		s.Require().NoError(err)
		s.Equal(roomID, resp.RoomID)
		s.NotNil(resp.RTPPort)
		s.Equal(port, *resp.RTPPort)
	})

	s.Run("get room with zero port in mixer data", func() {
		roomID := "room1"
		now := time.Now().UTC()
		roomData := &etcdstate.Meta{
			HLSPath:   "room1/stream.m3u8",
			CreatedAt: now,
		}
		mixerData := &etcdstate.Mixer{
			Port: 0,
		}

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(roomData, nil)

		s.mockStore.EXPECT().
			GetMixerData(gomock.Any(), roomID).
			Return(mixerData, nil)

		resp, err := s.svc.GetRoom(s.ctx, roomID)

		s.Require().NoError(err)
		s.Nil(resp.RTPPort)
	})

	s.Run("room not found - nil returned", func() {
		roomID := "nonexistent"

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(nil, nil)

		resp, err := s.svc.GetRoom(s.ctx, roomID)

		s.Require().Error(err)
		s.Nil(resp)
		var roomNotFoundErr *rooms.RoomNotFoundError
		s.Require().ErrorAs(err, &roomNotFoundErr)
		s.Equal(roomID, roomNotFoundErr.RoomID)
	})

	s.Run("get room fails", func() {
		roomID := "room1"

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(nil, errors.New("database error"))

		resp, err := s.svc.GetRoom(s.ctx, roomID)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to get room")
	})
}

func (s *RoomServiceTestSuite) TestListRooms() {
	s.Run("list rooms successfully", func() {
		now := time.Now().UTC()
		roomsData := map[string]*etcdstate.Meta{
			"room1": {
				HLSPath:   "room1/stream.m3u8",
				CreatedAt: now,
			},
			"room2": {
				HLSPath:   "room2/stream.m3u8",
				CreatedAt: now,
			},
		}

		s.mockStore.EXPECT().
			GetAllRooms(gomock.Any()).
			Return(roomsData, nil)

		resp, err := s.svc.ListRooms(s.ctx)

		s.Require().NoError(err)
		s.Equal(2, resp.Count)
		s.Len(resp.Rooms, 2)

		// Check that both rooms are present (order is not guaranteed due to map iteration)
		roomIDs := []string{resp.Rooms[0].RoomID, resp.Rooms[1].RoomID}
		s.Contains(roomIDs, "room1")
		s.Contains(roomIDs, "room2")

		// Verify each room has correct HLSURL
		for _, room := range resp.Rooms {
			expectedURL := "https://example.com/hls/" + room.RoomID + "/stream.m3u8"
			s.Equal(expectedURL, room.HLSURL)
		}
	})

	s.Run("list rooms empty", func() {
		s.mockStore.EXPECT().
			GetAllRooms(gomock.Any()).
			Return(map[string]*etcdstate.Meta{}, nil)

		resp, err := s.svc.ListRooms(s.ctx)

		s.Require().NoError(err)
		s.Equal(0, resp.Count)
		s.Empty(resp.Rooms)
	})

	s.Run("list rooms fails", func() {
		s.mockStore.EXPECT().
			GetAllRooms(gomock.Any()).
			Return(nil, errors.New("database error"))

		resp, err := s.svc.ListRooms(s.ctx)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to list rooms")
	})
}

func (s *RoomServiceTestSuite) TestDeleteRoom() {
	s.Run("delete room successfully", func() {
		roomID := "room1"
		now := time.Now().UTC()
		roomData := &etcdstate.Meta{
			HLSPath:   "room1/stream.m3u8",
			CreatedAt: now,
		}

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(roomData, nil)

		s.mockStore.EXPECT().
			StopRoom(gomock.Any(), roomID).
			Return(nil)

		resp, err := s.svc.DeleteRoom(s.ctx, roomID)

		s.Require().NoError(err)
		s.Contains(resp.Message, "Room room1 stopped")
	})

	s.Run("room not found", func() {
		roomID := "nonexistent"

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(nil, nil)

		resp, err := s.svc.DeleteRoom(s.ctx, roomID)

		s.Require().Error(err)
		s.Nil(resp)
		var roomNotFoundErr *rooms.RoomNotFoundError
		s.Require().ErrorAs(err, &roomNotFoundErr)
		s.Equal(roomID, roomNotFoundErr.RoomID)
	})

	s.Run("get room fails", func() {
		roomID := "room1"

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(nil, errors.New("database error"))

		resp, err := s.svc.DeleteRoom(s.ctx, roomID)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to get room")
	})

	s.Run("stop room fails", func() {
		roomID := "room1"
		now := time.Now().UTC()
		roomData := &etcdstate.Meta{
			HLSPath:   "room1/stream.m3u8",
			CreatedAt: now,
		}

		s.mockStore.EXPECT().
			GetRoom(gomock.Any(), roomID).
			Return(roomData, nil)

		s.mockStore.EXPECT().
			StopRoom(gomock.Any(), roomID).
			Return(errors.New("stop failed"))

		resp, err := s.svc.DeleteRoom(s.ctx, roomID)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to stop room")
	})
}

func (s *RoomServiceTestSuite) TestGetStats() {
	s.Run("get stats successfully", func() {
		stats := &rooms.RoomStats{
			Total:             10,
			TotalParticipants: 25,
		}

		s.mockStore.EXPECT().
			GetStats(gomock.Any()).
			Return(stats, nil)

		resp, err := s.svc.GetStats(s.ctx)

		s.Require().NoError(err)
		s.Equal(10, resp.Rooms.Total)
		s.Equal(25, resp.Rooms.TotalParticipants)
	})

	s.Run("get stats with zero values", func() {
		stats := &rooms.RoomStats{
			Total:             0,
			TotalParticipants: 0,
		}

		s.mockStore.EXPECT().
			GetStats(gomock.Any()).
			Return(stats, nil)

		resp, err := s.svc.GetStats(s.ctx)

		s.Require().NoError(err)
		s.Equal(0, resp.Rooms.Total)
		s.Equal(0, resp.Rooms.TotalParticipants)
	})

	s.Run("get stats fails", func() {
		s.mockStore.EXPECT().
			GetStats(gomock.Any()).
			Return(nil, errors.New("stats error"))

		resp, err := s.svc.GetStats(s.ctx)

		s.Require().Error(err)
		s.Nil(resp)
		s.Contains(err.Error(), "failed to get stats")
	})
}

func (s *RoomServiceTestSuite) TestNewRoomService() {
	s.Run("create new room service", func() {
		svc := NewRoomService(
			s.mockStore,
			s.mockResMgr,
			"https://test.com/",
			log.NewNop(),
		).(*roomSvcImpl)

		s.NotNil(svc)
		s.Equal(s.mockStore, svc.roomStore)
		s.Equal(s.mockResMgr, svc.resMgr)
		s.Equal("https://test.com/", svc.hlsAdvURL)
	})
}

func (s *RoomServiceTestSuite) TestErrorTypes() {
	s.Run("RoomExistsError", func() {
		err := &rooms.RoomExistsError{RoomID: "test-room"}
		s.Equal("Room test-room already exists", err.Error())
	})

	s.Run("RoomNotFoundError", func() {
		err := &rooms.RoomNotFoundError{RoomID: "missing-room"}
		s.Equal("Room missing-room not found", err.Error())
	})
}
