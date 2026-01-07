package status

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	jsonrpcmocks "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	jwtmocks "github.com/imtaco/audio-rtc-exp/internal/jwt/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/users"
)

type UserServiceUnitTestSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockPeer *jsonrpcmocks.MockPeer[any]
	jwtAuth  jwt.Auth
	svc      *userServiceImpl
	ctx      context.Context
}

func TestUserServiceUnitSuite(t *testing.T) {
	suite.Run(t, new(UserServiceUnitTestSuite))
}

func (s *UserServiceUnitTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockPeer = jsonrpcmocks.NewMockPeer[any](s.ctrl)
	s.jwtAuth = jwt.NewAuth("test-secret-key")
	s.ctx = context.Background()

	s.svc = &userServiceImpl{
		peerSvc: s.mockPeer,
		jwtAuth: s.jwtAuth,
		logger:  log.NewNop(),
	}
}

func (s *UserServiceUnitTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *UserServiceUnitTestSuite) TestCreateUser() {
	s.Run("create user successfully", func() {
		// Expect Call with correct parameters
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "createUser", gomock.Any(), nil).
			DoAndReturn(func(_ context.Context, _ string, params, _ any) error {
				// Verify the request parameters
				req, ok := params.(*users.CreateUserRequest)
				s.Require().True(ok, "params should be *createUserRequest")
				s.Equal("room1", req.RoomID)
				s.Equal("user1", req.UserID)
				s.Equal("anchor", req.Role)
				s.WithinDuration(time.Now(), req.TS, 1*time.Second)
				return nil
			})

		userID, token, err := s.svc.CreateUser(s.ctx, "room1", "user1", "anchor")

		s.Require().NoError(err)
		s.Equal("user1", userID)
		s.NotEmpty(token)

		// Verify JWT token
		claims, err := s.jwtAuth.Verify(token)
		s.Require().NoError(err)
		s.Equal("user1", claims.UserID)
		s.Equal("room1", claims.RoomID)
	})

	s.Run("RPC call fails", func() {
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "createUser", gomock.Any(), nil).
			Return(context.DeadlineExceeded)

		_, _, err := s.svc.CreateUser(s.ctx, "room2", "user2", "viewer")

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to create user")
	})
}

func (s *UserServiceUnitTestSuite) TestDeleteUser() {
	s.Run("delete user successfully", func() {
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "deleteUser", gomock.Any(), nil).
			DoAndReturn(func(_ context.Context, _ string, params, _ any) error {
				req, ok := params.(*users.DeleteUserRequest)
				s.Require().True(ok, "params should be *deleteUserRequest")
				s.Equal("room1", req.RoomID)
				s.Equal("user1", req.UserID)
				s.WithinDuration(time.Now(), req.TS, 1*time.Second)
				return nil
			})

		err := s.svc.DeleteUser(s.ctx, "room1", "user1")

		s.Require().NoError(err)
	})

	s.Run("RPC call fails", func() {
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "deleteUser", gomock.Any(), nil).
			Return(context.Canceled)

		err := s.svc.DeleteUser(s.ctx, "room2", "user2")

		s.Require().Error(err)
		s.Contains(err.Error(), "failed to delete user")
	})
}

func (s *UserServiceUnitTestSuite) TestSetUserStatus() {
	s.Run("set status successfully", func() {
		s.mockPeer.EXPECT().
			Notify(gomock.Any(), "setUserStatus", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params any) error {
				req, ok := params.(*users.SetStatusUserRequest)
				s.Require().True(ok, "params should be *SetStatusUserRequest")
				s.Equal("room1", req.RoomID)
				s.Equal("user1", req.UserID)
				s.Equal(constants.AnchorStatusOnAir, req.Status)
				s.Equal(int32(1), req.Gen)
				s.WithinDuration(time.Now(), req.TS, 1*time.Second)
				return nil
			})

		err := s.svc.SetUserStatus(s.ctx, "room1", "user1", constants.AnchorStatusOnAir, 1)

		s.Require().NoError(err)
	})

	s.Run("notify fails", func() {
		s.mockPeer.EXPECT().
			Notify(gomock.Any(), "setUserStatus", gomock.Any()).
			Return(context.DeadlineExceeded)

		err := s.svc.SetUserStatus(s.ctx, "room2", "user2", constants.AnchorStatusLeft, 2)

		s.Require().Error(err)
	})

	s.Run("empty status", func() {
		s.mockPeer.EXPECT().
			Notify(gomock.Any(), "setUserStatus", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params any) error {
				req := params.(*users.SetStatusUserRequest)
				s.Equal(constants.AnchorStatus(""), req.Status)
				return nil
			})

		err := s.svc.SetUserStatus(s.ctx, "room1", "user1", constants.AnchorStatus(""), 3)

		s.Require().NoError(err)
	})
}

