package jsonrpc

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
	"github.com/imtaco/audio-rtc-exp/internal/utils"
)

type messageType int

const (
	typeUnknown      messageType = iota
	typeRequst       messageType = iota
	typeResponse     messageType = iota
	typeNotification messageType = iota

	jsonRPCVersion = "2.0"
)

type Request struct {
	ID     *ID              `json:"id"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params,omitempty"`
}

type message struct {
	JSONRPC string `json:"jsonrpc,omitempty"`
	ID      *ID    `json:"id,omitempty"`
	// request fields
	Method *string          `json:"method,omitempty"`
	Params *json.RawMessage `json:"params,omitempty"`
	// response fields
	Result *json.RawMessage `json:"result,omitempty"`
	Error  *Error           `json:"error,omitempty"`

	msgType messageType `json:"-"`
}

func (m *message) validate() {
	// TODO: check rpc version ?

	if m.Method != nil {
		if m.Result != nil || m.Error != nil {
			m.msgType = typeUnknown
			return
		}
		if !m.ID.IsSet() {
			m.msgType = typeNotification
		} else {
			m.msgType = typeRequst
		}
	} else if m.Result != nil || m.Error != nil {
		if m.ID.IsSet() {
			m.msgType = typeResponse
		} else {
			m.msgType = typeUnknown
		}
	} else {
		m.msgType = typeUnknown
	}
}

func newRequestMessage(method string, params interface{}) (*message, error) {
	id := newStringID(uuid.New().String())

	bs, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(ErrCodeParseError, err, "failed to marshal params")
	}
	raw := json.RawMessage(bs)
	return &message{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  &method,
		Params:  &raw,
		msgType: typeRequst,
	}, nil
}

func newNotificationMessage(method string, params interface{}) (*message, error) {
	bs, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(ErrCodeParseError, err, "failed to marshal params")
	}
	raw := json.RawMessage(bs)
	return &message{
		JSONRPC: jsonRPCVersion,
		Method:  &method,
		Params:  &raw,
		msgType: typeNotification,
	}, nil
}

func newResponseMessage(id ID, result interface{}, Err *Error) (*message, error) {
	var resultRaw *json.RawMessage
	if Err == nil {
		bs, err := json.Marshal(result)
		if err != nil {
			return nil, errors.Wrap(ErrCodeParseError, err, "failed to marshal result")
		}
		resultRaw = utils.Ptr(json.RawMessage(bs))
	}
	return &message{
		JSONRPC: jsonRPCVersion,
		ID:      &id,
		Result:  resultRaw,
		Error:   Err,
		msgType: typeResponse,
	}, nil
}

// JSON-RPC 2.0 request ID, either a string or integer
type ID struct {
	Num      uint64
	Str      string
	isString bool
}

func newStringID(id string) *ID {
	return &ID{Str: id, isString: true}
}

// func newIntID(id uint64) *ID {
// 	return &ID{Num: id, isString: false}
// }

func (id *ID) IsSet() bool {
	return id != nil && (id.isString || id.Num != 0)
}

func (id *ID) String() string {
	if id.isString {
		return strconv.Quote(id.Str)
	}
	return strconv.FormatUint(id.Num, 10)
}

func (id *ID) MarshalJSON() ([]byte, error) {
	if id.isString {
		return json.Marshal(id.Str)
	}
	return json.Marshal(id.Num)
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *ID) UnmarshalJSON(data []byte) error {
	// Support both uint64 and string IDs.
	var vStr string
	if err := json.Unmarshal(data, &vStr); err == nil {
		*id = ID{Str: vStr, isString: true}
		return nil
	}
	var vInt uint64
	if err := json.Unmarshal(data, &vInt); err != nil {
		return err
	}
	*id = ID{Num: vInt, isString: false}
	return nil
}

// Error represents a JSON-RPC response error.
type Error struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    *json.RawMessage `json:"data,omitempty"`
}

// Error implements the Go error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc error: code %v, message: %s", e.Code, e.Message)
}

// http://www.jsonrpc.org/specification#error_object.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)
