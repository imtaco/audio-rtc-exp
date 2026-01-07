package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	janusapimocks "github.com/imtaco/audio-rtc-exp/internal/janus/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	jsonrpcmocks "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	usersmocks "github.com/imtaco/audio-rtc-exp/users/mocks"
	wsgymocks "github.com/imtaco/audio-rtc-exp/wsgateway/mocks"
)

type mockMethodCtx struct {
	rtcCtx *rtcContext
	peer   jsonrpc.Conn[rtcContext]
}

func (m *mockMethodCtx) Get() *rtcContext {
	return m.rtcCtx
}

func (m *mockMethodCtx) Set(ctx *rtcContext) {
	m.rtcCtx = ctx
}

func (m *mockMethodCtx) Peer() jsonrpc.Conn[rtcContext] {
	return m.peer
}

type mockPeer struct {
	closeFunc   func() error
	notifyFunc  func(ctx context.Context, method string, params any) error
	contextFunc func() jsonrpc.MethodContext[rtcContext]
}

func (m *mockPeer) Open(_ context.Context) error {
	return nil
}

func (m *mockPeer) Call(_ context.Context, _ string, _ any, _ any) error {
	return nil
}

func (m *mockPeer) Notify(ctx context.Context, method string, params any) error {
	if m.notifyFunc != nil {
		return m.notifyFunc(ctx, method, params)
	}
	return nil
}

func (m *mockPeer) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockPeer) Context() jsonrpc.MethodContext[rtcContext] {
	if m.contextFunc != nil {
		return m.contextFunc()
	}
	return nil
}

type ServerSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	janusProxy      *wsgymocks.MockJanusProxy
	janusAPI        *janusapimocks.MockAPI
	janusTokenCodec *wsgymocks.MockJanusTokenCodec
	userService     *usersmocks.MockUserService
	connGuard       *MockConnectionGuard
	core            *jsonrpcmocks.MockCore[rtcContext]
	clientManager   *WSConnManager
	server          *Server
	logger          *log.Logger
	janusServer     *httptest.Server
	realJanusAPI    janus.API // Keep for tests that still use httptest
	failJanus       bool
}

func TestServerSuite(t *testing.T) {
	suite.Run(t, new(ServerSuite))
}

func (s *ServerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.logger = log.NewNop()
	s.failJanus = false

	s.janusProxy = wsgymocks.NewMockJanusProxy(s.ctrl)
	s.janusAPI = janusapimocks.NewMockAPI(s.ctrl)
	s.janusTokenCodec = wsgymocks.NewMockJanusTokenCodec(s.ctrl)
	s.userService = usersmocks.NewMockUserService(s.ctrl)
	s.connGuard = NewMockConnectionGuard(s.ctrl)
	s.core = jsonrpcmocks.NewMockCore[rtcContext](s.ctrl)

	s.clientManager = &WSConnManager{
		room2clients: make(map[string]map[string]jsonrpc.Conn[rtcContext]),
		client2room:  make(map[string]string),
		logger:       s.logger,
	}

	s.server = NewServer(
		s.core,
		s.janusProxy,
		s.janusTokenCodec,
		s.clientManager,
		s.userService,
		s.connGuard,
		nil,
		s.logger,
	)

	// Setup Janus Mock Server
	// Keep httptest server for integration tests that still need it
	s.janusServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.failJanus {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		janusType, _ := req["janus"].(string)

		var resp map[string]any

		switch janusType {
		case "create": // Create Session
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 123,
				},
			}
		case "attach": // Attach Plugin
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 456,
				},
			}
		case "message": // Join or Offer or Candidate or Exists
			body, _ := req["body"].(map[string]any)
			request, _ := body["request"].(string)

			switch request {
			case "join":
				resp = map[string]any{
					"janus": "ack",
					"plugindata": map[string]any{
						"data": map[string]any{
							"result": "ok",
						},
					},
				}
			case "exists":
				// exists check for session validation
				resp = map[string]any{
					"janus": "success",
					"plugindata": map[string]any{
						"plugin": "janus.plugin.videoroom",
						"data": map[string]any{
							"videoroom": "success",
							"exists":    true,
						},
					},
				}
			default:
				resp = map[string]any{
					"janus": "success",
					"plugindata": map[string]any{
						"data": map[string]any{
							"result": "ok",
						},
					},
				}
			}

		case "trickle":
			resp = map[string]any{
				"janus": "ack",
			}

		case "keepalive":
			resp = map[string]any{
				"janus": "ack",
			}
		default:
		}

		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"janus": "event",
					"jsep": map[string]any{
						"type": "answer",
						"sdp":  "mock-sdp",
					},
				},
			})
			return
		}

		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))

	s.realJanusAPI = janus.New(s.janusServer.URL, s.logger)
}

