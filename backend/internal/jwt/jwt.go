package jwt

import (
	"github.com/golang-jwt/jwt/v5"

	"github.com/imtaco/audio-rtc-exp/internal/errors"
)

// NewJWTAuth creates a new JWT authenticator with HS256 algorithm (default)
func NewJWTAuth(secret string) JWTAuth {
	return NewJWTAuthWithAlgorithm(secret, jwt.SigningMethodHS256)
}

// NewJWTAuthWithAlgorithm creates a new JWT authenticator with specified algorithm
// Supported algorithms: HS256, HS384, HS512
func NewJWTAuthWithAlgorithm(secret string, method jwt.SigningMethod) JWTAuth {
	allowedMethods := map[string]bool{
		method.Alg(): true,
	}
	return &jwtAuthImpl{
		secret:         []byte(secret),
		signingMethod:  method,
		allowedMethods: allowedMethods,
	}
}

type jwtAuthImpl struct {
	secret         []byte
	signingMethod  jwt.SigningMethod
	allowedMethods map[string]bool
}

// Sign creates a JWT token for the given user and room
func (j *jwtAuthImpl) Sign(userID, roomID string) (string, error) {
	if userID == "" || roomID == "" {
		return "", errors.New(ErrInvalidRequest, "userID and roomID are required")
	}

	claims := &JWTPayload{
		UserID: userID,
		RoomID: roomID,
	}

	token := jwt.NewWithClaims(j.signingMethod, claims)
	return token.SignedString(j.secret)
}

// Verify verifies a JWT token with strict algorithm validation
func (j *jwtAuthImpl) Verify(tokenString string) (*JWTPayload, error) {
	if tokenString == "" {
		return nil, ErrNoToken
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTPayload{}, func(token *jwt.Token) (interface{}, error) {
		// Strictly validate the algorithm matches what we expect
		alg := token.Method.Alg()
		if !j.allowedMethods[alg] {
			return nil, errors.Newf(
				ErrInvalidToken,
				"unexpected signing method: %s (expected: %s)",
				alg, j.signingMethod.Alg(),
			)
		}
		return j.secret, nil
	})

	if err != nil {
		return nil, errors.Wrap(ErrInvalidToken, err, "missing required fields in token")
	}

	if claims, ok := token.Claims.(*JWTPayload); ok && token.Valid {
		if claims.UserID == "" || claims.RoomID == "" {
			return nil, errors.New(ErrInvalidToken, "missing required fields in token")
		}
		return claims, nil
	}

	return nil, ErrInvalidToken
}
