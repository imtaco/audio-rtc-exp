package jwt

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/suite"
)

type JWTTestSuite struct {
	suite.Suite
	auth   Auth
	secret string
	userID string
	roomID string
}

func TestJWTSuite(t *testing.T) {
	suite.Run(t, new(JWTTestSuite))
}

func (s *JWTTestSuite) SetupTest() {
	s.secret = "test-secret"
	s.userID = "user123"
	s.roomID = "room456"
	s.auth = NewAuth(s.secret)
}

func (s *JWTTestSuite) TestNewAuth() {
	auth := NewAuth(s.secret).(*jwtAuthImpl)
	s.NotNil(auth)
	s.Equal(jwt.SigningMethodHS256, auth.signingMethod)
	s.True(auth.allowedMethods["HS256"])
}

func (s *JWTTestSuite) TestNewAuthWithAlgorithm() {
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
		s.Run(tc.name, func() {
			auth := NewAuthWithAlgorithm(s.secret, tc.method).(*jwtAuthImpl)
			s.NotNil(auth)
			s.Equal(tc.method, auth.signingMethod)
			s.True(auth.allowedMethods[tc.alg])
			s.Len(auth.allowedMethods, 1)
		})
	}
}

func (s *JWTTestSuite) TestSign_Successful() {
	token, err := s.auth.Sign(s.userID, s.roomID)
	s.Require().NoError(err)
	s.NotEmpty(token)
	s.True(strings.HasPrefix(token, "eyJ"))
}

func (s *JWTTestSuite) TestSign_EmptyUserID() {
	token, err := s.auth.Sign("", s.roomID)
	s.Require().ErrorIs(err, ErrInvalidRequest)
	s.Empty(token)
	s.Contains(err.Error(), "required")
}

func (s *JWTTestSuite) TestSign_EmptyRoomID() {
	token, err := s.auth.Sign(s.userID, "")
	s.Require().ErrorIs(err, ErrInvalidRequest)
	s.Empty(token)
	s.Contains(err.Error(), "required")
}

func (s *JWTTestSuite) TestSign_BothEmpty() {
	token, err := s.auth.Sign("", "")
	s.Require().ErrorIs(err, ErrInvalidRequest)
	s.Empty(token)
}

func (s *JWTTestSuite) TestVerify_ValidToken() {
	token, err := s.auth.Sign(s.userID, s.roomID)
	s.Require().NoError(err)

	claims, err := s.auth.Verify(token)
	s.Require().NoError(err)
	s.NotNil(claims)
	s.Equal(s.userID, claims.UserID)
	s.Equal(s.roomID, claims.RoomID)
}

func (s *JWTTestSuite) TestVerify_EmptyToken() {
	claims, err := s.auth.Verify("")
	s.Require().ErrorIs(err, ErrNoToken)
	s.Nil(claims)
}

func (s *JWTTestSuite) TestVerify_InvalidTokenFormat() {
	claims, err := s.auth.Verify("invalid-token")
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(claims)
}

func (s *JWTTestSuite) TestVerify_MalformedToken() {
	claims, err := s.auth.Verify("eyJ.invalid.token")
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(claims)
}

func (s *JWTTestSuite) TestVerify_WrongSecret() {
	token, err := s.auth.Sign(s.userID, s.roomID)
	s.Require().NoError(err)

	wrongAuth := NewAuth("wrong-secret")
	claims, err := wrongAuth.Verify(token)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(claims)
}

func (s *JWTTestSuite) TestAlgorithmMismatch_RejectHS384() {
	// Create a token with HS384
	authHS384 := NewAuthWithAlgorithm(s.secret, jwt.SigningMethodHS384)
	token, err := authHS384.Sign(s.userID, s.roomID)
	s.Require().NoError(err)

	// Try to verify with HS256 auth (should fail)
	claims, err := s.auth.Verify(token)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(claims)
	s.Contains(err.Error(), "unexpected signing method")
	s.Contains(err.Error(), "HS384")
}