func (s *UserServiceUnitTestSuite) TestCreateUserRequestMarshaling() {
	s.Run("request can be marshaled to JSON", func() {
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "createUser", gomock.Any(), nil).
			DoAndReturn(func(_ context.Context, _ string, params, _ any) error {
				// Verify the struct can be marshaled
				data, err := json.Marshal(params)
				s.Require().NoError(err)
				s.Contains(string(data), "room1")
				s.Contains(string(data), "user1")
				s.Contains(string(data), "anchor")
				return nil
			})

		_, _, err := s.svc.CreateUser(s.ctx, "room1", "user1", "anchor")
		s.Require().NoError(err)
	})
}

func (s *UserServiceUnitTestSuite) TestMultipleOperations() {
	s.Run("multiple operations in sequence", func() {
		// Create user
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "createUser", gomock.Any(), nil).
			Return(nil)

		_, token, err := s.svc.CreateUser(s.ctx, "room1", "user1", "anchor")
		s.Require().NoError(err)
		s.NotEmpty(token)

		// Set status
		s.mockPeer.EXPECT().
			Notify(gomock.Any(), "setUserStatus", gomock.Any()).
			Return(nil)

		err = s.svc.SetUserStatus(s.ctx, "room1", "user1", "streaming", 1)
		s.Require().NoError(err)

		// Delete user
		s.mockPeer.EXPECT().
			Call(gomock.Any(), "deleteUser", gomock.Any(), nil).
			Return(nil)

		err = s.svc.DeleteUser(s.ctx, "room1", "user1")
		s.Require().NoError(err)
	})
}

func TestNewUserService(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	jwtAuth := jwt.NewAuth("test-secret-key")
	logger := log.NewNop()

	t.Run("create service successfully", func(t *testing.T) {
		svc, err := NewUserService(redisClient, jwtAuth, "stream-in", "stream-out", logger)
		assert.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("create service with empty stream names", func(t *testing.T) {
		svc, err := NewUserService(redisClient, jwtAuth, "", "", logger)
		assert.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestUserIsActive(t *testing.T) {
	t.Run("active user - recent timestamp", func(t *testing.T) {
		user := &users.User{
			Role:   "anchor",
			Status: constants.AnchorStatusOnAir,
			TS:     time.Now(),
			Gen:    1,
		}
		assert.True(t, user.IsActive())
	})

	t.Run("inactive user - old timestamp", func(t *testing.T) {
		user := &users.User{
			Role:   "anchor",
			Status: constants.AnchorStatusOnAir,
			TS:     time.Now().Add(-users.UserStatusTimeout - time.Second),
			Gen:    1,
		}
		assert.False(t, user.IsActive())
	})

	t.Run("nil user", func(t *testing.T) {
		var user *users.User
		assert.False(t, user.IsActive())
	})

	t.Run("user at boundary - exactly timeout", func(t *testing.T) {
		user := &users.User{
			Role:   "viewer",
			Status: constants.AnchorStatusLeft,
			TS:     time.Now().Add(-users.UserStatusTimeout),
			Gen:    2,
		}
		assert.False(t, user.IsActive())
	})

	t.Run("user just under timeout", func(t *testing.T) {
		user := &users.User{
			Role:   "anchor",
			Status: constants.AnchorStatusOnAir,
			TS:     time.Now().Add(-users.UserStatusTimeout + time.Second),
			Gen:    1,
		}
		assert.True(t, user.IsActive())
	})
}

func TestCreateUserJWTSigningFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPeer := jsonrpcmocks.NewMockPeer[any](ctrl)
	mockJWT := jwtmocks.NewMockAuth(ctrl)
	ctx := context.Background()

	svc := &userServiceImpl{
		peerSvc: mockPeer,
		jwtAuth: mockJWT,
		logger:  log.NewNop(),
	}

	t.Run("JWT signing fails after successful RPC call", func(t *testing.T) {
		mockPeer.EXPECT().
			Call(gomock.Any(), "createUser", gomock.Any(), nil).
			Return(nil)

		mockJWT.EXPECT().
			Sign("user1", "room1").
			Return("", assert.AnError)

		_, _, err := svc.CreateUser(ctx, "room1", "user1", "anchor")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to sign JWT")
	})
}
