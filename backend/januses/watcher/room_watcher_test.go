package watcher

import (
	"context"
	"testing"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/errors"
	etcdfakes "github.com/imtaco/audio-rtc-exp/internal/etcd/fakes"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
	"github.com/imtaco/audio-rtc-exp/internal/janus/mocks"
	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type RoomWatcherTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockJanus  *mocks.MockAdmin
	watcher    *RoomWatcher
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func TestRoomWatcherSuite(t *testing.T) {
	suite.Run(t, new(RoomWatcherTestSuite))
}

func (s *RoomWatcherTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockJanus = mocks.NewMockAdmin(s.ctrl)
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())

	logger := log.NewTest(s.T())

	// Create a minimal watcher for testing
	// Note: etcdClient is nil, so updateJanusStatus will fail
	// We need to override processChange or mock etcdClient for full integration tests
	s.watcher = &RoomWatcher{
		janusAdmin:  s.mockJanus,
		janusID:     "test-janus-01",
		prefixRooms: "/rooms/",
		logger:      logger,
		etcdClient:  nil, // Set to nil - tests that need it should create a mock
	}
}

func (s *RoomWatcherTestSuite) TearDownTest() {
	s.cancelFunc()
	s.ctrl.Finish()
}

func (s *RoomWatcherTestSuite) TestCryptoRandInt_WithinRange() {
	maxVal := int64(900000)

	for i := 0; i < 100; i++ {
		val, err := cryptoRandInt(maxVal)
		s.NoError(err)
		s.GreaterOrEqual(val, int64(0))
		s.Less(val, maxVal)
	}
}

func (s *RoomWatcherTestSuite) TestCryptoRandInt_SmallRange() {
	maxVal := int64(10)

	for i := 0; i < 50; i++ {
		val, err := cryptoRandInt(maxVal)
		s.NoError(err)
		s.GreaterOrEqual(val, int64(0))
		s.Less(val, maxVal)
	}
}

func (s *RoomWatcherTestSuite) TestCryptoRandInt_Distribution() {
	maxVal := int64(10)
	counts := make(map[int64]int)

	for i := 0; i < 1000; i++ {
		val, err := cryptoRandInt(maxVal)
		s.NoError(err)
		counts[val]++
	}

	s.Len(counts, int(maxVal), "Should generate all possible values")

	for i := int64(0); i < maxVal; i++ {
		s.Greater(counts[i], 0, "Value %d should appear at least once", i)
	}
}

func (s *RoomWatcherTestSuite) TestCreateRoom_Success() {
	roomID := "room-123"
	pin := "1234"

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(nil)

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.NoError(err)
	s.GreaterOrEqual(janusRoomID, int64(100000))
	s.Less(janusRoomID, int64(1000000))
}

func (s *RoomWatcherTestSuite) TestCreateRoom_RetryOnCollision() {
	roomID := "room-123"
	pin := "1234"

	// First attempt fails with collision
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(errors.New(janus.ErrAlreadyExisted, "room exists"))

	// Second attempt succeeds
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(nil)

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.NoError(err)
	s.GreaterOrEqual(janusRoomID, int64(100000))
}

func (s *RoomWatcherTestSuite) TestCreateRoom_MaxRetriesExceeded() {
	roomID := "room-123"
	pin := "1234"

	// All attempts fail with collision
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(errors.New(janus.ErrAlreadyExisted, "room exists")).
		Times(maxRoomCreationAttempts)

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.Error(err)
	s.Contains(err.Error(), "failed to create room after")
	s.Zero(janusRoomID)
}

func (s *RoomWatcherTestSuite) TestCreateRoom_OtherError() {
	roomID := "room-123"
	pin := "1234"

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(errors.New(janus.ErrFailedRequest, "network error"))

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.Error(err)
	s.Contains(err.Error(), "network error")
	s.Zero(janusRoomID)
}

