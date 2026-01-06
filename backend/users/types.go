package users

import (
	"context"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
)

const (
	// TODO: config ?!
	UserStatusTimeout = 30 * time.Second
	RoomMaxTTL        = 6 * time.Hour
)

// RoomsState combines in-memory and redis based room state management.
// Note that all operations are not thread-safe, and expected to be called from a single thread only.
// set memomry first, then actions, then redis ?
type RoomsState interface {
	// how to expose for timeouted ? trigger timeout check externally ?
	Rebuild(ctx context.Context) error
	CreateUser(ctx context.Context, roomID, userID string, u *User) (bool, error)
	UpdateUserStatus(ctx context.Context, roomID, userID string, u *User) (bool, error)
	RemoveUser(ctx context.Context, roomID, userID string) (bool, error)
	GetRoomUsers(ctx context.Context, roomID string) map[string]User
	CheckTimeout(ctx context.Context) (roomIDs []string, err error)
}

// UserService provides user management operations, requests are forwarded
// to UserController for centralized processing.
type UserService interface {
	Start(ctx context.Context) error
	CreateUser(ctx context.Context, roomID, userID, role string) (string, string, error)
	DeleteUser(ctx context.Context, roomID, userID string) error
	SetUserStatus(ctx context.Context, roomID, userID string, status constants.AnchorStatus, gen int32) error
	GetActiveRoomUsers(ctx context.Context, roomID string) ([]*RoomUser, error)
}

type RoomUser struct {
	UserID string                 `json:"userId"`
	Role   string                 `json:"role"`
	Status constants.AnchorStatus `json:"status"`
}

type NotifyRoomStatus struct {
	RoomID  string      `json:"roomId"`
	Members []*RoomUser `json:"members"`
}

type User struct {
	Role   string
	Status constants.AnchorStatus
	TS     time.Time
	Gen    int32
}

func (u *User) IsActive() bool {
	return u != nil && time.Since(u.TS) < UserStatusTimeout
}

type CreateUserRequest struct {
	RoomID string    `json:"roomId"`
	UserID string    `json:"userId"`
	Role   string    `json:"role"`
	TS     time.Time `json:"ts"`
}

type DeleteUserRequest struct {
	RoomID string    `json:"roomId"`
	UserID string    `json:"userId"`
	TS     time.Time `json:"ts"`
}

type SetStatusUserRequest struct {
	RoomID string                 `json:"roomId"`
	UserID string                 `json:"userId"`
	Status constants.AnchorStatus `json:"status"`
	Gen    int32                  `json:"gen"`
	TS     time.Time              `json:"ts"`
}