func (s *ServerSuite) TearDownTest() {
	s.janusServer.Close()
	s.ctrl.Finish()
}

func (s *ServerSuite) TestHandleJoin_AlreadyJoined() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: true,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	result, err := s.server.handleJoin(mctx, nil)
	s.Require().Error(err)
	s.Nil(result)
}

func (s *ServerSuite) TestHandleJoin_InvalidPin() {
	ctx := context.Background()
	roomID := "room1"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		joined: false,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	params, _ := json.Marshal(map[string]string{
		"pin":      "wrong-pin",
		"clientId": "550e8400-e29b-41d4-a716-446655440000",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "correct-pin", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
	})
	// Note: GetJanusAPI should NOT be called since PIN validation fails first

	result, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "invalid room pin")
}

func (s *ServerSuite) TestHandleJoin_RoomNotOnAir() {
	ctx := context.Background()
	roomID := "room1"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		joined: false,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	params, _ := json.Marshal(map[string]string{
		"pin":      "123456",
		"clientId": "550e8400-e29b-41d4-a716-446655440000",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123456", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusRemoving,
	})

	result, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(result)
}

func (s *ServerSuite) TestHandleJoin_NoJanusAPI() {
	ctx := context.Background()
	roomID := "room1"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		joined: false,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	params, _ := json.Marshal(map[string]string{
		"pin":      "123456",
		"clientId": "550e8400-e29b-41d4-a716-446655440000",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123456", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
	})
	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(nil)

	result, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(result)
}

func (s *ServerSuite) TestHandleLeave_NotJoined() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: false,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	result, err := s.server.handleLeave(mctx, nil)
	s.Require().Error(err)
	s.Nil(result)
}

func (s *ServerSuite) TestHandleLeave_Success() {
	ctx := context.Background()
	roomID := "room1"
	userID := "user1"
	connID := "conn1"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: userID,
		connID: connID,
		joined: true,
	}

	peerClosed := false
	peer := &mockPeer{
		closeFunc: func() error {
			peerClosed = true
			return nil
		},
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
		peer:   peer,
	}

	s.clientManager.AddClient(connID, roomID, peer)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, userID, constants.AnchorStatusLeft, int32(GEN)).Return(nil)

	result, err := s.server.handleLeave(mctx, nil)
	s.Require().NoError(err)
	s.Nil(result)
	s.True(peerClosed)

	_, exists := s.clientManager.client2room[connID]
	s.False(exists)
}

func (s *ServerSuite) TestHandleIceCandidate_NotJoined() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: false,
	}

	mctx := &mockMethodCtx{
		rtcCtx: rtcCtx,
	}

	result, err := s.server.handleIceCandidate(mctx, nil)
	s.Require().Error(err)
	s.Nil(result)
}

func (s *ServerSuite) TestMustHoldLock_Success() {
	ctx := context.Background()
	userID := "user1"
	connID := "conn1"
	clientID := "client1"

	mctx := &mockMethodCtx{
		rtcCtx: &rtcContext{
			reqCtx:   ctx,
			userID:   userID,
			connID:   connID,
			clientID: clientID,
		},
	}
	s.connGuard.EXPECT().MustHold(mctx).Return(true, nil)

	s.server.mustHoldLock(mctx)
}

func (s *ServerSuite) TestMustHoldLock_LockFailed() {
	ctx := context.Background()
	userID := "user1"
	connID := "conn1"
	clientID := "client1"

	mctx := &mockMethodCtx{
		rtcCtx: &rtcContext{
			reqCtx:   ctx,
			userID:   userID,
			connID:   connID,
			clientID: clientID,
		},
	}
	s.connGuard.EXPECT().MustHold(mctx).Return(false, nil)
	s.server.mustHoldLock(mctx)
}

func (s *ServerSuite) TestUpdateUserStatus() {
	ctx := context.Background()
	roomID := "room1"
	userID := "user1"
	status := constants.AnchorStatusOnAir

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, userID, status, int32(GEN)).Return(nil)

	s.server.updateUserStatus(ctx, roomID, userID, status)
}

