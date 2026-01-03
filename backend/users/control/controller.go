package control

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcd"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	redisrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/redis"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	etcdwatcher "github.com/imtaco/audio-rtc-exp/internal/reswatcher/etcd"
	"github.com/imtaco/audio-rtc-exp/users"

	"github.com/redis/go-redis/v9"
)

const (
	defaultExpireCheckInterval = 10 * time.Second
)

// Only one controller instance is expected to run in the system
type UserStatusControl struct {
	roomState   users.RoomsState
	roomWatcher etcdwatcher.RoomWatcher
	// rpc
	peer2svc            jsonrpc.Peer[interface{}]
	peer2ws             jsonrpc.Peer[interface{}]
	userEventCh         chan *userEvent
	logger              *log.Logger
	expireCheckInterval time.Duration
}

type userEvent struct {
	action func(ctx context.Context) error
	ts     time.Time
}

func NewUserStatusControl(
	redisClient *redis.Client,
	etcdClient etcd.Client,
	roomState users.RoomsState,
	etcdPrefixRoom string,
	streamIn string,
	streamReply string,
	wsStreamName string,
	logger *log.Logger,
) (*UserStatusControl, error) {

	// TODO: use forever client ?
	peer2svc, err := redisrpc.NewPeer[interface{}](
		redisClient,
		streamReply,
		streamIn,
		"user-status-controller",
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create WS RPC peer: %w", err)
	}

	roomWatcher := etcdwatcher.NewRoomWatcher(
		etcdClient,
		etcdPrefixRoom,
		[]string{constants.RoomKeyMeta},
		nil,
		logger.Module("Room"),
	)

	peer2ws, err := redisrpc.NewPeer[interface{}](
		redisClient,
		wsStreamName,
		"",
		"",
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC peer: %w", err)
	}

	return &UserStatusControl{
		roomState:           roomState,
		roomWatcher:         roomWatcher,
		peer2svc:            peer2svc,
		peer2ws:             peer2ws,
		userEventCh:         make(chan *userEvent, 10),
		logger:              logger,
		expireCheckInterval: defaultExpireCheckInterval,
	}, nil
}

func (c *UserStatusControl) Start(ctx context.Context) error {
	c.logger.Info("Starting")

	// Rebuild state from Redis
	if err := c.rebuildState(ctx); err != nil {
		return err
	}

	c.registerRPC()
	if err := c.roomWatcher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start room watcher: %w", err)
	}
	if err := c.peer2svc.Open(ctx); err != nil {
		return fmt.Errorf("failed to start svc RPC peer: %w", err)
	}
	if err := c.peer2ws.Open(ctx); err != nil {
		return fmt.Errorf("failed to start WS RPC peer: %w", err)
	}

	go c.loop(ctx)
	return nil
}

func (c *UserStatusControl) registerRPC() {
	c.peer2svc.DefAsync("createUser", c.handleCreate)
	c.peer2svc.DefAsync("deleteUser", c.handleDelete)
	c.peer2svc.DefAsync("setUserStatus", c.handleSetStatus)
}

func (c *UserStatusControl) handleCreate(
	_ jsonrpc.MethodContext[interface{}],
	params *json.RawMessage,
	reply jsonrpc.Reply,
) {

	req := users.CreateUserRequest{}
	if err := jsonrpc.ShouldBindParams(params, &req); err != nil {
		reply(nil, err)
		return
	}

	c.logger.Debug("receive create user RPC",
		log.String("roomId", req.RoomID),
		log.String("userId", req.UserID),
		log.Time("ts", req.TS),
	)

	room, ok := c.roomWatcher.GetCachedState(req.RoomID)
	if !ok {
		c.logger.Warn("Room not found",
			log.String("roomId", req.RoomID),
		)
		reply(nil, jsonrpc.ErrInvalidRequest("room not found"))
		return
	}
	maxAnchors := room.GetMeta().GetMaxAnchors()

	action := func(ctx context.Context) error {
		// Check current anchors count
		currentUsers := c.roomState.GetRoomUsers(ctx, req.RoomID)
		if len(currentUsers) >= maxAnchors {
			c.logger.Warn("Reached max anchors limit",
				log.String("roomId", req.RoomID),
				log.Int("currentUsers", len(currentUsers)),
				log.Int("maxAnchors", maxAnchors),
			)
			reply(nil, jsonrpc.ErrInvalidRequest("reached max anchors limit"))
			return nil
		}

		u := &users.User{
			Role: req.Role,
			Gen:  0,
			TS:   req.TS,
			// Status: constants.AnchorStatusIdle,
		}
		ok, err := c.roomState.CreateUser(ctx, req.RoomID, req.UserID, u)
		if err != nil {
			reply(nil, err)
			return err
		}

		c.logger.Info("User created",
			log.String("roomId", req.RoomID),
			log.String("userId", req.UserID),
			log.String("role", req.Role),
			log.Bool("ok", ok),
		)

		reply(nil, nil)
		return nil
	}

	c.userEventCh <- &userEvent{
		action: action,
		ts:     req.TS,
	}
}

