package jsonrpc

import "sync/atomic"

type MethodContext[T any] interface {
	Get() *T
	Set(value *T)
	Peer() Conn[T]
}

func NewContext[T any](conn Conn[T], v *T) MethodContext[T] {
	c := &contextImpl[T]{
		conn: conn,
	}
	c.v.Store(v)
	return c
}

// MethodContext contains request context and utilities for RPC operations
type contextImpl[T any] struct {
	conn Conn[T]
	v    atomic.Pointer[T]
}

// Set stores data in the connection-level store (shared across all requests on this connection)
func (m *contextImpl[T]) Set(value *T) {
	m.v.Store(value)
}

// Get retrieves data from the connection-level store
func (m *contextImpl[T]) Get() *T {
	return m.v.Load()
}

func (m *contextImpl[T]) Peer() Conn[T] {
	return m.conn
}
