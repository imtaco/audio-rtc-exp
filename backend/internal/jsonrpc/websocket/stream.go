package websocket

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

const (
	ErrBufferFull errors.Code = "buffer_full"
	ErrMarshal    errors.Code = "marshal_error"
)

const (
	pingInterval = 10 * time.Second
	pingTimeout  = 3 * time.Second
	writeTimeout = 3 * time.Second
	bufMessages  = 16
)

func newStream(conn *websocket.Conn, logger *log.Logger) *wsStream {
	return &wsStream{
		conn:   conn,
		chBuf:  make(chan func() error, bufMessages),
		logger: logger,
	}
}

// wsStream wraps a WebSocket connection to implement jsonrpc2.ObjectStream
type wsStream struct {
	conn  *websocket.Conn
	chBuf chan func() error

	connCtx   context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	logger    *log.Logger
}

// only marshal error or buffer full returne error
func (ws *wsStream) Write(ctx context.Context, obj any) error {
	// rcp reply might not have chance to close the connetion ?

	select {
	case <-ctx.Done():
		return net.ErrClosed
	default:
	}

	action := func() error {
		ctx, cancel := context.WithTimeout(ctx, writeTimeout)
		defer cancel()
		return wsjson.Write(ctx, ws.conn, obj)
	}

	select {
	case ws.chBuf <- action:
		return nil
	default:
		ws.close(ErrBufferFull)
		return ErrBufferFull
	}
}

func (ws *wsStream) Read(ctx context.Context, v any) error {
	// read loop share the same read ctx
	// read failure lead to connection close
	if err := wsjson.Read(ctx, ws.conn, v); err != nil {
		// TODO: what if json unmarshal error ? just ignore for next read ?
		ws.close(err)
		return err
	}
	return nil
}

func (ws *wsStream) Open(ctx context.Context) error {
	ws.connCtx, ws.cancel = context.WithCancel(ctx)

	go func() {
		err := ws.writePump(ws.connCtx)
		ws.close(err)
	}()

	return nil
}

func (ws *wsStream) Close() error {
	ws.close(nil)
	return nil
}

func (ws *wsStream) close(err error) {

	ws.closeOnce.Do(func() {
		closed := false
		code := websocket.StatusNormalClosure

		switch {
		case err == nil:
			ws.logger.Error("connect closed normally")
			code = websocket.StatusNormalClosure
		case func() bool { closeErr, ok := errors.As[*websocket.CloseError](err); return ok && closeErr != nil }():
			closeErr, _ := errors.As[websocket.CloseError](err)
			ws.logger.Error("connect closed", log.Any("code", closeErr.Code))
			closed = true
		case errors.Is(err, net.ErrClosed):
			ws.logger.Error("connect closed, net.ErrClosed")
			closed = true
		case errors.Is(err, ErrBufferFull):
			ws.logger.Error("connect closed due to buffer full")
			code = websocket.StatusPolicyViolation
		default:
			ws.logger.Error("connect closed due to unknown error", log.Error(err))
			code = websocket.StatusInternalError
		}

		if closed {
			_ = ws.conn.CloseNow()
		} else {
			ws.conn.Close(code, "bye")
		}
		ws.cancel()
	})
}

func (ws *wsStream) wait() {
	<-ws.connCtx.Done()
}

func (ws *wsStream) writePump(ctx context.Context) error {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ws.ping(ctx); err != nil {
				return err
			}
		case action, ok := <-ws.chBuf:
			if !ok {
				return net.ErrClosed
			}
			if err := action(); err != nil {
				return err
			}
		}
	}
}

func (ws *wsStream) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	return ws.conn.Ping(ctx)
}
