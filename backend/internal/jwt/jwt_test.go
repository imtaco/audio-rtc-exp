package jwt

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuth(t *testing.T) {
	auth := NewAuth("test-secret").(*jwtAuthImpl)
	assert.NotNil(t, auth)
	assert.Equal(t, jwt.SigningMethodHS256, auth.signingMethod)
	assert.True(t, auth.allowedMethods["HS256"])
}

func TestNewAuthWithAlgorithm(t *testing.T) {
	testCases := []struct {
		name   string
		method jwt.SigningMethod
		alg    string
	}{
		{"HS256", jwt.SigningMethodHS256, "HS256"},
		{"HS384", jwt.SigningMethodHS384, "HS384"},
		{"HS512", jwt.SigningMethodHS512, "HS512"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			auth := NewAuthWithAlgorithm("test-secret", tc.method).(*jwtAuthImpl)
			assert.NotNil(t, auth)
			assert.Equal(t, tc.method, auth.signingMethod)
			assert.True(t, auth.allowedMethods[tc.alg])
			assert.Len(t, auth.allowedMethods, 1)
		})
	}
}

func TestSign(t *testing.T) {
	auth := NewAuth("test-secret")

	t.Run("successful sign", func(t *testing.T) {
		token, err := auth.Sign("user123", "room456")
		require.NoError(t, err)
		assert.NotEmpty(t, token)
		assert.True(t, strings.HasPrefix(token, "eyJ"))
	})

	t.Run("empty userID", func(t *testing.T) {
		token, err := auth.Sign("", "room456")
		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("empty roomID", func(t *testing.T) {
		token, err := auth.Sign("user123", "")
		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("both empty", func(t *testing.T) {
		token, err := auth.Sign("", "")
		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Empty(t, token)
	})
}

func TestVerify(t *testing.T) {
	auth := NewAuth("test-secret")

	t.Run("verify valid token", func(t *testing.T) {
		token, err := auth.Sign("user123", "room456")
		require.NoError(t, err)

		claims, err := auth.Verify(token)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "user123", claims.UserID)
		assert.Equal(t, "room456", claims.RoomID)
	})

	t.Run("verify empty token", func(t *testing.T) {
		claims, err := auth.Verify("")
		assert.ErrorIs(t, err, ErrNoToken)
		assert.Nil(t, claims)
	})

	t.Run("verify invalid token format", func(t *testing.T) {
		claims, err := auth.Verify("invalid-token")
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, claims)
	})

	t.Run("verify malformed token", func(t *testing.T) {
		claims, err := auth.Verify("eyJ.invalid.token")
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, claims)
	})

	t.Run("verify wrong secret", func(t *testing.T) {
		token, err := auth.Sign("user123", "room456")
		require.NoError(t, err)

		wrongAuth := NewAuth("wrong-secret")
		claims, err := wrongAuth.Verify(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, claims)
	})
}

func TestAlgorithmMismatchAttack(t *testing.T) {
	auth := NewAuth("test-secret")

	t.Run("reject HS384 token when expecting HS256", func(t *testing.T) {
		// Create a token with HS384
		authHS384 := NewAuthWithAlgorithm("test-secret", jwt.SigningMethodHS384)
		token, err := authHS384.Sign("user123", "room456")
		require.NoError(t, err)

		// Try to verify with HS256 auth (should fail)
		claims, err := auth.Verify(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "unexpected signing method")
		assert.Contains(t, err.Error(), "HS384")
	})

	t.Run("reject HS512 token when expecting HS256", func(t *testing.T) {
		// Create a token with HS512
		authHS512 := NewAuthWithAlgorithm("test-secret", jwt.SigningMethodHS512)
		token, err := authHS512.Sign("user123", "room456")
		require.NoError(t, err)

		// Try to verify with HS256 auth (should fail)
		claims, err := auth.Verify(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "unexpected signing method")
		assert.Contains(t, err.Error(), "HS512")
	})

	t.Run("accept matching algorithm", func(t *testing.T) {
		authHS384 := NewAuthWithAlgorithm("test-secret", jwt.SigningMethodHS384)
		token, err := authHS384.Sign("user123", "room456")
		require.NoError(t, err)

		// Verify with same algorithm should succeed
		claims, err := authHS384.Verify(token)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "user123", claims.UserID)
	})
}

