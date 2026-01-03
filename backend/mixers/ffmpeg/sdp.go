package ffmpeg

import (
	"fmt"
	"os"
	"path/filepath"
)

// SDPGenerator generates SDP files for FFmpeg
type SDPGenerator struct {
	sdpDir string
}

// NewSDPGenerator creates a new SDPGenerator
func NewSDPGenerator(sdpDir string) *SDPGenerator {
	if sdpDir == "" {
		sdpDir = "/tmp/sdp"
	}
	return &SDPGenerator{
		sdpDir: sdpDir,
	}
}

// Generate creates an SDP file for the given room and RTP port
func (sg *SDPGenerator) Generate(roomID string, rtpPort int) (string, error) {
	sdpContent := fmt.Sprintf(`v=0
o=- 0 0 IN IP4 127.0.0.1
s=Janus AudioBridge Stream - Room %s
c=IN IP4 0.0.0.0
t=0 0
m=audio %d RTP/AVP 100
a=rtpmap:100 opus/48000/2
`, roomID, rtpPort)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(sg.sdpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create SDP directory: %w", err)
	}

	sdpPath := filepath.Join(sg.sdpDir, fmt.Sprintf("%s.sdp", roomID))
	if err := os.WriteFile(sdpPath, []byte(sdpContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write SDP file: %w", err)
	}

	return sdpPath, nil
}

// Delete removes the SDP file for the given room
func (sg *SDPGenerator) Delete(roomID string) error {
	sdpPath := filepath.Join(sg.sdpDir, fmt.Sprintf("%s.sdp", roomID))
	err := os.Remove(sdpPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete SDP file: %w", err)
	}
	return nil
}
