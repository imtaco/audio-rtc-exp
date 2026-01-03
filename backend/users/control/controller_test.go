package control

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	redisrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/redis"
	log "github.com/imtaco/audio-rtc-exp/internal/log"
	etcdmocks "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd/mocks"
	"github.com/imtaco/audio-rtc-exp/users"
	"github.com/imtaco/audio-rtc-exp/users/mocks"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type UserStatusControlTestSuite struct {
	suite.Suite
	ctrl            *UserStatusControl
	redisClient     *redis.Client
	mr              *miniredis.Miniredis
	ctx             context.Context
	mockRoomState   *mocks.MockRoomsState
	mockRoomWatcher *etcdmocks.MockRoomWatcher
	gomockCtrl      *gomock.Controller
}

func (s *UserStatusControlTestSuite) SetupTest() {
	logger := log.NewNop()

	mr, err := miniredis.Run()
	s.Require().NoError(err)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	s.gomockCtrl = gomock.NewController(s.T())
	s.mockRoomState = mocks.NewMockRoomsState(s.gomockCtrl)
	s.mockRoomWatcher = etcdmocks.NewMockRoomWatcher(s.gomockCtrl)

	peer2svc, err := redisrpc.NewPeer[interface{}](
		redisClient,
		"test:stream:reply",
		"test:stream:input",
		"test-controller",
		logger,
	)
	s.Require().NoError(err)

	peer2ws, err := redisrpc.NewPeer[interface{}](
		redisClient,
		"test:ws:stream",
		"",
		"",
		logger,
	)
	s.Require().NoError(err)

	ctrl := &UserStatusControl{
		roomState:           s.mockRoomState,
		roomWatcher:         s.mockRoomWatcher,
		peer2svc:            peer2svc,
		peer2ws:             peer2ws,
		userEventCh:         make(chan *userEvent, 10),
		logger:              logger,
		expireCheckInterval: defaultExpireCheckInterval,
	}

	s.ctrl = ctrl
	s.redisClient = redisClient
	s.mr = mr
	s.ctx = context.Background()
}

func (s *UserStatusControlTestSuite) TearDownTest() {
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.mr != nil {
		s.mr.Close()
	}
	if s.gomockCtrl != nil {
		s.gomockCtrl.Finish()
	}
}

func TestUserStatusControlSuite(t *testing.T) {
	suite.Run(t, new(UserStatusControlTestSuite))
}

func (s *UserStatusControlTestSuite) TestNewUserStatusControl() {
	s.Require().NotNil(s.ctrl.roomState)
	s.Assert().NotNil(s.ctrl.peer2svc)
	s.Assert().NotNil(s.ctrl.peer2ws)
	s.Assert().NotNil(s.ctrl.userEventCh)
}

func (s *UserStatusControlTestSuite) TestRebuildState() {
	s.mockRoomState.EXPECT().Rebuild(s.ctx).Return(nil)
	err := s.ctrl.rebuildState(s.ctx)
	s.Require().NoError(err)
}

