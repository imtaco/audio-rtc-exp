package signal

import (
	"net/http"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	wsrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/websocket"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/google/uuid"
)

func NewWSHook(
	connMgr *WSConnManager,
	connGuard ConnectionGuard,
	jwtAuth jwt.Auth,
	logger *log.Logger,
) wsrpc.ConnectionHooks[rtcContext] {
	return &wsHookImpl{
		connMgr:   connMgr,
		connGuard: connGuard,
		jwtAuth:   jwtAuth,
		logger:    logger,
	}
}

type wsHookImpl struct {
	connMgr   *WSConnManager
	connGuard ConnectionGuard
	jwtAuth   jwt.Auth
	logger    *log.Logger
}

func (h *wsHookImpl) OnVerify(r *http.Request) (*rtcContext, bool, error) {
	// Extract JWT from query parameter or header
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}
	if token == "" {
		return nil, false, nil
	}

	payload, err := h.jwtAuth.Verify(token)
	if err != nil {
		if errors.Is(err, jwt.ErrInvalidToken) || errors.Is(err, jwt.ErrNoToken) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rctCtx := &rtcContext{
		userID: payload.UserID,
		roomID: payload.RoomID,
		reqCtx: r.Context(),
		// rlimiter: rate.NewLimiter(1, 1),
	}

	return rctCtx, true, nil
}

func (h *wsHookImpl) OnConnect(mctx jsonrpc.MethodContext[rtcContext]) {
	rctCtx := mctx.Get()
	connID := uuid.New().String()
	rctCtx.connID = connID

	if ok, err := h.connGuard.MustHold(mctx); err != nil {
		h.logger.Error("Failed to acquire connect lock", log.Error(err))
	} else if !ok {
		return
	}

	h.connMgr.AddClient(connID, rctCtx.roomID, mctx.Peer())
	h.logger.Info("Client connected",
		log.String("connId", rctCtx.connID),
		log.String("userId", rctCtx.userID),
		log.String("roomId", rctCtx.roomID),
	)
}

func (h *wsHookImpl) OnDisconnect(mctx jsonrpc.MethodContext[rtcContext], errCode int) {
	rctCtx := mctx.Get()
	connID := rctCtx.connID
	h.connMgr.RemoveClient(connID)

	h.logger.Info("Client disconnected",
		log.String("connId", connID),
		log.Int("errorCode", errCode),
	)

	if err := h.connGuard.Release(mctx); err != nil {
		h.logger.Error("Failed to release connect lock", log.Error(err))
	}
}
