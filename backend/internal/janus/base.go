package janus

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type baseInstance struct {
	api       *apiImpl
	clientID  string
	sessionID int64
	handleID  int64

	keepaliveMu     sync.Mutex
	keepaliveCancel context.CancelFunc
}

func newBaseInstance(api *apiImpl, clientID string, sessionID int64, handleID int64) *baseInstance {
	return &baseInstance{
		api:       api,
		clientID:  clientID,
		sessionID: sessionID,
		handleID:  handleID,
	}
}

func (b *baseInstance) GetSessionID() int64 {
	return b.sessionID
}

func (b *baseInstance) GetHandleID() int64 {
	return b.handleID
}

func (b *baseInstance) post(ctx context.Context, payload map[string]interface{}) (*JanusResponse, error) {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	payload["session_id"] = b.sessionID
	payload["handle_id"] = b.handleID
	path := fmt.Sprintf("/janus/%d", b.sessionID)
	return b.api.post(ctx, path, payload)
}

// postMessage posts a message with a typed body payload.
func (b *baseInstance) postMessage(ctx context.Context, janus string, body interface{}) (*JanusResponse, error) {
	payload := map[string]interface{}{
		"janus":      janus,
		"session_id": b.sessionID,
		"handle_id":  b.handleID,
	}
	if body != nil {
		payload["body"] = body
	}
	path := fmt.Sprintf("/janus/%d", b.sessionID)
	return b.api.post(ctx, path, payload)
}

// postTrickle posts a trickle ICE candidate.
func (b *baseInstance) postTrickle(ctx context.Context, candidate ICECandidate) (*JanusResponse, error) {
	payload := map[string]interface{}{
		"janus":      "trickle",
		"session_id": b.sessionID,
		"handle_id":  b.handleID,
		"candidate":  candidate,
	}
	path := fmt.Sprintf("/janus/%d", b.sessionID)
	return b.api.post(ctx, path, payload)
}

// postMessageWithJSEP posts a message with body and JSEP.
func (b *baseInstance) postMessageWithJSEP(
	ctx context.Context,
	body interface{},
	jsep *JSEP,
) (*JanusResponse, error) {
	payload := map[string]interface{}{
		"janus":      "message",
		"session_id": b.sessionID,
		"handle_id":  b.handleID,
		"body":       body,
	}
	if jsep != nil {
		payload["jsep"] = jsep
	}
	path := fmt.Sprintf("/janus/%d", b.sessionID)
	return b.api.post(ctx, path, payload)
}

func (b *baseInstance) Close() {
	b.StopKeepalive()
}

func (b *baseInstance) Destroy(ctx context.Context) error {
	b.StopKeepalive()
	body := map[string]interface{}{
		"janus": "destroy",
	}
	_, err := b.post(ctx, body)
	return err
}

func (b *baseInstance) KeepAlive(ctx context.Context) error {
	body := map[string]interface{}{
		"janus": "keepalive",
	}
	_, err := b.post(ctx, body)
	return err
}

func (b *baseInstance) StartKeepalive() {
	b.keepaliveMu.Lock()
	defer b.keepaliveMu.Unlock()
	if b.keepaliveCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.keepaliveCancel = cancel
	go b.runKeepalive(ctx)
}

func (b *baseInstance) StopKeepalive() {
	b.keepaliveMu.Lock()
	defer b.keepaliveMu.Unlock()
	if b.keepaliveCancel != nil {
		b.keepaliveCancel()
		b.keepaliveCancel = nil
	}
}

func (b *baseInstance) runKeepalive(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := b.KeepAlive(ctx); err != nil {
				b.api.logger.Warn("janus keepalive failed", log.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (b *baseInstance) GetEvents(ctx context.Context, maxEvents int) ([]*JanusResponse, error) {
	if maxEvents <= 0 {
		maxEvents = 3
	}
	var payload []*JanusResponse
	path := fmt.Sprintf("/janus/%d", b.sessionID)
	resp, err := client.R().
		SetContext(ctx).
		SetResult(&payload).
		SetQueryParam("maxev", strconv.Itoa(maxEvents)).
		Get(b.api.baseURL + path)
	if err != nil {
		return nil, errors.Wrap(ErrFailedRequest, err, "restify error")
	}
	if resp.IsError() {
		return nil, errors.Newf(ErrNoneSuccessResponse, "janus http error: (code: %d, resp %v)", resp.StatusCode(), resp.Error())
	}
	// TODO: check success ?!
	b.api.logger.Debug("janus events resp", log.Any("payload", payload))
	return payload, nil
}