func (s *UserStatusControlTestSuite) TestHandleCreate() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	s.Run("handle create user request", func() {
		req := &users.CreateUserRequest{
			RoomID: "room1",
			UserID: "user1",
			Role:   "anchor",
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		reply := func(result interface{}, err error) {
			replyCalled = true
			s.Assert().NoError(err)
		}

		// Mock room watcher
		roomState := &etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				MaxAnchors: 5,
			},
		}
		s.mockRoomWatcher.EXPECT().GetCachedState(req.RoomID).Return(roomState, true)

		// Expect GetRoomUsers to check current count
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), req.RoomID).Return(map[string]users.User{})

		// Expect CreateUser call
		s.mockRoomState.EXPECT().CreateUser(gomock.Any(), req.RoomID, req.UserID, gomock.Any()).Return(true, nil)

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleCreate(methodCtx, &rawParams, reply)

		select {
		case event := <-s.ctrl.userEventCh:
			err := event.action(ctx)
			s.Assert().NoError(err)
		case <-time.After(1 * time.Second):
			s.T().Fatal("timeout waiting for event")
		}

		s.Assert().True(replyCalled)
	})

	s.Run("handle create with invalid JSON", func() {
		invalidParams := json.RawMessage(`{invalid json}`)

		replyCalled := false
		var replyErr error
		reply := func(result interface{}, err error) {
			replyCalled = true
			replyErr = err
		}

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleCreate(methodCtx, &invalidParams, reply)

		s.Assert().True(replyCalled)
		s.Assert().Error(replyErr)
	})

	s.Run("handle create when room not found", func() {
		req := &users.CreateUserRequest{
			RoomID: "nonexistent",
			UserID: "user1",
			Role:   "anchor",
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		var replyErr error
		reply := func(result interface{}, err error) {
			replyCalled = true
			replyErr = err
		}

		// Mock room watcher returning room not found
		s.mockRoomWatcher.EXPECT().GetCachedState(req.RoomID).Return(nil, false)

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleCreate(methodCtx, &rawParams, reply)

		s.Assert().True(replyCalled)
		s.Assert().Error(replyErr)
	})

	s.Run("handle create when maxAnchors limit reached", func() {
		req := &users.CreateUserRequest{
			RoomID: "room1",
			UserID: "user6",
			Role:   "anchor",
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		var replyErr error
		reply := func(result interface{}, err error) {
			replyCalled = true
			replyErr = err
		}

		// Mock room watcher with maxAnchors = 5
		roomState := &etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				MaxAnchors: 5,
			},
		}
		s.mockRoomWatcher.EXPECT().GetCachedState(req.RoomID).Return(roomState, true)

		// Mock current room has 5 users already
		existingUsers := map[string]users.User{
			"user1": {Role: "anchor"},
			"user2": {Role: "anchor"},
			"user3": {Role: "anchor"},
			"user4": {Role: "anchor"},
			"user5": {Role: "anchor"},
		}
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), req.RoomID).Return(existingUsers)

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleCreate(methodCtx, &rawParams, reply)

		select {
		case event := <-s.ctrl.userEventCh:
			err := event.action(ctx)
			s.Assert().NoError(err)
		case <-time.After(1 * time.Second):
			s.T().Fatal("timeout waiting for event")
		}

		s.Assert().True(replyCalled)
		s.Assert().Error(replyErr)
		s.Assert().Contains(replyErr.Error(), "reached max anchors limit")
	})

	s.Run("handle create with exactly maxAnchors users (edge case)", func() {
		req := &users.CreateUserRequest{
			RoomID: "room1",
			UserID: "user3",
			Role:   "anchor",
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		reply := func(result interface{}, err error) {
			replyCalled = true
			s.Assert().NoError(err)
		}

		// Mock room watcher with maxAnchors = 3
		roomState := &etcdstate.RoomState{
			Meta: &etcdstate.Meta{
				MaxAnchors: 3,
			},
		}
		s.mockRoomWatcher.EXPECT().GetCachedState(req.RoomID).Return(roomState, true)

		// Mock current room has 2 users (one slot available)
		existingUsers := map[string]users.User{
			"user1": {Role: "anchor"},
			"user2": {Role: "anchor"},
		}
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), req.RoomID).Return(existingUsers)

		// Expect CreateUser to be called since we haven't reached the limit yet
		s.mockRoomState.EXPECT().CreateUser(gomock.Any(), req.RoomID, req.UserID, gomock.Any()).Return(true, nil)

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleCreate(methodCtx, &rawParams, reply)

		select {
		case event := <-s.ctrl.userEventCh:
			err := event.action(ctx)
			s.Assert().NoError(err)
		case <-time.After(1 * time.Second):
			s.T().Fatal("timeout waiting for event")
		}

		s.Assert().True(replyCalled)
	})
}

func (s *UserStatusControlTestSuite) TestHandleDelete() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := &users.DeleteUserRequest{
		RoomID: "room1",
		UserID: "user1",
		TS:     time.Now(),
	}

	params, err := json.Marshal(req)
	s.Require().NoError(err)

	rawParams := json.RawMessage(params)

	replyCalled := false
	reply := func(result interface{}, err error) {
		replyCalled = true
	}

	// Expect RemoveUser call
	s.mockRoomState.EXPECT().RemoveUser(gomock.Any(), req.RoomID, req.UserID).Return(true, nil)
	// Expect GetRoomUsers call (for notification)
	s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), req.RoomID).Return(map[string]users.User{})

	methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
	s.ctrl.handleDelete(methodCtx, &rawParams, reply)

	select {
	case event := <-s.ctrl.userEventCh:
		err := event.action(ctx)
		s.Assert().NoError(err)
	case <-time.After(1 * time.Second):
		s.T().Fatal("timeout waiting for event")
	}

	s.Assert().True(replyCalled)
}

