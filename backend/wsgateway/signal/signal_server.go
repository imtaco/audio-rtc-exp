package signal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/users"
	"github.com/imtaco/audio-rtc-exp/wsgateway"
)

const (
	GEN = 1
)

type Server struct {
	jsonrpc.Handler[rtcContext]
	janusProxy      wsgateway.JanusProxy
	janusTokenCodec wsgateway.JanusTokenCodec
	connGuard       ConnectionGuard
	userService     users.UserService
	clientManager   *WSConnManager
	jwtAuth         jwt.Auth
	logger          *log.Logger
}

func NewServer(
	handler jsonrpc.Handler[rtcContext],
	janusProxy wsgateway.JanusProxy,
	janusTokenCodec wsgateway.JanusTokenCodec,
	clientManager *WSConnManager,
	userService users.UserService,
	connGuard ConnectionGuard,
	jwtAuth jwt.Auth,
	logger *log.Logger,
) *Server {
	// TODO: create client manager here ?
	return &Server{
		Handler:         handler,
		janusProxy:      janusProxy,
		connGuard:       connGuard,
		userService:     userService,
		janusTokenCodec: janusTokenCodec,
		clientManager:   clientManager,
		jwtAuth:         jwtAuth,
		logger:          logger,
	}
}

func (s *Server) Open(ctx context.Context) error {
	s.logger.Info("Opening Signal Server")
	s.register()

	if err := s.connGuard.Start(ctx); err != nil {
		return fmt.Errorf("failed to start heartbeat: %w", err)
	}

	return nil
}

func (s *Server) Close() error {
	s.logger.Info("Closing Signal Server")
	s.connGuard.Stop()
	return nil
}

func (s *Server) register() {
	// Register RPC methods
	// handler is single threaded, no need to lock here
	s.Def("join", s.handleJoin)
	s.Def("leave", s.handleLeave)
	s.Def("offer", s.handleOffer)
	s.Def("icecandidate", s.handleIceCandidate)
	s.Def("keepalive", s.handleKeepAlive)
	s.Def("status", s.handleKeepAlive)
}

func (s *Server) updateUserStatus(ctx context.Context, roomID, userID string, status constants.AnchorStatus) {
	// TODO: handle gen
	if err := s.userService.SetUserStatus(
		ctx,
		roomID,
		userID,
		status,
		GEN,
	); err != nil {
		s.logger.Error("Failed to update user status",
			log.String("roomId", roomID),
			log.String("userId", userID),
			log.Any("status", status),
			log.Error(err),
		)
	}
}

func (s *Server) mustHoldLock(mctx jsonrpc.MethodContext[rtcContext]) {
	if _, err := s.connGuard.MustHold(mctx); err != nil {
		s.logger.Error("Failed to acquire connect lock", log.Error(err))
	}
}

