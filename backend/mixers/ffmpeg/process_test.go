package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type ProcessTestSuite struct {
	suite.Suite
	tmpDir      string
	sdpPath     string
	hlsDir      string
	keyInfoPath string
}

func TestProcessSuite(t *testing.T) {
	suite.Run(t, new(ProcessTestSuite))
}

func (s *ProcessTestSuite) SetupTest() {
	var err error
	s.tmpDir, err = os.MkdirTemp("", "process-test-*")
	s.Require().NoError(err)

	s.sdpPath = filepath.Join(s.tmpDir, "test.sdp")
	s.hlsDir = filepath.Join(s.tmpDir, "hls")
	s.keyInfoPath = filepath.Join(s.tmpDir, "enc.keyinfo")

	// Create necessary files
	err = os.WriteFile(s.sdpPath, []byte("v=0\n"), 0600)
	s.Require().NoError(err)

	err = os.MkdirAll(s.hlsDir, 0755)
	s.Require().NoError(err)

	err = os.WriteFile(s.keyInfoPath, []byte("key\n"), 0600)
	s.Require().NoError(err)
}

func (s *ProcessTestSuite) TearDownTest() {
	if s.tmpDir != "" {
		os.RemoveAll(s.tmpDir)
	}
}

func (s *ProcessTestSuite) TestProcessInfo_StartStopWithEcho() {
	processInfo := NewProcessInfo(
		"test-room",
		5004,
		s.sdpPath,
		s.hlsDir,
		s.keyInfoPath,
		0,
		log.NewNop(),
	)

	started := make(chan struct{})
	// Use echo command instead of ffmpeg (exits immediately)
	processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
		close(started)
		return exec.Command("echo", "test")
	}

	// Start process
	processInfo.Start()

	// Wait for process to actually spawn
	select {
	case <-started:
	case <-time.After(50 * time.Millisecond):
		s.Fail("Process didn't start")
	}

	// Stop process
	processInfo.Stop()
}

func (s *ProcessTestSuite) TestProcessInfo_StartStopWithSleep() {
	processInfo := NewProcessInfo(
		"sleep-room",
		5006,
		s.sdpPath,
		s.hlsDir,
		s.keyInfoPath,
		0,
		log.NewNop(),
	)

	started := make(chan struct{})
	// Use sleep command (runs for a while)
	processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
		close(started)
		return exec.Command("sleep", "10")
	}

	// Start process
	processInfo.Start()

	// Wait for process to actually spawn
	select {
	case <-started:
	case <-time.After(50 * time.Millisecond):
		s.Fail("Process didn't start")
	}

	// Stop process immediately (should kill the sleep)
	processInfo.Stop()
}

func (s *ProcessTestSuite) TestProcessInfo_StoresCorrectValues() {
	processInfo := NewProcessInfo(
		"info-room",
		5008,
		s.sdpPath,
		s.hlsDir,
		s.keyInfoPath,
		10,
		log.NewNop(),
	)

	s.Equal("info-room", processInfo.roomID)
	s.Equal(5008, processInfo.rtpPort)
	s.Equal(s.sdpPath, processInfo.sdpPath)
	s.Equal(s.hlsDir, processInfo.hlsDir)
	s.Equal(s.keyInfoPath, processInfo.keyInfoPath)
	s.Equal(10, processInfo.initSeq)
	s.NotNil(processInfo.chanStop)
	s.NotNil(processInfo.logger)
}

func (s *ProcessTestSuite) TestProcessInfo_QuickExitCommands() {
	processInfo := NewProcessInfo(
		"quick-room",
		5010,
		s.sdpPath,
		s.hlsDir,
		s.keyInfoPath,
		0,
		log.NewNop(),
	)

	started := make(chan struct{})
	// Use true command (exits successfully immediately)
	processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
		close(started)
		return exec.Command("true")
	}

	processInfo.Start()

	select {
	case <-started:
	case <-time.After(50 * time.Millisecond):
		s.Fail("Process didn't start")
	}

	processInfo.Stop()
}

func (s *ProcessTestSuite) TestProcessInfo_FailingCommands() {
	processInfo := NewProcessInfo(
		"fail-room",
		5012,
		s.sdpPath,
		s.hlsDir,
		s.keyInfoPath,
		0,
		log.NewNop(),
	)

	started := make(chan struct{})
	// Use false command (exits with failure immediately)
	processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
		close(started)
		return exec.Command("false")
	}

	processInfo.Start()

	select {
	case <-started:
	case <-time.After(50 * time.Millisecond):
		s.Fail("Process didn't start")
	}

	processInfo.Stop()
}