func (s *UserStatusControlTestSuite) TestHandleSetStatus() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	s.Run("handle set status request", func() {
		req := &users.SetStatusUserRequest{
			RoomID: "room1",
			UserID: "user1",
			Status: constants.AnchorStatusOnAir,
			Gen:    1,
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		reply := func(result interface{}, err error) {
			replyCalled = true
		}

		// Expect UpdateUserStatus call
		s.mockRoomState.EXPECT().UpdateUserStatus(gomock.Any(), req.RoomID, req.UserID, gomock.Any()).Return(true, nil)
		// Expect GetRoomUsers call (for notification)
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), req.RoomID).Return(map[string]users.User{
			"user1": {Status: constants.AnchorStatusOnAir, TS: time.Now()},
		})

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleSetStatus(methodCtx, &rawParams, reply)

		select {
		case event := <-s.ctrl.userEventCh:
			err := event.action(ctx)
			s.Assert().NoError(err)
		case <-time.After(1 * time.Second):
			s.T().Fatal("timeout waiting for event")
		}

		s.Assert().True(replyCalled)
	})

	s.Run("handle set status for non-existent user", func() {
		req := &users.SetStatusUserRequest{
			RoomID: "room1",
			UserID: "user999",
			Status: constants.AnchorStatusOnAir,
			Gen:    1,
			TS:     time.Now(),
		}

		params, err := json.Marshal(req)
		s.Require().NoError(err)

		rawParams := json.RawMessage(params)

		replyCalled := false
		reply := func(result interface{}, err error) {
			replyCalled = true
		}

		// Expect UpdateUserStatus call returning false (not updated)
		s.mockRoomState.EXPECT().UpdateUserStatus(gomock.Any(), req.RoomID, req.UserID, gomock.Any()).Return(false, nil)

		methodCtx := jsonrpc.NewContext[interface{}](nil, nil)
		s.ctrl.handleSetStatus(methodCtx, &rawParams, reply)

		select {
		case event := <-s.ctrl.userEventCh:
			event.action(ctx)
		case <-time.After(1 * time.Second):
			s.T().Fatal("timeout waiting for event")
		}

		s.Assert().True(replyCalled)
	})
}

func (s *UserStatusControlTestSuite) TestNotifyUserStatus() {
	s.Run("notify with active users", func() {
		roomID := "room1"
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), roomID).Return(map[string]users.User{
			"user1": {Status: constants.AnchorStatusOnAir, TS: time.Now()},
		})

		err := s.ctrl.notifyUserStatus(s.ctx, roomID)
		s.Assert().NoError(err)
	})

	s.Run("notify with no active users", func() {
		roomID := "room999"
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), roomID).Return(map[string]users.User{})

		err := s.ctrl.notifyUserStatus(s.ctx, roomID)
		s.Assert().NoError(err)
	})
}

func (s *UserStatusControlTestSuite) TestStop() {
	s.mockRoomWatcher.EXPECT().Stop().Return(nil)
	err := s.ctrl.Stop()
	s.Assert().NoError(err)
}

func (s *UserStatusControlTestSuite) TestUserEvent() {
	now := time.Now()
	called := false

	event := &userEvent{
		action: func(ctx context.Context) error {
			called = true
			return nil
		},
		ts: now,
	}

	s.Assert().Equal(now, event.ts)
	s.Assert().NotNil(event.action)

	err := event.action(context.Background())
	s.Assert().NoError(err)
	s.Assert().True(called)
}

func (s *UserStatusControlTestSuite) TestRegisterRPC() {
	s.ctrl.registerRPC()
}

func (s *UserStatusControlTestSuite) TestLoop() {
	s.Run("loop processes events", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go s.ctrl.loop(ctx)

		called := make(chan bool, 1)
		event := &userEvent{
			action: func(ctx context.Context) error {
				called <- true
				return nil
			},
			ts: time.Now(),
		}

		s.ctrl.userEventCh <- event

		select {
		case <-called:
			// Success
		case <-time.After(1 * time.Second):
			s.T().Fatal("loop did not process event")
		}
	})

	s.Run("loop exits on context cancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			s.ctrl.loop(ctx)
			close(done)
		}()

		cancel()

		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			s.T().Fatal("loop did not exit on context cancellation")
		}
	})

	s.Run("loop processes expiration", func() {
		// Set a very small interval for testing
		s.ctrl.expireCheckInterval = 10 * time.Millisecond

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Expect CheckTimeout to be called
		s.mockRoomState.EXPECT().CheckTimeout(gomock.Any()).Return([]string{"room1"}, nil).MinTimes(1)

		// Expect GetRoomUsers to be called for the expired room (inside notifyUserStatus)
		s.mockRoomState.EXPECT().GetRoomUsers(gomock.Any(), "room1").Return(map[string]users.User{}).MinTimes(1)

		go s.ctrl.loop(ctx)

		// Wait a bit to let at least one tick happen
		time.Sleep(50 * time.Millisecond)
	})
}
