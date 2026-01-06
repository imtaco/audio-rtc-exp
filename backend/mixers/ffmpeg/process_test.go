package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

func TestProcessInfoWithTestCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "process-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	sdpPath := filepath.Join(tmpDir, "test.sdp")
	hlsDir := filepath.Join(tmpDir, "hls")
	keyInfoPath := filepath.Join(tmpDir, "enc.keyinfo")

	// Create necessary files
	err = os.WriteFile(sdpPath, []byte("v=0\n"), 0600)
	require.NoError(t, err)

	err = os.MkdirAll(hlsDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(keyInfoPath, []byte("key\n"), 0600)
	require.NoError(t, err)

	t.Run("can start and stop with echo command", func(t *testing.T) {
		processInfo := NewProcessInfo(
			"test-room",
			5004,
			sdpPath,
			hlsDir,
			keyInfoPath,
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
			t.Fatal("Process didn't start")
		}

		// Stop process
		processInfo.Stop()
	})

	t.Run("can start and stop with sleep command", func(t *testing.T) {
		processInfo := NewProcessInfo(
			"sleep-room",
			5006,
			sdpPath,
			hlsDir,
			keyInfoPath,
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
			t.Fatal("Process didn't start")
		}

		// Stop process immediately (should kill the sleep)
		processInfo.Stop()
	})

	t.Run("process info stores correct values", func(t *testing.T) {
		processInfo := NewProcessInfo(
			"info-room",
			5008,
			sdpPath,
			hlsDir,
			keyInfoPath,
			10,
			log.NewNop(),
		)

		assert.Equal(t, "info-room", processInfo.roomID)
		assert.Equal(t, 5008, processInfo.rtpPort)
		assert.Equal(t, sdpPath, processInfo.sdpPath)
		assert.Equal(t, hlsDir, processInfo.hlsDir)
		assert.Equal(t, keyInfoPath, processInfo.keyInfoPath)
		assert.Equal(t, 10, processInfo.initSeq)
		assert.NotNil(t, processInfo.chanStop)
		assert.NotNil(t, processInfo.logger)
	})

	t.Run("can handle quick exit commands", func(t *testing.T) {
		processInfo := NewProcessInfo(
			"quick-room",
			5010,
			sdpPath,
			hlsDir,
			keyInfoPath,
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
			t.Fatal("Process didn't start")
		}

		processInfo.Stop()
	})

	t.Run("can handle failing commands", func(t *testing.T) {
		processInfo := NewProcessInfo(
			"fail-room",
			5012,
			sdpPath,
			hlsDir,
			keyInfoPath,
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
			t.Fatal("Process didn't start")
		}

		processInfo.Stop()
	})
}

func TestFFmpegManagerWithTestCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-mgr-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	sdpDir := filepath.Join(tmpDir, "sdp")
	hlsDir := filepath.Join(tmpDir, "hls")

	encGen := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
	sdpGen := NewSDPGenerator(sdpDir)

	mgr := NewFFmpegManager(
		hlsDir,
		encGen,
		sdpGen,
		100*time.Millisecond,
		500*time.Millisecond,
		log.NewNop(),
	).(*ffmpegMgrImpl)

	t.Run("start ffmpeg with test command injection", func(t *testing.T) {
		roomID := "test-inject"
		rtpPort := 5004
		createdAt := time.Now()
		nonce := "testnonce"

		// Start FFmpeg (creates files but doesn't start real process)
		err := mgr.StartFFmpeg(roomID, rtpPort, createdAt, nonce)
		require.NoError(t, err)

		// Get process info and inject test command
		val, exists := mgr.processes.Load(roomID)
		require.True(t, exists)

		processInfo := val.(*ProcessInfo)
		processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
			return exec.Command("echo", "mock ffmpeg")
		}

		// Manual start (will use echo instead of ffmpeg)
		// Note: StartFFmpeg already called processInfo.Start()
		// so echo is already running

		// Give it a moment to process
		time.Sleep(5 * time.Millisecond)

		// Stop
		err = mgr.StopFFmpeg(roomID)
		require.NoError(t, err)

		// Note: Cleanup happens asynchronously after forceKillTimeout+1s,
		// but we don't wait for it in this test to keep tests fast
	})

	t.Run("manage multiple processes with test commands", func(t *testing.T) {
		rooms := []struct {
			roomID  string
			rtpPort int
			cmdFunc func() *exec.Cmd
		}{
			{
				"room1",
				5010,
				func() *exec.Cmd { return exec.Command("sleep", "0.5") },
			},
			{
				"room2",
				5012,
				func() *exec.Cmd { return exec.Command("echo", "test") },
			},
			{
				"room3",
				5014,
				func() *exec.Cmd { return exec.Command("true") },
			},
		}

		// Start all rooms
		for _, room := range rooms {
			err := mgr.StartFFmpeg(room.roomID, room.rtpPort, time.Now(), "nonce")
			require.NoError(t, err)

			// Inject test command
			val, _ := mgr.processes.Load(room.roomID)
			processInfo := val.(*ProcessInfo)
			cmdFunc := room.cmdFunc
			processInfo.SpawnFFmpeg = func(_, _ string, _ int, _ string) *exec.Cmd {
				return cmdFunc()
			}
		}

		// Give processes time to start
		time.Sleep(5 * time.Millisecond)

		// Stop all
		err := mgr.Stop()
		require.NoError(t, err)

		// Note: Cleanup happens asynchronously, but we don't wait to keep tests fast
	})
}
