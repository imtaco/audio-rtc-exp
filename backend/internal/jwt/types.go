package jwt

import (
	"github.com/golang-jwt/jwt/v5"
)

// JWTAuth handles JWT authentication
type JWTAuth interface {
	Sign(userID, roomID string) (string, error)
	Verify(tokenString string) (*JWTPayload, error)
}

// JWTPayload represents the JWT token payload
type JWTPayload struct {
	UserID string `json:"userId"`
	RoomID string `json:"roomId"`
	jwt.RegisteredClaims
}
