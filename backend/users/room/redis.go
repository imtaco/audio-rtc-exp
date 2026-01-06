package room

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/redis"
	"github.com/imtaco/audio-rtc-exp/users"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

var (
	zeroTime time.Time
)

type roomStateRedis struct {
	client redis.Forever
	prefix string
	logger *log.Logger
}

func (r *roomStateRedis) createRoomUser(
	ctx context.Context,
	roomID string,
	userID string,
	u *users.User,
) error {
	if err := r.addRoomTrack(ctx, roomID, u.TS); err != nil {
		return fmt.Errorf("failed to set room: %w", err)
	}
	if err := r.client.HSet(ctx, r.userStatusKey(roomID), metaField(userID), u.Role); err != nil {
		return fmt.Errorf("failed to set user meta: %w", err)
	}
	return nil
}

func (r *roomStateRedis) parseRoomTracks(ctx context.Context) (map[string]time.Time, error) {
	result := make(map[string]time.Time)
	data, err := r.client.HGetAll(ctx, r.roomsKey())
	if err != nil {
		return nil, fmt.Errorf("failed to get rooms: %w", err)
	}
	for roomID, tsStr := range data {
		tsInt, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			r.logger.Warn("invalid timestamp for room", log.String("roomID", roomID), log.String("ts", tsStr))
			result[roomID] = time.Now()
		} else {
			result[roomID] = time.Unix(tsInt, 0)
		}
	}
	return result, nil
}

func (r *roomStateRedis) addRoomTrack(ctx context.Context, roomID string, ts time.Time) error {
	if err := r.client.HSet(ctx, r.roomsKey(), roomID, ts.Unix()); err != nil {
		return fmt.Errorf("failed to set room: %w", err)
	}
	return nil
}

func (r *roomStateRedis) removeRoomTrack(ctx context.Context, roomID string) error {
	if err := r.client.HDel(ctx, r.roomsKey(), roomID); err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}
	return nil
}

func (r *roomStateRedis) removeRoomUserStatus(ctx context.Context, roomID string) error {
	if err := r.client.Del(ctx, r.userStatusKey(roomID)); err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}
	return nil
}

func (r *roomStateRedis) setUserStatus(ctx context.Context,
	roomID string,
	userID string,
	u *users.User,
) error {
	if u.Status == "" {
		// remove status
		if err := r.client.HDel(ctx, r.userStatusKey(roomID), statusField(userID)); err != nil {
			return fmt.Errorf("failed to delete user status: %w", err)
		}
		return nil
	}

	if err := r.client.HSet(
		ctx,
		r.userStatusKey(roomID),
		statusField(userID),
		packStatus(u),
	); err != nil {
		return fmt.Errorf("failed to set user meta: %w", err)
	}
	return nil
}

func (r *roomStateRedis) removeRoomUser(
	ctx context.Context,
	roomID string,
	userID string,
	lastUser bool,
) error {
	if err := r.client.HDel(ctx, r.userStatusKey(roomID), statusField(userID), metaField(userID)); err != nil {
		return fmt.Errorf("failed to delete user from Redis: %w", err)
	}
	if !lastUser {
		return nil
	}
	if err := r.removeRoomTrack(ctx, roomID); err != nil {
		return fmt.Errorf("failed to delete room from Redis: %w", err)
	}

	return nil
}

func (r *roomStateRedis) roomsKey() string {
	return fmt.Sprintf("%s:rooms", r.prefix)
}

func (r *roomStateRedis) userStatusKey(roomID string) string {
	return fmt.Sprintf("%s:r:%s:us", r.prefix, roomID)
}

func statusField(userID string) string {
	return fmt.Sprintf("s:%s", userID)
}

func metaField(userID string) string {
	return fmt.Sprintf("m:%s", userID)
}

// TODO: better serialization/deserialization
func packStatus(u *users.User) string {
	return fmt.Sprintf("%d,%s,%d", u.TS.Unix(), u.Status, u.Gen)
}

func unpackStatus(value string) (time.Time, constants.AnchorStatus, int32, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return zeroTime, "", 0, fmt.Errorf("invalid status format")
	}

	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return zeroTime, "", 0, fmt.Errorf("invalid timestamp: %w", err)
	}

	gen, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil {
		return zeroTime, "", 0, fmt.Errorf("invalid generation: %w", err)
	}

	return time.Unix(ts, 0), constants.AnchorStatus(parts[1]), int32(gen), nil
}

func parseUsersData(data map[string]string) map[string]*users.User {
	users := make(map[string]*users.User)

	for field, value := range data {
		if strings.HasPrefix(field, "m:") {
			// Role field: m:<userId> -> <role>
			userID := field[2:]
			user := ensureUser(users, userID)
			user.Role = value
		} else if strings.HasPrefix(field, "s:") {
			// Status field: s:<userId> -> <ts>,<status>,<gen>
			userID := field[2:]
			user := ensureUser(users, userID)

			var err error
			user.TS, user.Status, user.Gen, err = unpackStatus(value)
			if err != nil {
				// TODO: log error
				continue
			}
		}
	}

	return users
}
