package watcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/janus/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type JanusHealthMonitorTestSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockJanus *mocks.MockAdmin
	monitor   *JanusHealthMonitor
	ctx       context.Context
	cancel    context.CancelFunc
}

func TestJanusHealthMonitorSuite(t *testing.T) {
	suite.Run(t, new(JanusHealthMonitorTestSuite))
}

func (s *JanusHealthMonitorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockJanus = mocks.NewMockAdmin(s.ctrl)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	logger := log.NewTest(s.T())

	s.monitor = NewJanusHealthMonitor(
		s.mockJanus,
		12345,
		100*time.Millisecond,
		logger,
	)
}

func (s *JanusHealthMonitorTestSuite) TearDownTest() {
	s.cancel()
	s.ctrl.Finish()
}

func (s *JanusHealthMonitorTestSuite) TestNewJanusHealthMonitor() {
	logger := log.NewTest(s.T())

	monitor := NewJanusHealthMonitor(s.mockJanus, 12345, 5*time.Second, logger)

	s.NotNil(monitor)
	s.Equal(int64(12345), monitor.canaryRoomID)
	s.Equal(5*time.Second, monitor.interval)
	s.NotNil(monitor.stopped)
}

func (s *JanusHealthMonitorTestSuite) TestSetRestartHandler() {
	handlerCalled := false
	handler := func(reason string) {
		handlerCalled = true
		s.Equal("test_reason", reason)
	}

	s.monitor.SetRestartHandler(handler)
	s.NotNil(s.monitor.restartHandler)

	s.monitor.restartHandler("test_reason")
	s.True(handlerCalled)
}

func (s *JanusHealthMonitorTestSuite) TestStart_CreateCanaryRoom_Success() {
	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(false, nil)

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(nil)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.cancel()
	}()

	err := s.monitor.Start(s.ctx)
	s.NoError(err)
}

func (s *JanusHealthMonitorTestSuite) TestStart_CanaryRoomExists() {
	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(true, nil)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.cancel()
	}()

	err := s.monitor.Start(s.ctx)
	s.NoError(err)
}

func (s *JanusHealthMonitorTestSuite) TestStart_CreateCanaryRoom_Error() {
	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(false, nil)

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(errors.New("create failed"))

	err := s.monitor.Start(s.ctx)
	s.Error(err)
	s.Contains(err.Error(), "create failed")
}

func (s *JanusHealthMonitorTestSuite) TestCheckCanaryRoom_Healthy() {
	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(true, nil)

	s.monitor.checkCanaryRoom()
}

func (s *JanusHealthMonitorTestSuite) TestCheckCanaryRoom_NotFound() {
	handlerCalled := false
	s.monitor.SetRestartHandler(func(reason string) {
		handlerCalled = true
		s.Equal("canary_room_disappeared", reason)
	})

	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(false, nil)

	// Recreate canary after detecting disappearance
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(nil)

	s.monitor.checkCanaryRoom()
	s.True(handlerCalled)
}

func (s *JanusHealthMonitorTestSuite) TestCheckCanaryRoom_GetRoomError() {
	s.mockJanus.EXPECT().
		GetRoom(gomock.Any(), s.monitor.canaryRoomID).
		Return(false, errors.New("connection error"))

	s.monitor.checkCanaryRoom()
}

func (s *JanusHealthMonitorTestSuite) TestHandleJanusRestart_CallsHandler() {
	handlerCalled := false
	var capturedReason string
	handler := func(reason string) {
		handlerCalled = true
		capturedReason = reason
	}

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(nil)

	s.monitor.SetRestartHandler(handler)
	s.monitor.handleJanusRestart("test_restart")

	s.True(handlerCalled)
	s.Equal("test_restart", capturedReason)
}

func (s *JanusHealthMonitorTestSuite) TestHandleJanusRestart_NoHandler() {
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(nil)

	s.NotPanics(func() {
		s.monitor.handleJanusRestart("test_restart")
	})
}

func (s *JanusHealthMonitorTestSuite) TestHandleJanusRestart_CreateCanaryFails() {
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), s.monitor.canaryRoomID, gomock.Any(), "111111").
		Return(errors.New("create failed"))

	s.NotPanics(func() {
		s.monitor.handleJanusRestart("test_restart")
	})
}

func (s *JanusHealthMonitorTestSuite) TestStop() {
	ctx, cancel := context.WithCancel(context.Background())
	s.monitor.cancel = cancel

	go func() {
		<-ctx.Done()
		close(s.monitor.stopped)
	}()

	s.NotPanics(func() {
		s.monitor.Stop()
	})
}