func (s *Server) handleJoin(mctx jsonrpc.MethodContext[rtcContext], params *json.RawMessage) (any, error) {

	rtcCtx := mctx.Get()
	if rtcCtx.joined {
		return nil, jsonrpc.ErrInvalidRequest("already joined")
	}

	var data struct {
		Pin        string `json:"pin"`
		ClientID   string `json:"clientId" validate:"required,uuid4"`
		JanusToken string `json:"jtoken"`
	}
	if err := jsonrpc.ShouldBindParams(params, &data); err != nil {
		return nil, jsonrpc.ErrInvalidParams("invalid join parameters")
	}
	// TODO: validation

	ctx := rtcCtx.reqCtx
	roomID := rtcCtx.roomID

	roomMeta := s.janusProxy.GetRoomMeta(roomID)
	if roomMeta == nil {
		return nil, jsonrpc.ErrInvalidRequest("no room found")
	}

	liveMeta := s.janusProxy.GetRoomLiveMeta(roomID)
	if liveMeta == nil || liveMeta.Status != constants.RoomStatusOnAir {
		return nil, jsonrpc.ErrInvalidRequest("room does not exist or not allowed to join")
	}

	if roomMeta.GetPin() != "" && data.Pin != roomMeta.GetPin() {
		return nil, jsonrpc.ErrInvalidRequest("invalid room pin")
	}

	janusAPI := s.janusProxy.GetJanusAPI(roomID)
	if janusAPI == nil {
		return nil, jsonrpc.ErrInternal("fail to get janus api")
	}

	// sessionID and handleID are encoded into janus token, such that we can restore janus instance
	// when connection drops and reconnects without re-creating janus session/handle to interrupt ongoing RTC session
	var sessionID, handleID int64
	var err error
	if data.JanusToken != "" {
		sessionID, handleID, err = s.janusTokenCodec.Decode(liveMeta.Nonce, data.JanusToken)
		if err != nil {
			s.logger.Error("Failed to decode janus token", log.Error(err))
			sessionID, handleID = 0, 0
		}
	}

	apiInst, err := s.restoreJanusInstance(rtcCtx, janusAPI, sessionID, handleID)
	if err != nil {
		return nil, err
	}
	// resumed session no need to negotiate RTC again
	resume := (sessionID == apiInst.GetSessionID() && handleID == apiInst.GetHandleID())

	janusToken, err := s.janusTokenCodec.Encode(liveMeta.Nonce, apiInst.GetSessionID(), apiInst.GetHandleID())
	if err != nil {
		s.logger.Error("Failed to encode janus token", log.Error(err))
		return nil, jsonrpc.ErrInternal("fail to create janus token")
	}

	rtcCtx.janus = apiInst
	rtcCtx.joined = true

	s.updateUserStatus(ctx, roomID, rtcCtx.userID, constants.AnchorStatusIdle)

	// pass janus token back to client for future reconnect
	return map[string]any{
		"jtoken": janusToken,
		"resume": resume,
	}, nil
}

func (s *Server) handleLeave(mctx jsonrpc.MethodContext[rtcContext], _ *json.RawMessage) (any, error) {
	rtcCtx := mctx.Get()
	if !rtcCtx.joined {
		return nil, jsonrpc.ErrInvalidRequest("not joined yet")
	}

	// remove in advanced
	s.clientManager.RemoveClient(rtcCtx.connID)
	if err := mctx.Peer().Close(); err != nil {
		s.logger.Error("Failed to close connection", log.Error(err))
		//nolint:nilnil
		return nil, nil
	}

	ctx := rtcCtx.reqCtx
	s.updateUserStatus(ctx, rtcCtx.roomID, rtcCtx.userID, constants.AnchorStatusLeft)

	//nolint:nilnil
	return nil, nil
}

func (s *Server) handleOffer(mctx jsonrpc.MethodContext[rtcContext], params *json.RawMessage) (any, error) {
	rtcCtx := mctx.Get()
	if !rtcCtx.joined {
		return nil, jsonrpc.ErrInvalidRequest("not joined yet")
	}

	// TODO: check room exists and is ONAIR
	var data struct {
		SDP *janus.JSEP `json:"sdp" validate:"required"`
	}
	if err := jsonrpc.ShouldBindParams(params, &data); err != nil {
		return nil, jsonrpc.ErrInvalidParams("invalid offer parameters")
	}
	// TODO: validate
	if data.SDP == nil {
		return nil, jsonrpc.ErrInvalidParams("missing SDP")
	}

	janusRoomID := s.janusProxy.GetJanusRoomID(rtcCtx.roomID)
	if janusRoomID == 0 {
		s.logger.Error("No Janus room found for this room", log.String("roomId", rtcCtx.roomID))
		return nil, jsonrpc.ErrInternal("no janus room found")
	}

	roomMeta := s.janusProxy.GetRoomMeta(rtcCtx.roomID)
	if roomMeta == nil {
		return nil, jsonrpc.ErrInvalidRequest("no room found")
	}

	ctx := rtcCtx.reqCtx
	displayName := fmt.Sprintf("user-%s", rtcCtx.userID)

	_, err := rtcCtx.janus.Join(ctx, janusRoomID, roomMeta.GetPin(), displayName, data.SDP)
	if err != nil {
		s.logger.Error("Failed to join Janus room", log.Error(err))
		return nil, jsonrpc.ErrInternal("failed to join janus room")
	}

	// 	Wait for Janus answer
	jsep, err := s.eventLoop(ctx, rtcCtx.janus)
	if err != nil {
		s.logger.Error("Failed get janus events", log.Error(err))
		return nil, jsonrpc.ErrInternal("fail to get janus events")
	}

	return map[string]any{
		"sdp": jsep,
	}, nil
}

