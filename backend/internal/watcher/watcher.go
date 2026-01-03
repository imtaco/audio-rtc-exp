package watcher

import "context"

// Watcher defines the interface for watching and caching state changes.
// Implementations typically monitor external data sources (like etcd) and maintain
// an in-memory cache of the current state (structure might vary based on use case).
type Watcher[T any] interface {
	// Initialize performs initial data fetching.
	// It blocks until the initial state is loaded or the context is canceled.
	Start(ctx context.Context) error

	// Stop stops the watcher and releases all resources.
	Stop() error

	// Restart forces the watcher to restart, re-fetching initial data, rebuild and resuming watching.
	Restart()

	// GetCachedState retrieves the cached state for a given ID.
	// Returns the state and a boolean indicating whether the ID exists in the cache.
	GetCachedState(id string) (*T, bool)
}

// ProcessChangeFunc is a callback function invoked when a state change is detected.
// It receives the ID of the changed entity and its new state (nil if deleted).
// Returning an error will trigger retry logic with exponential backoff.
type ProcessChangeFunc[T any] func(ctx context.Context, id string, state *T) error

// StateTransformer handles state transformation and rebuilding during watcher initialization.
// It defines how raw data is parsed into typed state objects and how the state is rebuilt
// when the watcher reconnects or restarts.
type StateTransformer[T any] interface {
	// RebuildStart is called before rebuilding the state cache.
	// Use this to reset or prepare any internal state.
	RebuildStart(ctx context.Context) error

	// RebuildState is called for each cached entry during state rebuild.
	// This is invoked after initial data fetch but before change processing begins.
	RebuildState(ctx context.Context, id string, etcdData *T) error

	// RebuildEnd is called after all cached entries have been rebuilt.
	// Use this to finalize or validate the rebuilt state.
	RebuildEnd(ctx context.Context) error

	// NewState creates or updates the state for a given ID based on incoming data.
	// The keyType parameter indicates which part of the state is being updated.
	// Returns nil state to indicate the entry should be removed from the cache.
	// NewState should not involve any side effects like I/O or state mutation outside of the returned state.
	NewState(id, keyType string, data []byte, currentState *T) (*T, error)
}
