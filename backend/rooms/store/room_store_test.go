package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	etcdmocks "github.com/imtaco/audio-rtc-exp/internal/etcd/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/rooms"
)

type RoomStoreTestSuite struct {
	suite.Suite
	ctrl           *gomock.Controller
	mockEtcdClient *etcdmocks.MockClient
	store          rooms.RoomStore
	ctx            context.Context
	cancel         context.CancelFunc
}

func TestRoomStoreSuite(t *testing.T) {
	suite.Run(t, new(RoomStoreTestSuite))
}

func (s *RoomStoreTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockEtcdClient = etcdmocks.NewMockClient(s.ctrl)
	logger := log.NewTest(s.T())
	s.store = NewRoomStore(s.mockEtcdClient, "/rooms/", logger)
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *RoomStoreTestSuite) TearDownTest() {
	s.cancel()
	s.ctrl.Finish()
}

// CreateRoom Tests

func (s *RoomStoreTestSuite) TestCreateRoom_Success() {
	roomData := &etcdstate.Meta{
		Pin:     "1234",
		HLSPath: "/hls/room-123",
	}

	// Mock: room doesn't exist
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	// Mock: Put succeeds
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/meta", gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			// Verify JSON structure
			var stored etcdstate.Meta
			err := json.Unmarshal([]byte(val), &stored)
			s.NoError(err)
			s.Equal("1234", stored.Pin)
			s.Equal("/hls/room-123", stored.HLSPath)
			s.NotEmpty(stored.CreatedAt)

			return &clientv3.PutResponse{}, nil
		})

	result, err := s.store.CreateRoom(s.ctx, "room-123", roomData)
	s.NoError(err)
	s.NotNil(result)
	s.NotEmpty(result.CreatedAt)
}

func (s *RoomStoreTestSuite) TestCreateRoom_AlreadyExists() {
	roomData := &etcdstate.Meta{
		Pin:     "1234",
		HLSPath: "/hls/room-123",
	}

	// Mock: room already exists
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/meta"), Value: []byte(`{"pin":"1234"}`)},
			},
		}, nil)

	result, err := s.store.CreateRoom(s.ctx, "room-123", roomData)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "already exists")
}

func (s *RoomStoreTestSuite) TestCreateRoom_GetError() {
	roomData := &etcdstate.Meta{
		Pin: "1234",
	}

	// Mock: Get fails
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(nil, errors.New("etcd connection error"))

	result, err := s.store.CreateRoom(s.ctx, "room-123", roomData)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to check room existence")
}

func (s *RoomStoreTestSuite) TestCreateRoom_PutError() {
	roomData := &etcdstate.Meta{
		Pin: "1234",
	}

	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/meta", gomock.Any()).
		Return(nil, errors.New("etcd write error"))

	result, err := s.store.CreateRoom(s.ctx, "room-123", roomData)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to store room")
}

// GetRoom Tests

func (s *RoomStoreTestSuite) TestGetRoom_Success() {
	roomJSON := `{"pin":"1234","hlsPath":"/hls/room-123","createdAt":"2024-01-01T00:00:00Z"}`

	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/meta"), Value: []byte(roomJSON)},
			},
		}, nil)

	room, err := s.store.GetRoom(s.ctx, "room-123")
	s.NoError(err)
	s.NotNil(room)
	s.Equal("1234", room.Pin)
	s.Equal("/hls/room-123", room.HLSPath)
}

func (s *RoomStoreTestSuite) TestGetRoom_NotFound() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	room, err := s.store.GetRoom(s.ctx, "room-123")
	s.NoError(err)
	s.Nil(room)
}

func (s *RoomStoreTestSuite) TestGetRoom_GetError() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(nil, errors.New("etcd error"))

	room, err := s.store.GetRoom(s.ctx, "room-123")
	s.Error(err)
	s.Nil(room)
	s.Contains(err.Error(), "failed to get room")
}

