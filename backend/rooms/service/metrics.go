package service

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// Resource allocation metrics
	janusPickAttempts metric.Int64Counter
	janusPickSuccess  metric.Int64Counter
	janusPickFailed   metric.Int64Counter
	mixerPickAttempts metric.Int64Counter
	mixerPickSuccess  metric.Int64Counter
	mixerPickFailed   metric.Int64Counter

	// Module availability metrics
	availableJanuses metric.Int64UpDownCounter
	availableMixers  metric.Int64UpDownCounter

	// Housekeeping metrics
	housekeepingRuns         metric.Int64Counter
	housekeepingDuration     metric.Float64Histogram
	staleRoomsChecked        metric.Int64Counter
	staleRoomsDeleted        metric.Int64Counter
	malformedRoomsDeleted    metric.Int64Counter
	inactiveRoomsDeleted     metric.Int64Counter
	expiredRoomsDeleted      metric.Int64Counter
	unhealthyMixersDetected  metric.Int64Counter
	unhealthyJanusesDetected metric.Int64Counter

	// Module watcher metrics
	watcherStarted metric.Int64Counter
	watcherStopped metric.Int64Counter
	watcherErrors  metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("room.service", intotel.PrefixRoomMixers)

	// Resource allocation
	f.Int64Counter(&janusPickAttempts, "janus.pick.attempts",
		metric.WithDescription("Total Janus server pick attempts"))

	f.Int64Counter(&janusPickSuccess, "janus.pick.success",
		metric.WithDescription("Successful Janus server picks"))

	f.Int64Counter(&janusPickFailed, "janus.pick.failed",
		metric.WithDescription("Failed Janus server picks (no available capacity)"))

	f.Int64Counter(&mixerPickAttempts, "mixer.pick.attempts",
		metric.WithDescription("Total mixer server pick attempts"))

	f.Int64Counter(&mixerPickSuccess, "mixer.pick.success",
		metric.WithDescription("Successful mixer server picks"))

	f.Int64Counter(&mixerPickFailed, "mixer.pick.failed",
		metric.WithDescription("Failed mixer server picks (no available capacity)"))

	// Module availability
	f.Int64UpDownCounter(&availableJanuses, "janus.available",
		metric.WithDescription("Number of available Janus servers"))

	f.Int64UpDownCounter(&availableMixers, "mixer.available",
		metric.WithDescription("Number of available mixer servers"))

	// Housekeeping
	f.Int64Counter(&housekeepingRuns, "housekeeping.runs",
		metric.WithDescription("Total housekeeping cycles executed"))

	f.Float64Histogram(&housekeepingDuration, "housekeeping.duration",
		metric.WithDescription("Duration of housekeeping cycles in seconds"),
		metric.WithUnit("s"))

	f.Int64Counter(&staleRoomsChecked, "housekeeping.stale_rooms.checked",
		metric.WithDescription("Total rooms checked for staleness"))

	f.Int64Counter(&staleRoomsDeleted, "housekeeping.stale_rooms.deleted",
		metric.WithDescription("Total stale rooms deleted"))

	f.Int64Counter(&malformedRoomsDeleted, "housekeeping.malformed_rooms.deleted",
		metric.WithDescription("Total malformed rooms deleted"))

	f.Int64Counter(&inactiveRoomsDeleted, "housekeeping.inactive_rooms.deleted",
		metric.WithDescription("Total inactive rooms deleted (failed to start)"))

	f.Int64Counter(&expiredRoomsDeleted, "housekeeping.expired_rooms.deleted",
		metric.WithDescription("Total expired rooms deleted (exceeded max age)"))

	f.Int64Counter(&unhealthyMixersDetected, "housekeeping.unhealthy_mixers.detected",
		metric.WithDescription("Total unhealthy mixers detected during checks"))

	f.Int64Counter(&unhealthyJanusesDetected, "housekeeping.unhealthy_januses.detected",
		metric.WithDescription("Total unhealthy Janus servers detected during checks"))

	// Watcher lifecycle
	f.Int64Counter(&watcherStarted, "watcher.started",
		metric.WithDescription("Total watcher start operations"))

	f.Int64Counter(&watcherStopped, "watcher.stopped",
		metric.WithDescription("Total watcher stop operations"))

	f.Int64Counter(&watcherErrors, "watcher.errors",
		metric.WithDescription("Total watcher errors"))
}