func (s *RoomWatcherTestSuite) TestDestroyRoom_Success() {
	janusRoomID := int64(123456)

	s.mockJanus.EXPECT().
		DestroyRoom(gomock.Any(), janusRoomID).
		Return(nil)

	err := s.watcher.destroyRoom(s.ctx, janusRoomID)
	s.NoError(err)
}

func (s *RoomWatcherTestSuite) TestDestroyRoom_NotFound() {
	janusRoomID := int64(123456)

	s.mockJanus.EXPECT().
		DestroyRoom(gomock.Any(), janusRoomID).
		Return(errors.New(janus.ErrNotFound, "room not found"))

	err := s.watcher.destroyRoom(s.ctx, janusRoomID)
	s.NoError(err) // Should not return error for not found
}

func (s *RoomWatcherTestSuite) TestDestroyRoom_OtherError() {
	janusRoomID := int64(123456)

	s.mockJanus.EXPECT().
		DestroyRoom(gomock.Any(), janusRoomID).
		Return(errors.New(janus.ErrFailedRequest, "network error"))

	err := s.watcher.destroyRoom(s.ctx, janusRoomID)
	s.Error(err)
	s.Contains(err.Error(), "network error")
}

func (s *RoomWatcherTestSuite) TestCreateRtpForwarder_Success() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
	}
	fwip := "10.0.0.1"
	fwport := 5000
	streamID := int64(7890)

	s.mockJanus.EXPECT().
		CreateRTPForwarder(gomock.Any(), activeRoom.JanusRoomID, fwip, fwport).
		Return(streamID, nil)

	err := s.watcher.createRtpForwarder(s.ctx, roomID, activeRoom, fwip, fwport)
	s.NoError(err)
	s.Equal(streamID, activeRoom.StreamID)
	s.Equal(fwip, activeRoom.FwIP)
	s.Equal(fwport, activeRoom.FwPort)
}

func (s *RoomWatcherTestSuite) TestCreateRtpForwarder_NoJanusRoom() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 0, // No Janus room
	}
	fwip := "10.0.0.1"
	fwport := 5000

	// Should not call Janus API
	err := s.watcher.createRtpForwarder(s.ctx, roomID, activeRoom, fwip, fwport)
	s.NoError(err)
	s.Zero(activeRoom.StreamID)
}

func (s *RoomWatcherTestSuite) TestCreateRtpForwarder_Error() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
	}
	fwip := "10.0.0.1"
	fwport := 5000

	s.mockJanus.EXPECT().
		CreateRTPForwarder(gomock.Any(), activeRoom.JanusRoomID, fwip, fwport).
		Return(int64(0), janus.ErrNoneSuccessResponse)

	err := s.watcher.createRtpForwarder(s.ctx, roomID, activeRoom, fwip, fwport)
	s.ErrorIs(err, janus.ErrNoneSuccessResponse)
	// s.Contains(err.Error(), "forwarder creation failed")
	s.Zero(activeRoom.StreamID)
}

func (s *RoomWatcherTestSuite) TestStopRtpForwarder_Success() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), activeRoom.JanusRoomID, activeRoom.StreamID).
		Return(nil)

	err := s.watcher.stopRtpForwarder(s.ctx, roomID, activeRoom)
	s.NoError(err)
	s.Zero(activeRoom.StreamID)
	s.Empty(activeRoom.FwIP)
	s.Zero(activeRoom.FwPort)
}

func (s *RoomWatcherTestSuite) TestStopRtpForwarder_NotFound() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), activeRoom.JanusRoomID, activeRoom.StreamID).
		Return(errors.New(janus.ErrNotFound, "forwarder not found"))

	err := s.watcher.stopRtpForwarder(s.ctx, roomID, activeRoom)
	s.NoError(err) // Should not return error for not found
	s.Zero(activeRoom.StreamID)
	s.Empty(activeRoom.FwIP)
	s.Zero(activeRoom.FwPort)
}

