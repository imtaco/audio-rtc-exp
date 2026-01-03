package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/hlsserver/mocks"
	"github.com/imtaco/audio-rtc-exp/hlsserver/transport"
	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type RouterSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockWatcher *mocks.MockRoomWatcher
	jwtAuth     jwt.JWTAuth
	secret      string
}

func (s *RouterSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockWatcher = mocks.NewMockRoomWatcher(s.ctrl)
	s.secret = "very-secret-key"
	s.jwtAuth = jwt.NewJWTAuth(s.secret)
	gin.SetMode(gin.TestMode)
}

func (s *RouterSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RouterSuite) TestTokenRouter_HealthCheck() {
	router := transport.NewTokenRouter(s.mockWatcher, s.jwtAuth, log.NewTest(s.T()))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	// gin.H marshals to json, so we expect json
	s.JSONEq(`{"status": "ok"}`, w.Body.String())
}

func (s *RouterSuite) TestTokenRouter_GenerateToken() {
	router := transport.NewTokenRouter(s.mockWatcher, s.jwtAuth, log.NewTest(s.T()))

	// Test Success
	body := map[string]string{"roomId": "room123"}
	jsonBody, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/token", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.NoError(err)
	s.NotEmpty(resp["token"])

	// Verify generated token
	claims, err := s.jwtAuth.Verify(resp["token"])
	s.NoError(err)
	s.Equal("room123", claims.RoomID)
	s.NotEmpty(claims.UserID)

	// Test Missing RoomID
	body = map[string]string{}
	jsonBody, _ = json.Marshal(body)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/token", bytes.NewBuffer(jsonBody))
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Contains(w.Body.String(), "Validation failed")

	// Test Invalid RoomID
	body = map[string]string{"roomId": "invalid@id"}
	jsonBody, _ = json.Marshal(body)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/token", bytes.NewBuffer(jsonBody))
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Contains(w.Body.String(), "Validation failed")
}

func (s *RouterSuite) TestKeyRouter_HealthCheck() {
	router := transport.NewKeyRouter(s.mockWatcher, s.jwtAuth, log.NewTest(s.T()))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
}

func (s *RouterSuite) TestKeyRouter_GetEncryptionKey() {
	router := transport.NewKeyRouter(s.mockWatcher, s.jwtAuth, log.NewTest(s.T()))
	roomID := "room123"

	// Create valid token
	token, _ := s.jwtAuth.Sign("user1", roomID)

	// Case 1: Success (Not in cache, active room)
	s.mockWatcher.EXPECT().GetActiveLiveMeta(roomID).Return(&etcdstate.LiveMeta{
		Status: constants.RoomStatusOnAir,
		Nonce:  "nonce123",
	}).Times(1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/hls/rooms/"+roomID+"/enc.key", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.NotEmpty(w.Body.Bytes())
	s.Equal("application/octet-stream", w.Header().Get("Content-Type"))

	// Case 2: Success (Served from cache)
	// The mockWatcher should NOT be called again if caching works.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+roomID+"/enc.key", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.Handler().ServeHTTP(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.NotEmpty(w.Body.Bytes())

	// Case 3: Invalid Token
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+roomID+"/enc.key", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	router.Handler().ServeHTTP(w, req)
	s.Equal(http.StatusForbidden, w.Code)
	assert.Contains(s.T(), w.Body.String(), "Access denied 1")

	// Case 4: Room Mismatch
	tokenOtherRoom, _ := s.jwtAuth.Sign("user1", "otherRoom")
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+roomID+"/enc.key", nil)
	req.Header.Set("Authorization", "Bearer "+tokenOtherRoom)
	router.Handler().ServeHTTP(w, req)
	s.Equal(http.StatusForbidden, w.Code)
	assert.Contains(s.T(), w.Body.String(), "Access denied 2")

	// Case 5: Room Not Active (and not in cache)
	roomInactive := "inactiveRoom"
	tokenInactive, _ := s.jwtAuth.Sign("user1", roomInactive)

	s.mockWatcher.EXPECT().GetActiveLiveMeta(roomInactive).Return(nil).Times(1)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+roomInactive+"/enc.key", nil)
	req.Header.Set("Authorization", "Bearer "+tokenInactive)
	router.Handler().ServeHTTP(w, req)
	s.Equal(http.StatusForbidden, w.Code)
	assert.Contains(s.T(), w.Body.String(), "Access denied 3")

	// Case 6: Missing Auth Header
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+roomID+"/enc.key", nil)
	router.Handler().ServeHTTP(w, req)
	s.Equal(http.StatusUnauthorized, w.Code)

	// Case 7: Invalid Room ID
	invalidRoomID := "invalid@room"
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/hls/rooms/"+invalidRoomID+"/enc.key", nil)
	router.Handler().ServeHTTP(w, req)
	s.Equal(http.StatusBadRequest, w.Code)
}

func TestRouterSuite(t *testing.T) {
	suite.Run(t, new(RouterSuite))
}
