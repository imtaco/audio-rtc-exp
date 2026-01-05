package ffmpeg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/stretchr/testify/suite"
)

type FFmpegManagerTestSuite struct {
	suite.Suite
	tmpDir    string
	sdpDir    string
	encGen    *EncryptionGenerator
	sdpGen    *SDPGenerator
	ffmpegMgr *ffmpegMgrImpl
}

func TestFFmpegManagerSuite(t *testing.T) {
	suite.Run(t, new(FFmpegManagerTestSuite))
}

func (s *FFmpegManagerTestSuite) SetupTest() {
	var err error
	s.tmpDir, err = os.MkdirTemp("", "ffmpeg-test-*")
	s.Require().NoError(err)

	s.sdpDir = filepath.Join(s.tmpDir, "sdp")
	hlsDir := filepath.Join(s.tmpDir, "hls")

	s.encGen = NewEncryptionGenerator("https://example.com/keys/", s.tmpDir)
	s.sdpGen = NewSDPGenerator(s.sdpDir)

	mgr := NewFFmpegManager(
		hlsDir,
		s.encGen,
		s.sdpGen,
		100*time.Millisecond,
		500*time.Millisecond,
		log.NewNop(),
	)

	s.ffmpegMgr = mgr.(*ffmpegMgrImpl)
}

func (s *FFmpegManagerTestSuite) TearDownTest() {
	if s.tmpDir != "" {
		os.RemoveAll(s.tmpDir)
	}
}

func (s *FFmpegManagerTestSuite) TestNewFFmpegManager() {
	s.Run("create with default values", func() {
		mgr := NewFFmpegManager(
			"/tmp/hls",
			s.encGen,
			s.sdpGen,
			0,
			0,
			log.NewNop(),
		).(*ffmpegMgrImpl)

		s.Assert().Equal("/tmp/hls", mgr.hlsDir)
		s.Assert().Equal(1*time.Second, mgr.retryDelay)
		s.Assert().Equal(5*time.Second, mgr.forceKillTimeout)
	})

	s.Run("create with custom values", func() {
		mgr := NewFFmpegManager(
			"/custom/hls",
			s.encGen,
			s.sdpGen,
			2*time.Second,
			10*time.Second,
			log.NewNop(),
		).(*ffmpegMgrImpl)

		s.Assert().Equal("/custom/hls", mgr.hlsDir)
		s.Assert().Equal(2*time.Second, mgr.retryDelay)
		s.Assert().Equal(10*time.Second, mgr.forceKillTimeout)
	})

	s.Run("clean path", func() {
		mgr := NewFFmpegManager(
			"/tmp/hls/",
			s.encGen,
			s.sdpGen,
			0,
			0,
			log.NewNop(),
		).(*ffmpegMgrImpl)

		s.Assert().Equal("/tmp/hls", mgr.hlsDir)
	})
}

func (s *FFmpegManagerTestSuite) TestCalculateSeqNo() {
	s.Run("with valid createdAt", func() {
		roomID := "room1"
		createdAt := time.Now().Add(-10 * time.Second)

		seqNo := s.ffmpegMgr.calculateSeqNo(roomID, createdAt)

		s.Assert().Greater(seqNo, 0)
		s.Assert().LessOrEqual(seqNo, 10)
	})

	s.Run("with empty createdAt", func() {
		roomID := "room1"
		createdAt := time.Time{}

		seqNo := s.ffmpegMgr.calculateSeqNo(roomID, createdAt)

		s.Assert().Equal(0, seqNo)
	})

	s.Run("with old createdAt", func() {
		roomID := "room1"
		createdAt := time.Now().Add(-100 * time.Second)

		seqNo := s.ffmpegMgr.calculateSeqNo(roomID, createdAt)

		s.Assert().Greater(seqNo, 40)
	})

	s.Run("with recent createdAt", func() {
		roomID := "room1"
		createdAt := time.Now().Add(-1 * time.Second)

		seqNo := s.ffmpegMgr.calculateSeqNo(roomID, createdAt)

		s.Assert().GreaterOrEqual(seqNo, 0)
	})
}