func (s *RoomStoreTestSuite) TestGetRoom_UnmarshalError() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/meta"), Value: []byte(`invalid json`)},
			},
		}, nil)

	room, err := s.store.GetRoom(s.ctx, "room-123")
	s.Error(err)
	s.Nil(room)
	s.Contains(err.Error(), "failed to unmarshal")
}

// Exists Tests

func (s *RoomStoreTestSuite) TestExists_True() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/meta"), Value: []byte(`{}`)},
			},
		}, nil)

	exists, err := s.store.Exists(s.ctx, "room-123")
	s.NoError(err)
	s.True(exists)
}

func (s *RoomStoreTestSuite) TestExists_False() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	exists, err := s.store.Exists(s.ctx, "room-123")
	s.NoError(err)
	s.False(exists)
}

func (s *RoomStoreTestSuite) TestExists_Error() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(nil, errors.New("etcd error"))

	exists, err := s.store.Exists(s.ctx, "room-123")
	s.Error(err)
	s.False(exists)
}

// DeleteRoom Tests

func (s *RoomStoreTestSuite) TestDeleteRoom_Success() {
	s.mockEtcdClient.EXPECT().
		Delete(gomock.Any(), "/rooms/room-123/", gomock.Any()).
		Return(&clientv3.DeleteResponse{Deleted: 3}, nil)

	deleted, err := s.store.DeleteRoom(s.ctx, "room-123")
	s.NoError(err)
	s.True(deleted)
}

func (s *RoomStoreTestSuite) TestDeleteRoom_NotFound() {
	s.mockEtcdClient.EXPECT().
		Delete(gomock.Any(), "/rooms/room-123/", gomock.Any()).
		Return(&clientv3.DeleteResponse{Deleted: 0}, nil)

	deleted, err := s.store.DeleteRoom(s.ctx, "room-123")
	s.NoError(err)
	s.False(deleted)
}

func (s *RoomStoreTestSuite) TestDeleteRoom_Error() {
	s.mockEtcdClient.EXPECT().
		Delete(gomock.Any(), "/rooms/room-123/", gomock.Any()).
		Return(nil, errors.New("etcd error"))

	deleted, err := s.store.DeleteRoom(s.ctx, "room-123")
	s.Error(err)
	s.False(deleted)
	s.Contains(err.Error(), "failed to delete room")
}

// CreateLiveMeta Tests

func (s *RoomStoreTestSuite) TestCreateLiveMeta_Success() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/livemeta", gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			var livemeta rooms.LiveMeta
			err := json.Unmarshal([]byte(val), &livemeta)
			s.NoError(err)
			s.Equal(constants.RoomStatusOnAir, livemeta.Status)
			s.Equal("mixer-1", livemeta.MixerID)
			s.Equal("janus-1", livemeta.JanusID)
			s.Equal("nonce-123", livemeta.Nonce)
			s.NotEmpty(livemeta.CreatedAt)

			return &clientv3.PutResponse{}, nil
		})

	err := s.store.CreateLiveMeta(s.ctx, "room-123", "mixer-1", "janus-1", "nonce-123")
	s.NoError(err)
}

func (s *RoomStoreTestSuite) TestCreateLiveMeta_PutError() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/livemeta", gomock.Any()).
		Return(nil, errors.New("etcd error"))

	err := s.store.CreateLiveMeta(s.ctx, "room-123", "mixer-1", "janus-1", "nonce-123")
	s.Error(err)
	s.Contains(err.Error(), "failed to store livemeta")
}

// StopLiveMeta Tests

func (s *RoomStoreTestSuite) TestStopLiveMeta_Success() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/livemeta", gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			var livemeta rooms.LiveMeta
			err := json.Unmarshal([]byte(val), &livemeta)
			s.NoError(err)
			s.Equal(constants.RoomStatusRemoving, livemeta.Status)
			s.NotEmpty(livemeta.DiscardAt)

			return &clientv3.PutResponse{}, nil
		})

	err := s.store.StopLiveMeta(s.ctx, "room-123")
	s.NoError(err)
}