func (s *ServerSuite) TestOpen() {
	ctx := context.Background()

	s.core.EXPECT().Def("join", gomock.Any())
	s.core.EXPECT().Def("leave", gomock.Any())
	s.core.EXPECT().Def("offer", gomock.Any())
	s.core.EXPECT().Def("icecandidate", gomock.Any())
	s.core.EXPECT().Def("keepalive", gomock.Any())
	s.core.EXPECT().Def("status", gomock.Any())
	s.connGuard.EXPECT().Start(gomock.Any()).Return(nil)

	err := s.server.Open(ctx)
	s.Require().NoError(err)
}

func (s *ServerSuite) TestHandleJoin_Success() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440000",
	})
	rawParams := json.RawMessage(params)

	// Mock JanusProxy
	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	// Return mock Janus API
	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(s.janusAPI)

	// Mock Anchor instance for new session (sessionID=0, handleID=0)
	mockAnchor := janusapimocks.NewMockAnchor(s.ctrl)
	mockAnchor.EXPECT().GetSessionID().Return(int64(123)).AnyTimes()
	mockAnchor.EXPECT().GetHandleID().Return(int64(456)).AnyTimes()

	// CreateAnchorInstance called with sessionID=0, handleID=0 for new session
	s.janusAPI.EXPECT().CreateAnchorInstance(gomock.Any(), "conn1", int64(0), int64(0)).Return(mockAnchor, nil)

	// Mock Encrypt to return a token after creating the instance
	s.janusTokenCodec.EXPECT().Encode(nonce, int64(123), int64(456)).Return("encoded-token", nil)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, "user1", constants.AnchorStatusIdle, gomock.Any()).Return(nil)

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)
	s.True(rtcCtx.joined)
	s.NotNil(rtcCtx.janus)

	// Verify response contains jtoken and resume flag
	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Contains(resMap, "jtoken")
	s.Contains(resMap, "resume")
	s.Equal("encoded-token", resMap["jtoken"])
	s.Equal(false, resMap["resume"]) // New session, so resume should be false
}

func (s *ServerSuite) TestHandleJoin_WithInvalidToken() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440001",
		"jtoken":   "invalid-token",
	})
	rawParams := json.RawMessage(params)

	// Mock JanusProxy
	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(s.janusAPI)

	// Decode fails - token is invalid, falls back to sessionID=0, handleID=0
	s.janusTokenCodec.EXPECT().Decode(nonce, "invalid-token").Return(int64(0), int64(0), fmt.Errorf("invalid token"))

	// Mock Anchor instance for new session
	mockAnchor := janusapimocks.NewMockAnchor(s.ctrl)
	mockAnchor.EXPECT().GetSessionID().Return(int64(999)).AnyTimes()
	mockAnchor.EXPECT().GetHandleID().Return(int64(888)).AnyTimes()

	// Should still create a new session (sessionID=0, handleID=0)
	s.janusAPI.EXPECT().CreateAnchorInstance(gomock.Any(), "conn1", int64(0), int64(0)).Return(mockAnchor, nil)

	s.janusTokenCodec.EXPECT().Encode(nonce, int64(999), int64(888)).Return("new-token", nil)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, "user1", constants.AnchorStatusIdle, gomock.Any()).Return(nil)

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)
	s.True(rtcCtx.joined)

	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Equal("new-token", resMap["jtoken"])
	s.Equal(false, resMap["resume"]) // Invalid token results in new session
}

func (s *ServerSuite) TestHandleJoin_WithValidTokenButExpiredSession() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440002",
		"jtoken":   "valid-but-expired-token",
	})
	rawParams := json.RawMessage(params)

	// Create a special test server that returns session not found for old session
	expiredJanusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		janusType, _ := req["janus"].(string)
		var resp map[string]any

		switch janusType {
		case "create":
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 999, // New session ID
				},
			}
		case "attach":
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 888, // New handle ID
				},
			}
		case "message":
			body, _ := req["body"].(map[string]any)
			request, _ := body["request"].(string)
			if request == "exists" {
				// Session expired - return error
				resp = map[string]any{
					"janus": "error",
					"error": map[string]any{
						"code":   458,
						"reason": "Session not found",
					},
				}
			}
		}

		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer expiredJanusServer.Close()

	expiredJanusAPI := janus.New(expiredJanusServer.URL, s.logger)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(expiredJanusAPI)

	// Decode succeeds - token is valid
	s.janusTokenCodec.EXPECT().Decode(nonce, "valid-but-expired-token").Return(int64(123), int64(456), nil)

	// Should create a new session after detecting expiration
	s.janusTokenCodec.EXPECT().Encode(nonce, int64(999), int64(888)).Return("new-session-token", nil)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, "user1", constants.AnchorStatusIdle, gomock.Any()).Return(nil)

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)

	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Equal("new-session-token", resMap["jtoken"])
	s.Equal(false, resMap["resume"]) // Session expired, new session created
}

