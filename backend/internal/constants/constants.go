package constants

type RoomStatus string
type MarkLabel string
type AnchorStatus string
type UserRole string

const (
	// Room status
	RoomStatusOnAir    RoomStatus = "onair"
	RoomStatusRemoving RoomStatus = "removing"
)

const (
	RoomKeyMeta     = "meta"
	RoomKeyLiveMeta = "livemeta"
	RoomKeyJanus    = "janus"
	RoomKeyMixer    = "mixer"
)

const (
	ModuleKeyHeartbeat = "heartbeat"
	ModuleKeyMark      = "mark"
)

const (
	ModuleStatusHealthy = "healthy"
)

const (
	MarkLabelUnready  MarkLabel = "unready"
	MarkLabelReady    MarkLabel = "ready"
	MarkLabelCordon   MarkLabel = "cordon"
	MarkLabelDraining MarkLabel = "draining"
	MarkLabelDrained  MarkLabel = "drained"
)

const (
	// can stop/close a room, invite anchors, start live streaming
	UserRoleHost UserRole = "host"
	// can join as anchor to send/receive live streams
	UserRoleAnchor UserRole = "anchor"
	// can join as viewer to only receive live streams
	UserRoleGuest UserRole = "guest"
)

const (
	AnchorStatusOnAir AnchorStatus = "onair"
	AnchorStatusIdle  AnchorStatus = "idle"
	AnchorStatusLeft  AnchorStatus = "left"
)
