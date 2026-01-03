package status

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	redisRpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/redis"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/users"
)

const (
	DefaultRequestTimeoutMS = 5000 // 5 seconds
)

type userServiceImpl struct {
	redisClient *redis.Client
	jwtAuth     jwt.JWTAuth
	peerSvc     jsonrpc.Peer[interface{}]
	logger      *log.Logger
}

func NewUserService(
	redisClient *redis.Client,
	jwtAuth jwt.JWTAuth,
	streamIn string,
	streamOut string,
	logger *log.Logger,
) (users.UserService, error) {

	peerSvc, err := redisRpc.NewPeer[interface{}](
		redisClient,
		streamIn,
		streamOut,
		"", // request only, no consumer group needed
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC peer: %w", err)
	}

	return &userServiceImpl{
		redisClient: redisClient,
		jwtAuth:     jwtAuth,
		peerSvc:     peerSvc,
		logger:      logger,
	}, nil
}

func (s *userServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Starting user service RPC peer")
	return s.peerSvc.Open(ctx)
}

func (s *userServiceImpl) CreateUser(
	ctx context.Context,
	roomId string,
	userId string,
	role string,
) (string, string, error) {
	// Send RPC request and wait for reply
	request := &users.CreateUserRequest{
		RoomID: roomId,
		UserID: userId,
		Role:   role,
		TS:     time.Now(),
	}
	if err := s.peerSvc.Call(ctx, "createUser", request, nil); err != nil {
		return "", "", fmt.Errorf("failed to create user: %w", err)
	}

	// Generate JWT token
	token, err := s.jwtAuth.Sign(userId, roomId)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return userId, token, nil
}

func (s *userServiceImpl) DeleteUser(ctx context.Context, roomId, userId string) error {
	// Send RPC request and wait for reply
	request := &users.DeleteUserRequest{
		RoomID: roomId,
		UserID: userId,
		TS:     time.Now(),
	}
	if err := s.peerSvc.Call(ctx, "deleteUser", request, nil); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (s *userServiceImpl) SetUserStatus(
	ctx context.Context,
	roomId, userId string,
	status constants.AnchorStatus,
	gen int32,
) error {
	event := &users.SetStatusUserRequest{
		RoomID: roomId,
		UserID: userId,
		Status: status,
		Gen:    gen,
		TS:     time.Now(),
	}
	return s.peerSvc.Notify(ctx, "setUserStatus", event)
}

func (s *userServiceImpl) GetActiveRoomUsers(ctx context.Context, roomId string) ([]*users.RoomUser, error) {
	return nil, nil
}