func (s *RoomWatcherTestSuite) TestStopRtpForwarder_OtherError() {
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), activeRoom.JanusRoomID, activeRoom.StreamID).
		Return(errors.New(janus.ErrFailedRequest, "network error"))

	err := s.watcher.stopRtpForwarder(s.ctx, roomID, activeRoom)
	s.Error(err)
	s.Contains(err.Error(), "network error")
}

func (s *RoomWatcherTestSuite) TestProcessChange_StateLogic_NotAssignedToUs() {
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{
		Pin: "1234", MaxAnchors: 5,
	})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "other-janus",
		Status:  constants.RoomStatusOnAir,
	})

	s.NotNil(state.GetMeta())
	s.NotNil(state.GetLiveMeta())
	s.NotEqual(s.watcher.janusID, state.GetLiveMeta().JanusID)
}

func (s *RoomWatcherTestSuite) TestProcessChange_StateLogic_AssignedToUs() {
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{
		Pin: "1234", MaxAnchors: 5,
	})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	s.NotNil(state.GetMeta())
	s.NotNil(state.GetLiveMeta())
	s.NotNil(state.GetMixer())
	s.Equal(s.watcher.janusID, state.GetLiveMeta().JanusID)
	s.Equal(constants.RoomStatusOnAir, state.GetLiveMeta().Status)
	s.NotZero(state.GetMixer().Port)
}

func (s *RoomWatcherTestSuite) TestNewRoomWatcher_Construction() {
	logger := log.NewTest(s.T())

	watcher := NewRoomWatcher(
		nil,
		"janus-01",
		"192.168.1.100",
		s.mockJanus,
		"/rooms/",
		"/januses/",
		999,
		logger,
	)

	s.NotNil(watcher)
	s.Equal("janus-01", watcher.janusID)
	s.Equal("192.168.1.100", watcher.janusAdvHost)
	s.Equal("/rooms/", watcher.prefixRooms)
	s.Equal("/januses/", watcher.prefixJanuses)
	s.Equal(int64(999), watcher.canaryRoomID)
}

// Business Logic Tests - Testing individual components and state logic
// Note: Full processChange integration tests require etcd client mocking
// We test the business logic through the individual methods instead

func (s *RoomWatcherTestSuite) TestBusinessLogic_CreateRoom_ThenAddForwarder() {
	// Test the sequence: create room -> add forwarder
	roomID := "room-123"
	pin := "1234"

	// Step 1: Create room
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(nil)

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.NoError(err)
	s.NotZero(janusRoomID)

	// Step 2: Add forwarder to the room
	activeRoom := &ActiveRoom{
		JanusRoomID: janusRoomID,
	}

	s.mockJanus.EXPECT().
		CreateRTPForwarder(gomock.Any(), janusRoomID, "10.0.0.1", 5000).
		Return(int64(7890), nil)

	err = s.watcher.createRtpForwarder(s.ctx, roomID, activeRoom, "10.0.0.1", 5000)
	s.NoError(err)
	s.Equal(int64(7890), activeRoom.StreamID)
	s.Equal("10.0.0.1", activeRoom.FwIP)
	s.Equal(5000, activeRoom.FwPort)
}

func (s *RoomWatcherTestSuite) TestBusinessLogic_StopForwarder_ThenDestroyRoom() {
	// Test the sequence: stop forwarder -> destroy room
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	// Step 1: Stop forwarder
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(nil)

	err := s.watcher.stopRtpForwarder(s.ctx, roomID, activeRoom)
	s.NoError(err)
	s.Zero(activeRoom.StreamID)

	// Step 2: Destroy room
	s.mockJanus.EXPECT().
		DestroyRoom(gomock.Any(), int64(123456)).
		Return(nil)

	err = s.watcher.destroyRoom(s.ctx, int64(123456))
	s.NoError(err)
}

