package rooms

import (
	"context"
	"fmt"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
)

// RoomService defines the interface for room management operations
type RoomService interface {
	CreateRoom(ctx context.Context, roomID, pin string, maxAnchors int) (*RoomResponse, error)
	GetRoom(ctx context.Context, roomID string) (*RoomResponse, error)
	ListRooms(ctx context.Context) (*ListRoomsResponse, error)
	DeleteRoom(ctx context.Context, roomID string) (*DeleteRoomResponse, error)
	GetStats(ctx context.Context) (*StatsResponse, error)
	StartLive(ctx context.Context, roomID string) error
}

type RoomStore interface {
	CreateRoom(ctx context.Context, roomID string, roomData *etcdstate.Meta) (*etcdstate.Meta, error)
	GetRoom(ctx context.Context, roomID string) (*etcdstate.Meta, error)
	Exists(ctx context.Context, roomID string) (bool, error)
	StopRoom(ctx context.Context, roomID string) error

	DeleteRoom(ctx context.Context, roomID string) (bool, error)
	GetAllRooms(ctx context.Context) (map[string]*etcdstate.Meta, error)

	CreateLiveMeta(ctx context.Context, roomID, mixerID, janusID, nonce string) error
	StopLiveMeta(ctx context.Context, roomID string) error

	GetMixerData(ctx context.Context, roomID string) (*etcdstate.Mixer, error)
	GetStats(ctx context.Context) (*RoomStats, error)

	// Module mark operations
	SetModuleMark(ctx context.Context, moduleType, moduleID string, label constants.MarkLabel, ttlSeconds int64) error
	DeleteModuleMark(ctx context.Context, moduleType, moduleID string) error
}

type ResourceManager interface {
	Start(context.Context) error
	Stop() error
	PickJanus() (string, error)
	PickMixer() (string, error)
	// PickResource(module string) (string, error)
}

// Alias types from etcdstate for convenience
type LiveMeta = etcdstate.LiveMeta
type Mixer = etcdstate.Mixer

type RoomStats struct {
	Total             int `json:"total"`
	TotalParticipants int `json:"totalParticipants"`
}

// Response types for RoomService
type RoomResponse struct {
	RoomID    string    `json:"roomId"`
	HLSURL    string    `json:"hlsUrl"`
	Pin       string    `json:"pin,omitempty"`
	RTPPort   *int      `json:"rtpPort,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type ListRoomsResponse struct {
	Count int             `json:"count"`
	Rooms []*RoomResponse `json:"rooms"`
}

type DeleteRoomResponse struct {
	Message string `json:"message"`
}

type StatsResponse struct {
	Rooms *RoomStats `json:"rooms"`
}

// Custom error types
type RoomExistsError struct {
	RoomID string
}

func (e *RoomExistsError) Error() string {
	return fmt.Sprintf("Room %s already exists", e.RoomID)
}

type RoomNotFoundError struct {
	RoomID string
}

func (e *RoomNotFoundError) Error() string {
	return fmt.Sprintf("Room %s not found", e.RoomID)
}
