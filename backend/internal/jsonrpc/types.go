package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
)

type Handler[T any] interface {
	pureHandler[T]
	// all connections created by this handler will share the same method handlers (Def & DefAsync)
	NewConn(stream ObjectStream, v *T) Conn[T]
}

// Peer combines Conn and Handler interfaces
type Peer[T any] interface {
	Conn[T]
	pureHandler[T]
}

type Client[T any] interface {
	Call(ctx context.Context, method string, params, result interface{}) error
	Notify(ctx context.Context, method string, params interface{}) error
	io.Closer
}

type Conn[T any] interface {
	Client[T]
	Open(ctx context.Context) error
	Context() MethodContext[T]
}

type pureHandler[T any] interface {
	Def(method string, handler MethodHandler[T])
	DefAsync(method string, handler AsyncMethodHandler[T])
}

// MethodHandler is a function that handles a JSON-RPC method
// method context is shared across all method calls for a connection
type MethodHandler[T any] func(mctx MethodContext[T], params *json.RawMessage) (interface{}, error)

// AsyncMethodHandler is a function that handles a JSON-RPC method asynchronously
type AsyncMethodHandler[T any] func(mctx MethodContext[T], params *json.RawMessage, reply Reply)

type Reply func(result interface{}, err error)

type ObjectStream interface {
	Open(ctx context.Context) error
	Read(ctx context.Context, v interface{}) error
	Write(ctx context.Context, obj interface{}) error
	io.Closer
}