func (s *RoomWatcherTestSuite) TestBusinessLogic_RecreateForwarder_OnEndpointChange() {
	// Test the sequence: stop old forwarder -> create new forwarder
	roomID := "room-123"
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	// Step 1: Stop old forwarder
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(nil)

	err := s.watcher.stopRtpForwarder(s.ctx, roomID, activeRoom)
	s.NoError(err)

	// Step 2: Create new forwarder with different endpoint
	s.mockJanus.EXPECT().
		CreateRTPForwarder(gomock.Any(), int64(123456), "10.0.0.2", 5001).
		Return(int64(9999), nil)

	err = s.watcher.createRtpForwarder(s.ctx, roomID, activeRoom, "10.0.0.2", 5001)
	s.NoError(err)
	s.Equal(int64(9999), activeRoom.StreamID)
	s.Equal("10.0.0.2", activeRoom.FwIP)
	s.Equal(5001, activeRoom.FwPort)
}

// State Machine Logic Tests - Testing conditions without full processChange

func (s *RoomWatcherTestSuite) TestStateLogic_ShouldHaveForwarder_AllConditionsMet() {
	// shouldHaveForwarder = isAssignedToUs && mixer != nil && mixer.Port != 0
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	// Verify conditions for shouldHaveForwarder
	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()
	mixer := state.GetMixer()

	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == s.watcher.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	shouldHaveForwarder := isAssignedToUs && mixer != nil && mixer.Port != 0

	s.True(isAssignedToUs)
	s.True(shouldHaveForwarder)
}

func (s *RoomWatcherTestSuite) TestStateLogic_ShouldNotHaveForwarder_MixerPortZero() {
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 0, // Port is 0
	})

	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()
	mixer := state.GetMixer()

	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == s.watcher.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	shouldHaveForwarder := isAssignedToUs && mixer != nil && mixer.Port != 0

	s.True(isAssignedToUs)
	s.False(shouldHaveForwarder, "Should not have forwarder when port is 0")
}

func (s *RoomWatcherTestSuite) TestStateLogic_ShouldNotHaveForwarder_StatusNotOnAir() {
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  "idle", // Not on-air
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()
	mixer := state.GetMixer()

	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == s.watcher.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	shouldHaveForwarder := isAssignedToUs && mixer != nil && mixer.Port != 0

	s.False(isAssignedToUs, "Should not be assigned when status != on-air")
	s.False(shouldHaveForwarder)
}

func (s *RoomWatcherTestSuite) TestStateLogic_NotAssignedToUs_DifferentJanusID() {
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "other-janus",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()
	mixer := state.GetMixer()

	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == s.watcher.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	shouldHaveForwarder := isAssignedToUs && mixer != nil && mixer.Port != 0

	s.False(isAssignedToUs, "Should not be assigned when JanusID doesn't match")
	s.False(shouldHaveForwarder)
}

func (s *RoomWatcherTestSuite) TestStateLogic_NoMetaData() {
	state := &etcdstate.RoomState{}
	// No meta set
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})

	meta := state.GetMeta()
	livemeta := state.GetLiveMeta()

	isAssignedToUs := meta != nil && livemeta != nil &&
		livemeta.JanusID == s.watcher.janusID &&
		livemeta.Status == constants.RoomStatusOnAir

	s.False(isAssignedToUs, "Should not be assigned when meta is nil")
}

func (s *RoomWatcherTestSuite) TestStateLogic_EndpointChanged_DetectIPChange() {
	oldRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	newMixer := &etcdstate.Mixer{
		IP:   "10.0.0.2", // Changed
		Port: 5000,
	}

	endpointChanged := oldRoom.FwIP != newMixer.IP || oldRoom.FwPort != newMixer.Port
	s.True(endpointChanged, "Should detect IP change")
}

func (s *RoomWatcherTestSuite) TestStateLogic_EndpointChanged_DetectPortChange() {
	oldRoom := &ActiveRoom{
		// JanusRoomID: 123456,
		// StreamID:    7890,
		FwIP:   "10.0.0.1",
		FwPort: 5000,
	}

	newMixer := &etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5001, // Changed
	}

	endpointChanged := oldRoom.FwIP != newMixer.IP || oldRoom.FwPort != newMixer.Port
	s.True(endpointChanged, "Should detect port change")
}

