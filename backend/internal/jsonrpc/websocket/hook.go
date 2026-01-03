package websocket

import (
	"net/http"

	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
)

// ConnectionHooks allows customizing connection lifecycle behavior
type ConnectionHooks[T any] interface {
	// OnVerify is called before upgrading to WebSocket
	// Return false to reject the connection
	OnVerify(r *http.Request) (*T, bool, error)

	// OnConnect is called after WebSocket connection is established
	OnConnect(mctx jsonrpc.MethodContext[T])

	// OnDisconnect is called when WebSocket connection is closed
	OnDisconnect(mctx jsonrpc.MethodContext[T], closeCode int)
}

// DefaultHooks provides default no-op implementations for ConnectionHooks
// Embed this in your custom hooks struct to only override methods you need
type defaultHooks[T any] struct{}

func (h *defaultHooks[T]) OnVerify(*http.Request) (*T, bool, error) {
	return nil, false, nil
}

func (h *defaultHooks[T]) OnConnect(jsonrpc.MethodContext[T]) {}

func (h *defaultHooks[T]) OnDisconnect(jsonrpc.MethodContext[T], int) {}

// NOTE: Standard WebSocket close codes
// WEB_SOCKET_SUCCESS_CLOSE_STATUS = 1000,
// WEB_SOCKET_ENDPOINT_UNAVAILABLE_CLOSE_STATUS = 1001,
// WEB_SOCKET_PROTOCOL_ERROR_CLOSE_STATUS = 1002,
// WEB_SOCKET_INVALID_DATA_TYPE_CLOSE_STATUS = 1003,
// WEB_SOCKET_EMPTY_CLOSE_STATUS = 1005,
// WEB_SOCKET_ABORTED_CLOSE_STATUS = 1006,
// WEB_SOCKET_INVALID_PAYLOAD_CLOSE_STATUS = 1007,
// WEB_SOCKET_POLICY_VIOLATION_CLOSE_STATUS = 1008,
// WEB_SOCKET_MESSAGE_TOO_BIG_CLOSE_STATUS = 1009,
// WEB_SOCKET_UNSUPPORTED_EXTENSIONS_CLOSE_STATUS = 1010,
// WEB_SOCKET_SERVER_ERROR_CLOSE_STATUS = 1011,
// WEB_SOCKET_SECURE_HANDSHAKE_ERROR_CLOSE_STATUS = 1015
