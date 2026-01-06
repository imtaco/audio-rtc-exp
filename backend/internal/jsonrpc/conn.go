package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

// Handler handles JSON-RPC requests and notifications.
// type Handler interface {
// 	Handle(context.Context, *Conn, *Request)
// }

type handlerFunc[T any] func(context.Context, *connImpl[T], *Request)

type connImpl[T any] struct {
	stream   ObjectStream
	mctx     MethodContext[T]
	handler  handlerFunc[T]
	sendLock sync.Mutex
	closed   atomic.Bool
	pendings sync.Map // map[ID]*call
	logger   *log.Logger
}

func newConn[T any](
	stream ObjectStream,
	v *T,
	handler handlerFunc[T],
	logger *log.Logger,
) *connImpl[T] {
	c := &connImpl[T]{
		stream:   stream,
		closed:   atomic.Bool{},
		pendings: sync.Map{},
		handler:  handler,
		logger:   logger,
	}
	c.mctx = NewContext(c, v)
	return c
}

func (c *connImpl[T]) Open(ctx context.Context) error {
	if err := c.stream.Open(ctx); err != nil {
		return err
	}

	go c.readLoop(ctx)
	return nil
}

func (c *connImpl[T]) Close() error {
	return c.close(nil)
}

func (c *connImpl[T]) Context() MethodContext[T] {
	return c.mctx
}

func (c *connImpl[T]) Call(ctx context.Context, method string, params, result interface{}) error {
	req, err := newRequestMessage(method, params)
	if err != nil {
		return err
	}
	done, err := c.send(ctx, req)
	if err != nil {
		return err
	}
	return c.wait(ctx, req.ID, done, result)
}

func (c *connImpl[T]) Notify(ctx context.Context, method string, params interface{}) error {
	req, err := newNotificationMessage(method, params)
	if err != nil {
		return err
	}
	_, err = c.send(ctx, req)
	return err
}

// reply sends a successful response with a result.
func (c *connImpl[T]) reply(ctx context.Context, id *ID, result interface{}) error {
	if id == nil {
		return nil
	}
	resp, err := newResponseMessage(*id, result, nil)
	if err != nil {
		return err
	}
	_, err = c.send(ctx, resp)
	return err
}

// ReplyWithError sends a response with an error.
func (c *connImpl[T]) replyError(ctx context.Context, id *ID, respErr *Error) error {
	if id == nil {
		return nil
	}

	resp, err := newResponseMessage(*id, nil, respErr)
	if err != nil {
		return err
	}
	_, err = c.send(ctx, resp)
	return err
}

func (c *connImpl[T]) close(err error) error {
	if !c.closed.CompareAndSwap(false, true) {
		// already closed
		return ErrClosed
	}

	// to avoid race condition, first collect all pending keys
	// then delete them with popPending
	key2del := make([]ID, 0)
	c.pendings.Range(func(key, _ interface{}) bool {
		key2del = append(key2del, key.(ID))
		return true
	})
	c.pendings.Clear()

	for _, key := range key2del {
		done := c.popPending(key)
		if done != nil {
			close(done)
		}
	}

	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		c.logger.Error("jsonrpc unknown error", log.Error(err))
	}

	return c.stream.Close()
}

func (c *connImpl[T]) readLoop(ctx context.Context) {
	for {
		var m message
		err := c.stream.Read(ctx, &m)
		// TODO: deal with JSON unmarshal errors ?
		if err != nil {
			c.logger.Error("jsonrpc read loop error", log.Error(err))
			c.close(err)
			return
		}

		if m.Result == nil {
			c.logger.Debug("m.Result is nil")
		} else {
			c.logger.Debug("m.Result is", log.Any("v", *m.Result))
		}

		// validation failure -> UnknownType
		m.validate()

		switch m.msgType {
		case typeRequst, typeNotification:
			c.logger.Debug("jsonrpc handle message", log.Any("msgType", m.msgType))
			req := &Request{
				ID:     m.ID,
				Method: *m.Method,
				Params: m.Params,
			}
			c.logger.Info("jsonrpc handle request", log.Any("req", req))
			c.handler(ctx, c, req)

		case typeResponse:
			if !m.ID.IsSet() {
				c.logger.Debug("ignore response without id")
				continue
			}

			done := c.popPending(*m.ID)
			if done == nil {
				c.logger.Debug("ignore response with unmatched id", log.Any("id", m.ID))
				continue
			}
			done <- &m
			close(done)

		default:
			c.logger.Warn("ignore invalid message: neither request nor response is set")
		}
	}
}

func (c *connImpl[T]) send(ctx context.Context, m *message) (doneChan, error) {
	// not allow concurrent sends
	c.sendLock.Lock()
	defer c.sendLock.Unlock()

	if c.closed.Load() {
		return nil, ErrClosed
	}

	var done doneChan
	if m.msgType == typeRequst {
		done = make(doneChan, 1)
		c.logger.Debug("put to pendings", log.Any("id", m.ID))
		c.pendings.Store(*m.ID, done)
	}

	if err := c.stream.Write(ctx, m); err != nil {
		// delete from pending on send error
		if m.msgType == typeRequst {
			c.pendings.Delete(*m.ID)
		}
		return nil, err
	}
	return done, nil
}

func (c *connImpl[T]) wait(ctx context.Context, id *ID, done doneChan, result interface{}) error {
	select {
	case <-ctx.Done():
		// remove pending on context done (timeout/cancel etc)
		c.pendings.Delete(*id)
		return ctx.Err()

	case resp, ok := <-done:
		if !ok {
			// remove pending on context done (timeout/cancel etc)
			c.pendings.Delete(*id)
			return ErrClosed
		}
		if resp.Error != nil {
			return resp.Error
		}
		if resp.Result != nil && result != nil {
			return json.Unmarshal(*resp.Result, result)
		}
		return nil
	}
}

func (c *connImpl[T]) popPending(id ID) doneChan {
	v, ok := c.pendings.LoadAndDelete(id)
	if !ok {
		return nil
	}
	return v.(doneChan)
}

type doneChan chan *message