func (s *ServerSuite) TestHandleJoin_WithValidTokenAndActiveSession() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"
	validSessionID := int64(123)
	validHandleID := int64(456)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440003",
		"jtoken":   "valid-active-token",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(s.janusAPI)

	// Decode succeeds - token is valid and returns the existing session
	s.janusTokenCodec.EXPECT().Decode(nonce, "valid-active-token").Return(validSessionID, validHandleID, nil)

	// Mock Anchor instance with existing session
	mockAnchor := janusapimocks.NewMockAnchor(s.ctrl)
	mockAnchor.EXPECT().GetSessionID().Return(validSessionID).AnyTimes()
	mockAnchor.EXPECT().GetHandleID().Return(validHandleID).AnyTimes()

	// CreateAnchorInstance called with existing sessionID/handleID
	s.janusAPI.EXPECT().CreateAnchorInstance(gomock.Any(), "conn1", validSessionID, validHandleID).Return(mockAnchor, nil)

	// CRITICAL: Mock the Check method to return success (session is still active)
	mockAnchor.EXPECT().Check(gomock.Any()).Return(true, nil)

	// Should encrypt with the same session IDs (session is still active)
	s.janusTokenCodec.EXPECT().Encode(nonce, validSessionID, validHandleID).Return("resumed-token", nil)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, "user1", constants.AnchorStatusIdle, gomock.Any()).Return(nil)

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)

	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Equal("resumed-token", resMap["jtoken"])
	s.Equal(true, resMap["resume"]) // Session resumed successfully
}

func (s *ServerSuite) TestHandleJoin_CheckFailsWithHTTPError() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440004",
		"jtoken":   "valid-token",
	})
	rawParams := json.RawMessage(params)

	// Create a server that returns 500 for check requests
	// This will trigger ErrNoneSuccessResponse, causing a new session to be created
	errorJanusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		janusType, _ := req["janus"].(string)
		var resp map[string]any

		switch janusType {
		case "create":
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 777, // New session after check fails
				},
			}
		case "attach":
			resp = map[string]any{
				"janus": "success",
				"data": map[string]any{
					"id": 666, // New handle after check fails
				},
			}
		case "message":
			body, _ := req["body"].(map[string]any)
			request, _ := body["request"].(string)
			if request == "exists" {
				// Return 500 error - this is treated as ErrNoneSuccessResponse
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer errorJanusServer.Close()

	errorJanusAPI := janus.New(errorJanusServer.URL, s.logger)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(errorJanusAPI)

	// Decode succeeds
	s.janusTokenCodec.EXPECT().Decode(nonce, "valid-token").Return(int64(123), int64(456), nil)

	// HTTP 500 is treated as ErrNoneSuccessResponse, so a new session is created
	s.janusTokenCodec.EXPECT().Encode(nonce, int64(777), int64(666)).Return("new-session-after-check-fail", nil)

	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, "user1", constants.AnchorStatusIdle, gomock.Any()).Return(nil)

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)

	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Equal("new-session-after-check-fail", resMap["jtoken"])
	s.Equal(false, resMap["resume"]) // Check failed, so new session created
}

