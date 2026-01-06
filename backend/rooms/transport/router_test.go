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
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"

	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/rooms"
	"github.com/imtaco/audio-rtc-exp/rooms/mocks"
)

func setupRouter(t *testing.T) (*Router, *mocks.MockRoomService, *mocks.MockRoomStore) {
	gin.SetMode(gin.TestMode)

	ctrl := gomock.NewController(t)
	mockService := mocks.NewMockRoomService(ctrl)
	mockStore := mocks.NewMockRoomStore(ctrl)
	router := NewRouter(mockService, mockStore, log.NewTest(t))
	return router, mockService, mockStore
}

func TestHealthCheck(t *testing.T) {
	router, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
	assert.Equal(t, "rooms", response["service"])
}

func TestCreateRoom(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		pin := "123456"
		expectedRoom := &rooms.RoomResponse{
			RoomID: roomID,
			Pin:    pin,
			HLSURL: "http://example.com/hls/test-room/index.m3u8",
		}

		mockService.EXPECT().CreateRoom(gomock.Any(), roomID, pin, defaultMaxAnchors).Return(expectedRoom, nil)
		mockService.EXPECT().StartLive(gomock.Any(), roomID).Return(nil)

		payload := map[string]string{
			"roomId": roomID,
			"pin":    pin,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])

		roomData := response["room"].(map[string]interface{})
		assert.Equal(t, roomID, roomData["roomId"])
	})

	t.Run("RoomExists", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "existing-room"
		pin := "123456"

		mockService.EXPECT().CreateRoom(gomock.Any(), roomID, pin, defaultMaxAnchors).Return(nil, &rooms.RoomExistsError{RoomID: roomID})

		payload := map[string]string{
			"roomId": roomID,
			"pin":    pin,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("InternalError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		pin := "123456"

		mockService.EXPECT().CreateRoom(gomock.Any(), roomID, pin, defaultMaxAnchors).Return(nil, errors.New("internal error"))

		payload := map[string]string{
			"roomId": roomID,
			"pin":    pin,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("StartLiveError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		pin := "123456"
		expectedRoom := &rooms.RoomResponse{
			RoomID: roomID,
			Pin:    pin,
		}

		mockService.EXPECT().CreateRoom(gomock.Any(), roomID, pin, defaultMaxAnchors).Return(expectedRoom, nil)
		mockService.EXPECT().StartLive(gomock.Any(), roomID).Return(errors.New("start live failed"))

		payload := map[string]string{
			"roomId": roomID,
			"pin":    pin,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GeneratedValues", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		// Expect CreateRoom to be called with ANY string for roomID and pin, and default maxAnchors
		mockService.EXPECT().CreateRoom(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, roomID, pin string, maxAnchors int) (*rooms.RoomResponse, error) {
			assert.Len(t, roomID, 20)                      // Generated roomID is 10 bytes = 20 hex chars
			assert.Len(t, pin, 6)                          // Generated pin is 3 bytes = 6 hex chars
			assert.Equal(t, defaultMaxAnchors, maxAnchors) // Should use default value
			return &rooms.RoomResponse{RoomID: roomID, Pin: pin}, nil
		})
		mockService.EXPECT().StartLive(gomock.Any(), gomock.Any()).Return(nil)

		// Empty payload to trigger generation
		jsonValue, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("ValidationError", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		// Invalid payload (e.g. wrong types or malformed JSON)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBufferString("invalid json"))
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("CustomMaxAnchors", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		pin := "123456"
		customMaxAnchors := 5
		expectedRoom := &rooms.RoomResponse{
			RoomID: roomID,
			Pin:    pin,
			HLSURL: "http://example.com/hls/test-room/index.m3u8",
		}

		mockService.EXPECT().CreateRoom(gomock.Any(), roomID, pin, customMaxAnchors).Return(expectedRoom, nil)
		mockService.EXPECT().StartLive(gomock.Any(), roomID).Return(nil)

		payload := map[string]interface{}{
			"roomId":     roomID,
			"pin":        pin,
			"maxAnchors": customMaxAnchors,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("InvalidMaxAnchors", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		payload := map[string]interface{}{
			"roomId":     "test-room",
			"pin":        "123456",
			"maxAnchors": 10, // Invalid: exceeds max of 5
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/rooms", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})
}

func TestGetRoom(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		expectedRoom := &rooms.RoomResponse{
			RoomID: roomID,
			HLSURL: "http://example.com/hls/test-room/index.m3u8",
		}

		mockService.EXPECT().GetRoom(gomock.Any(), roomID).Return(expectedRoom, nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("NotFound", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "unknown-room"
		mockService.EXPECT().GetRoom(gomock.Any(), roomID).Return(nil, &rooms.RoomNotFoundError{RoomID: roomID})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("InternalError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		mockService.EXPECT().GetRoom(gomock.Any(), roomID).Return(nil, errors.New("internal error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("InvalidID", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		roomID := "invalid@id"
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestListRooms(t *testing.T) {
	router, mockService, _ := setupRouter(t)

	expectedResponse := &rooms.ListRoomsResponse{
		Count: 1,
		Rooms: []*rooms.RoomResponse{
			{RoomID: "room1", HLSURL: "url1"},
		},
	}

	mockService.EXPECT().ListRooms(gomock.Any()).Return(expectedResponse, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/rooms", nil)
	router.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, true, response["success"])
	assert.Equal(t, float64(1), response["count"])

	t.Run("InternalError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		mockService.EXPECT().ListRooms(gomock.Any()).Return(nil, errors.New("internal error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/rooms", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestDeleteRoom(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		mockService.EXPECT().DeleteRoom(gomock.Any(), roomID).Return(&rooms.DeleteRoomResponse{Message: "deleted"}, nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "unknown-room"
		mockService.EXPECT().DeleteRoom(gomock.Any(), roomID).Return(nil, &rooms.RoomNotFoundError{RoomID: roomID})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("InternalError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		roomID := "test-room"
		mockService.EXPECT().DeleteRoom(gomock.Any(), roomID).Return(nil, errors.New("internal error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("InvalidID", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		roomID := "invalid@id"
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/rooms/"+roomID, nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetStats(t *testing.T) {
	router, mockService, _ := setupRouter(t)

	expectedStats := &rooms.StatsResponse{
		Rooms: &rooms.RoomStats{
			Total:             10,
			TotalParticipants: 50,
		},
	}

	mockService.EXPECT().GetStats(gomock.Any()).Return(expectedStats, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats", nil)
	router.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, true, response["success"])

	t.Run("InternalError", func(t *testing.T) {
		router, mockService, _ := setupRouter(t)

		mockService.EXPECT().GetStats(gomock.Any()).Return(nil, errors.New("internal error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/stats", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestSetModuleMark(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"
		label := "cordon"
		ttl := int64(3600)

		mockStore.EXPECT().SetModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
			gomock.Any(),
			ttl,
		).Return(nil)

		payload := map[string]interface{}{
			"label": label,
			"ttl":   ttl,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
		assert.Equal(t, "Module mark set successfully", response["message"])

		module := response["module"].(map[string]interface{})
		assert.Equal(t, moduleType, module["type"])
		assert.Equal(t, moduleID, module["id"])
		assert.Equal(t, label, module["label"])
		assert.Equal(t, float64(ttl), module["ttl"])
	})

	t.Run("SuccessWithoutTTL", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "januses"
		moduleID := "janus1"
		label := "ready"

		mockStore.EXPECT().SetModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
			gomock.Any(),
			int64(0),
		).Return(nil)

		payload := map[string]interface{}{
			"label": label,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("InvalidModuleType", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "invalid"
		moduleID := "module1"

		payload := map[string]interface{}{
			"label": "ready",
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("InvalidLabel", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		payload := map[string]interface{}{
			"label": "invalid",
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("MissingLabel", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		payload := map[string]interface{}{}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("InvalidTTL", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		payload := map[string]interface{}{
			"label": "ready",
			"ttl":   100000, // Exceeds max of 86400
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("NegativeTTL", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		payload := map[string]interface{}{
			"label": "ready",
			"ttl":   -1,
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("StoreError", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		mockStore.EXPECT().SetModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
			gomock.Any(),
			gomock.Any(),
		).Return(errors.New("etcd error"))

		payload := map[string]interface{}{
			"label": "ready",
		}
		jsonValue, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
		assert.Equal(t, "Failed to set module mark", response["error"])
	})

	t.Run("AllLabels", func(t *testing.T) {
		labels := []string{"ready", "cordon", "draining", "drained", "unready"}

		for _, label := range labels {
			t.Run("Label_"+label, func(t *testing.T) {
				router, _, mockStore := setupRouter(t)

				moduleType := "mixers"
				moduleID := "mixer1"

				mockStore.EXPECT().SetModuleMark(
					gomock.Any(),
					moduleType,
					moduleID,
					gomock.Any(),
					gomock.Any(),
				).Return(nil)

				payload := map[string]interface{}{
					"label": label,
				}
				jsonValue, _ := json.Marshal(payload)

				w := httptest.NewRecorder()
				req, _ := http.NewRequest("PUT", "/api/modules/"+moduleType+"/"+moduleID+"/mark", bytes.NewBuffer(jsonValue))
				req.Header.Set("Content-Type", "application/json")
				router.Handler().ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, true, response["success"])
			})
		}
	})
}

func TestDeleteModuleMark(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		mockStore.EXPECT().DeleteModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
		).Return(nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/modules/"+moduleType+"/"+moduleID+"/mark", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
		assert.Equal(t, "Module mark deleted successfully", response["message"])

		module := response["module"].(map[string]interface{})
		assert.Equal(t, moduleType, module["type"])
		assert.Equal(t, moduleID, module["id"])
	})

	t.Run("SuccessJanus", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "januses"
		moduleID := "janus1"

		mockStore.EXPECT().DeleteModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
		).Return(nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/modules/"+moduleType+"/"+moduleID+"/mark", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, true, response["success"])
	})

	t.Run("InvalidModuleType", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "invalid"
		moduleID := "module1"

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/modules/"+moduleType+"/"+moduleID+"/mark", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("ShortModuleID", func(t *testing.T) {
		router, _, _ := setupRouter(t)

		moduleType := "mixers"
		moduleID := "m" // Too short

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/modules/"+moduleType+"/"+moduleID+"/mark", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
	})

	t.Run("StoreError", func(t *testing.T) {
		router, _, mockStore := setupRouter(t)

		moduleType := "mixers"
		moduleID := "mixer1"

		mockStore.EXPECT().DeleteModuleMark(
			gomock.Any(),
			moduleType,
			moduleID,
		).Return(errors.New("etcd error"))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/modules/"+moduleType+"/"+moduleID+"/mark", nil)
		router.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, false, response["success"])
		assert.Equal(t, "Failed to delete module mark", response["error"])
	})
}