func (s *RoomWatcherTestSuite) TestStateLogic_EndpointNotChanged_SameEndpoint() {
	oldRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}

	newMixer := &etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	}

	endpointChanged := oldRoom.FwIP != newMixer.IP || oldRoom.FwPort != newMixer.Port
	s.False(endpointChanged, "Should not detect change when endpoint is same")
}

func (s *RoomWatcherTestSuite) TestStateLogic_HasJanusRoom() {
	roomID := "room-123"

	// No active room
	_, ok := s.watcher.activeRooms.Load(roomID)
	s.False(ok)

	// Store active room
	activeRoom := &ActiveRoom{JanusRoomID: 123456}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	val, ok := s.watcher.activeRooms.Load(roomID)
	s.True(ok)
	s.NotNil(val)
}

func (s *RoomWatcherTestSuite) TestStateLogic_HasRTPForwarder() {
	// Room without forwarder
	room1 := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    0,
	}
	hasForwarder1 := room1.StreamID != 0
	s.False(hasForwarder1)

	// Room with forwarder
	room2 := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
	}
	hasForwarder2 := room2.StreamID != 0
	s.True(hasForwarder2)
}

func (s *RoomWatcherTestSuite) TestBusinessLogic_RetryMechanism() {
	// Test that createRoom retries on collision up to max attempts
	roomID := "room-123"
	pin := "1234"

	// Simulate 3 collisions then success
	gomock.InOrder(
		s.mockJanus.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
			Return(errors.New(janus.ErrAlreadyExisted, "exists")),
		s.mockJanus.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
			Return(errors.New(janus.ErrAlreadyExisted, "exists")),
		s.mockJanus.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
			Return(errors.New(janus.ErrAlreadyExisted, "exists")),
		s.mockJanus.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
			Return(nil),
	)

	janusRoomID, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.NoError(err)
	s.NotZero(janusRoomID)
}

func (s *RoomWatcherTestSuite) TestBusinessLogic_ErrorPropagation() {
	// Test that non-collision errors are not retried
	roomID := "room-123"
	pin := "1234"

	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(errors.New(janus.ErrFailedRequest, "network error")).
		Times(1) // Only called once, not retried

	_, err := s.watcher.createRoom(s.ctx, roomID, pin)
	s.Error(err)
	s.Contains(err.Error(), "network error")
}

func (s *RoomWatcherTestSuite) TestRebuildStart_EmptyJanus() {
	// Test rebuild when Janus has no rooms
	s.mockJanus.EXPECT().
		ListRooms(gomock.Any()).
		Return([]janus.RoomInfo{}, nil)

	err := s.watcher.RebuildStart(context.Background())
	s.NoError(err)

	// Verify activeRooms is empty
	count := 0
	s.watcher.activeRooms.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	s.Equal(0, count)
}

func (s *RoomWatcherTestSuite) TestRebuildStart_WithRooms() {
	s.watcher.canaryRoomID = 999

	// Janus has some rooms
	rooms := []janus.RoomInfo{
		{Room: 123456, Description: "room-123"},
		{Room: 789012, Description: "room-456"},
		{Room: 999, Description: "canary"}, // Should be skipped
	}

	s.mockJanus.EXPECT().
		ListRooms(gomock.Any()).
		Return(rooms, nil)

	// Expect forwarder listing for each non-canary room
	s.mockJanus.EXPECT().
		ListRTPForwarders(gomock.Any(), int64(123456)).
		Return([]janus.RTPForwarderInfo{
			{StreamID: 7890, Host: "10.0.0.1", Port: 5000},
		}, nil)

	s.mockJanus.EXPECT().
		ListRTPForwarders(gomock.Any(), int64(789012)).
		Return([]janus.RTPForwarderInfo{}, nil)

	err := s.watcher.RebuildStart(context.Background())
	s.NoError(err)

	// Verify rooms were stored
	val1, ok1 := s.watcher.activeRooms.Load("room-123")
	s.True(ok1)
	room1 := val1.(*ActiveRoom)
	s.Equal(int64(123456), room1.JanusRoomID)
	s.Equal(int64(7890), room1.StreamID)
	s.Equal("10.0.0.1", room1.FwIP)
	s.Equal(5000, room1.FwPort)

	val2, ok2 := s.watcher.activeRooms.Load("room-456")
	s.True(ok2)
	room2 := val2.(*ActiveRoom)
	s.Equal(int64(789012), room2.JanusRoomID)
	s.Zero(room2.StreamID) // No forwarder

	// Verify canary room was skipped
	_, okCanary := s.watcher.activeRooms.Load("canary")
	s.False(okCanary)
}