func (s *ServerSuite) TestHandleJoin_CheckFailsWithUnexpectedError() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"
	validSessionID := int64(123)
	validHandleID := int64(456)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440005",
		"jtoken":   "valid-token",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(s.janusAPI)

	// Decode succeeds - token is valid
	s.janusTokenCodec.EXPECT().Decode(nonce, "valid-token").Return(validSessionID, validHandleID, nil)

	// Mock Anchor instance with existing session
	mockAnchor := janusapimocks.NewMockAnchor(s.ctrl)

	// CreateAnchorInstance called with existing sessionID/handleID
	s.janusAPI.EXPECT().CreateAnchorInstance(gomock.Any(), "conn1", validSessionID, validHandleID).Return(mockAnchor, nil)

	// CRITICAL: Check fails with unexpected error (NOT ErrNoneSuccessResponse)
	// This should cause handleJoin to return an error
	mockAnchor.EXPECT().Check(gomock.Any()).Return(false, fmt.Errorf("network timeout"))

	// Should NOT call Encrypt or SetUserStatus because the join should fail

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().Error(err) // Should return error
	s.Nil(res)
	s.False(rtcCtx.joined) // Should not be joined
}

func (s *ServerSuite) TestHandleJoin_InvalidParams() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: "room1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	// Invalid JSON
	invalidParams := json.RawMessage(`{invalid json}`)

	res, err := s.server.handleJoin(mctx, &invalidParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "invalid join parameters")
}

func (s *ServerSuite) TestHandleJoin_EncryptFailure() {
	ctx := context.Background()
	roomID := "room1"
	nonce := "test-nonce"

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		connID: "conn1",
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"pin":      "123",
		"clientId": "550e8400-e29b-41d4-a716-446655440006",
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})
	s.janusProxy.EXPECT().GetRoomLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  nonce,
	})

	s.janusProxy.EXPECT().GetJanusAPI(roomID).Return(s.janusAPI)

	// Mock Anchor instance for new session
	mockAnchor := janusapimocks.NewMockAnchor(s.ctrl)
	mockAnchor.EXPECT().GetSessionID().Return(int64(123)).AnyTimes()
	mockAnchor.EXPECT().GetHandleID().Return(int64(456)).AnyTimes()

	s.janusAPI.EXPECT().CreateAnchorInstance(gomock.Any(), "conn1", int64(0), int64(0)).Return(mockAnchor, nil)

	// Encrypt fails
	s.janusTokenCodec.EXPECT().Encode(nonce, int64(123), int64(456)).Return("", fmt.Errorf("encryption error"))

	res, err := s.server.handleJoin(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "fail to create janus token")
}

func (s *ServerSuite) TestHandleOffer_Success() {
	// Setup context
	ctx := context.Background()
	roomID := "room1"

	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		joined: true,
		janus:  inst,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	// Params
	sdp := janus.JSEP{Type: "offer", SDP: "offer-sdp"}
	params, _ := json.Marshal(map[string]any{
		"sdp": sdp,
	})
	rawParams := json.RawMessage(params)

	// Expectations
	s.janusProxy.EXPECT().GetJanusRoomID(roomID).Return(int64(1234))
	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(&etcdstate.Meta{Pin: "123", MaxAnchors: 5})

	// Execute
	res, err := s.server.handleOffer(mctx, &rawParams)
	s.Require().NoError(err)
	s.NotNil(res)

	resMap, ok := res.(map[string]any)
	s.True(ok)
	s.Contains(resMap, "sdp")
}

func (s *ServerSuite) TestHandleOffer_JanusError() {
	ctx := context.Background()
	roomID := "room1"

	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		joined: true,
		janus:  inst,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	sdp := janus.JSEP{Type: "offer", SDP: "offer-sdp"}
	params, _ := json.Marshal(map[string]any{
		"sdp": sdp,
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetJanusRoomID("room2").Return(int64(0))
	rtcCtx.roomID = "room2"
	_, err = s.server.handleOffer(mctx, &rawParams)
	s.Require().Error(err)
}

func (s *ServerSuite) TestHandleOffer_NotJoined() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	res, err := s.server.handleOffer(mctx, nil)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "not joined yet")
}

func (s *ServerSuite) TestHandleOffer_InvalidParams() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: true,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	// Invalid JSON
	invalidParams := json.RawMessage(`{invalid json}`)

	res, err := s.server.handleOffer(mctx, &invalidParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "invalid offer parameters")
}

func (s *ServerSuite) TestHandleOffer_MissingSDP() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: true,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{})
	rawParams := json.RawMessage(params)

	res, err := s.server.handleOffer(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "invalid offer parameters")
}