func (c *UserStatusControl) handleDelete(
	_ jsonrpc.MethodContext[interface{}],
	params *json.RawMessage,
	reply jsonrpc.Reply,
) {

	req := users.DeleteUserRequest{}
	if err := jsonrpc.ShouldBindParams(params, &req); err != nil {
		reply(nil, err)
		return
	}

	action := func(ctx context.Context) error {

		ok, err := c.roomState.RemoveUser(ctx, req.RoomID, req.UserID)
		if ok {
			if err := c.notifyUserStatus(ctx, req.RoomID); err != nil {
				c.logger.Error("Failed to send WS room members", log.Error(err))
			}
		}

		c.logger.Info("User deleted",
			log.String("roomId", req.RoomID),
			log.String("userId", req.UserID),
			log.Bool("ok", ok),
		)

		reply(nil, err)
		return err
	}

	c.userEventCh <- &userEvent{
		action: action,
		ts:     req.TS,
	}
}

func (c *UserStatusControl) handleSetStatus(
	_ jsonrpc.MethodContext[interface{}],
	params *json.RawMessage,
	reply jsonrpc.Reply,
) {
	req := users.SetStatusUserRequest{}
	if err := jsonrpc.ShouldBindParams(params, &req); err != nil {
		c.logger.Debug("handleSetStatus called error", log.Error(err))
		reply(nil, err)
		return
	}

	action := func(ctx context.Context) error {
		u := &users.User{
			Status: req.Status,
			TS:     req.TS,
			Gen:    req.Gen,
		}
		ok, err := c.roomState.UpdateUserStatus(ctx, req.RoomID, req.UserID, u)
		if ok {
			if err := c.notifyUserStatus(ctx, req.RoomID); err != nil {
				c.logger.Error("Failed to send WS room members", log.Error(err))
			}
		}

		c.logger.Debug("User status updated",
			log.String("roomId", req.RoomID),
			log.String("userId", req.UserID),
			log.Any("status", req.Status),
			log.Bool("ok", ok),
		)

		reply(nil, err)
		return err
	}

	c.userEventCh <- &userEvent{
		action: action,
		ts:     req.TS,
	}
}

func (c *UserStatusControl) notifyUserStatus(ctx context.Context, roomID string) error {

	us := c.roomState.GetRoomUsers(ctx, roomID)
	members := make([]*users.RoomUser, 0, len(us))

	c.logger.Debug("Notifying room user status",
		log.String("roomId", roomID),
		log.Any("members", members),
	)

	for userID, u := range us {
		if !u.IsActive() {
			continue
		}
		members = append(members, &users.RoomUser{
			UserID: userID,
			Role:   u.Role,
			Status: u.Status,
		})
	}

	req := &users.NotifyRoomStatus{
		RoomID:  roomID,
		Members: members,
	}
	if err := c.peer2ws.Notify(ctx, "broadcastRoomStatus", req); err != nil {
		c.logger.Error("Failed to send WS room members", log.Error(err))
		return err
	}

	return nil
}

func (c *UserStatusControl) rebuildState(ctx context.Context) error {
	c.logger.Info("Rebuilding")
	if err := c.roomState.Rebuild(ctx); err != nil {
		return err
	}
	return nil
}

func (c *UserStatusControl) loop(ctx context.Context) {

	expireTicker := time.NewTicker(c.expireCheckInterval)
	defer expireTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-c.userEventCh:
			// TODO: check outdated ts
			// if event.ts.Before(time.Now().Add(-userStatusTimeout)) {
			// outdated event, skip
			// return
			// }
			if err := event.action(ctx); err != nil {
				c.logger.Error("Failed to process user action", log.Error(err))
			}
		case <-expireTicker.C:
			// TODO: stop scheduler when suffer some errors ?

			roomIDs, err := c.roomState.CheckTimeout(ctx)
			if err != nil {
				c.logger.Error("Failed to check timeouts", log.Error(err))
				continue
			}

			for _, roomID := range roomIDs {
				if err := c.notifyUserStatus(ctx, roomID); err != nil {
					c.logger.Error("Failed to notify user status", log.Error(err))
				}
			}
		}
	}
}

func (c *UserStatusControl) Stop() error {
	c.logger.Info("Closing")

	if err := c.peer2svc.Close(); err != nil {
		return fmt.Errorf("failed to close svc RPC peer: %w", err)
	}
	if err := c.peer2ws.Close(); err != nil {
		return fmt.Errorf("failed to close ws RPC peer: %w", err)
	}
	if err := c.roomWatcher.Stop(); err != nil {
		return fmt.Errorf("failed to stop room watcher: %w", err)
	}
	return nil
}
