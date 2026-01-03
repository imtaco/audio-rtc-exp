package janus

import "github.com/imtaco/audio-rtc-exp/internal/errors"

const (
	ErrFailedRequest       errors.Code = "fail to make request"
	ErrInvalidPayload      errors.Code = "invalid payload"
	ErrInvalidResponse     errors.Code = "invalid response"
	ErrNoneSuccessResponse errors.Code = "none success response"
	ErrNotFound            errors.Code = "not found"
	ErrAlreadyExisted      errors.Code = "already existed"
)

// // JanusError indicates Janus responded with a failure payload.
// type JanusError struct {
// 	Message string
// }

// func (e *JanusError) Error() string {
// 	return e.Message
// }

// // JanusExistedError indicates the requested Janus entity already exists.
// type JanusExistedError struct {
// 	Message string
// }

// func (e *JanusExistedError) Error() string {
// 	return e.Message
// }

// // JanusNotFound indicates the requested Janus entity was not found.
// type JanusNotFound struct {
// 	Message string
// }

// func (e *JanusNotFound) Error() string {
// 	return e.Message
// }