func (s *Server) eventLoop(ctx context.Context, apiInst janus.Anchor) (json.RawMessage, error) {
	resps, err := apiInst.GetEvents(ctx, 10)
	if err != nil {
		return nil, err
	}
	for _, resp := range resps {
		if resp.Janus != "event" || resp.JSEP == nil {
			continue
		}
		return *resp.JSEP, nil
	}
	return nil, fmt.Errorf("no SDP answer found in Janus events")
}

func (s *Server) handleIceCandidate(mctx jsonrpc.MethodContext[rtcContext], params *json.RawMessage) (any, error) {
	// ice candidate might called several times before answered
	rtcCtx := mctx.Get()
	if !rtcCtx.joined {
		return nil, jsonrpc.ErrInvalidRequest("not joined yet")
	}

	var data struct {
		Candidate *janus.ICECandidate `json:"candidate" validate:"required"`
	}
	if err := jsonrpc.ShouldBindParams(params, &data); err != nil {
		return nil, jsonrpc.ErrInvalidParams("invalid ice candidate parameters")
	}
	//  TODO: more validation
	if data.Candidate == nil {
		return nil, jsonrpc.ErrInvalidParams("missing candidate")
	}

	ctx := rtcCtx.reqCtx
	if _, err := rtcCtx.janus.IceCandidate(ctx, *data.Candidate); err != nil {
		s.logger.Error("Failed exhange ice candidate", log.Error(err))
		return nil, jsonrpc.ErrInternal("failed to exchange ice candidate")
	}

	// cause too many status updates, we skip updating status here
	// s.updateUserStatus(ctx, rtcCtx.roomID, rtcCtx.userID, constants.AnchorStatusOnAir)

	//nolint:nilnil
	return nil, nil
}

func (s *Server) handleKeepAlive(mctx jsonrpc.MethodContext[rtcContext], params *json.RawMessage) (any, error) {
	rtcCtx := mctx.Get()
	if !rtcCtx.joined {
		return nil, fmt.Errorf("not joined yet")
	}

	var data struct {
		Status constants.AnchorStatus `json:"status"`
	}
	if err := jsonrpc.ShouldBindParams(params, &data); err == nil && data.Status == "" {
		data.Status = constants.AnchorStatusIdle
	}
	// TODO: data.status validation

	ctx := rtcCtx.reqCtx
	if err := rtcCtx.janus.KeepAlive(ctx); err != nil {
		return nil, fmt.Errorf("failed to keep Janus session alive: %w", err)
	}

	s.mustHoldLock(mctx)
	s.updateUserStatus(ctx, rtcCtx.roomID, rtcCtx.userID, data.Status)

	//nolint:nilnil
	return nil, nil
}

func (*Server) restoreJanusInstance(
	rtcCtx *rtcContext,
	janusAPI janus.API,
	sessionID, handleID int64,
) (janus.Anchor, error) {
	ctx := rtcCtx.reqCtx

	apiInst, err := janusAPI.CreateAnchorInstance(ctx, rtcCtx.connID, sessionID, handleID)
	if err != nil {
		return nil, jsonrpc.ErrInternal("fail to create janus instance")
	}
	// newly created instance, no need to check
	if sessionID == 0 {
		return apiInst, nil
	}

	// check existing instance
	if ok, err := apiInst.Check(ctx); err == nil && ok {
		// call successful, session is valid
		return apiInst, nil
	} else if errors.Is(err, janus.ErrNoneSuccessResponse) {
		// api not success, session expired
		return janusAPI.CreateAnchorInstance(ctx, rtcCtx.connID, 0, 0)
	}
	return nil, jsonrpc.ErrInternal("fail to check janus instance")
}
