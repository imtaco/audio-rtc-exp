package signal

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	wsrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/websocket"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	jwtmocks "github.com/imtaco/audio-rtc-exp/internal/jwt/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type WSHookSuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	logger        *log.Logger
	connGuard     *MockConnectionGuard
	clientManager *wsConnManager
	jwtAuth       *jwtmocks.MockJWTAuth
	hook          wsrpc.ConnectionHooks[rtcContext]
}

func TestWSHookSuite(t *testing.T) {
	suite.Run(t, new(WSHookSuite))
}

func (s *WSHookSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.logger = log.NewNop()
	s.connGuard = NewMockConnectionGuard(s.ctrl)
	s.jwtAuth = jwtmocks.NewMockJWTAuth(s.ctrl)

	s.clientManager = &wsConnManager{
		room2clients: make(map[string]map[string]jsonrpc.Conn[rtcContext]),
		client2room:  make(map[string]string),
		logger:       s.logger,
	}

	s.hook = NewWSHook(
		s.clientManager,
		s.connGuard,
		s.jwtAuth,
		s.logger,
	)
}

func (s *WSHookSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *WSHookSuite) TestOnVerify_Success() {
	req := httptest.NewRequest("GET", "/?token=valid-token", nil)

	s.jwtAuth.EXPECT().Verify("valid-token").Return(&jwt.JWTPayload{
		UserID: "user1",
		RoomID: "room1",
	}, nil)

	ctx, pass, err := s.hook.OnVerify(req)
	s.NoError(err)
	s.True(pass)
	s.Equal("user1", ctx.userID)
	s.Equal("room1", ctx.roomID)
}

func (s *WSHookSuite) TestOnVerify_BearerToken() {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	s.jwtAuth.EXPECT().Verify("valid-token").Return(&jwt.JWTPayload{
		UserID: "user1",
		RoomID: "room1",
	}, nil)

	ctx, pass, err := s.hook.OnVerify(req)
	s.NoError(err)
	s.True(pass)
	s.Equal("user1", ctx.userID)
}

func (s *WSHookSuite) TestOnVerify_Failures() {
	// No token
	req := httptest.NewRequest("GET", "/", nil)
	_, pass, err := s.hook.OnVerify(req)
	s.NoError(err)
	s.False(pass)

	// Invalid token
	req = httptest.NewRequest("GET", "/?token=bad", nil)
	s.jwtAuth.EXPECT().Verify("bad").Return(nil, errors.New(jwt.ErrInvalidToken, "invalid token"))
	_, pass, err = s.hook.OnVerify(req)
	s.NoError(err)
	s.False(pass)
}

func (s *WSHookSuite) TestOnConnect_Success() {
	mctx := &mockMethodCtx{
		rtcCtx: &rtcContext{
			userID: "user1",
			roomID: "room1",
			connID: "nonce1",
			reqCtx: context.Background(),
		},
		peer: &mockPeer{},
	}

	s.connGuard.EXPECT().MustHold(mctx).Return(true, nil)
	s.hook.OnConnect(mctx)

	// Check if added to manager
	conns := s.clientManager.getRoomConns("room1")
	s.Len(conns, 1)
}

func (s *WSHookSuite) TestOnConnect_LockFail() {
	mctx := &mockMethodCtx{
		rtcCtx: &rtcContext{
			userID: "user1",
			roomID: "room1",
			reqCtx: context.Background(),
		},
	}

	// Acquire returns false (already locked)
	s.connGuard.EXPECT().MustHold(mctx).Return(false, nil)

	s.hook.OnConnect(mctx)
}

func (s *WSHookSuite) TestOnDisconnect() {
	connID := uuid.New().String()
	mctx := &mockMethodCtx{
		rtcCtx: &rtcContext{
			userID: "user1",
			roomID: "room1",
			connID: connID,
			reqCtx: context.Background(),
		},
	}

	// Pre-add client
	s.clientManager.AddClient(connID, "room1", &mockPeer{})
	s.connGuard.EXPECT().Release(mctx).Return(nil)

	s.hook.OnDisconnect(mctx, 1000)

	conns := s.clientManager.getRoomConns("room1")
	s.Len(conns, 0)
}
