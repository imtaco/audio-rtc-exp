package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type RoomWatcherTestSuite struct {
	suite.Suite
	watcher *roomWatcherWithStats
	ctx     context.Context
}

func TestRoomWatcherSuite(t *testing.T) {
	suite.Run(t, new(RoomWatcherTestSuite))
}

func (s *RoomWatcherTestSuite) SetupTest() {
	s.ctx = context.Background()
	logger := log.NewTest(s.T())

	s.watcher = &roomWatcherWithStats{
		janusUsage: newModuleUsage("janus", logger),
		mixerUsage: newModuleUsage("mixer", logger),
		logger:     logger,
	}
}

func (s *RoomWatcherTestSuite) TestProcessChange_NewJanusAndMixer() {
	newState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}

	err := s.watcher.processChange(s.ctx, "room-1", newState)
	s.Require().NoError(err)

	s.Equal(1, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestProcessChange_JanusChanged() {
	// Setup initial state - assign room-1 to janus-1 first
	initialState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", initialState))

	// Also add another room to janus-1
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-2", initialState))

	// Verify initial counts
	s.Equal(2, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(2, s.watcher.GetMixerStreamCount("mixer-1"))

	// Now change room-1 to janus-2
	newState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-2",
			MixerID: "mixer-1",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", newState))

	// janus-1 should be decremented
	s.Equal(1, s.watcher.GetJanusStreamCount("janus-1"))
	// janus-2 should be incremented
	s.Equal(1, s.watcher.GetJanusStreamCount("janus-2"))
	// mixer should remain at 2
	s.Equal(2, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestProcessChange_MixerChanged() {
	// Setup initial state - assign room-1 to mixer-1 first
	initialState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", initialState))

	// Also add another room to mixer-1
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-2", initialState))

	// Verify initial counts
	s.Equal(2, s.watcher.GetMixerStreamCount("mixer-1"))
	s.Equal(2, s.watcher.GetJanusStreamCount("janus-1"))

	// Now change room-1 to mixer-2
	newState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-2",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", newState))

	// mixer-1 should be decremented
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-1"))
	// mixer-2 should be incremented
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-2"))
	// janus should remain at 2
	s.Equal(2, s.watcher.GetJanusStreamCount("janus-1"))
}

func (s *RoomWatcherTestSuite) TestProcessChange_ModuleRemoved() {
	// Setup initial state
	initialState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", initialState))

	// Verify they exist
	s.Equal(1, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-1"))

	// Remove modules
	newState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "",
			MixerID: "",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", newState))

	// Both should be removed
	s.Equal(0, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(0, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestProcessChange_NilLiveMeta() {
	newState := &etcdstate.RoomState{}

	// Should not panic and should not modify usage
	err := s.watcher.processChange(s.ctx, "room-1", newState)
	s.Require().NoError(err)

	s.Equal(0, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(0, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestGetJanusStreamCount() {
	// Add multiple rooms to different janus instances
	state1 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{JanusID: "janus-1"},
	}
	state2 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{JanusID: "janus-2"},
	}

	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", state1))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-2", state1))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-3", state1))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-4", state1))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-5", state1))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-6", state2))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-7", state2))
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-8", state2))

	s.Equal(5, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(3, s.watcher.GetJanusStreamCount("janus-2"))
	s.Equal(0, s.watcher.GetJanusStreamCount("non-existent"))
}

func (s *RoomWatcherTestSuite) TestGetMixerStreamCount() {
	// Add multiple rooms to different mixer instances
	state1 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{MixerID: "mixer-1"},
	}
	state2 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{MixerID: "mixer-2"},
	}

	for i := 0; i < 10; i++ {
		s.Require().NoError(s.watcher.processChange(s.ctx, "room-"+string(rune('a'+i)), state1))
	}
	for i := 0; i < 7; i++ {
		s.Require().NoError(s.watcher.processChange(s.ctx, "room-"+string(rune('k'+i)), state2))
	}

	s.Equal(10, s.watcher.GetMixerStreamCount("mixer-1"))
	s.Equal(7, s.watcher.GetMixerStreamCount("mixer-2"))
	s.Equal(0, s.watcher.GetMixerStreamCount("non-existent"))
}

