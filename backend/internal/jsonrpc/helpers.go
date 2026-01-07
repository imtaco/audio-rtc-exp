package jsonrpc

import (
	"encoding/json"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// ShouldBindParams is a helper to unmarshal and validate params
func ShouldBindParams(params *json.RawMessage, v any) error {
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
func UnmarshalParams(params *json.RawMessage, v any) error {
	return ShouldBindParams(params, v)
}

// Ptr returns a pointer to the passed value.
func Ptr[T any](t T) *T {
	return &t
}

func Get[T any](t *T) T {
	if t == nil {
		var v T
		return v
	}
	return *t
}
