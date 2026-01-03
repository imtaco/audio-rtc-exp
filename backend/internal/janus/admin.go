package janus

import (
	"context"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// adminInst provides AudioBridge administrative helpers.
type adminInst struct {
	*baseInstance
	adminKey string
}

func newAdminInstance(api *apiImpl, sessionID int64, handleID int64, adminKey string) Admin {
	return &adminInst{
		baseInstance: newBaseInstance(api, "admin", sessionID, handleID),
		adminKey:     adminKey,
	}
}

// CreateRTPForwarder configures Janus to forward RTP to the destination host/port and returns the stream ID.
func (a *adminInst) CreateRTPForwarder(
	ctx context.Context,
	roomID int64,
	host string,
	port int,
) (int64, error) {
	a.api.logger.Info("creating janus RTP forwarder", log.Int64("room", roomID), log.String("host", host), log.Int("port", port))

	req := RTPForwardRequest{
		Request:  "rtp_forward",
		Room:     roomID,
		Host:     host,
		Port:     port,
		Codec:    "opus",
		AdminKey: a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return 0, err
	}
	if err := checkSuccess(resp); err != nil {
		return 0, err
	}

	var payload StreamIDResponse
	if err := resp.DecodePluginData(&payload); err != nil {
		return 0, err
	}
	if payload.StreamID == 0 {
		return 0, errors.New(ErrInvalidPayload, "janus stream_id missing")
	}
	return payload.StreamID, nil
}

// StopRTPForwarder stops a previously created RTP forwarder.
func (a *adminInst) StopRTPForwarder(ctx context.Context, roomID, streamID int64) error {
	req := StopRTPForwardRequest{
		Request:  "stop_rtp_forwarder",
		Room:     roomID,
		StreamID: streamID,
		AdminKey: a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return err
	}
	if err := checkSuccess(resp); err != nil {
		return err
	}
	if code, ok := pluginErrorCode(resp); ok && code == 485 {
		return errors.Newf(ErrNotFound, "rtp forwarder %d not found", streamID)
	}
	return nil
}

// GetRoom returns true when the specified room exists.
func (a *adminInst) GetRoom(ctx context.Context, roomID int64) (bool, error) {
	req := ExistsRequest{
		Request:  "exists",
		Room:     roomID,
		AdminKey: a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return false, err
	}
	if err := checkSuccess(resp); err != nil {
		return false, err
	}

	var payload ExistsResponse
	if err := resp.DecodePluginData(&payload); err != nil {
		return false, err
	}
	return payload.Exists, nil
}

// CreateRoom provisions a new AudioBridge room.
func (a *adminInst) CreateRoom(ctx context.Context, roomID int64, description, pin string) error {
	req := CreateRoomRequest{
		Request:      "create",
		Room:         roomID,
		Description:  description,
		SamplingRate: 16000,
		SpatialAudio: false,
		Record:       false,
		Pin:          pin,
		AdminKey:     a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return err
	}
	if err := checkSuccess(resp); err != nil {
		return err
	}
	if code, ok := pluginErrorCode(resp); ok && code == 486 {
		return errors.Newf(ErrAlreadyExisted, "janus room %d already exists", roomID)
	}
	return nil
}

// DestroyRoom removes an existing AudioBridge room.
func (a *adminInst) DestroyRoom(ctx context.Context, roomID int64) error {
	req := DestroyRoomRequest{
		Request:  "destroy",
		Room:     roomID,
		AdminKey: a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return err
	}
	if err := checkSuccess(resp); err != nil {
		return err
	}
	if code, ok := pluginErrorCode(resp); ok && code == 485 {
		return errors.Newf(ErrNotFound, "janus room %d not found", roomID)
	}
	return nil
}

// ListRTPForwarders enumerates RTP forwarders for the given room.
func (a *adminInst) ListRTPForwarders(ctx context.Context, roomID int64) ([]RTPForwarderInfo, error) {
	req := ListForwardersRequest{
		Request:  "listforwarders",
		Room:     roomID,
		AdminKey: a.adminKey,
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return nil, err
	}
	if err := checkSuccess(resp); err != nil {
		return nil, err
	}

	var payload ListForwardersResponse
	if err := resp.DecodePluginData(&payload); err != nil {
		return nil, err
	}
	if payload.Forwarders == nil {
		return []RTPForwarderInfo{}, nil
	}
	return payload.Forwarders, nil
}

// ListRooms lists available AudioBridge rooms.
func (a *adminInst) ListRooms(ctx context.Context) ([]RoomInfo, error) {
	req := ListRoomsRequest{
		Request: "list",
	}

	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return nil, err
	}
	if err := checkSuccess(resp); err != nil {
		return nil, err
	}

	var payload ListRoomsResponse
	if err := resp.DecodePluginData(&payload); err != nil {
		return nil, err
	}
	if payload.Rooms == nil {
		return []RoomInfo{}, nil
	}
	return payload.Rooms, nil
}
