package jsonrpc

import (
	"encoding/json"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// ShouldBindParams is a helper to unmarshal and validate params
func ShouldBindParams(params *json.RawMessage, v interface{}) error {
	if params == nil {
		return ErrInvalidParams("params required")
	}
	if err := json.Unmarshal(*params, v); err != nil {
		return ErrInvalidParams("invalid params")
	}
	if err := validate.Struct(v); err != nil {
		return ErrInvalidParams("invalid params")
	}
	return nil
}

// UnmarshalParams is deprecated, use ShouldBindParams instead
func UnmarshalParams(params *json.RawMessage, v interface{}) error {
	return ShouldBindParams(params, v)
}
