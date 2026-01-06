package jwt

import (
	"github.com/golang-jwt/jwt/v5"
)

// Auth handles JWT authentication
type Auth interface {
	Sign(userID, roomID string) (string, error)
	Verify(tokenString string) (*Payload, error)
}

// Payload represents the JWT token payload
type Payload struct {
	UserID string `json:"userId"`
	RoomID string `json:"roomId"`
	jwt.RegisteredClaims
}
