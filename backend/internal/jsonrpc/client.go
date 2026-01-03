package jsonrpc

import (
	"context"
	"time"
)

func TimeoutClient[T any](conn Conn[T], timeout time.Duration) Client[T] {
	if timeout <= 0 {
		panic("timeout must be greater than zero")
	}
	return &timeoutConn[T]{
		Conn:    conn,
		timeout: timeout,
	}
}

type timeoutConn[T any] struct {
	Conn[T]
	timeout time.Duration
}

func (c *timeoutConn[T]) Call(
	ctx context.Context,
	method string,
	params, result interface{},
) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.Conn.Call(ctx, method, params, result)
}

func (c *timeoutConn[T]) Notify(
	ctx context.Context,
	method string,
	params interface{},
) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.Conn.Notify(ctx, method, params)
}
