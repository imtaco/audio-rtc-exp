package signal

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type ConnLockSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	miniRedis *miniredis.Miniredis
	client    *redis.Client
	guard     ConnectionGuard
	logger    *log.Logger
}

func TestConnLockSuite(t *testing.T) {
	suite.Run(t, new(ConnLockSuite))
}

func (s *ConnLockSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	mr, err := miniredis.Run()
	s.Require().NoError(err)
	s.miniRedis = mr

	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	s.logger = log.NewNop()
	s.guard = NewConnGuard(s.client, "test", "server1", s.logger)

	// Start heartbeat so server is considered "alive" for lock conflict tests
	err = s.guard.Start(context.Background())
	s.Require().NoError(err)
}

func (s *ConnLockSuite) TearDownTest() {
	if s.guard != nil {
		s.guard.Stop()
	}
	if s.client != nil {
		s.client.Close()
	}
	if s.miniRedis != nil {
		s.miniRedis.Close()
	}
}

func (s *ConnLockSuite) TestMustHold_Success() {
	ctx := context.Background()
	rtcCtx := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx := jsonrpc.NewContext(conn, &rtcCtx)

	ok, err := s.guard.MustHold(mctx)
	s.NoError(err)
	s.True(ok)

	value, err := s.client.Get(ctx, "test:c:user1").Result()
	s.NoError(err)
	s.Equal("server1:nonce1", value)
}

func (s *ConnLockSuite) TestMustHold_AlreadyLocked() {
	ctx := context.Background()
	rtcCtx1 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn1 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx1 := jsonrpc.NewContext(conn1, &rtcCtx1)

	rtcCtx2 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce2",
	}
	conn2 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx2 := jsonrpc.NewContext(conn2, &rtcCtx2)
	conn2.EXPECT().Close().Return(nil)

	ok, err := s.guard.MustHold(mctx1)
	s.NoError(err)
	s.True(ok)

	ok, err = s.guard.MustHold(mctx2)
	s.NoError(err)
	s.False(ok)

	value, err := s.client.Get(ctx, "test:c:user1").Result()
	s.NoError(err)
	s.Equal("server1:nonce1", value)
}

func (s *ConnLockSuite) TestMustHold_Reacquire() {
	rtcCtx := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx := jsonrpc.NewContext(conn, &rtcCtx)

	ok, err := s.guard.MustHold(mctx)
	s.NoError(err)
	s.True(ok)

	ok, err = s.guard.MustHold(mctx)
	s.NoError(err)
	s.True(ok)
}

func (s *ConnLockSuite) TestMustHold_WrongNonce() {
	ctx := context.Background()
	rtcCtx1 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn1 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx1 := jsonrpc.NewContext(conn1, &rtcCtx1)

	rtcCtx2 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce2",
	}
	conn2 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx2 := jsonrpc.NewContext(conn2, &rtcCtx2)
	conn2.EXPECT().Close().Return(nil)

	ok, err := s.guard.MustHold(mctx1)
	s.NoError(err)
	s.True(ok)

	ok, err = s.guard.MustHold(mctx2)
	s.NoError(err)
	s.False(ok)

	// still exists with original value
	value, err := s.client.Get(ctx, "test:c:user1").Result()
	s.NoError(err)
	s.Equal("server1:nonce1", value)
}

func (s *ConnLockSuite) TestRelease_Success() {
	ctx := context.Background()
	rtcCtx := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx := jsonrpc.NewContext(conn, &rtcCtx)

	ok, err := s.guard.MustHold(mctx)
	s.NoError(err)
	s.True(ok)

	err = s.guard.Release(mctx)
	s.NoError(err)

	_, err = s.client.Get(ctx, "test:c:user1").Result()
	s.Equal(redis.Nil, err)
}

func (s *ConnLockSuite) TestRelease_WrongNonce() {
	ctx := context.Background()
	rtcCtx1 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn1 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx1 := jsonrpc.NewContext(conn1, &rtcCtx1)

	rtcCtx2 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce2",
	}
	conn2 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx2 := jsonrpc.NewContext(conn2, &rtcCtx2)

	ok, err := s.guard.MustHold(mctx1)
	s.NoError(err)
	s.True(ok)

	err = s.guard.Release(mctx2)
	s.NoError(err)

	// still exists with original value
	value, err := s.client.Get(ctx, "test:c:user1").Result()
	s.NoError(err)
	s.Equal("server1:nonce1", value)
}

func (s *ConnLockSuite) TestRelease_NotExists() {
	rtcCtx := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx := jsonrpc.NewContext(conn, &rtcCtx)

	err := s.guard.Release(mctx)
	s.NoError(err)
}

func (s *ConnLockSuite) TestMustHold_ServerStopped() {
	ctx := context.Background()

	lock1 := NewConnGuard(s.client, "test", "server1", s.logger)
	rtcCtx1 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce1",
	}
	conn1 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx1 := jsonrpc.NewContext(conn1, &rtcCtx1)

	err := lock1.Start(ctx)
	s.NoError(err)

	ok, err := lock1.MustHold(mctx1)
	s.NoError(err)
	s.True(ok)

	lock1.Stop()

	lock2 := NewConnGuard(s.client, "test", "server2", s.logger)
	rtcCtx2 := rtcContext{
		reqCtx: context.Background(),
		userID: "user1",
		connID: "nonce2",
	}
	conn2 := mocks.NewMockPeer[rtcContext](s.ctrl)
	mctx2 := jsonrpc.NewContext(conn2, &rtcCtx2)

	ok, err = lock2.MustHold(mctx2)
	s.NoError(err)
	s.True(ok)

	value, err := s.client.Get(ctx, "test:c:user1").Result()
	s.NoError(err)
	s.Equal("server2:nonce2", value)
}
