package errors

import (
	stderrors "errors"
	"fmt"

	"github.com/pkg/errors"
)

// Code is a sentinel error for classification (errors.Is).
type Code string

func (c Code) Error() string { return string(c) }

// Error keeps a code and an underlying error (with stack/message from pkg/errors).
type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	// Keep message readable; code is still available via errors.Is(err, Code(...))
	if e.Err == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// Is lets errors.Is(err, ErrNotFound) work even when wrapped.
func (e *Error) Is(target error) bool {
	t, ok := target.(Code)
	if !ok {
		return false
	}
	return e.Code == t
}

func Newf(code Code, format string, args ...any) error {
	return &Error{
		Code: code,
		Err:  errors.Errorf(format, args...),
	}
}

func New(code Code, message string) error {
	return &Error{
		Code: code,
		Err:  errors.New(message),
	}
}

func PureNew(message string) error {
	return stderrors.New(message)
}

// Wrapf wraps an existing error with message+stack.
// If err is nil, returns nil (Go convention).
func Wrapf(code Code, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &Error{
		Code: code,
		Err:  errors.Wrapf(err, format, args...),
	}
}

// If err is nil, returns nil (Go convention).
func Wrap(code Code, err error, message string) error {
	if err == nil {
		return nil
	}
	return &Error{
		Code: code,
		Err:  errors.Wrap(err, message),
	}
}

func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

func As[T error](err error) (*T, bool) {
	var target T
	if errors.As(err, &target) {
		return &target, true
	}
	return nil, false
}