func (s *RoomStoreTestSuite) TestStopRoom_CallsStopLiveMeta() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/livemeta", gomock.Any()).
		Return(&clientv3.PutResponse{}, nil)

	err := s.store.StopRoom(s.ctx, "room-123")
	s.NoError(err)
}

// GetAllRooms Tests

func (s *RoomStoreTestSuite) TestGetAllRooms_Success() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{
					Key:   []byte("/rooms/room-1/meta"),
					Value: []byte(`{"roomId":"room-1","pin":"1111"}`),
				},
				{
					Key:   []byte("/rooms/room-2/meta"),
					Value: []byte(`{"roomId":"room-2","pin":"2222"}`),
				},
				{
					Key:   []byte("/rooms/room-1/livemeta"),
					Value: []byte(`{"status":"on-air"}`),
				},
			},
		}, nil)

	rooms, err := s.store.GetAllRooms(s.ctx)
	s.NoError(err)
	s.Len(rooms, 2)
	s.NotNil(rooms["room-1"])
	s.NotNil(rooms["room-2"])
}

func (s *RoomStoreTestSuite) TestGetAllRooms_EmptyResult() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	rooms, err := s.store.GetAllRooms(s.ctx)
	s.NoError(err)
	s.Empty(rooms)
}

func (s *RoomStoreTestSuite) TestGetAllRooms_Error() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(nil, errors.New("etcd error"))

	rooms, err := s.store.GetAllRooms(s.ctx)
	s.Error(err)
	s.Nil(rooms)
}

func (s *RoomStoreTestSuite) TestGetAllRooms_SkipsInvalidJSON() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{
					Key:   []byte("/rooms/room-1/meta"),
					Value: []byte(`{"roomId":"room-1"}`),
				},
				{
					Key:   []byte("/rooms/room-2/meta"),
					Value: []byte(`invalid json`),
				},
				{
					Key:   []byte("/rooms/room-3/meta"),
					Value: []byte(`{"roomId":"room-3"}`),
				},
			},
		}, nil)

	rooms, err := s.store.GetAllRooms(s.ctx)
	s.NoError(err)
	s.Len(rooms, 2) // room-2 should be skipped
	s.NotNil(rooms["room-1"])
	s.NotNil(rooms["room-3"])
}

// GetStats Tests

func (s *RoomStoreTestSuite) TestGetStats_Success() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-1/meta"), Value: []byte(`{"roomId":"room-1"}`)},
				{Key: []byte("/rooms/room-2/meta"), Value: []byte(`{"roomId":"room-2"}`)},
				{Key: []byte("/rooms/room-3/meta"), Value: []byte(`{"roomId":"room-3"}`)},
			},
		}, nil)

	stats, err := s.store.GetStats(s.ctx)
	s.NoError(err)
	s.NotNil(stats)
	s.Equal(3, stats.Total)
	s.Equal(0, stats.TotalParticipants)
}

func (s *RoomStoreTestSuite) TestGetStats_Error() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/", gomock.Any()).
		Return(nil, errors.New("etcd error"))

	stats, err := s.store.GetStats(s.ctx)
	s.Error(err)
	s.Nil(stats)
}

// GetMixerData Tests

func (s *RoomStoreTestSuite) TestGetMixerData_Success() {
	mixerJSON := `{"port":5000}`

	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/mixer").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/mixer"), Value: []byte(mixerJSON)},
			},
		}, nil)

	mixerData, err := s.store.GetMixerData(s.ctx, "room-123")
	s.NoError(err)
	s.NotNil(mixerData)
	s.Equal(5000, mixerData.Port)
}

func (s *RoomStoreTestSuite) TestGetMixerData_NotFound() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/mixer").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	mixerData, err := s.store.GetMixerData(s.ctx, "room-123")
	s.NoError(err)
	s.Nil(mixerData)
}

func (s *RoomStoreTestSuite) TestGetMixerData_GetError() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/mixer").
		Return(nil, errors.New("etcd error"))

	mixerData, err := s.store.GetMixerData(s.ctx, "room-123")
	s.Error(err)
	s.Nil(mixerData)
}

