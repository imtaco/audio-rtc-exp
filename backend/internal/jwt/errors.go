package jwt

import "github.com/imtaco/audio-rtc-exp/internal/errors"

const (
	ErrInvalidRequest errors.Code = "invalid request"
	ErrInvalidToken   errors.Code = "invalid token"
	ErrNoToken        errors.Code = "no token"
)
