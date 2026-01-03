package janus

import "context"

// anchorInstance represents a publisher/subscriber connection to Janus.
type anchorInstance struct {
	*baseInstance
}

func newAnchorInstance(
	api *apiImpl,
	clientID string,
	sessionID int64,
	handleID int64) Anchor {
	return &anchorInstance{
		baseInstance: newBaseInstance(api, clientID, sessionID, handleID),
	}
}

// Join instructs the Janus AudioBridge plugin to join a room.
func (a *anchorInstance) Join(
	ctx context.Context,
	roomID int64,
	pin string,
	displayName string,
	jsep *JSEP) (*JanusResponse, error) {
	req := JoinRequest{
		Request: "join",
		Room:    roomID,
		Display: displayName,
		Muted:   false,
		Pin:     pin,
	}
	return a.postMessageWithJSEP(ctx, req, jsep)
}

// Leave instructs Janus to leave the current room.
func (a *anchorInstance) Leave(ctx context.Context) (*JanusResponse, error) {
	req := LeaveRequest{
		Request: "leave",
	}
	return a.postMessage(ctx, "message", req)
}

// IceCandidate forwards an ICE candidate (or completion message) to Janus.
func (a *anchorInstance) IceCandidate(ctx context.Context, candidate ICECandidate) (*JanusResponse, error) {
	return a.postTrickle(ctx, candidate)
}

// Check verifies the session is still alive via a lightweight exists call.
func (a *anchorInstance) Check(ctx context.Context) (bool, error) {
	req := ExistsRequest{
		Request: "exists",
		Room:    1, // arbitrary room ID for exists check
	}
	resp, err := a.postMessage(ctx, "message", req)
	if err != nil {
		return false, err
	}
	if err := checkSuccess(resp); err != nil {
		return false, err
	}
	return true, nil
}
