package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	redisrpc "github.com/imtaco/audio-rtc-exp/internal/jsonrpc/redis"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/users"
)

// wsConnManager manages WebSocket connections and broadcasts messages to clients in rooms
type wsConnManager struct {
	room2clients map[string]map[string]jsonrpc.Conn[rtcContext] // roomId -> connId -> Client
	client2room  map[string]string                              // connId -> roomId
	clientsMux   sync.RWMutex
	peer2ws      jsonrpc.Peer[interface{}]
	logger       *log.Logger
}

func NewWSConnMgr(
	redisClient *redis.Client,
	wsStreamName string,
	logger *log.Logger,
) (*wsConnManager, error) {
	peer2ws, err := redisrpc.NewPeer[interface{}](
		redisClient,
		"", // consumer only, no need to specify producer name
		wsStreamName,
		"", // broadcast to all consumers, no need to specify group name
		logger.Module("RPCWsIN"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create WS RPC peer: %w", err)
	}

	return &wsConnManager{
		peer2ws:      peer2ws,
		room2clients: make(map[string]map[string]jsonrpc.Conn[rtcContext]),
		client2room:  make(map[string]string),
		logger:       logger,
	}, nil
}

func (m *wsConnManager) Start(ctx context.Context) error {
	m.logger.Info("Starting WebSocket client manager")
	m.register()

	if err := m.peer2ws.Open(ctx); err != nil {
		return fmt.Errorf("failed to open WS RPC peer: %w", err)
	}
	return nil
}

func (m *wsConnManager) Stop(ctx context.Context) error {
	m.logger.Info("Stopping WebSocket client manager")
	if err := m.peer2ws.Close(); err != nil {
		m.logger.Error("Failed to close WS RPC peer", log.Error(err))
	}
	return nil
}

func (m *wsConnManager) register() {
	m.peer2ws.Def("broadcastRoomStatus", m.handleBroadcast)

}

func (m *wsConnManager) handleBroadcast(
	mctx jsonrpc.MethodContext[interface{}],
	params *json.RawMessage,
) (interface{}, error) {

	m.logger.Debug("Handle broadcastRoomStatus")

	var req users.NotifyRoomStatus
	if err := jsonrpc.ShouldBindParams(params, &req); err != nil {
		return nil, err
	}

	m.logger.Debug("broadcastRoomStatus request", log.Any("req", req))
	m.notifyRoomLocalPeer(req.RoomID, "roomStatus", req.Members)

	return nil, nil
}

func (m *wsConnManager) AddClient(connID, roomID string, peer jsonrpc.Conn[rtcContext]) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	m.client2room[connID] = roomID

	room, ok := m.room2clients[roomID]
	if !ok {
		room = make(map[string]jsonrpc.Conn[rtcContext])
		m.room2clients[roomID] = room
	}
	room[connID] = peer

	m.logger.Debug("Client joined",
		log.String("connId", connID),
		log.String("roomId", roomID),
	)
}

func (m *wsConnManager) RemoveClient(connID string) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	roomID, ok := m.client2room[connID]
	if !ok {
		return
	}
	if room, ok := m.room2clients[roomID]; ok {
		delete(room, connID)
		if len(room) == 0 {
			delete(m.room2clients, roomID)
		}
	}

	delete(m.client2room, connID)

	m.logger.Debug("Client removed from room",
		log.String("connId", connID),
		log.String("roomId", roomID),
	)
}

func (m *wsConnManager) RemoveRoom(roomID string) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	room, ok := m.room2clients[roomID]
	if !ok {
		return
	}

	for connID := range room {
		delete(m.client2room, connID)
	}
	delete(m.room2clients, roomID)

	m.logger.Debug("Room removed", log.String("roomId", roomID))
}

func (m *wsConnManager) getRoomConns(roomId string) []jsonrpc.Conn[rtcContext] {
	m.clientsMux.RLock()
	defer m.clientsMux.RUnlock()

	clients := m.room2clients[roomId]
	if clients == nil {
		return nil
	}

	conns := make([]jsonrpc.Conn[rtcContext], 0, len(clients))
	for _, client := range clients {
		conns = append(conns, client)
	}
	return conns
}

func (m *wsConnManager) notifyRoomLocalPeer(
	roomId,
	method string,
	data interface{}) {

	conns := m.getRoomConns(roomId)
	if conns == nil {
		return
	}

	// TODO: goroutine pool ?!
	for _, conn := range conns {
		ctx := conn.Context().Get().reqCtx
		if err := conn.Notify(ctx, method, data); err != nil {
			m.logger.Error("Failed to send to client",
				log.String("roomId", roomId),
				log.Error(err),
			)
		}
	}

	m.logger.Debug("Notified room local peers", log.String("roomId", roomId))
}
