package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSDPGenerator(t *testing.T) {
	t.Run("create with custom sdpDir", func(t *testing.T) {
		sg := NewSDPGenerator("/custom/sdp")
		assert.Equal(t, "/custom/sdp", sg.sdpDir)
	})

	t.Run("create with empty sdpDir uses default", func(t *testing.T) {
		sg := NewSDPGenerator("")
		assert.Equal(t, "/tmp/sdp", sg.sdpDir)
	})
}

func TestSDPGenerate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sdp-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("generate SDP file successfully", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "room1"
		rtpPort := 5004

		sdpPath, err := sg.Generate(roomID, rtpPort)

		assert.NoError(t, err)
		assert.NotEmpty(t, sdpPath)
		assert.Equal(t, filepath.Join(tmpDir, "room1.sdp"), sdpPath)
		assert.FileExists(t, sdpPath)

		content, err := os.ReadFile(sdpPath)
		assert.NoError(t, err)

		sdpStr := string(content)
		assert.Contains(t, sdpStr, "v=0")
		assert.Contains(t, sdpStr, "s=Janus AudioBridge Stream - Room room1")
		assert.Contains(t, sdpStr, "m=audio 5004 RTP/AVP 100")
		assert.Contains(t, sdpStr, "a=rtpmap:100 opus/48000/2")
	})

	t.Run("generate SDP with different ports", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "room2"
		rtpPort := 6008

		sdpPath, err := sg.Generate(roomID, rtpPort)

		assert.NoError(t, err)

		content, err := os.ReadFile(sdpPath)
		assert.NoError(t, err)

		assert.Contains(t, string(content), "m=audio 6008 RTP/AVP 100")
	})

	t.Run("generate creates directory if not exists", func(t *testing.T) {
		newDir := filepath.Join(tmpDir, "new-sdp-dir")
		sg := NewSDPGenerator(newDir)
		roomID := "room3"

		sdpPath, err := sg.Generate(roomID, 5010)

		assert.NoError(t, err)
		assert.FileExists(t, sdpPath)
		assert.DirExists(t, newDir)
	})

	t.Run("generate overwrites existing file", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "room4"

		sdpPath1, err := sg.Generate(roomID, 5012)
		assert.NoError(t, err)

		content1, err := os.ReadFile(sdpPath1)
		assert.NoError(t, err)

		sdpPath2, err := sg.Generate(roomID, 5014)
		assert.NoError(t, err)

		content2, err := os.ReadFile(sdpPath2)
		assert.NoError(t, err)

		assert.NotEqual(t, string(content1), string(content2))
		assert.Contains(t, string(content2), "5014")
	})

	t.Run("SDP content format is correct", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "format-test"
		rtpPort := 5016

		sdpPath, err := sg.Generate(roomID, rtpPort)
		assert.NoError(t, err)

		content, err := os.ReadFile(sdpPath)
		assert.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		assert.GreaterOrEqual(t, len(lines), 6)

		assert.True(t, strings.HasPrefix(lines[0], "v="))
		assert.True(t, strings.HasPrefix(lines[1], "o="))
		assert.True(t, strings.HasPrefix(lines[2], "s="))
		assert.True(t, strings.HasPrefix(lines[3], "c="))
		assert.True(t, strings.HasPrefix(lines[4], "t="))
		assert.True(t, strings.HasPrefix(lines[5], "m="))
	})
}

func TestSDPDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sdp-delete-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("delete existing SDP file", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "room1"

		sdpPath, err := sg.Generate(roomID, 5004)
		assert.NoError(t, err)
		assert.FileExists(t, sdpPath)

		err = sg.Delete(roomID)
		assert.NoError(t, err)

		assert.NoFileExists(t, sdpPath)
	})

	t.Run("delete non-existent SDP file", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)
		roomID := "nonexistent"

		err := sg.Delete(roomID)
		assert.NoError(t, err)
	})

	t.Run("delete multiple files", func(t *testing.T) {
		sg := NewSDPGenerator(tmpDir)

		rooms := []string{"room1", "room2", "room3"}
		for _, roomID := range rooms {
			_, err := sg.Generate(roomID, 5004)
			assert.NoError(t, err)
		}

		for _, roomID := range rooms {
			err := sg.Delete(roomID)
			assert.NoError(t, err)

			sdpPath := filepath.Join(tmpDir, roomID+".sdp")
			assert.NoFileExists(t, sdpPath)
		}
	})
}
