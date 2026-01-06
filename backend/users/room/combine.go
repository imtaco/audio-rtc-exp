package room

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	fredis "github.com/imtaco/audio-rtc-exp/internal/redis"
	"github.com/imtaco/audio-rtc-exp/internal/zset"
	"github.com/imtaco/audio-rtc-exp/users"
)

func New(
	redisClient *redis.Client,
	prefix string,
	logger *log.Logger,
) users.RoomsState {
	fclient := fredis.NewForever(
		redisClient,
		5*time.Millisecond,
		5*time.Second,
		logger,
	)

	return &combinedRoom{
		redisClient: redisClient,
		memState: &roomsStateMem{
			rooms:      make(map[string]map[string]*users.User),
			userTracks: zset.New[string](),
			roomTracks: zset.New[string](),
			logger:     logger,
		},
		redisState: &roomStateRedis{
			client: fclient,
			prefix: prefix,
			logger: logger,
		},
		logger: logger,
	}
}

type combinedRoom struct {
	memState    *roomsStateMem
	redisState  *roomStateRedis
	redisClient *redis.Client
	logger      *log.Logger
}

func (c *combinedRoom) CreateUser(
	ctx context.Context,
	roomID string,
	userID string,
	u *users.User,

) (bool, error) {
	if !c.memState.createRoomUser(roomID, userID, u) {
		return false, nil
	}
	return true, c.redisState.createRoomUser(ctx, roomID, userID, u)
}

func (c *combinedRoom) UpdateUserStatus(
	ctx context.Context,
	roomID string,
	userID string,
	u *users.User,
) (bool, error) {
	if !c.memState.setUserStatus(roomID, userID, u) {
		return false, nil
	}
	return true, c.redisState.setUserStatus(ctx, roomID, userID, u)
}

func (c *combinedRoom) RemoveUser(ctx context.Context, roomID, userID string) (bool, error) {
	ok, lastUser := c.memState.removeRoomUser(roomID, userID)
	if !ok {
		return false, nil
	}
	return ok, c.redisState.removeRoomUser(ctx, roomID, userID, lastUser)
}

func (c *combinedRoom) GetRoomUsers(_ context.Context, roomID string) map[string]users.User {
	return c.memState.getRoomUsers(roomID)
}

func (c *combinedRoom) Rebuild(ctx context.Context) error {
	logger := c.logger
	client := c.redisClient
	roomsMem := c.memState.rooms

	logger.Info("Rebuilding in-memory state from Redis")

	roomsTracking, err := c.redisState.parseRoomTracks(ctx)
	if err != nil {
		c.logger.Error("Failed to parse room tracks", log.Error(err))
		return fmt.Errorf("failed to get rooms: %w", err)
	}

	if len(roomsTracking) == 0 {
		logger.Info("No rooms found in Redis")
		return nil
	}

	totalUsers := 0
	for roomID, ts := range roomsTracking {
		c.memState.addRoomTrack(roomID, ts)

		usersData, err := client.HGetAll(ctx, c.redisState.userStatusKey(roomID)).Result()
		if err != nil {
			logger.Error("Failed to get users for room", log.String("roomID", roomID), log.Error(err))
			// TODO: continue or return error, or retry ?
			return err
		}

		if len(usersData) == 0 {
			continue
		}

		room := parseUsersData(usersData)
		roomsMem[roomID] = room

		for userID, user := range room {
			if user.Role == "" {
				delete(room, userID)
				continue
			}

			if user.Status != "" {
				if user.TS.IsZero() {
					user.TS = time.Now()
				}
				c.memState.userTracks.Put(userID, roomID, user.TS)
			}

			totalUsers++
		}
		logger.Debug("room state rebuild member", log.String("roomID", roomID), log.Any("members", room))
	}

	logger.Info("State rebuilt successfully",
		log.Int("roomCount", len(c.memState.rooms)),
		log.Int("userCount", totalUsers),
	)

	return nil
}

func (c *combinedRoom) CheckTimeout(ctx context.Context) ([]string, error) {

	effectedRooms := make(map[string]struct{})
	now := time.Now()

	userTracker := c.memState.userTracks
	roomTracker := c.memState.roomTracks

	for userTracker.Len() > 0 {
		userID, roomID, ts, ok := userTracker.Peek()
		if !ok {
			// no more users
			break
		}
		if time.Since(ts) < users.UserStatusTimeout {
			break
		}

		c.logger.Debug("User status timeout",
			log.String("userID", userID),
			log.String("roomID", roomID),
			log.Time("lastTS", ts),
		)

		// set user status to empty
		effectedRooms[roomID] = struct{}{}
		if _, err := c.UpdateUserStatus(ctx, roomID, userID, &users.User{
			Status: "",
			TS:     now,
			Gen:    0,
		}); err != nil {
			return nil, err
		}

		// ensure popped after processing to avoid issues with continue
		userTracker.Pop()
	}

	for roomTracker.Len() > 0 {
		roomID, _, ts, ok := roomTracker.Peek()
		if !ok {
			break
		}
		if time.Since(ts) < users.RoomMaxTTL {
			break
		}

		// remove user tracking
		for userID := range c.memState.rooms[roomID] {
			userTracker.Remove(userID)
		}
		delete(c.memState.rooms, roomID)

		if err := c.redisState.removeRoomUserStatus(ctx, roomID); err != nil {
			return nil, err
		}
		if err := c.redisState.removeRoomTrack(ctx, roomID); err != nil {
			return nil, err
		}
		// remove tracking
		roomTracker.Pop()
	}

	results := make([]string, 0, len(effectedRooms))
	for roomID := range effectedRooms {
		results = append(results, roomID)
	}
	return results, nil
}