func TestTokenWithMissingFields(t *testing.T) {
	auth := NewAuth("test-secret")

	t.Run("token missing userID", func(t *testing.T) {
		// Manually create a token without userID
		claims := &Payload{
			UserID: "",
			RoomID: "room456",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString([]byte("test-secret"))
		require.NoError(t, err)

		// Should fail verification
		verifiedClaims, err := auth.Verify(tokenString)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, verifiedClaims)
		assert.Contains(t, err.Error(), "missing required fields")
	})

	t.Run("token missing roomID", func(t *testing.T) {
		// Manually create a token without roomID
		claims := &Payload{
			UserID: "user123",
			RoomID: "",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString([]byte("test-secret"))
		require.NoError(t, err)

		// Should fail verification
		verifiedClaims, err := auth.Verify(tokenString)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, verifiedClaims)
		assert.Contains(t, err.Error(), "missing required fields")
	})

	t.Run("token missing both fields", func(t *testing.T) {
		// Manually create a token without any fields
		claims := &Payload{
			UserID: "",
			RoomID: "",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString([]byte("test-secret"))
		require.NoError(t, err)

		// Should fail verification
		verifiedClaims, err := auth.Verify(tokenString)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Nil(t, verifiedClaims)
		assert.Contains(t, err.Error(), "missing required fields")
	})
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	algorithms := []struct {
		name   string
		method jwt.SigningMethod
	}{
		{"HS256", jwt.SigningMethodHS256},
		{"HS384", jwt.SigningMethodHS384},
		{"HS512", jwt.SigningMethodHS512},
	}

	for _, alg := range algorithms {
		t.Run(alg.name, func(t *testing.T) {
			auth := NewAuthWithAlgorithm("test-secret", alg.method)

			// Sign
			token, err := auth.Sign("user123", "room456")
			require.NoError(t, err)
			assert.NotEmpty(t, token)

			// Verify
			claims, err := auth.Verify(token)
			require.NoError(t, err)
			assert.Equal(t, "user123", claims.UserID)
			assert.Equal(t, "room456", claims.RoomID)
		})
	}
}

func TestConcurrentSignAndVerify(t *testing.T) {
	auth := NewAuth("test-secret")
	concurrency := 100

	errChan := make(chan error, concurrency)
	tokenChan := make(chan string, concurrency)

	// Concurrent signing
	for i := 0; i < concurrency; i++ {
		go func(_ int) {
			token, err := auth.Sign("user123", "room456")
			if err != nil {
				errChan <- err
			} else {
				tokenChan <- token
			}
		}(i)
	}

	// Collect tokens
	var tokens []string
	var errors []error
	for i := 0; i < concurrency; i++ {
		select {
		case token := <-tokenChan:
			tokens = append(tokens, token)
		case err := <-errChan:
			errors = append(errors, err)
		}
	}

	assert.Empty(t, errors)
	assert.Len(t, tokens, concurrency)

	// Verify all tokens concurrently
	verifyChan := make(chan *Payload, concurrency)
	verifyErrChan := make(chan error, concurrency)

	for _, token := range tokens {
		go func(t string) {
			claims, err := auth.Verify(t)
			if err != nil {
				verifyErrChan <- err
			} else {
				verifyChan <- claims
			}
		}(token)
	}

	// Collect verification results
	var verifiedClaims []*Payload
	var verifyErrors []error
	for i := 0; i < concurrency; i++ {
		select {
		case claims := <-verifyChan:
			verifiedClaims = append(verifiedClaims, claims)
		case err := <-verifyErrChan:
			verifyErrors = append(verifyErrors, err)
		}
	}

	assert.Empty(t, verifyErrors)
	assert.Len(t, verifiedClaims, concurrency)

	// Verify all claims are correct
	for _, claims := range verifiedClaims {
		assert.Equal(t, "user123", claims.UserID)
		assert.Equal(t, "room456", claims.RoomID)
	}
}
