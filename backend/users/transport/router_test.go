package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"

	jwtmocks "github.com/imtaco/audio-rtc-exp/internal/jwt/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	usermocks "github.com/imtaco/audio-rtc-exp/users/mocks"
)

func setupRouter(t *testing.T) (*Router, *usermocks.MockUserService, *jwtmocks.MockAuth) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	mockUserService := usermocks.NewMockUserService(ctrl)
	mockJWTAuth := jwtmocks.NewMockAuth(ctrl)
	router := NewRouter(mockUserService, mockJWTAuth, log.NewTest(t))
	return router, mockUserService, mockJWTAuth
}

func TestHealthCheck(t *testing.T) {
	router, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
}

func TestCreateUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, mockUserService, _ := setupRouter(t)

		roomID := "test-room"
		role := "host"
		expectedToken := "jwt-token"

		mockUserService.EXPECT().CreateUser(gomock.Any(), roomID, gomock.Any(), role).DoAndReturn(func(_ context.Context, rID, uID, r string) (string, string, error) {
			assert.Equal(t, roomID, rID)
			assert.Equal(t, role, r)
			assert.NotEmpty(t, uID) // UserID is generated inside handler
			return uID, expectedToken, nil
		})

		payload := map[string]string{
			"role": role,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms/"+roomID+"/users", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, response["token"])
		assert.NotEmpty(t, response["userID"])
	})

	t.Run("ServiceError", func(t *testing.T) {
		router, mockUserService, _ := setupRouter(t)

		roomID := "test-room"
		role := "host"

		mockUserService.EXPECT().CreateUser(gomock.Any(), roomID, gomock.Any(), role).Return("", "", errors.New("service error"))

		payload := map[string]string{
			"role": role,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms/"+roomID+"/users", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("ValidationError", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		// Invalid payload
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms/test-room/users", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDeleteUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, mockUserService, _ := setupRouter(t)

		roomID := "test-room"
		userID := uuid.New().String()

		mockUserService.EXPECT().DeleteUser(gomock.Any(), roomID, userID).Return(nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID+"/users/"+userID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("ServiceError", func(t *testing.T) {
		router, mockUserService, _ := setupRouter(t)

		roomID := "test-room"
		userID := uuid.New().String()

		mockUserService.EXPECT().DeleteUser(gomock.Any(), roomID, userID).Return(errors.New("service error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID+"/users/"+userID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("ValidationError", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		// Invalid roomId
		roomID := "invalid@room"
		userID := uuid.New().String()
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID+"/users/"+userID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("InvalidUserID", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		roomID := "test-room"
		userID := "invalid@id"

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID+"/users/"+userID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