func (s *RoomWatcherTestSuite) TestRebuildStart() {
	// Setup some existing data
	state := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	s.Require().NoError(s.watcher.processChange(s.ctx, "room-1", state))

	// Verify data exists
	s.Equal(1, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-1"))

	err := s.watcher.RebuildStart(s.ctx)
	s.Require().NoError(err)

	// RebuildEnd must be called to release the write lock
	s.Require().NoError(s.watcher.RebuildEnd(s.ctx))

	// Usage should be cleared
	s.Equal(0, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(0, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestRebuildState_WithLiveMeta() {
	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}

	// RebuildState should be called between RebuildStart and RebuildEnd
	s.Require().NoError(s.watcher.RebuildStart(s.ctx))
	err := s.watcher.RebuildState(s.ctx, "room-1", roomState)
	s.Require().NoError(err)
	s.Require().NoError(s.watcher.RebuildEnd(s.ctx))

	s.Equal(1, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestRebuildState_MultipleRooms() {
	room1 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	room2 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-2",
		},
	}
	room3 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-2",
			MixerID: "mixer-1",
		},
	}

	// RebuildState should be called between RebuildStart and RebuildEnd
	s.Require().NoError(s.watcher.RebuildStart(s.ctx))
	s.Require().NoError(s.watcher.RebuildState(s.ctx, "room-1", room1))
	s.Require().NoError(s.watcher.RebuildState(s.ctx, "room-2", room2))
	s.Require().NoError(s.watcher.RebuildState(s.ctx, "room-3", room3))
	s.Require().NoError(s.watcher.RebuildEnd(s.ctx))

	// janus-1 used by 2 rooms, janus-2 by 1
	s.Equal(2, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(1, s.watcher.GetJanusStreamCount("janus-2"))

	// mixer-1 used by 2 rooms, mixer-2 by 1
	s.Equal(2, s.watcher.GetMixerStreamCount("mixer-1"))
	s.Equal(1, s.watcher.GetMixerStreamCount("mixer-2"))
}

func (s *RoomWatcherTestSuite) TestRebuildState_WithoutLiveMeta() {
	roomState := &etcdstate.RoomState{
		Meta: &etcdstate.Meta{
			Pin: "1234",
		},
	}

	s.Require().NoError(s.watcher.RebuildStart(s.ctx))
	err := s.watcher.RebuildState(s.ctx, "room-1", roomState)
	s.Require().NoError(err)
	s.Require().NoError(s.watcher.RebuildEnd(s.ctx))

	// No usage should be tracked
	s.Equal(0, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(0, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestRebuildState_EmptyModuleIDs() {
	roomState := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "",
			MixerID: "",
		},
	}

	s.Require().NoError(s.watcher.RebuildStart(s.ctx))
	err := s.watcher.RebuildState(s.ctx, "room-1", roomState)
	s.Require().NoError(err)
	s.Require().NoError(s.watcher.RebuildEnd(s.ctx))

	// No usage should be tracked for empty IDs
	s.Equal(0, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(0, s.watcher.GetMixerStreamCount("mixer-1"))
}

func (s *RoomWatcherTestSuite) TestRebuildEnd() {
	// RebuildEnd should be called after RebuildStart
	err := s.watcher.RebuildStart(s.ctx)
	s.Require().NoError(err)

	err = s.watcher.RebuildEnd(s.ctx)
	s.Require().NoError(err)
}

func (s *RoomWatcherTestSuite) TestNewState_Meta() {
	data := []byte(`{"pin":"1234","hlsPath":"/hls/room-1","createdAt":"2024-01-01T00:00:00Z"}`)
	newState, err := s.watcher.NewState("room-1", constants.RoomKeyMeta, data, nil)

	s.Require().NoError(err)
	s.NotNil(newState)
	s.NotNil(newState.Meta)
	s.Equal("1234", newState.Meta.Pin)
}

func (s *RoomWatcherTestSuite) TestNewState_LiveMeta() {
	data := []byte(`{"janusID":"janus-1","mixerID":"mixer-1","status":"on-air"}`)
	curState := &etcdstate.RoomState{}
	newState, err := s.watcher.NewState("room-1", constants.RoomKeyLiveMeta, data, curState)

	s.Require().NoError(err)
	s.NotNil(newState)
	s.NotNil(newState.LiveMeta)
	s.Equal("janus-1", newState.LiveMeta.JanusID)
	s.Equal("mixer-1", newState.LiveMeta.MixerID)
}

func (s *RoomWatcherTestSuite) TestNewState_Janus() {
	data := []byte(`{"roomID":"room-1","instanceID":"janus-1"}`)
	curState := &etcdstate.RoomState{}
	newState, err := s.watcher.NewState("room-1", constants.RoomKeyJanus, data, curState)

	s.Require().NoError(err)
	s.NotNil(newState)
	s.NotNil(newState.Janus)
}

func (s *RoomWatcherTestSuite) TestNewState_Mixer() {
	data := []byte(`{"roomID":"room-1","instanceID":"mixer-1"}`)
	curState := &etcdstate.RoomState{}
	newState, err := s.watcher.NewState("room-1", constants.RoomKeyMixer, data, curState)

	s.Require().NoError(err)
	s.NotNil(newState)
	s.NotNil(newState.Mixer)
}

func (s *RoomWatcherTestSuite) TestNewState_EmptyData() {
	curState := &etcdstate.RoomState{
		Meta: &etcdstate.Meta{
			Pin: "1234",
		},
	}

	newState, err := s.watcher.NewState("room-1", constants.RoomKeyMeta, []byte{}, curState)

	s.Require().NoError(err)
	// When data is empty and state is now empty, should return nil
	s.Nil(newState)
}

func (s *RoomWatcherTestSuite) TestNewState_CreateNewStateWhenNil() {
	data := []byte(`{"pin":"1234"}`)
	newState, err := s.watcher.NewState("room-1", constants.RoomKeyMeta, data, nil)

	s.Require().NoError(err)
	s.NotNil(newState)
	s.NotNil(newState.Meta)
}

func (s *RoomWatcherTestSuite) TestConcurrentStreamCountReads() {
	// Setup initial data
	state := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			JanusID: "janus-1",
			MixerID: "mixer-1",
		},
	}
	for i := 0; i < 10; i++ {
		s.Require().NoError(s.watcher.processChange(s.ctx, "room-"+string(rune('a'+i)), state))
	}

	state2 := &etcdstate.RoomState{
		LiveMeta: &etcdstate.LiveMeta{
			MixerID: "mixer-1",
		},
	}
	for i := 0; i < 10; i++ {
		s.Require().NoError(s.watcher.processChange(s.ctx, "room-"+string(rune('k'+i)), state2))
	}

	// Verify counts
	s.Equal(10, s.watcher.GetJanusStreamCount("janus-1"))
	s.Equal(20, s.watcher.GetMixerStreamCount("mixer-1"))

	// Simulate concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			count := s.watcher.GetJanusStreamCount("janus-1")
			s.Equal(10, count)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
