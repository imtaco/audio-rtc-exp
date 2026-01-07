package jsonrpc

import (
	"context"
	"encoding/json"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// Server manages JSON-RPC method handlers
type handlerImpl[T any] struct {
	methods map[string]AsyncMethodHandler[T]
	logger  *log.Logger
}

type peerImpl[T any] struct {
	Handler[T]
	Conn[T]
}

func NewPeer[T any](stream ObjectStream, _ *T, logger *log.Logger) Peer[T] {
	if logger == nil {
		panic("logger cannot be nil")
	}
	h := NewHandler[T](logger)
	return &peerImpl[T]{
		Handler: h,
		Conn:    h.NewConn(stream, new(T)),
	}
}

// NewHandler creates a new RPC server with the given logger
func NewHandler[T any](logger *log.Logger) Handler[T] {
	if logger == nil {
		panic("logger cannot be nil")
	}
	return &handlerImpl[T]{
		methods: make(map[string]AsyncMethodHandler[T]),
		logger:  logger,
	}
}

// Def registers a method handler (thread-safe)
func (s *handlerImpl[T]) Def(method string, handler MethodHandler[T]) {
	if _, ok := s.methods[method]; ok {
		panic("method already defined: " + method)
	}
	s.methods[method] = func(mctx MethodContext[T], params *json.RawMessage, replier Reply) {
		replier(handler(mctx, params))
	}
}

func (s *handlerImpl[T]) DefAsync(method string, handler AsyncMethodHandler[T]) {
	if _, ok := s.methods[method]; ok {
		panic("method already defined: " + method)
	}
	// run with goroutine, so that handler is non-blocking
	// TODO: limit max concurrent goroutines ?
	s.methods[method] = func(mctx MethodContext[T], params *json.RawMessage, replier Reply) {
		go handler(mctx, params, replier)
	}
}

func (s *handlerImpl[T]) NewConn(stream ObjectStream, v *T) Conn[T] {
	return newConn(stream, v, s.handle, s.logger)
}

func (s *handlerImpl[T]) handle(ctx context.Context, conn *connImpl[T], req *Request) {

	s.logger.Debug("RPC request received",
		log.String("method", req.Method),
		log.Any("id", req.ID))

	handler, ok := s.methods[req.Method]
	if !ok {
		s.logger.Warn("Method not found",
			log.Int("len", len(s.methods)),
			log.String("method", req.Method),
			log.Any("id", req.ID))

		_ = conn.replyError(ctx, req.ID, ErrMethodNotFound(req.Method))
		return
	}

	reply := func(result any, err error) {
		if err := s.reply(ctx, conn, req, result, err); err != nil {
			s.logger.Error("Failed to send RPC reply",
				log.String("method", req.Method),
				log.Any("id", req.ID),
				log.Error(err))
		}
	}
	handler(conn.mctx, req.Params, reply)
}

func (s *handlerImpl[T]) reply(
	ctx context.Context,
	conn *connImpl[T],
	req *Request,
	result any,
	err error,
) error {

	if err == nil {
		s.logger.Debug("RPC request completed",
			log.Any("id", req.ID))
		return conn.reply(ctx, req.ID, result)
	}

	if rpcErr, ok := errors.As[*Error](err); ok {
		s.logger.Error("RPC handler returned error",
			log.String("method", req.Method),
			log.Any("id", req.ID),
			log.Int64("error_code", rpcErr.Code),
			log.String("error_message", rpcErr.Message))
		return conn.replyError(ctx, req.ID, rpcErr)
	}
	s.logger.Error("RPC handler returned unexpected error",
		log.String("method", req.Method),
		log.Any("id", req.ID),
		log.Error(err))

	// do not disclose internal error details to client
	return conn.replyError(ctx, req.ID, ErrInternal("unknown error"))
}