func (s *RoomWatcherTestSuite) TestRebuildStart_ListRoomsError() {
	s.mockJanus.EXPECT().
		ListRooms(gomock.Any()).
		Return(nil, errors.New(janus.ErrFailedRequest, "janus connection error"))

	err := s.watcher.RebuildStart(context.Background())
	s.Error(err)
	s.Contains(err.Error(), "janus connection error")
}

func (s *RoomWatcherTestSuite) TestRebuildStart_ListForwardersError() {
	rooms := []janus.RoomInfo{
		{Room: 123456, Description: "room-123"},
	}

	s.mockJanus.EXPECT().
		ListRooms(gomock.Any()).
		Return(rooms, nil)

	// ListRTPForwarders fails but should not fail the entire rebuild
	s.mockJanus.EXPECT().
		ListRTPForwarders(gomock.Any(), int64(123456)).
		Return(nil, errors.New(janus.ErrNoneSuccessResponse, "forwarder list error"))

	err := s.watcher.RebuildStart(context.Background())
	s.NoError(err) // Should continue despite forwarder list error

	// Room should NOT be stored when forwarder listing fails
	_, ok := s.watcher.activeRooms.Load("room-123")
	s.False(ok, "Room should be skipped when forwarder list fails")
}

func (s *RoomWatcherTestSuite) TestRebuildStart_MultipleForwarders() {
	// Test that we pick the first forwarder when multiple exist
	rooms := []janus.RoomInfo{
		{Room: 123456, Description: "room-123"},
	}

	s.mockJanus.EXPECT().
		ListRooms(gomock.Any()).
		Return(rooms, nil)

	// Multiple forwarders
	s.mockJanus.EXPECT().
		ListRTPForwarders(gomock.Any(), int64(123456)).
		Return([]janus.RTPForwarderInfo{
			{StreamID: 111, Host: "10.0.0.1", Port: 5000},
			{StreamID: 222, Host: "10.0.0.2", Port: 5001},
			{StreamID: 333, Host: "10.0.0.3", Port: 5002},
		}, nil)

	err := s.watcher.RebuildStart(context.Background())
	s.NoError(err)

	// Should use first forwarder
	val, _ := s.watcher.activeRooms.Load("room-123")
	room := val.(*ActiveRoom)
	s.Equal(int64(111), room.StreamID)
	s.Equal("10.0.0.1", room.FwIP)
	s.Equal(5000, room.FwPort)
}

func (s *RoomWatcherTestSuite) TestRebuildEnd() {
	err := s.watcher.RebuildEnd(context.Background())
	s.NoError(err)
}

func (s *RoomWatcherTestSuite) TestRebuildState_NoActiveRoom() {
	roomID := "room-123"
	state := &etcdstate.RoomState{}
	state.Mixer = &etcdstate.Mixer{IP: "10.0.0.1", Port: 5000}

	// No active room in watcher
	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err) // Should be a no-op
}

func (s *RoomWatcherTestSuite) TestRebuildState_EndpointMatches() {
	roomID := "room-123"

	// Active room with forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	// State matches active room
	state := &etcdstate.RoomState{}
	state.Mixer = &etcdstate.Mixer{IP: "10.0.0.1", Port: 5000}

	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err)

	// Forwarder should still exist (not stopped)
	val, _ := s.watcher.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Equal(int64(7890), room.StreamID)
}