func (s *ServerSuite) TestHandleOffer_NoRoomMeta() {
	ctx := context.Background()
	roomID := "room1"

	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		joined: true,
		janus:  inst,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	sdp := janus.JSEP{Type: "offer", SDP: "offer-sdp"}
	params, _ := json.Marshal(map[string]any{
		"sdp": sdp,
	})
	rawParams := json.RawMessage(params)

	s.janusProxy.EXPECT().GetJanusRoomID(roomID).Return(int64(1234))
	s.janusProxy.EXPECT().GetRoomMeta(roomID).Return(nil)

	res, err := s.server.handleOffer(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "no room found")
}

func (s *ServerSuite) TestHandleIceCandidate_Success() {
	ctx := context.Background()
	roomID := "room1"

	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: roomID,
		userID: "user1",
		joined: true,
		janus:  inst,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	candidate := janus.ICECandidate{Candidate: "candidate:..."}
	params, _ := json.Marshal(map[string]any{
		"candidate": candidate,
	})
	rawParams := json.RawMessage(params)

	res, err := s.server.handleIceCandidate(mctx, &rawParams)
	s.Require().NoError(err)
	s.Nil(res)
}

func (s *ServerSuite) TestHandleIceCandidate_JanusError() {
	ctx := context.Background()
	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: "room1",
		joined: true,
		janus:  inst,
	}
	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	candidate := janus.ICECandidate{Candidate: "candidate:..."}
	params, _ := json.Marshal(map[string]any{
		"candidate": candidate,
	})
	rawParams := json.RawMessage(params)

	s.failJanus = true

	_, err = s.server.handleIceCandidate(mctx, &rawParams)
	s.Require().Error(err)
}

func (s *ServerSuite) TestHandleIceCandidate_InvalidParams() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: true,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	// Invalid JSON
	invalidParams := json.RawMessage(`{invalid json}`)

	res, err := s.server.handleIceCandidate(mctx, &invalidParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "invalid ice candidate parameters")
}

func (s *ServerSuite) TestHandleIceCandidate_MissingCandidate() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: true,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{})
	rawParams := json.RawMessage(params)

	res, err := s.server.handleIceCandidate(mctx, &rawParams)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "invalid ice candidate parameters")
}

func (s *ServerSuite) TestHandleKeepAlive_Success() {
	ctx := context.Background()
	roomID := "room1"
	userID := "user1"
	connID := "conn1"
	clientID := "client1"

	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx:   ctx,
		roomID:   roomID,
		userID:   userID,
		connID:   connID,
		clientID: clientID,
		joined:   true,
		janus:    inst,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{
		"status": constants.AnchorStatusOnAir,
	})
	rawParams := json.RawMessage(params)

	s.connGuard.EXPECT().GetServerID().Return("test-server").AnyTimes()
	s.connGuard.EXPECT().MustHold(mctx).Return(true, nil)
	s.userService.EXPECT().SetUserStatus(gomock.Any(), roomID, userID, constants.AnchorStatusOnAir, gomock.Any()).Return(nil)

	res, err := s.server.handleKeepAlive(mctx, &rawParams)
	s.Require().NoError(err)
	s.Nil(res)
}

func (s *ServerSuite) TestHandleKeepAlive_JanusError() {
	ctx := context.Background()
	inst, err := s.realJanusAPI.CreateAnchorInstance(ctx, "client1", 0, 0)
	s.Require().NoError(err)

	rtcCtx := &rtcContext{
		reqCtx: ctx,
		roomID: "room1",
		joined: true,
		janus:  inst,
	}
	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	params, _ := json.Marshal(map[string]any{})
	rawParams := json.RawMessage(params)

	s.failJanus = true

	_, err = s.server.handleKeepAlive(mctx, &rawParams)
	s.Require().Error(err)
}

func (s *ServerSuite) TestHandleKeepAlive_NotJoined() {
	ctx := context.Background()
	rtcCtx := &rtcContext{
		reqCtx: ctx,
		joined: false,
	}

	mctx := &mockMethodCtx{rtcCtx: rtcCtx}

	res, err := s.server.handleKeepAlive(mctx, nil)
	s.Require().Error(err)
	s.Nil(res)
	s.Contains(err.Error(), "not joined yet")
}

func (s *ServerSuite) TestUpdateUserStatus_Error() {
	ctx := context.Background()

	s.userService.EXPECT().SetUserStatus(gomock.Any(), "room1", "user1", constants.AnchorStatusOnAir, gomock.Any()).Return(fmt.Errorf("error"))

	s.server.updateUserStatus(ctx, "room1", "user1", constants.AnchorStatusOnAir)
}
