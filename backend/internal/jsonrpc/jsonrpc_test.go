package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/utils"

	"github.com/stretchr/testify/suite"
)

type JSONRPCSuite struct {
	suite.Suite
	stream *stubStream
	conn   *connImpl[map[string]string]
}

func TestJSONRPCSuite(t *testing.T) {
	suite.Run(t, new(JSONRPCSuite))
}

func (s *JSONRPCSuite) SetupTest() {
	s.stream = newStubStream()
	logger := log.NewTest(s.T())
	handler := func(context.Context, *connImpl[map[string]string], *Request) {}
	s.conn = newConn(s.stream, nil, handler, logger)
}

func (s *JSONRPCSuite) newHandler() *handlerImpl[map[string]string] {
	return NewHandler[map[string]string](log.NewTest(s.T())).(*handlerImpl[map[string]string])
}

func (s *JSONRPCSuite) newConnWithHandler(handler handlerFunc[map[string]string]) (*connImpl[map[string]string], *stubStream) {
	stream := newStubStream()
	if handler == nil {
		handler = func(context.Context, *connImpl[map[string]string], *Request) {}
	}
	conn := newConn(stream, nil, handler, log.NewTest(s.T()))
	return conn, stream
}

func (s *JSONRPCSuite) TestNewCoreRequiresLogger() {
	s.Panics(func() {
		NewHandler[map[string]string](nil)
	})
}

func (s *JSONRPCSuite) TestDefRejectsDuplicateMethods() {
	core := s.newHandler()
	h := func(MethodContext[map[string]string], *json.RawMessage) (interface{}, error) {
		return nil, nil
	}
	core.Def("sum", h)
	s.Panics(func() {
		core.Def("sum", h)
	})
}

func (s *JSONRPCSuite) TestHandleMethodNotFoundSendsError() {
	core := s.newHandler()
	conn, stream := s.newConnWithHandler(nil)
	req := &Request{ID: newStringID("1"), Method: "missing"}
	core.handle(context.Background(), conn, req)
	s.Require().Len(stream.writes, 1)
	s.NotNil(stream.writes[0].Error)
	s.EqualValues(CodeMethodNotFound, stream.writes[0].Error.Code)
}

func (s *JSONRPCSuite) TestHandleDispatchesRegisteredHandler() {
	core := s.newHandler()
	core.Def("echo", func(MethodContext[map[string]string], *json.RawMessage) (interface{}, error) {
		return map[string]string{"status": "ok"}, nil
	})
	conn, stream := s.newConnWithHandler(nil)
	req := &Request{ID: newStringID("2"), Method: "echo"}
	core.handle(context.Background(), conn, req)
	s.Require().Len(stream.writes, 1)
	var out map[string]string
	s.Require().NoError(json.Unmarshal(*stream.writes[0].Result, &out))
	s.Equal("ok", out["status"])
}

func (s *JSONRPCSuite) TestDefAsyncRunsHandler() {
	core := s.newHandler()
	done := make(chan struct{})
	core.DefAsync("async", func(_ MethodContext[map[string]string], _ *json.RawMessage, reply Reply) {
		reply(map[string]string{"mode": "async"}, nil)
		close(done)
	})
	conn, stream := s.newConnWithHandler(nil)
	req := &Request{ID: newStringID("3"), Method: "async"}
	core.handle(context.Background(), conn, req)
	<-done
	s.Require().Len(stream.writes, 1)
	var out map[string]string
	s.Require().NoError(json.Unmarshal(*stream.writes[0].Result, &out))
	s.Equal("async", out["mode"])
}

func (s *JSONRPCSuite) TestCoreReplyWithRPCErrors() {
	core := s.newHandler()
	conn, stream := s.newConnWithHandler(nil)
	req := &Request{ID: newStringID("4"), Method: "rpc"}
	rpcErr := ErrInvalidRequest("bad")
	s.Require().NoError(core.reply(context.Background(), conn, req, nil, rpcErr))
	s.Require().Len(stream.writes, 1)
	s.EqualValues(rpcErr.Code, stream.writes[0].Error.Code)
}

func (s *JSONRPCSuite) TestCoreReplyWithUnexpectedErrors() {
	core := s.newHandler()
	conn, stream := s.newConnWithHandler(nil)
	req := &Request{ID: newStringID("5"), Method: "rpc"}
	unexpected := errors.New("boom")
	s.Require().NoError(core.reply(context.Background(), conn, req, nil, unexpected))
	s.Require().Len(stream.writes, 1)
	s.EqualValues(CodeInternalError, stream.writes[0].Error.Code)
}

func (s *JSONRPCSuite) TestNotifySendsNotification() {
	s.Require().NoError(s.conn.Notify(context.Background(), "ping", map[string]int{"v": 1}))
	s.Require().Len(s.stream.writes, 1)
	s.Nil(s.stream.writes[0].ID)
	s.Equal(typeNotification, s.stream.writes[0].msgType)
}

func (s *JSONRPCSuite) TestCallPropagatesSendError() {
	s.stream.writeErr = errors.New("send failed")
	err := s.conn.Call(context.Background(), "sum", map[string]int{"v": 1}, nil)
	s.Error(err)
}

