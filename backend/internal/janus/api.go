package janus

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

const (
	janusPluginAudioBridge = "janus.plugin.audiobridge"
	janusAPITimeout        = 10 * time.Second
)

var (
	client = resty.New().
		SetHeader("Content-Type", "application/json").
		SetTimeout(janusAPITimeout)
)

// API manages Janus sessions and handles.
type apiImpl struct {
	baseURL string
	logger  *log.Logger
}

// New creates a Janus API helper backed by go-resty.
func New(baseURL string, logger *log.Logger) API {
	if logger == nil {
		panic("logger is required")
	}
	// TODO: timeout configurable ?
	return &apiImpl{
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  logger,
	}
}

// CreateAnchorInstance returns an anchor handle, creating session/handle IDs when needed.
func (api *apiImpl) CreateAnchorInstance(
	ctx context.Context,
	clientID string,
	sessionID int64,
	handleID int64,
) (Anchor, error) {
	var err error
	if sessionID == 0 {
		sessionID, err = api.createSession(ctx)
		if err != nil {
			return nil, err
		}
	}
	if handleID == 0 {
		handleID, err = api.attach(ctx, sessionID)
		if err != nil {
			return nil, err
		}
	}
	return newAnchorInstance(api, clientID, sessionID, handleID), nil
}

// CreateAdminInstance creates a fresh admin session/handle pair.
func (api *apiImpl) CreateAdminInstance(ctx context.Context, adminKey string) (Admin, error) {
	sessionID, err := api.createSession(ctx)
	if err != nil {
		return nil, err
	}
	handleID, err := api.attach(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return newAdminInstance(api, sessionID, handleID, adminKey), nil
}

func (api *apiImpl) createSession(ctx context.Context) (int64, error) {
	body := map[string]interface{}{
		"janus": "create",
	}
	resp, err := api.post(ctx, "/janus", body)
	if err != nil {
		return 0, err
	}
	// TODO: check success
	if resp.Data == nil {
		return 0, errors.New(ErrInvalidPayload, "janus create session missing data")
	}
	return resp.Data.ID, nil
}

func (api *apiImpl) attach(ctx context.Context, sessionID int64) (int64, error) {
	body := map[string]interface{}{
		"janus":      "attach",
		"session_id": sessionID,
		"plugin":     janusPluginAudioBridge,
	}
	path := fmt.Sprintf("/janus/%d", sessionID)
	resp, err := api.post(ctx, path, body)
	if err != nil {
		return 0, err
	}
	// TODO: check success
	if resp.Data == nil {
		return 0, errors.New(ErrInvalidPayload, "janus attach missing data")
	}
	return resp.Data.ID, nil
}

func (api *apiImpl) post(ctx context.Context, path string, payload map[string]interface{}) (*JanusResponse, error) {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	if _, ok := payload["transaction"]; !ok {
		payload["transaction"] = genTransaction()
	}
	api.logger.Debug("janus req", log.String("path", path), log.Any("body", payload))

	var respPayload JanusResponse
	resp, err := client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&respPayload).
		Post(api.baseURL + path)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.Newf(ErrNoneSuccessResponse, "janus http error: (code: %d, resp %v)", resp.StatusCode(), resp.Error())
	}
	api.logger.Debug("janus resp", log.Int("status", resp.StatusCode()), log.Any("payload", respPayload))

	if err := checkSuccess(&respPayload); err != nil {
		return nil, err
	}
	return &respPayload, nil
}

var txCounter uint64

func genTransaction() string {
	id := atomic.AddUint64(&txCounter, 1)
	return fmt.Sprintf("tx-%d", id)
}
