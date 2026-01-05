package signal

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// WebSocket connection metrics
	wsConnectionsActive metric.Int64UpDownCounter
	wsConnectionsTotal  metric.Int64Counter
	wsDisconnectsTotal  metric.Int64Counter

	// Message metrics
	messagesReceived metric.Int64Counter
	messagesSent     metric.Int64Counter
	messagesFailed   metric.Int64Counter

	// RPC metrics
	rpcRequestsTotal  metric.Int64Counter
	rpcRequestsFailed metric.Int64Counter

	// Auth metrics
	authAttempts metric.Int64Counter
	authFailures metric.Int64Counter

	// Notification metrics
	notificationsSent   metric.Int64Counter
	notificationsFailed metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("wsgateway.signal", intotel.PrefixWSGateway)

	f.Int64UpDownCounter(&wsConnectionsActive, "connections.active",
		metric.WithDescription("Number of active WebSocket connections"))

	f.Int64Counter(&wsConnectionsTotal, "connections.total",
		metric.WithDescription("Total WebSocket connections established"))

	f.Int64Counter(&wsDisconnectsTotal, "disconnects.total",
		metric.WithDescription("Total WebSocket disconnections"))

	f.Int64Counter(&messagesReceived, "messages.received",
		metric.WithDescription("Total messages received from clients"))

	f.Int64Counter(&messagesSent, "messages.sent",
		metric.WithDescription("Total messages sent to clients"))

	f.Int64Counter(&messagesFailed, "messages.failed",
		metric.WithDescription("Total failed message deliveries"))

	f.Int64Counter(&rpcRequestsTotal, "rpc.requests.total",
		metric.WithDescription("Total RPC requests processed"))

	f.Int64Counter(&rpcRequestsFailed, "rpc.requests.failed",
		metric.WithDescription("Total failed RPC requests"))

	f.Int64Counter(&authAttempts, "auth.attempts",
		metric.WithDescription("Total authentication attempts"))

	f.Int64Counter(&authFailures, "auth.failures",
		metric.WithDescription("Total authentication failures"))

	f.Int64Counter(&notificationsSent, "notifications.sent",
		metric.WithDescription("Total notifications sent to clients"))

	f.Int64Counter(&notificationsFailed, "notifications.failed",
		metric.WithDescription("Total failed notification deliveries"))
}