func (s *JSONRPCSuite) TestCallClearsPendingWhenContextCanceled() {
	ctx, cancel := context.WithCancel(context.Background())
	s.stream.writeHook = func(*message) {
		cancel()
	}
	err := s.conn.Call(ctx, "sum", map[string]int{"v": 1}, nil)
	s.Require().ErrorIs(err, context.Canceled)

	empty := true
	s.conn.pendings.Range(func(_, _ interface{}) bool {
		empty = false
		return false
	})
	s.True(empty, "pending calls should be cleared when call exits abnormally")
}

func (s *JSONRPCSuite) TestRequestAndResponseIncludeJSONRPCVersion() {
	req, err := newRequestMessage("sum", map[string]int{"a": 1})
	s.Require().NoError(err)
	s.Equal("2.0", req.JSONRPC)

	id := newStringID("resp")
	resp, err := newResponseMessage(*id, map[string]string{"ok": "yes"}, nil)
	s.Require().NoError(err)
	s.Equal("2.0", resp.JSONRPC)
}

func (s *JSONRPCSuite) TestNewRequestMessageSetsFields() {
	msg, err := newRequestMessage("sum", map[string]int{"a": 1})
	s.Require().NoError(err)
	s.Require().NotNil(msg.ID)
	s.True(msg.ID.IsSet())
	s.Require().NotNil(msg.Method)
	s.Equal("sum", *msg.Method)
	s.Equal(typeRequst, msg.msgType)
	s.Equal("{\"a\":1}", string(*msg.Params))
}

func (s *JSONRPCSuite) TestNewNotificationMessageClearsID() {
	msg, err := newNotificationMessage("ping", map[string]string{"status": "ok"})
	s.Require().NoError(err)
	s.Nil(msg.ID)
	s.Equal(typeNotification, msg.msgType)
	s.Equal("{\"status\":\"ok\"}", string(*msg.Params))
}

func (s *JSONRPCSuite) TestNewResponseMessageEncodesResult() {
	id := newStringID("abc")
	msg, err := newResponseMessage(*id, map[string]string{"ready": "yes"}, nil)
	s.Require().NoError(err)
	s.Require().NotNil(msg.ID)
	s.Equal(id.String(), msg.ID.String())
	s.Equal(typeResponse, msg.msgType)
	s.Equal("{\"ready\":\"yes\"}", string(*msg.Result))
	s.Nil(msg.Error)
}

func (s *JSONRPCSuite) TestMessageValidateClassifiesMessages() {
	method := "echo"
	raw := json.RawMessage("{}")

	req := &message{Method: &method, ID: newStringID("1"), Params: &raw}
	req.validate()
	s.Equal(typeRequst, req.msgType)

	notify := &message{Method: &method, Params: &raw}
	notify.validate()
	s.Equal(typeNotification, notify.msgType)

	resp := &message{ID: newStringID("2"), Result: utils.Ptr(json.RawMessage("{}"))}
	resp.validate()
	s.Equal(typeResponse, resp.msgType)

	invalid := &message{Method: &method, Result: utils.Ptr(json.RawMessage("{}"))}
	invalid.validate()
	s.Equal(typeUnknown, invalid.msgType)
}

func (s *JSONRPCSuite) TestMessageValidateUnknownWithoutResultOrID() {
	msg := &message{}
	msg.validate()
	s.Equal(typeUnknown, msg.msgType)
}

func (s *JSONRPCSuite) TestShouldBindParamsValidation() {
	var dst struct {
		Value int `json:"value" validate:"required,min=1"`
	}

	err := ShouldBindParams(nil, &dst)
	s.Require().Error(err)
	s.EqualValues(CodeInvalidParams, err.(*Error).Code)

	raw := json.RawMessage("{\"value\":\"bad\"")
	err = ShouldBindParams(&raw, &dst)
	s.Require().Error(err)
	s.EqualValues(CodeInvalidParams, err.(*Error).Code)

	raw = json.RawMessage("{\"value\":5}")
	s.Require().NoError(ShouldBindParams(&raw, &dst))
	s.Equal(5, dst.Value)

	// Test validation error
	raw = json.RawMessage("{\"value\":0}")
	err = ShouldBindParams(&raw, &dst)
	s.Require().Error(err)
	s.EqualValues(CodeInvalidParams, err.(*Error).Code)
}

func (s *JSONRPCSuite) TestUnmarshalParamsValidation() {
	var dst struct {
		Value int `json:"value"`
	}

	err := UnmarshalParams(nil, &dst)
	s.Require().Error(err)
	s.EqualValues(CodeInvalidParams, err.(*Error).Code)

	raw := json.RawMessage("{\"value\":\"bad\"")
	err = UnmarshalParams(&raw, &dst)
	s.Require().Error(err)
	s.EqualValues(CodeInvalidParams, err.(*Error).Code)

	raw = json.RawMessage("{\"value\":5}")
	s.Require().NoError(UnmarshalParams(&raw, &dst))
	s.Equal(5, dst.Value)
}

