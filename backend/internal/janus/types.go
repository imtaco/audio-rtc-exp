package janus

import (
	"context"
	"encoding/json"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
)

type API interface {
	CreateAnchorInstance(
		ctx context.Context,
		clientID string,
		sessionID int64,
		handleID int64,
	) (Anchor, error)
	CreateAdminInstance(ctx context.Context, adminKey string) (Admin, error)
}

// Admin defines the interface for Janus administrative operations
type Admin interface {
	Base
	CreateRoom(ctx context.Context, roomID int64, description, pin string) error
	DestroyRoom(ctx context.Context, roomID int64) error
	GetRoom(ctx context.Context, roomID int64) (bool, error)
	CreateRTPForwarder(ctx context.Context, roomID int64, host string, port int) (int64, error)
	StopRTPForwarder(ctx context.Context, roomID, streamID int64) error
	ListRTPForwarders(ctx context.Context, roomID int64) ([]RTPForwarderInfo, error)
	ListRooms(ctx context.Context) ([]RoomInfo, error)
}

type Anchor interface {
	Base
	Join(ctx context.Context, roomID int64, pin string, displayName string, jsep *JSEP) (*JanusResponse, error)
	Leave(ctx context.Context) (*JanusResponse, error)
	IceCandidate(ctx context.Context, candidate ICECandidate) (*JanusResponse, error)
	Check(ctx context.Context) (bool, error)
}

type Base interface {
	GetSessionID() int64
	GetHandleID() int64
	Close()
	GetEvents(ctx context.Context, maxEvents int) ([]*JanusResponse, error)
	Destroy(ctx context.Context) error
	KeepAlive(ctx context.Context) error
	StartKeepalive()
	StopKeepalive()
}

// JanusResponse models the subset of Janus fields this client cares about.
type JanusResponse struct {
	Janus      string           `json:"janus"`
	SessionID  int64            `json:"session_id,omitempty"`
	Sender     int64            `json:"sender,omitempty"`
	Data       *JanusData       `json:"data,omitempty"`
	Plugindata *JanusPluginData `json:"plugindata,omitempty"`
	JSEP       *json.RawMessage `json:"jsep,omitempty"`
}

// JanusData contains Janus identifiers present in many responses.
type JanusData struct {
	ID int64 `json:"id"`
}

// JanusPluginData wraps plugin-specific payloads.
type JanusPluginData struct {
	Data json.RawMessage `json:"data"`
}

// DecodePluginData unmarshals the plugin data payload into v.
func (r *JanusResponse) DecodePluginData(v interface{}) error {
	if r == nil || r.Plugindata == nil {
		return errors.New(ErrInvalidResponse, "plugin data unavailable")
	}
	if len(r.Plugindata.Data) == 0 {
		return errors.New(ErrInvalidResponse, "plugin data empty")
	}
	return json.Unmarshal(r.Plugindata.Data, v)
}

func checkSuccess(resp *JanusResponse) error {
	if resp == nil {
		return errors.Newf(ErrInvalidResponse, "janus is nil")
	}
	if resp.Janus == "success" || resp.Janus == "ack" {
		return nil
	}
	return errors.Newf(ErrNoneSuccessResponse, "janus not success: (resp %v)", resp)
}

func pluginErrorCode(resp *JanusResponse) (int, bool) {
	if resp == nil || resp.Plugindata == nil || len(resp.Plugindata.Data) == 0 {
		return 0, false
	}
	var payload struct {
		ErrorCode int `json:"error_code"`
	}
	if err := json.Unmarshal(resp.Plugindata.Data, &payload); err != nil {
		return 0, false
	}
	if payload.ErrorCode == 0 {
		return 0, false
	}
	return payload.ErrorCode, true
}

