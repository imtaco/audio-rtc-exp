package ffmpeg

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/imtaco/audio-rtc-exp/internal/cryptoutil"
)

// EncryptionGenerator generates encryption key files for HLS
type EncryptionGenerator struct {
	keyBaseURL string
	tmpDir     string
}

// NewEncryptionGenerator creates a new EncryptionGenerator
func NewEncryptionGenerator(keyBaseURL, tmpDir string) *EncryptionGenerator {
	if tmpDir == "" {
		tmpDir = "/tmp"
	}
	return &EncryptionGenerator{
		keyBaseURL: keyBaseURL,
		tmpDir:     tmpDir,
	}
}

// Generate creates encryption key and keyinfo files for FFmpeg
// Note: nonce should not change for a given room to ensure consistent key generation
func (eg *EncryptionGenerator) Generate(roomID, nonce, _ string) (string, error) {
	keyPath := filepath.Join(eg.tmpDir, "enc.key")
	keyInfoPath := filepath.Join(eg.tmpDir, fmt.Sprintf("enc-%s.keyinfo", roomID))

	// Generate deterministic AES key
	key := cryptoutil.GenerateAESKey(roomID, nonce)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return "", fmt.Errorf("failed to write key file: %w", err)
	}

	// Generate random IV
	iv, err := cryptoutil.GenerateIV()
	if err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// Construct key URI
	keyURI := "enc.key"
	if eg.keyBaseURL != "" {
		keyURI = fmt.Sprintf("%s%s/enc.key", eg.keyBaseURL, roomID)
	}

	// Create keyinfo file for FFmpeg
	// Format:
	// Line 1: Key URI (for m3u8 playlist)
	// Line 2: Path to key file
	// Line 3: IV in hex
	keyInfoContent := fmt.Sprintf("%s\n%s\n%s\n", keyURI, keyPath, hex.EncodeToString(iv))

	if err := os.WriteFile(keyInfoPath, []byte(keyInfoContent), 0600); err != nil {
		return "", fmt.Errorf("failed to write keyinfo file: %w", err)
	}

	return keyInfoPath, nil
}

// Delete removes the keyinfo file for the given room
func (eg *EncryptionGenerator) Delete(roomID string) error {
	keyInfoPath := filepath.Join(eg.tmpDir, fmt.Sprintf("enc-%s.keyinfo", roomID))
	err := os.Remove(keyInfoPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete keyinfo file: %w", err)
	}
	return nil
}
