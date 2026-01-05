package watcher

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Package-level metrics
	activeRoomsGauge metric.Int64UpDownCounter
	roomsProcessed   metric.Int64Counter
	roomsStarted     metric.Int64Counter
	roomsStopped     metric.Int64Counter
	roomsFailed      metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("mixer.watcher", intotel.PrefixMixers)

	f.Int64UpDownCounter(&activeRoomsGauge, "rooms.active",
		metric.WithDescription("Number of active rooms being processed"))

	f.Int64Counter(&roomsProcessed, "rooms.processed",
		metric.WithDescription("Total number of room changes processed"))

	f.Int64Counter(&roomsStarted, "rooms.started",
		metric.WithDescription("Total number of rooms started"))

	f.Int64Counter(&roomsStopped, "rooms.stopped",
		metric.WithDescription("Total number of rooms stopped"))

	f.Int64Counter(&roomsFailed, "rooms.failed",
		metric.WithDescription("Total number of rooms that failed to start"))
}
