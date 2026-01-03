package websocket

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/imtaco/audio-rtc-exp/internal/jsonrpc"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// Server manages JSON-RPC method handlers
// Thread-safe, allows registering methods even after server starts
type Server[T any] struct {
	jsonrpc.Handler[T]
	hooks          ConnectionHooks[T]
	allowedOrigins []string
	logger         *log.Logger
}

// NewServer creates a new RPC server with the given logger
// If logger is nil, a no-op logger will be used
func NewServer[T any](
	hooks ConnectionHooks[T],
	allowedOrigins []string,
	logger *log.Logger,
) *Server[T] {
	if logger == nil {
		panic("logger cannot be nil")
	}
	if hooks == nil {
		hooks = &defaultHooks[T]{}
	}
	server := &Server[T]{
		Handler:        jsonrpc.NewHandler[T](logger),
		allowedOrigins: allowedOrigins,
		hooks:          hooks,
		logger:         logger,
	}
	return server
}

// HandleWebSocket handles WebSocket connection upgrade and JSON-RPC communication
func (s *Server[T]) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Create connection-specific store and handler
	initValue, passed, err := s.hooks.OnVerify(r)
	if err != nil {
		s.logger.Warn("Connection verification error",
			log.String("remote_addr", r.RemoteAddr),
			log.Error(err))
		http.Error(w, "fail to verify", http.StatusInternalServerError)
		return
	} else if !passed {
		s.logger.Info("Connection verification failed",
			log.String("remote_addr", r.RemoteAddr))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// TODO: setup origin check
		OriginPatterns: s.allowedOrigins,
	})
	if err != nil {
		s.logger.Error("WebSocket open failed",
			log.String("remote_addr", r.RemoteAddr),
			log.Error(err))
		return
	}

	stream := newStream(wsConn, s.logger)
	rpcConn := s.Handler.NewConn(stream, initValue)

	s.logger.Info("WebSocket connection established",
		log.String("remote_addr", r.RemoteAddr),
		log.String("user_agent", r.UserAgent()))

	// Create object stream from WebSocket
	// stream.conn = wsConn
	s.hooks.OnConnect(rpcConn.Context())
	if err := rpcConn.Open(r.Context()); err != nil {
		s.logger.Error("Failed to open RPC connection",
			log.String("remote_addr", r.RemoteAddr),
			log.Error(err))
		return
	}

	// Wait for connection to close
	stream.wait()

	// Call OnDisconnect hook
	// TODO: fix close code
	s.hooks.OnDisconnect(rpcConn.Context(), 1006)
}