func (s *FFmpegManagerTestSuite) TestStartFFmpeg() {
	s.Run("start ffmpeg creates necessary files", func() {
		roomID := "test-room"
		rtpPort := 5004
		createdAt := time.Now()
		nonce := "abc123"

		err := s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, createdAt, nonce)

		s.Require().NoError(err)

		sdpPath := filepath.Join(s.sdpDir, roomID+".sdp")
		s.Assert().FileExists(sdpPath)

		hlsDir := filepath.Join(s.ffmpegMgr.hlsDir, roomID)
		s.Assert().DirExists(hlsDir)

		keyInfoPath := filepath.Join(s.tmpDir, "enc-"+roomID+".keyinfo")
		s.Assert().FileExists(keyInfoPath)
	})

	s.Run("start ffmpeg stores process info", func() {
		roomID := "test-room-2"
		rtpPort := 5006
		createdAt := time.Now()
		nonce := "def456"

		err := s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, createdAt, nonce)

		s.Require().NoError(err)

		val, exists := s.ffmpegMgr.processes.Load(roomID)
		s.Assert().True(exists)
		s.Assert().NotNil(val)

		processInfo := val.(*ProcessInfo)
		s.Assert().Equal(roomID, processInfo.roomID)
		s.Assert().Equal(rtpPort, processInfo.rtpPort)
	})

	s.Run("start ffmpeg when already running", func() {
		roomID := "existing-room"
		rtpPort := 5008

		err := s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, time.Now(), "nonce1")
		s.Require().NoError(err)

		err = s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, time.Now(), "nonce2")

		s.Require().Error(err)
		s.Assert().Contains(err.Error(), "already running")
	})
}

func (s *FFmpegManagerTestSuite) TestStopFFmpeg() {
	s.Run("stop existing ffmpeg process", func() {
		roomID := "stop-test"
		rtpPort := 5010

		err := s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, time.Now(), "nonce")
		s.Require().NoError(err)

		err = s.ffmpegMgr.StopFFmpeg(roomID)

		s.Require().NoError(err)
	})

	s.Run("stop non-existent ffmpeg process", func() {
		roomID := "nonexistent-room"

		err := s.ffmpegMgr.StopFFmpeg(roomID)

		s.Require().Error(err)
		s.Assert().Contains(err.Error(), "no FFmpeg process found")
	})

	s.Run("cleanup resources after stop", func() {
		roomID := "cleanup-test"
		rtpPort := 5012

		err := s.ffmpegMgr.StartFFmpeg(roomID, rtpPort, time.Now(), "nonce")
		s.Require().NoError(err)

		sdpPath := filepath.Join(s.sdpDir, roomID+".sdp")
		keyInfoPath := filepath.Join(s.tmpDir, "enc-"+roomID+".keyinfo")
		s.Assert().FileExists(sdpPath)
		s.Assert().FileExists(keyInfoPath)

		err = s.ffmpegMgr.StopFFmpeg(roomID)
		s.Require().NoError(err)

		time.Sleep(100 * time.Millisecond)

		s.Assert().NoFileExists(sdpPath)
		s.Assert().NoFileExists(keyInfoPath)
	})
}

func (s *FFmpegManagerTestSuite) TestStopAll() {
	s.Run("stop all processes", func() {
		rooms := []string{"room1", "room2", "room3"}

		for i, roomID := range rooms {
			err := s.ffmpegMgr.StartFFmpeg(roomID, 5020+i*2, time.Now(), "nonce")
			s.Require().NoError(err)
		}

		count := 0
		s.ffmpegMgr.processes.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		s.Assert().Equal(3, count)

		err := s.ffmpegMgr.Stop()

		s.Require().NoError(err)
	})

	s.Run("stop empty processes", func() {
		err := s.ffmpegMgr.Stop()

		s.Require().NoError(err)
	})
}