// JSEP represents a standard WebRTC SDP payload.
type JSEP struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// ICECandidate models the Janus trickle candidate payload.
type ICECandidate struct {
	Candidate     string `json:"candidate,omitempty"`
	SdpMid        string `json:"sdpMid,omitempty"`
	SdpMLineIndex *int   `json:"sdpMLineIndex,omitempty"`
	Completed     bool   `json:"completed,omitempty"`
}

// Request structs for AudioBridge plugin

// JoinRequest represents an AudioBridge join request.
type JoinRequest struct {
	Request string `json:"request"`
	Room    int64  `json:"room"`
	Display string `json:"display"`
	Muted   bool   `json:"muted"`
	Pin     string `json:"pin,omitempty"`
}

// LeaveRequest represents an AudioBridge leave request.
type LeaveRequest struct {
	Request string `json:"request"`
}

// ExistsRequest represents an exists check request.
type ExistsRequest struct {
	Request  string `json:"request"`
	Room     int64  `json:"room"`
	AdminKey string `json:"admin_key,omitempty"`
}

// CreateRoomRequest represents a room creation request.
type CreateRoomRequest struct {
	Request      string `json:"request"`
	Room         int64  `json:"room"`
	Description  string `json:"description,omitempty"`
	SamplingRate int    `json:"sampling_rate,omitempty"`
	SpatialAudio bool   `json:"spatial_audio,omitempty"`
	Record       bool   `json:"record,omitempty"`
	Pin          string `json:"pin,omitempty"`
	AdminKey     string `json:"admin_key,omitempty"`
}

// DestroyRoomRequest represents a room destruction request.
type DestroyRoomRequest struct {
	Request  string `json:"request"`
	Room     int64  `json:"room"`
	AdminKey string `json:"admin_key,omitempty"`
}

// RTPForwardRequest represents an RTP forwarder creation request.
type RTPForwardRequest struct {
	Request  string `json:"request"`
	Room     int64  `json:"room"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Codec    string `json:"codec,omitempty"`
	AdminKey string `json:"admin_key,omitempty"`
}

// StopRTPForwardRequest represents an RTP forwarder stop request.
type StopRTPForwardRequest struct {
	Request  string `json:"request"`
	Room     int64  `json:"room"`
	StreamID int64  `json:"stream_id"`
	AdminKey string `json:"admin_key,omitempty"`
}

// ListForwardersRequest represents a list forwarders request.
type ListForwardersRequest struct {
	Request  string `json:"request"`
	Room     int64  `json:"room"`
	AdminKey string `json:"admin_key,omitempty"`
}

// ListRoomsRequest represents a list rooms request.
type ListRoomsRequest struct {
	Request string `json:"request"`
}

// Response structs

// RoomInfo represents information about an AudioBridge room.
type RoomInfo struct {
	Room         int64  `json:"room"`
	Description  string `json:"description"`
	Pin          bool   `json:"pin_required,omitempty"`
	SamplingRate int    `json:"sampling_rate,omitempty"`
	SpatialAudio bool   `json:"spatial_audio,omitempty"`
	Record       bool   `json:"record,omitempty"`
	NumParts     int    `json:"num_participants,omitempty"`
}

// RTPForwarderInfo represents information about an RTP forwarder.
type RTPForwarderInfo struct {
	StreamID int64  `json:"stream_id"`
	Host     string `json:"ip,omitempty"`
	Port     int    `json:"port,omitempty"`
	Codec    string `json:"codec,omitempty"`
}

// ExistsResponse represents the response to an exists check.
type ExistsResponse struct {
	Exists bool `json:"exists"`
}

// StreamIDResponse represents a response containing a stream ID.
type StreamIDResponse struct {
	StreamID int64 `json:"stream_id"`
}

// ListRoomsResponse represents the response to a list rooms request.
type ListRoomsResponse struct {
	Rooms []RoomInfo `json:"list"`
}

// ListForwardersResponse represents the response to a list forwarders request.
type ListForwardersResponse struct {
	Forwarders []RTPForwarderInfo `json:"rtp_forwarders"`
}