func (s *JSONRPCSuite) TestSendStoresPendingRequests() {
	msg, err := newRequestMessage("sum", map[string]int{"v": 1})
	s.Require().NoError(err)

	done, err := s.conn.send(context.Background(), msg)
	s.Require().NoError(err)
	s.NotNil(done)

	_, ok := s.conn.pendings.Load(*msg.ID)
	s.True(ok)
}

func (s *JSONRPCSuite) TestSendRemovesPendingWhenWriteFails() {
	msg, err := newRequestMessage("sum", map[string]int{"v": 1})
	s.Require().NoError(err)
	s.stream.writeErr = errors.New("boom")

	done, err := s.conn.send(context.Background(), msg)
	s.Require().Error(err)
	s.Nil(done)

	_, ok := s.conn.pendings.Load(*msg.ID)
	s.False(ok)
}

func (s *JSONRPCSuite) TestSendRejectsClosedConn() {
	msg, err := newRequestMessage("sum", map[string]int{"v": 1})
	s.Require().NoError(err)
	s.conn.closed.Store(true)

	done, err := s.conn.send(context.Background(), msg)
	s.Require().ErrorIs(err, ErrClosed)
	s.Nil(done)
}

func (s *JSONRPCSuite) TestWaitReturnsContextError() {
	id := *newStringID("ctx")
	done := make(doneChan, 1)
	s.conn.pendings.Store(id, done)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.conn.wait(ctx, &id, done, nil)
	s.Require().ErrorIs(err, context.Canceled)

	_, ok := s.conn.pendings.Load(id)
	s.False(ok)
}

func (s *JSONRPCSuite) TestWaitUnmarshalsResult() {
	id := *newStringID("ok")
	done := make(doneChan, 1)
	done <- &message{Result: utils.Ptr(json.RawMessage("{\"value\":\"done\"}"))}

	var dst struct {
		Value string `json:"value"`
	}
	err := s.conn.wait(context.Background(), &id, done, &dst)
	s.Require().NoError(err)
	s.Equal("done", dst.Value)
}

func (s *JSONRPCSuite) TestWaitPropagatesRPCErrors() {
	id := *newStringID("err")
	done := make(doneChan, 1)
	rpcErr := ErrInvalidRequest("bad")
	done <- &message{Error: rpcErr}

	err := s.conn.wait(context.Background(), &id, done, nil)
	s.Require().Equal(rpcErr, err)
}

func (s *JSONRPCSuite) TestReadLoopDispatchesRequests() {
	reqCh := make(chan *Request, 1)
	handler := func(_ context.Context, _ *connImpl[map[string]string], req *Request) {
		reqCh <- req
	}
	conn, stream := s.newConnWithHandler(handler)
	method := "hello"
	id := newStringID("req")
	stream.enqueueRead(&message{ID: id, Method: &method})
	conn.readLoop(context.Background())
	got := <-reqCh
	s.Equal("hello", got.Method)
	s.True(stream.closed)
}

func (s *JSONRPCSuite) TestReadLoopDeliversResponses() {
	conn, stream := s.newConnWithHandler(nil)
	id := newStringID("resp")
	done := make(doneChan, 1)
	conn.pendings.Store(*id, done)
	stream.enqueueRead(&message{ID: id, Result: utils.Ptr(json.RawMessage("{\"value\":42}"))})
	conn.readLoop(context.Background())
	resp := <-done
	s.Equal("{\"value\":42}", string(*resp.Result))
	s.True(stream.closed)
}

func (s *JSONRPCSuite) TestReadLoopIgnoresResponseWithUnmatchedID() {
	conn, stream := s.newConnWithHandler(nil)
	trackedID := *newStringID("tracked")
	done := make(doneChan, 1)
	conn.pendings.Store(trackedID, done)
	conn.closed.Store(true)
	unmatched := newStringID("other")
	stream.enqueueRead(&message{ID: unmatched, Result: utils.Ptr(json.RawMessage("\"noop\""))})
	conn.readLoop(context.Background())
	_, ok := conn.pendings.Load(trackedID)
	s.True(ok)
	select {
	case <-done:
		s.Fail("unmatched response should not complete pending call")
	default:
	}
}

type stubStream struct {
	writes    []*message
	writeErr  error
	readErr   error
	closed    bool
	readQueue []*message
	writeHook func(*message)
}

func newStubStream() *stubStream {
	return &stubStream{}
}

func (s *stubStream) enqueueRead(msg *message) {
	s.readQueue = append(s.readQueue, msg)
}

func (s *stubStream) Open(context.Context) error {
	return nil
}

func (s *stubStream) Read(_ context.Context, dst interface{}) error {
	if s.readErr != nil {
		return s.readErr
	}
	if len(s.readQueue) == 0 {
		return io.EOF
	}
	msg := s.readQueue[0]
	s.readQueue = s.readQueue[1:]
	out := dst.(*message)
	*out = *msg
	return nil
}

func (s *stubStream) Write(_ context.Context, obj interface{}) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	msg := obj.(*message)
	s.writes = append(s.writes, msg)
	if s.writeHook != nil {
		s.writeHook(msg)
	}
	return nil
}

func (s *stubStream) Close() error {
	s.closed = true
	return nil
}
