package service

import (
	"context"
	"fmt"

	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/rooms"
	utils "github.com/imtaco/audio-rtc-exp/rooms/utils"
)

type roomSvcImpl struct {
	roomStore rooms.RoomStore
	resMgr    rooms.ResourceManager
	hlsAdvURL string
	logger    *log.Logger
}

func NewRoomService(
	roomStore rooms.RoomStore,
	resMgr rooms.ResourceManager,
	hlsAdvURL string,
	logger *log.Logger,
) rooms.RoomService {
	return &roomSvcImpl{
		roomStore: roomStore,
		resMgr:    resMgr,
		hlsAdvURL: hlsAdvURL,
		logger:    logger,
	}
}

func (rs *roomSvcImpl) CreateRoom(ctx context.Context, roomID, pin string, maxAnchors int) (*rooms.RoomResponse, error) {
	// Check if room already exists
	exists, err := rs.roomStore.Exists(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists {
		return nil, &rooms.RoomExistsError{RoomID: roomID}
	}

	// Store room data
	room, err := rs.roomStore.CreateRoom(ctx, roomID, &etcdstate.Meta{
		Pin:        pin,
		HLSPath:    fmt.Sprintf("%s/stream.m3u8", roomID),
		MaxAnchors: maxAnchors,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	return &rooms.RoomResponse{
		RoomID:    roomID,
		HLSURL:    rs.hlsAdvURL + room.HLSPath,
		Pin:       room.Pin,
		CreatedAt: room.CreatedAt,
	}, nil
}

func (rs *roomSvcImpl) StartLive(ctx context.Context, roomID string) error {
	mixerID, err := rs.resMgr.PickMixer()
	if err != nil || mixerID == "" {
		return fmt.Errorf("no available mixer")
	}

	janusID, err := rs.resMgr.PickJanus()
	if err != nil || janusID == "" {
		return fmt.Errorf("no available Janus server")
	}

	exists, err := rs.roomStore.Exists(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if !exists {
		return &rooms.RoomNotFoundError{RoomID: roomID}
	}

	// Generate nonce
	nonce, err := utils.GenerateRandomHex(10)
	if err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	return rs.roomStore.CreateLiveMeta(ctx, roomID, mixerID, janusID, nonce)
}

func (rs *roomSvcImpl) GetRoom(ctx context.Context, roomID string) (*rooms.RoomResponse, error) {
	room, err := rs.roomStore.GetRoom(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}
	if room == nil {
		return nil, &rooms.RoomNotFoundError{RoomID: roomID}
	}

	// Get mixer data to include current RTP port if available
	mixerData, err := rs.roomStore.GetMixerData(ctx, roomID)
	if err != nil {
		rs.logger.Warn("Failed to get mixer data", log.String("roomId", roomID), log.Error(err))
	}

	response := &rooms.RoomResponse{
		RoomID:    roomID,
		HLSURL:    rs.hlsAdvURL + room.HLSPath,
		CreatedAt: room.CreatedAt,
	}

	if mixerData != nil && mixerData.Port > 0 {
		response.RTPPort = &mixerData.Port
	}

	return response, nil
}

func (rs *roomSvcImpl) ListRooms(ctx context.Context) (*rooms.ListRoomsResponse, error) {
	rms, err := rs.roomStore.GetAllRooms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rooms: %w", err)
	}

	response := &rooms.ListRoomsResponse{
		Count: len(rms),
		Rooms: make([]*rooms.RoomResponse, 0, len(rms)),
	}

	for roomID, room := range rms {
		response.Rooms = append(response.Rooms, &rooms.RoomResponse{
			RoomID:    roomID,
			HLSURL:    rs.hlsAdvURL + room.HLSPath,
			CreatedAt: room.CreatedAt,
		})
	}

	return response, nil
}

func (rs *roomSvcImpl) DeleteRoom(ctx context.Context, roomID string) (*rooms.DeleteRoomResponse, error) {
	room, err := rs.roomStore.GetRoom(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}
	if room == nil {
		return nil, &rooms.RoomNotFoundError{RoomID: roomID}
	}

	// Delete room from store (etcd watcher will handle stopping FFmpeg and destroying Janus room reactively)
	if err := rs.roomStore.StopRoom(ctx, roomID); err != nil {
		return nil, fmt.Errorf("failed to stop room: %w", err)
	}

	return &rooms.DeleteRoomResponse{
		Message: fmt.Sprintf("Room %s stopped", roomID),
	}, nil
}

func (rs *roomSvcImpl) GetStats(ctx context.Context) (*rooms.StatsResponse, error) {
	roomStats, err := rs.roomStore.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return &rooms.StatsResponse{
		Rooms: roomStats,
	}, nil
}