func (s *RoomWatcherTestSuite) TestRebuildState_EndpointMismatch_StopsForwarder() {
	roomID := "room-123"

	// Active room with forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	// State has different endpoint
	state := &etcdstate.RoomState{}
	state.Mixer = &etcdstate.Mixer{IP: "10.0.0.2", Port: 5001}

	// Expect forwarder to be stopped
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(nil)

	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err)

	// Forwarder should be cleared
	val, _ := s.watcher.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Zero(room.StreamID)
}

func (s *RoomWatcherTestSuite) TestRebuildState_NoMixerData() {
	roomID := "room-123"

	// Active room with forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	// State has no mixer data
	state := &etcdstate.RoomState{}
	state.Mixer = nil

	// Expect forwarder to be stopped (mismatch)
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(nil)

	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err)

	// Forwarder should be cleared
	val, _ := s.watcher.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Zero(room.StreamID)
}

func (s *RoomWatcherTestSuite) TestRebuildState_StopForwarderError() {
	roomID := "room-123"

	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	state := &etcdstate.RoomState{}
	state.Mixer = &etcdstate.Mixer{IP: "10.0.0.2", Port: 5001}

	// Stop forwarder fails but should not return error
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(errors.New(janus.ErrNoneSuccessResponse, "stop failed"))

	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err) // Should log error but not fail
}

func (s *RoomWatcherTestSuite) TestRebuildState_NoForwarder() {
	roomID := "room-123"

	// Active room without forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    0,
	}
	s.watcher.activeRooms.Store(roomID, activeRoom)

	state := &etcdstate.RoomState{}
	state.Mixer = &etcdstate.Mixer{IP: "10.0.0.1", Port: 5000}

	// No Janus API calls expected (no forwarder to stop)

	err := s.watcher.RebuildState(context.Background(), roomID, state)
	s.NoError(err)
}

// ProcessChange Integration Tests
// Testing the full state machine logic

func (s *RoomWatcherTestSuite) TestProcessChange_NotAssignedToUs_NoAction() {
	roomID := "room-123"

	// State: NOT assigned to us
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "other-janus", // Different janus
		Status:  constants.RoomStatusOnAir,
	})

	// No Janus API calls expected
	err := s.watcher.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify no room was created
	_, ok := s.watcher.activeRooms.Load(roomID)
	s.False(ok)
}

func (s *RoomWatcherTestSuite) TestProcessChange_NoMetaOrLiveMeta_NoAction() {
	roomID := "room-123"

	// State: missing livemeta
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	// No livemeta

	err := s.watcher.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify no room was created
	_, ok := s.watcher.activeRooms.Load(roomID)
	s.False(ok)
}

