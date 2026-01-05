package janusproxy

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Janus instance cache metrics
	janusInstCacheHits   metric.Int64Counter
	janusInstCacheMisses metric.Int64Counter
	janusInstCacheSize   metric.Int64UpDownCounter

	// Janus proxy metrics
	janusProxyRequests metric.Int64Counter
	janusProxyFailures metric.Int64Counter

	// Room lookup metrics
	roomLookupsTotal  metric.Int64Counter
	roomLookupsFailed metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("wsgateway.janusproxy", intotel.PrefixWSGateway)

	f.Int64Counter(&janusInstCacheHits, "janus_cache.hits",
		metric.WithDescription("Janus instance cache hits"))

	f.Int64Counter(&janusInstCacheMisses, "janus_cache.misses",
		metric.WithDescription("Janus instance cache misses"))

	f.Int64UpDownCounter(&janusInstCacheSize, "janus_cache.size",
		metric.WithDescription("Current Janus instance cache size"))

	f.Int64Counter(&janusProxyRequests, "proxy.requests",
		metric.WithDescription("Total requests proxied to Janus"))

	f.Int64Counter(&janusProxyFailures, "proxy.failures",
		metric.WithDescription("Total failed proxy requests"))

	f.Int64Counter(&roomLookupsTotal, "room_lookups.total",
		metric.WithDescription("Total room lookups from etcd"))

	f.Int64Counter(&roomLookupsFailed, "room_lookups.failed",
		metric.WithDescription("Total failed room lookups"))
}
