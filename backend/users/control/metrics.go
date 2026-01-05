package control

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// User lifecycle metrics
	usersCreated      metric.Int64Counter
	usersDeleted      metric.Int64Counter
	userStatusUpdated metric.Int64Counter
	userCreateFailed  metric.Int64Counter
	userDeleteFailed  metric.Int64Counter
	userStatusFailed  metric.Int64Counter
	activeUsers       metric.Int64UpDownCounter
	maxAnchorsReached metric.Int64Counter

	// RPC metrics
	rpcRequestsReceived    metric.Int64Counter
	rpcRequestsProcessed   metric.Int64Counter
	rpcRequestsFailed      metric.Int64Counter
	rpcNotificationsSent   metric.Int64Counter
	rpcNotificationsFailed metric.Int64Counter

	// User event processing metrics
	userEventsQueued    metric.Int64Counter
	userEventsProcessed metric.Int64Counter
	userEventsFailed    metric.Int64Counter
	userEventQueueDepth metric.Int64UpDownCounter

	// Timeout/expiration metrics
	timeoutChecksRun      metric.Int64Counter
	expiredUsersDetected  metric.Int64Counter
	roomsWithExpiredUsers metric.Int64Counter

	// State management metrics
	stateRebuildRuns     metric.Int64Counter
	stateRebuildFailed   metric.Int64Counter
	stateRebuildDuration metric.Float64Histogram

	// Watcher metrics
	watcherStarted metric.Int64Counter
	watcherStopped metric.Int64Counter
	watcherErrors  metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("user.control", intotel.PrefixUserService)

	// User lifecycle
	f.Int64Counter(&usersCreated, "users.created",
		metric.WithDescription("Total users created"))

	f.Int64Counter(&usersDeleted, "users.deleted",
		metric.WithDescription("Total users deleted"))

	f.Int64Counter(&userStatusUpdated, "users.status.updated",
		metric.WithDescription("Total user status updates"))

	f.Int64Counter(&userCreateFailed, "users.create.failed",
		metric.WithDescription("Failed user creation attempts"))

	f.Int64Counter(&userDeleteFailed, "users.delete.failed",
		metric.WithDescription("Failed user deletion attempts"))

	f.Int64Counter(&userStatusFailed, "users.status.failed",
		metric.WithDescription("Failed user status updates"))

	f.Int64UpDownCounter(&activeUsers, "users.active",
		metric.WithDescription("Number of currently active users"))

	f.Int64Counter(&maxAnchorsReached, "users.max_anchors_reached",
		metric.WithDescription("Times max anchors limit was reached"))

	// RPC
	f.Int64Counter(&rpcRequestsReceived, "rpc.requests.received",
		metric.WithDescription("Total RPC requests received"))

	f.Int64Counter(&rpcRequestsProcessed, "rpc.requests.processed",
		metric.WithDescription("Total RPC requests successfully processed"))

	f.Int64Counter(&rpcRequestsFailed, "rpc.requests.failed",
		metric.WithDescription("Total failed RPC requests"))

	f.Int64Counter(&rpcNotificationsSent, "rpc.notifications.sent",
		metric.WithDescription("Total RPC notifications sent to websocket gateway"))

	f.Int64Counter(&rpcNotificationsFailed, "rpc.notifications.failed",
		metric.WithDescription("Total failed RPC notifications"))

	// Event processing
	f.Int64Counter(&userEventsQueued, "events.queued",
		metric.WithDescription("Total user events queued for processing"))

	f.Int64Counter(&userEventsProcessed, "events.processed",
		metric.WithDescription("Total user events successfully processed"))

	f.Int64Counter(&userEventsFailed, "events.failed",
		metric.WithDescription("Total failed user event processing attempts"))

	f.Int64UpDownCounter(&userEventQueueDepth, "events.queue_depth",
		metric.WithDescription("Current depth of user event queue"))

	// Timeouts
	f.Int64Counter(&timeoutChecksRun, "timeout.checks.run",
		metric.WithDescription("Total timeout check cycles executed"))

	f.Int64Counter(&expiredUsersDetected, "timeout.users.expired",
		metric.WithDescription("Total expired users detected"))

	f.Int64Counter(&roomsWithExpiredUsers, "timeout.rooms.affected",
		metric.WithDescription("Total rooms with expired users"))

	// State management
	f.Int64Counter(&stateRebuildRuns, "state.rebuild.runs",
		metric.WithDescription("Total state rebuild operations"))

	f.Int64Counter(&stateRebuildFailed, "state.rebuild.failed",
		metric.WithDescription("Failed state rebuild operations"))

	f.Float64Histogram(&stateRebuildDuration, "state.rebuild.duration",
		metric.WithDescription("Duration of state rebuild operations in seconds"),
		metric.WithUnit("s"))

	// Watcher
	f.Int64Counter(&watcherStarted, "watcher.started",
		metric.WithDescription("Total watcher start operations"))

	f.Int64Counter(&watcherStopped, "watcher.stopped",
		metric.WithDescription("Total watcher stop operations"))

	f.Int64Counter(&watcherErrors, "watcher.errors",
		metric.WithDescription("Total watcher errors"))
}