func (s *JWTTestSuite) TestAlgorithmMismatch_RejectHS512() {
	// Create a token with HS512
	authHS512 := NewAuthWithAlgorithm(s.secret, jwt.SigningMethodHS512)
	token, err := authHS512.Sign(s.userID, s.roomID)
	s.Require().NoError(err)

	// Try to verify with HS256 auth (should fail)
	claims, err := s.auth.Verify(token)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(claims)
	s.Contains(err.Error(), "unexpected signing method")
	s.Contains(err.Error(), "HS512")
}

func (s *JWTTestSuite) TestAlgorithmMismatch_AcceptMatching() {
	authHS384 := NewAuthWithAlgorithm(s.secret, jwt.SigningMethodHS384)
	token, err := authHS384.Sign(s.userID, s.roomID)
	s.Require().NoError(err)

	// Verify with same algorithm should succeed
	claims, err := authHS384.Verify(token)
	s.Require().NoError(err)
	s.NotNil(claims)
	s.Equal(s.userID, claims.UserID)
}

func (s *JWTTestSuite) TestTokenMissingFields_UserID() {
	// Manually create a token without userID
	claims := &Payload{
		UserID: "",
		RoomID: s.roomID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.secret))
	s.Require().NoError(err)

	// Should fail verification
	verifiedClaims, err := s.auth.Verify(tokenString)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(verifiedClaims)
	s.Contains(err.Error(), "missing required fields")
}

func (s *JWTTestSuite) TestTokenMissingFields_RoomID() {
	// Manually create a token without roomID
	claims := &Payload{
		UserID: s.userID,
		RoomID: "",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.secret))
	s.Require().NoError(err)

	// Should fail verification
	verifiedClaims, err := s.auth.Verify(tokenString)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(verifiedClaims)
	s.Contains(err.Error(), "missing required fields")
}

func (s *JWTTestSuite) TestTokenMissingFields_BothFields() {
	// Manually create a token without any fields
	claims := &Payload{
		UserID: "",
		RoomID: "",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.secret))
	s.Require().NoError(err)

	// Should fail verification
	verifiedClaims, err := s.auth.Verify(tokenString)
	s.Require().ErrorIs(err, ErrInvalidToken)
	s.Nil(verifiedClaims)
	s.Contains(err.Error(), "missing required fields")
}

func (s *JWTTestSuite) TestSignAndVerifyRoundTrip() {
	algorithms := []struct {
		name   string
		method jwt.SigningMethod
	}{
		{"HS256", jwt.SigningMethodHS256},
		{"HS384", jwt.SigningMethodHS384},
		{"HS512", jwt.SigningMethodHS512},
	}

	for _, alg := range algorithms {
		s.Run(alg.name, func() {
			auth := NewAuthWithAlgorithm(s.secret, alg.method)

			// Sign
			token, err := auth.Sign(s.userID, s.roomID)
			s.Require().NoError(err)
			s.NotEmpty(token)

			// Verify
			claims, err := auth.Verify(token)
			s.Require().NoError(err)
			s.Equal(s.userID, claims.UserID)
			s.Equal(s.roomID, claims.RoomID)
		})
	}
}

func (s *JWTTestSuite) TestConcurrentSignAndVerify() {
	concurrency := 100

	errChan := make(chan error, concurrency)
	tokenChan := make(chan string, concurrency)

	// Concurrent signing
	for i := 0; i < concurrency; i++ {
		go func(_ int) {
			token, err := s.auth.Sign(s.userID, s.roomID)
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

	s.Empty(errors)
	s.Len(tokens, concurrency)

	// Verify all tokens concurrently
	verifyChan := make(chan *Payload, concurrency)
	verifyErrChan := make(chan error, concurrency)

	for _, token := range tokens {
		go func(t string) {
			claims, err := s.auth.Verify(t)
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

	s.Empty(verifyErrors)
	s.Len(verifiedClaims, concurrency)

	// Verify all claims are correct
	for _, claims := range verifiedClaims {
		s.Equal(s.userID, claims.UserID)
		s.Equal(s.roomID, claims.RoomID)
	}
}
