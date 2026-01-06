package signal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	rpcmocks "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/users"
)

type mockConn struct {
	context    *rtcContext
	notifyFunc func(ctx context.Context, method string, params interface{}) error
}

func (m *mockConn) Open(_ context.Context) error {
	return nil
}

func (m *mockConn) Notify(ctx context.Context, method string, params interface{}) error {
	if m.notifyFunc != nil {
		return m.notifyFunc(ctx, method, params)
	}
	return nil
}

func (m *mockConn) Call(_ context.Context, _ string, _ interface{}, _ interface{}) error {
	return nil
}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) Context() jsonrpc.MethodContext[rtcContext] {
	return &mockMethodContext{context: m.context}
}

type mockMethodContext struct {
	context *rtcContext
}

func (m *mockMethodContext) Get() *rtcContext {
	return m.context
}

func (m *mockMethodContext) Set(ctx *rtcContext) {
	m.context = ctx
}

func (m *mockMethodContext) Peer() jsonrpc.Conn[rtcContext] {
	return nil
}

type ClientManagerSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	miniRedis *miniredis.Miniredis
	client    *redis.Client
	mockPeer  *rpcmocks.MockPeer[interface{}]
	manager   *WSConnManager
	logger    *log.Logger
}

func TestClientManagerSuite(t *testing.T) {
	suite.Run(t, new(ClientManagerSuite))
}

func (s *ClientManagerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())

	mr, err := miniredis.Run()
	s.Require().NoError(err)
	s.miniRedis = mr

	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	s.logger = log.NewNop()
	s.mockPeer = rpcmocks.NewMockPeer[interface{}](s.ctrl)

	s.manager, err = NewWSConnMgr(s.client, "test:ws:stream", s.logger)
	s.Require().NoError(err)

	// Replace real peer with mock for tests that need it
	s.manager.peer2ws = s.mockPeer
}

func (s *ClientManagerSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
	if s.miniRedis != nil {
		s.miniRedis.Close()
	}
	s.ctrl.Finish()
}

func (s *ClientManagerSuite) TestAddClient() {
	connID := "conn1"
	roomID := "room1"
	ctx := &rtcContext{
		connID: connID,
		roomID: roomID,
	}
	peer := &mockConn{context: ctx}

	s.manager.AddClient(connID, roomID, peer)

	s.Equal(roomID, s.manager.client2room[connID])
	s.NotNil(s.manager.room2clients[roomID])
	s.Equal(peer, s.manager.room2clients[roomID][connID])
}

func (s *ClientManagerSuite) TestAddClient_MultipleClientsInRoom() {
	roomID := "room1"

	peer1 := &mockConn{context: &rtcContext{connID: "conn1", roomID: roomID}}
	peer2 := &mockConn{context: &rtcContext{connID: "conn2", roomID: roomID}}

	s.manager.AddClient("conn1", roomID, peer1)
	s.manager.AddClient("conn2", roomID, peer2)

	s.Len(s.manager.room2clients[roomID], 2)
	s.Equal(peer1, s.manager.room2clients[roomID]["conn1"])
	s.Equal(peer2, s.manager.room2clients[roomID]["conn2"])
}

func (s *ClientManagerSuite) TestRemoveClient() {
	connID := "conn1"
	roomID := "room1"
	ctx := &rtcContext{connID: connID, roomID: roomID}
	peer := &mockConn{context: ctx}

	s.manager.AddClient(connID, roomID, peer)
	s.manager.RemoveClient(connID)

	_, ok := s.manager.client2room[connID]
	s.False(ok)

	_, ok = s.manager.room2clients[roomID]
	s.False(ok)
}

func (s *ClientManagerSuite) TestRemoveClient_OneOfMultiple() {
	roomID := "room1"
	peer1 := &mockConn{context: &rtcContext{connID: "conn1", roomID: roomID}}
	peer2 := &mockConn{context: &rtcContext{connID: "conn2", roomID: roomID}}

	s.manager.AddClient("conn1", roomID, peer1)
	s.manager.AddClient("conn2", roomID, peer2)

	s.manager.RemoveClient("conn1")

	_, ok := s.manager.client2room["conn1"]
	s.False(ok)

	s.Len(s.manager.room2clients[roomID], 1)
	s.Equal(peer2, s.manager.room2clients[roomID]["conn2"])
}

func (s *ClientManagerSuite) TestRemoveClient_NotExists() {
	s.manager.RemoveClient("nonexistent")

	s.Len(s.manager.client2room, 0)
	s.Len(s.manager.room2clients, 0)
}

func (s *ClientManagerSuite) TestRemoveRoom() {
	roomID := "room1"
	peer1 := &mockConn{context: &rtcContext{connID: "conn1", roomID: roomID}}
	peer2 := &mockConn{context: &rtcContext{connID: "conn2", roomID: roomID}}

	s.manager.AddClient("conn1", roomID, peer1)
	s.manager.AddClient("conn2", roomID, peer2)

	s.manager.RemoveRoom(roomID)

	_, ok := s.manager.room2clients[roomID]
	s.False(ok)

	_, ok = s.manager.client2room["conn1"]
	s.False(ok)

	_, ok = s.manager.client2room["conn2"]
	s.False(ok)
}