// Helper to create watcher with fake etcd for full processChange testing
func (s *RoomWatcherTestSuite) createWatcherWithFakeEtcd() *RoomWatcher {
	logger := log.NewTest(s.T())
	return &RoomWatcher{
		janusAdmin:  s.mockJanus,
		janusID:     "test-janus-01",
		prefixRooms: "/rooms/",
		logger:      logger,
		etcdClient:  &etcdfakes.EtcdKV{},
	}
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_CreateRoomAndForwarder() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"
	pin := "1234"

	// State: assigned to us with mixer
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: pin, MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	// Expect room creation then forwarder creation
	gomock.InOrder(
		s.mockJanus.EXPECT().
			CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
			Return(nil),
		s.mockJanus.EXPECT().
			CreateRTPForwarder(gomock.Any(), gomock.Any(), "10.0.0.1", 5000).
			Return(int64(7890), nil),
	)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify room and forwarder were created
	val, ok := w.activeRooms.Load(roomID)
	s.True(ok)
	activeRoom := val.(*ActiveRoom)
	s.NotZero(activeRoom.JanusRoomID)
	s.Equal(int64(7890), activeRoom.StreamID)
	s.Equal("10.0.0.1", activeRoom.FwIP)
	s.Equal(5000, activeRoom.FwPort)
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_CreateRoomOnly_NoMixer() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"
	pin := "1234"

	// State: assigned to us but no mixer
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: pin, MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	// No mixer

	// Expect only room creation
	s.mockJanus.EXPECT().
		CreateRoom(gomock.Any(), gomock.Any(), roomID, pin).
		Return(nil)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify room was created but no forwarder
	val, ok := w.activeRooms.Load(roomID)
	s.True(ok)
	activeRoom := val.(*ActiveRoom)
	s.NotZero(activeRoom.JanusRoomID)
	s.Zero(activeRoom.StreamID)
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_DestroyRoom() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"

	// Pre-existing room
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    0,
	}
	w.activeRooms.Store(roomID, activeRoom)

	// State: no longer assigned to us
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "other-janus",
		Status:  constants.RoomStatusOnAir,
	})

	// Expect room destruction
	s.mockJanus.EXPECT().
		DestroyRoom(gomock.Any(), int64(123456)).
		Return(nil)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify room was removed
	_, ok := w.activeRooms.Load(roomID)
	s.False(ok)
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_AddForwarder() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"

	// Pre-existing room WITHOUT forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    0,
	}
	w.activeRooms.Store(roomID, activeRoom)

	// State: should have forwarder
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 5000,
	})

	// Expect forwarder creation
	s.mockJanus.EXPECT().
		CreateRTPForwarder(gomock.Any(), int64(123456), "10.0.0.1", 5000).
		Return(int64(7890), nil)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify forwarder was created
	val, _ := w.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Equal(int64(7890), room.StreamID)
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_RemoveForwarder() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"

	// Pre-existing room WITH forwarder
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	w.activeRooms.Store(roomID, activeRoom)

	// State: should NOT have forwarder (port = 0)
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.1",
		Port: 0,
	})

	// Expect forwarder to be stopped
	s.mockJanus.EXPECT().
		StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
		Return(nil)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify forwarder was removed
	val, _ := w.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Zero(room.StreamID)
}

func (s *RoomWatcherTestSuite) TestProcessChange_Full_RecreateForwarder() {
	w := s.createWatcherWithFakeEtcd()
	roomID := "room-123"

	// Pre-existing room WITH forwarder at old endpoint
	activeRoom := &ActiveRoom{
		JanusRoomID: 123456,
		StreamID:    7890,
		FwIP:        "10.0.0.1",
		FwPort:      5000,
	}
	w.activeRooms.Store(roomID, activeRoom)

	// State: forwarder needed at NEW endpoint
	state := &etcdstate.RoomState{}
	state.SetMeta(&etcdstate.Meta{Pin: "1234", MaxAnchors: 5})
	state.SetLiveMeta(&etcdstate.LiveMeta{
		JanusID: "test-janus-01",
		Status:  constants.RoomStatusOnAir,
	})
	state.SetMixer(&etcdstate.Mixer{
		IP:   "10.0.0.2",
		Port: 5001,
	})

	// Expect old forwarder stopped and new one created
	gomock.InOrder(
		s.mockJanus.EXPECT().
			StopRTPForwarder(gomock.Any(), int64(123456), int64(7890)).
			Return(nil),
		s.mockJanus.EXPECT().
			CreateRTPForwarder(gomock.Any(), int64(123456), "10.0.0.2", 5001).
			Return(int64(9999), nil),
	)

	err := w.processChange(context.Background(), roomID, state)
	s.NoError(err)

	// Verify forwarder was recreated
	val, _ := w.activeRooms.Load(roomID)
	room := val.(*ActiveRoom)
	s.Equal(int64(9999), room.StreamID)
	s.Equal("10.0.0.2", room.FwIP)
	s.Equal(5001, room.FwPort)
}
