package wsgateway

import (
	"context"

	"github.com/imtaco/audio-rtc-exp/internal/etcdstate"
	"github.com/imtaco/audio-rtc-exp/internal/janus"
)

// JanusProxy provides methods to interact with Janus instances based on roomID.
// It allows retrieval of Janus session and room IDs, as well as the Janus API instance for a given room.
type JanusProxy interface {
	Open(ctx context.Context) error
	Close() error
	GetJanusRoomID(roomID string) int64
	GetJanusAPI(roomID string) janus.API
	GetRoomMeta(roomId string) *etcdstate.Meta
	GetRoomLiveMeta(roomId string) *etcdstate.LiveMeta
}

// JanusTokenCodec provides methods to encode/decode Janus tokens.
// anchors can use this to resume Janus sessions when websocket connections are re-established.
type JanusTokenCodec interface {
	Encode(roomKey string, sessionID, handleID int64) (string, error)
	Decode(roomKey string, token string) (int64, int64, error)
}
