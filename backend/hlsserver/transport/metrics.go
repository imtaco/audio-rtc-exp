package transport

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Token metrics
	tokensGenerated metric.Int64Counter
	tokensFailed    metric.Int64Counter

	// Key metrics
	keysServed  metric.Int64Counter
	cacheHits   metric.Int64Counter
	cacheMisses metric.Int64Counter
	activeRooms metric.Int64UpDownCounter

	// Error metrics
	authFailures metric.Int64Counter
	roomNotFound metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("hls.server", intotel.PrefixHLSServer)

	f.Int64Counter(&tokensGenerated, "tokens.generated",
		metric.WithDescription("Total JWT tokens generated"))

	f.Int64Counter(&tokensFailed, "tokens.failed",
		metric.WithDescription("Failed token generation attempts"))

	f.Int64Counter(&keysServed, "keys.served",
		metric.WithDescription("Total encryption keys served"))

	f.Int64Counter(&cacheHits, "keys.cache_hits",
		metric.WithDescription("Encryption key cache hits"))

	f.Int64Counter(&cacheMisses, "keys.cache_misses",
		metric.WithDescription("Encryption key cache misses"))

	f.Int64UpDownCounter(&activeRooms, "rooms.active",
		metric.WithDescription("Number of active rooms"))

	f.Int64Counter(&authFailures, "auth.failures",
		metric.WithDescription("Authorization failures"))

	f.Int64Counter(&roomNotFound, "room.not_found",
		metric.WithDescription("Requests for non-existent rooms"))
}
