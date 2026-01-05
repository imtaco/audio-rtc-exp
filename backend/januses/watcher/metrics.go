package watcher

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Room metrics
	roomsActive    metric.Int64UpDownCounter
	roomsProcessed metric.Int64Counter
	roomsCreated   metric.Int64Counter
	roomsDeleted   metric.Int64Counter
	roomsFailed    metric.Int64Counter

	// Janus health metrics
	janusRestarts   metric.Int64Counter
	janusHealthy    metric.Int64UpDownCounter
	monitorFailures metric.Int64Counter

	// Heartbeat metrics
	heartbeatUpdates  metric.Int64Counter
	heartbeatFailures metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("janus.watcher", intotel.PrefixJanuses)

	f.Int64UpDownCounter(&roomsActive, "rooms.active",
		metric.WithDescription("Number of active rooms managed by this Janus"))

	f.Int64Counter(&roomsProcessed, "rooms.processed",
		metric.WithDescription("Total number of room events processed"))

	f.Int64Counter(&roomsCreated, "rooms.created",
		metric.WithDescription("Total number of rooms created"))

	f.Int64Counter(&roomsDeleted, "rooms.deleted",
		metric.WithDescription("Total number of rooms deleted"))

	f.Int64Counter(&roomsFailed, "rooms.failed",
		metric.WithDescription("Total number of room operations that failed"))

	f.Int64Counter(&janusRestarts, "restarts",
		metric.WithDescription("Number of Janus server restarts detected"))

	f.Int64UpDownCounter(&janusHealthy, "health.status",
		metric.WithDescription("Janus server health status (1=healthy, 0=unhealthy)"))

	f.Int64Counter(&monitorFailures, "monitor.failures",
		metric.WithDescription("Number of health monitor check failures"))

	f.Int64Counter(&heartbeatUpdates, "heartbeat.updates",
		metric.WithDescription("Total number of heartbeat updates sent to etcd"))

	f.Int64Counter(&heartbeatFailures, "heartbeat.failures",
		metric.WithDescription("Number of heartbeat update failures"))
}