func (s *RoomStoreTestSuite) TestGetMixerData_UnmarshalError() {
	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/mixer").
		Return(&clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/rooms/room-123/mixer"), Value: []byte(`invalid`)},
			},
		}, nil)

	mixerData, err := s.store.GetMixerData(s.ctx, "room-123")
	s.Error(err)
	s.Nil(mixerData)
}

// Helper method tests

func (s *RoomStoreTestSuite) TestKeyGeneration() {
	store := s.store.(*roomStoreImpl)

	s.Equal("/rooms/room-123/meta", store.metaKey("room-123"))
	s.Equal("/rooms/room-123/livemeta", store.livemetaKey("room-123"))
	s.Equal("/rooms/room-123/mixer", store.mixerKey("room-123"))
}

// Timestamp tests

func (s *RoomStoreTestSuite) TestCreateRoom_SetsTimestamp() {
	before := time.Now().Add(-1 * time.Second) // Set before to 1 second in the past

	s.mockEtcdClient.EXPECT().
		Get(gomock.Any(), "/rooms/room-123/meta").
		Return(&clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil)

	var storedData etcdstate.Meta
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "/rooms/room-123/meta", gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			json.Unmarshal([]byte(val), &storedData)
			return &clientv3.PutResponse{}, nil
		})

	roomData := &etcdstate.Meta{}
	result, err := s.store.CreateRoom(s.ctx, "room-123", roomData)

	s.NoError(err)
	// Verify timestamp is recent and in RFC3339 format
	s.True(result.CreatedAt.After(before), "CreatedAt should be after 'before' time")
	s.True(result.CreatedAt.Before(time.Now().Add(time.Second)), "CreatedAt should be recent")
}

// SetModuleMark Tests

func (s *RoomStoreTestSuite) TestSetModuleMark_SuccessWithoutTTL() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "mixersmixer-1/mark", gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			// Verify JSON structure
			var markData etcdstate.MarkData
			err := json.Unmarshal([]byte(val), &markData)
			s.NoError(err)
			s.Equal(constants.MarkLabelReady, markData.Label)

			// Verify no lease option is set
			s.Empty(opts)

			return &clientv3.PutResponse{}, nil
		})

	err := s.store.SetModuleMark(s.ctx, "mixers", "mixer-1", constants.MarkLabelReady, 0)
	s.NoError(err)
}

func (s *RoomStoreTestSuite) TestSetModuleMark_SuccessWithTTL() {
	leaseID := clientv3.LeaseID(12345)

	s.mockEtcdClient.EXPECT().
		Grant(gomock.Any(), int64(3600)).
		Return(&clientv3.LeaseGrantResponse{ID: leaseID}, nil)

	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "janusesjan-1/mark", gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
			// Verify JSON structure
			var markData etcdstate.MarkData
			err := json.Unmarshal([]byte(val), &markData)
			s.NoError(err)
			s.Equal(constants.MarkLabelCordon, markData.Label)

			// Verify that lease option is set (should have 1 option with the lease)
			s.Len(opts, 1, "Expected exactly one option (the lease)")

			return &clientv3.PutResponse{}, nil
		})

	err := s.store.SetModuleMark(s.ctx, "januses", "jan-1", constants.MarkLabelCordon, 3600)
	s.NoError(err)
}

func (s *RoomStoreTestSuite) TestSetModuleMark_GrantLeaseError() {
	s.mockEtcdClient.EXPECT().
		Grant(gomock.Any(), int64(3600)).
		Return(nil, errors.New("lease grant failed"))

	err := s.store.SetModuleMark(s.ctx, "januses", "jan-1", constants.MarkLabelCordon, 3600)
	s.Error(err)
	s.Contains(err.Error(), "failed to create lease")
}

