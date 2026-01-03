package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEncryptionGenerator(t *testing.T) {
	t.Run("create with custom tmpDir", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", "/custom/tmp")
		assert.Equal(t, "https://example.com/keys/", eg.keyBaseURL)
		assert.Equal(t, "/custom/tmp", eg.tmpDir)
	})

	t.Run("create with empty tmpDir uses default", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", "")
		assert.Equal(t, "/tmp", eg.tmpDir)
	})
}

func TestEncryptionGenerate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "enc-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	hlsDir := filepath.Join(tmpDir, "hls", "room1")
	err = os.MkdirAll(hlsDir, 0755)
	assert.NoError(t, err)

	t.Run("generate encryption key and keyinfo", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
		roomID := "room1"
		nonce := "testnonce123"

		keyInfoPath, err := eg.Generate(roomID, nonce, hlsDir)

		assert.NoError(t, err)
		assert.NotEmpty(t, keyInfoPath)

		keyPath := filepath.Join(tmpDir, "enc.key")
		assert.FileExists(t, keyPath)

		keyInfo, err := os.ReadFile(keyInfoPath)
		assert.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(keyInfo)), "\n")
		assert.Len(t, lines, 3)

		assert.Contains(t, lines[0], "https://example.com/keys/room1/enc.key")
		assert.Contains(t, lines[1], "enc.key")
		assert.NotEmpty(t, lines[2])
	})

	t.Run("generate with empty keyBaseURL", func(t *testing.T) {
		eg := NewEncryptionGenerator("", tmpDir)
		roomID := "room2"
		nonce := "nonce456"

		keyInfoPath, err := eg.Generate(roomID, nonce, hlsDir)

		assert.NoError(t, err)

		keyInfo, err := os.ReadFile(keyInfoPath)
		assert.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(keyInfo)), "\n")
		assert.Equal(t, "enc.key", lines[0])
	})

	t.Run("generate creates key file", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
		roomID := "room3"
		nonce := "nonce789"

		_, err := eg.Generate(roomID, nonce, hlsDir)

		assert.NoError(t, err)

		keyPath := filepath.Join(tmpDir, "enc.key")
		keyData, err := os.ReadFile(keyPath)
		assert.NoError(t, err)
		assert.Len(t, keyData, 16)
	})

	t.Run("generate with same nonce produces consistent key", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
		roomID := "room4"
		nonce := "consistentnonce"

		_, err := eg.Generate(roomID, nonce, hlsDir)
		assert.NoError(t, err)

		keyPath := filepath.Join(tmpDir, "enc.key")
		key1, err := os.ReadFile(keyPath)
		assert.NoError(t, err)

		_, err = eg.Generate(roomID, nonce, hlsDir)
		assert.NoError(t, err)

		key2, err := os.ReadFile(keyPath)
		assert.NoError(t, err)

		assert.Equal(t, key1, key2)
	})
}

func TestEncryptionDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "enc-delete-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	hlsDir := filepath.Join(tmpDir, "hls", "room1")
	err = os.MkdirAll(hlsDir, 0755)
	assert.NoError(t, err)

	t.Run("delete existing keyinfo file", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
		roomID := "room1"

		_, err := eg.Generate(roomID, "nonce", hlsDir)
		assert.NoError(t, err)

		keyInfoPath := filepath.Join(tmpDir, "enc-room1.keyinfo")
		assert.FileExists(t, keyInfoPath)

		err = eg.Delete(roomID)
		assert.NoError(t, err)

		assert.NoFileExists(t, keyInfoPath)
	})

	t.Run("delete non-existent keyinfo file", func(t *testing.T) {
		eg := NewEncryptionGenerator("https://example.com/keys/", tmpDir)
		roomID := "nonexistent"

		err := eg.Delete(roomID)
		assert.NoError(t, err)
	})
}
