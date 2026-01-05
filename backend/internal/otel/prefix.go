package otel

// Metric prefixes for each service
// Each service should define its own metric names and use these prefixes
const (
	PrefixMixers      = "mixer"
	PrefixJanuses     = "janus"
	PrefixRoomMixers  = "room_mixer"
	PrefixWSGateway   = "wsgateway"
	PrefixUserService = "user_service"
	PrefixHLSServer   = "hls_server"
)
