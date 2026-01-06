package janus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type JanusAPITestSuite struct {
	suite.Suite
	server *httptest.Server
	api    *apiImpl
	logger *log.Logger
}

func (s *JanusAPITestSuite) SetupTest() {
	s.logger = log.NewNop()
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handleJanusRequest(w, r)
	}))
	s.api = New(s.server.URL, s.logger).(*apiImpl)
}

func (s *JanusAPITestSuite) TearDownTest() {
	s.server.Close()
}

func (s *JanusAPITestSuite) handleJanusRequest(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	janusType, _ := req["janus"].(string)

	var resp JanusResponse
	resp.Janus = "success"

	switch janusType {
	case "create":
		resp.Data = &JanusData{ID: 1234}
	case "attach":
		resp.Data = &JanusData{ID: 5678}
	case "message":
		body, _ := req["body"].(map[string]interface{})
		request, _ := body["request"].(string)

		var pluginData interface{}
		switch request {
		case "join":
			pluginData = map[string]interface{}{"audiobridge": "joined", "room": 123}
		case "exists":
			pluginData = map[string]interface{}{"audiobridge": "success", "exists": true}
		case "create":
			pluginData = map[string]interface{}{"audiobridge": "created", "room": 123}
		case "rtp_forward":
			pluginData = map[string]interface{}{"audiobridge": "rtp_forward", "stream_id": int64(999)}
		case "list":
			pluginData = map[string]interface{}{"audiobridge": "success", "list": []RoomInfo{{Room: 123, Description: "Test Room"}}}
		default:
			pluginData = map[string]interface{}{"audiobridge": "success"}
		}

		data, _ := json.Marshal(pluginData)
		resp.Plugindata = &JanusPluginData{Data: data}
	case "keepalive":
		// success
	case "destroy":
		// success
	case "trickle":
		// success
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *JanusAPITestSuite) TestCreateSession() {
	ctx := context.Background()
	sessionID, err := s.api.createSession(ctx)
	s.NoError(err)
	s.Equal(int64(1234), sessionID)
}

func (s *JanusAPITestSuite) TestAttach() {
	ctx := context.Background()
	handleID, err := s.api.attach(ctx, 1234)
	s.NoError(err)
	s.Equal(int64(5678), handleID)
}

func (s *JanusAPITestSuite) TestCreateAnchorInstance() {
	ctx := context.Background()
	inst, err := s.api.CreateAnchorInstance(ctx, "client-1", 0, 0)
	anchorInst := inst.(*anchorInstance)
	s.NoError(err)
	s.NotNil(anchorInst)
	s.Equal(int64(1234), anchorInst.sessionID)
	s.Equal(int64(5678), anchorInst.handleID)
}

func (s *JanusAPITestSuite) TestAnchorMethods() {
	ctx := context.Background()
	anchor, _ := s.api.CreateAnchorInstance(ctx, "client-1", 1234, 5678)

	s.Run("Join", func() {
		resp, err := anchor.Join(ctx, 123, "pin", "display", nil)
		s.NoError(err)
		s.Equal("success", resp.Janus)
	})

	s.Run("Leave", func() {
		resp, err := anchor.Leave(ctx)
		s.NoError(err)
		s.Equal("success", resp.Janus)
	})

	s.Run("IceCandidate", func() {
		resp, err := anchor.IceCandidate(ctx, ICECandidate{Candidate: "dummy"})
		s.NoError(err)
		s.Equal("success", resp.Janus)
	})

	s.Run("Check", func() {
		ok, err := anchor.Check(ctx)
		s.NoError(err)
		s.True(ok)
	})
}

func (s *JanusAPITestSuite) TestAdminMethods() {
	ctx := context.Background()
	admin, _ := s.api.CreateAdminInstance(ctx, "admin-key")

	s.Run("CreateRoom", func() {
		err := admin.CreateRoom(ctx, 123, "desc", "pin")
		s.NoError(err)
	})

	s.Run("GetRoom", func() {
		ok, err := admin.GetRoom(ctx, 123)
		s.NoError(err)
		s.True(ok)
	})

	s.Run("CreateRTPForwarder", func() {
		streamID, err := admin.CreateRTPForwarder(ctx, 123, "localhost", 5000)
		s.NoError(err)
		s.Equal(int64(999), streamID)
	})

	s.Run("ListRooms", func() {
		rooms, err := admin.ListRooms(ctx)
		s.NoError(err)
		s.Len(rooms, 1)
		s.Equal(int64(123), rooms[0].Room)
	})
}

func (s *JanusAPITestSuite) TestKeepAlive() {
	ctx := context.Background()
	anchor, _ := s.api.CreateAnchorInstance(ctx, "client-1", 1234, 5678)

	s.Run("ManualKeepAlive", func() {
		err := anchor.KeepAlive(ctx)
		s.NoError(err)
	})

	s.Run("BackgroundKeepAlive", func() {
		// This is hard to test because of the 15s ticker, but we can check if it starts/stops without panic
		anchor.StartKeepalive()
		anchor.StopKeepalive()
	})
}

func (s *JanusAPITestSuite) TestErrorHandling() {
	ctx := context.Background()

	s.Run("JanusError", func() {
		// Re-configure server to return error for a specific request
		// For simplicity, I'll just use a one-off server or check if I can modify handleJanusRequest behavior
		// Let's just create a new API pointing to a failing server
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := JanusResponse{Janus: "error"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer failServer.Close()

		failAPI := New(failServer.URL, s.logger).(*apiImpl)
		_, err := failAPI.createSession(ctx)
		s.Error(err)
	})

	s.Run("HTTPError", func() {
		// Shutdown server or use invalid URL
		failAPI := New("http://invalid-url-that-does-not-exist", s.logger).(*apiImpl)
		_, err := failAPI.createSession(ctx)
		s.Error(err)
	})
}

func TestJanusAPITestSuite(t *testing.T) {
	suite.Run(t, new(JanusAPITestSuite))
}
