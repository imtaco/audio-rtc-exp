package status

import (
	"go.opentelemetry.io/otel/metric"

	intotel "github.com/imtaco/audio-rtc-exp/internal/otel"
)

var (
	// RPC call metrics
	rpcCallsStarted metric.Int64Counter
	rpcCallsSuccess metric.Int64Counter
	rpcCallsFailed  metric.Int64Counter

	// JWT metrics
	tokensGenerated metric.Int64Counter
	tokensFailed    metric.Int64Counter

	// User operation metrics
	userCreatesRequested metric.Int64Counter
	userDeletesRequested metric.Int64Counter
	userStatusRequested  metric.Int64Counter
)

func init() {
	f := intotel.NewFactory("user.status", intotel.PrefixUserService)

	// RPC calls
	f.Int64Counter(&rpcCallsStarted, "rpc.calls.started",
		metric.WithDescription("Total RPC calls initiated to user controller"))

	f.Int64Counter(&rpcCallsSuccess, "rpc.calls.success",
		metric.WithDescription("Successful RPC calls to user controller"))

	f.Int64Counter(&rpcCallsFailed, "rpc.calls.failed",
		metric.WithDescription("Failed RPC calls to user controller"))

	// JWT tokens
	f.Int64Counter(&tokensGenerated, "tokens.generated",
		metric.WithDescription("Total JWT tokens generated"))

	f.Int64Counter(&tokensFailed, "tokens.failed",
		metric.WithDescription("Failed JWT token generation attempts"))

	// User operations
	f.Int64Counter(&userCreatesRequested, "user.creates.requested",
		metric.WithDescription("Total user creation requests"))

	f.Int64Counter(&userDeletesRequested, "user.deletes.requested",
		metric.WithDescription("Total user deletion requests"))

	f.Int64Counter(&userStatusRequested, "user.status.requested",
		metric.WithDescription("Total user status update requests"))
}