func (s *RoomStoreTestSuite) TestSetModuleMark_AllLabels() {
	labels := []constants.MarkLabel{
		constants.MarkLabelReady,
		constants.MarkLabelCordon,
		constants.MarkLabelDraining,
		constants.MarkLabelDrained,
		constants.MarkLabelUnready,
	}

	for _, label := range labels {
		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
				var markData etcdstate.MarkData
				err := json.Unmarshal([]byte(val), &markData)
				s.NoError(err)
				s.Equal(label, markData.Label)
				return &clientv3.PutResponse{}, nil
			})

		err := s.store.SetModuleMark(s.ctx, "mixers", "mixer-1", label, 0)
		s.NoError(err)
	}
}

func (s *RoomStoreTestSuite) TestSetModuleMark_PutError() {
	s.mockEtcdClient.EXPECT().
		Put(gomock.Any(), "mixersmixer-1/mark", gomock.Any()).
		Return(nil, errors.New("etcd write error"))

	err := s.store.SetModuleMark(s.ctx, "mixers", "mixer-1", constants.MarkLabelReady, 0)
	s.Error(err)
	s.Contains(err.Error(), "failed to set module mark")
}

func (s *RoomStoreTestSuite) TestSetModuleMark_ModuleTypes() {
	moduleTypes := []string{"mixers", "januses"}

	for _, moduleType := range moduleTypes {
		s.mockEtcdClient.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
				// Verify the key contains the correct module type
				s.Contains(key, moduleType)
				return &clientv3.PutResponse{}, nil
			})

		err := s.store.SetModuleMark(s.ctx, moduleType, "module-1", constants.MarkLabelReady, 0)
		s.NoError(err)
	}
}

// DeleteModuleMark Tests

func (s *RoomStoreTestSuite) TestDeleteModuleMark_Success() {
	s.mockEtcdClient.EXPECT().
		Delete(gomock.Any(), "mixersmixer-1/mark").
		Return(&clientv3.DeleteResponse{Deleted: 1}, nil)

	err := s.store.DeleteModuleMark(s.ctx, "mixers", "mixer-1")
	s.NoError(err)
}

func (s *RoomStoreTestSuite) TestDeleteModuleMark_DeleteError() {
	s.mockEtcdClient.EXPECT().
		Delete(gomock.Any(), "mixersmixer-1/mark").
		Return(nil, errors.New("etcd delete error"))

	err := s.store.DeleteModuleMark(s.ctx, "mixers", "mixer-1")
	s.Error(err)
	s.Contains(err.Error(), "failed to delete module mark")
}

func (s *RoomStoreTestSuite) TestDeleteModuleMark_AllModuleTypes() {
	moduleTypes := []string{"mixers", "januses"}

	for _, moduleType := range moduleTypes {
		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
				// Verify the key contains the correct module type
				s.Contains(key, moduleType)
				return &clientv3.DeleteResponse{Deleted: 1}, nil
			})

		err := s.store.DeleteModuleMark(s.ctx, moduleType, "module-1")
		s.NoError(err)
	}
}

func (s *RoomStoreTestSuite) TestDeleteModuleMark_MultipleModules() {
	moduleIDs := []string{"mixer-1", "mixer-2", "mixer-3"}

	for _, moduleID := range moduleIDs {
		s.mockEtcdClient.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
				// Verify the key contains the correct module ID
				s.Contains(key, moduleID)
				return &clientv3.DeleteResponse{Deleted: 1}, nil
			})

		err := s.store.DeleteModuleMark(s.ctx, "mixers", moduleID)
		s.NoError(err)
	}
}

// ModuleMarkKey Tests

func (s *RoomStoreTestSuite) TestModuleMarkKey_Generation() {
	store := s.store.(*roomStoreImpl)

	s.Equal("mixersmixer-1/mark", store.moduleMarkKey("mixers", "mixer-1"))
	s.Equal("janusesjan-1/mark", store.moduleMarkKey("januses", "jan-1"))
	s.Equal("mixerstest-module/mark", store.moduleMarkKey("mixers", "test-module"))
}