func (s *ClientManagerSuite) TestGetRoomConns() {
	roomID := "room1"
	peer1 := &mockConn{context: &rtcContext{connID: "conn1", roomID: roomID}}
	peer2 := &mockConn{context: &rtcContext{connID: "conn2", roomID: roomID}}

	s.manager.AddClient("conn1", roomID, peer1)
	s.manager.AddClient("conn2", roomID, peer2)

	conns := s.manager.getRoomConns(roomID)
	s.Len(conns, 2)
}

func (s *ClientManagerSuite) TestGetRoomConns_EmptyRoom() {
	conns := s.manager.getRoomConns("nonexistent")
	s.Nil(conns)
}

func (s *ClientManagerSuite) TestNotifyRoomLocalPeer() {
	roomID := "room1"
	notified := make(map[string]bool)

	peer1 := &mockConn{
		context: &rtcContext{
			connID: "conn1",
			roomID: roomID,
			reqCtx: context.Background(),
		},
		notifyFunc: func(_ context.Context, method string, _ interface{}) error {
			notified["conn1"] = true
			s.Equal("testMethod", method)
			return nil
		},
	}

	peer2 := &mockConn{
		context: &rtcContext{
			connID: "conn2",
			roomID: roomID,
			reqCtx: context.Background(),
		},
		notifyFunc: func(_ context.Context, method string, _ interface{}) error {
			notified["conn2"] = true
			s.Equal("testMethod", method)
			return nil
		},
	}

	s.manager.AddClient("conn1", roomID, peer1)
	s.manager.AddClient("conn2", roomID, peer2)

	s.manager.notifyRoomLocalPeer(roomID, "testMethod", map[string]string{"data": "value"})

	s.True(notified["conn1"])
	s.True(notified["conn2"])
}

func (s *ClientManagerSuite) TestHandleBroadcast() {
	roomID := "room1"
	var notifiedMethod string
	var notifiedParams interface{}
	notified := false

	peer := &mockConn{
		context: &rtcContext{
			connID: "conn1",
			roomID: roomID,
			reqCtx: context.Background(),
		},
		notifyFunc: func(_ context.Context, method string, params interface{}) error {
			notified = true
			notifiedMethod = method
			notifiedParams = params
			return nil
		},
	}

	s.manager.AddClient("conn1", roomID, peer)

	members := []*users.RoomUser{
		{
			UserID: "user1",
			Role:   "anchor",
			Status: constants.AnchorStatusOnAir,
		},
		{
			UserID: "user2",
			Role:   "anchor",
			Status: constants.AnchorStatusIdle,
		},
	}

	req := users.NotifyRoomStatus{
		RoomID:  roomID,
		Members: members,
	}

	params, err := json.Marshal(req)
	s.Require().NoError(err)
	rawParams := json.RawMessage(params)

	_, err = s.manager.handleBroadcast(nil, &rawParams)
	s.NoError(err)
	s.True(notified)
	s.Equal("roomStatus", notifiedMethod)
	s.Equal(members, notifiedParams)
}

func (s *ClientManagerSuite) TestClientManager_StartStop() {
	ctx := context.Background()

	s.mockPeer.EXPECT().Open(ctx).Return(nil)
	s.mockPeer.EXPECT().Def("broadcastRoomStatus", gomock.Any())

	err := s.manager.Start(ctx)
	s.NoError(err)

	s.mockPeer.EXPECT().Close().Return(nil)
	err = s.manager.Stop(ctx)
	s.NoError(err)
}

func (s *ClientManagerSuite) TestClientManager_Errors() {
	ctx := context.Background()

	// Start error
	s.mockPeer.EXPECT().Open(ctx).Return(context.DeadlineExceeded)
	s.mockPeer.EXPECT().Def(gomock.Any(), gomock.Any())
	err := s.manager.Start(ctx)
	s.Error(err)

	// Stop error
	s.mockPeer.EXPECT().Close().Return(context.DeadlineExceeded)
	err = s.manager.Stop(ctx)
	s.NoError(err) // Stop swallows error
}

func (s *ClientManagerSuite) TestHandleBroadcast_Error() {
	// invalid params
	badParams := json.RawMessage(`{invalid`)
	_, err := s.manager.handleBroadcast(nil, &badParams)
	s.Error(err)
}

func (s *ClientManagerSuite) TestNotifyRoomLocalPeer_Error() {
	roomID := "room1"
	// Setup
	peer := &mockConn{
		context: &rtcContext{
			reqCtx: context.Background(),
		},
		notifyFunc: func(_ context.Context, _ string, _ interface{}) error {
			return context.DeadlineExceeded
		},
	}
	s.manager.AddClient("conn1", roomID, peer)

	// Should log error but continue
	s.manager.notifyRoomLocalPeer(roomID, "method", nil)
}
