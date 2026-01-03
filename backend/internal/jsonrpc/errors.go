package jsonrpc

import "github.com/imtaco/audio-rtc-exp/internal/errors"

const (
	ErrCodeParseError errors.Code = "parse error"
	ErrClosed         errors.Code = "closed"
)

// Helper functions for error handling
func ErrInvalidParams(message string) *Error {
	return &Error{
		Code:    CodeInvalidParams,
		Message: message,
	}
}

func ErrInvalidRequest(message string) *Error {
	return &Error{
		Code:    CodeInvalidRequest,
		Message: message,
	}
}

func ErrMethodNotFound(method string) *Error {
	return &Error{
		Code:    CodeMethodNotFound,
		Message: "method not found: " + method,
	}
}

func ErrInternal(message string) *Error {
	return &Error{
		Code:    CodeInternalError,
		Message: message,
	}
}

func ErrCustom(code int64, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}
