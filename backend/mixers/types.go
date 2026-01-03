package mixers

import "time"

type FFmpegManager interface {
	StartFFmpeg(roomID string, rtpPort int, createdAt time.Time, nonce string) error
	StopFFmpeg(roomID string) error
	Stop() error
}

type PortManager interface {
	GetFreeRTPPort() (int, error)
}
