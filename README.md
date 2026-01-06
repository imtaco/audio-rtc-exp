# RTC Experiment

[![Go](https://github.com/imtaco/audio-rtc-exp/actions/workflows/go.yml/badge.svg)](https://github.com/imtaco/audio-rtc-exp/actions/workflows/go.yml)

A scalable multi-room audio mixing real-time communication system based on WebRTC, Janus Gateway, and FFmpeg.

## Overview

This project implements a distributed microservices architecture for real-time audio communication, supporting multiple users joining audio rooms with automatic audio mixing and HLS streaming to viewers.

**Core Objective**: Support multiple users joining audio rooms, real-time mixing of all participants' audio, and pushing the mixed audio stream to all participants.

## Technology Stack

- **Backend**: Go 1.24, Gin, Gorilla WebSocket
- **State Management**: etcd v3.5.11, Redis 8.4
- **Media Processing**: Janus Gateway (AudioBridge), FFmpeg
- **Frontend**: Svelte 4.2.0, Vite 5.2.0, hls.js
- **Infrastructure**: Docker, Docker Compose, Nginx

## Quick Start

```bash
# Start all services
docker-compose up -d

# Access the application
# Anchor (broadcaster): http://localhost:8080/anchor
# Viewer (HLS): http://localhost:8080/audience-hls
```

## Documentation

### Architecture & Design
- [System Architecture](docs/architecture.md) - Overall architecture diagrams, data flow, and service dependencies
- [Data Structures](docs/data-structures.md) - etcd and Redis data structures, state machines
- [Watcher Pattern](docs/watcher-pattern.md) - Core reactive pattern for state synchronization

### Implementation
- [Business Flows](docs/business-flows.md) - Key business flows and fault tolerance mechanisms
- [Components](docs/components.md) - Service components, security, frontend architecture
- [Development](docs/development.md) - Tech stack, best practices, and optimization roadmap

## Key Features

### Reactive State Synchronization
- **Watcher Pattern**: Automatic state synchronization between etcd and services
- **Event-Driven**: Services react to state changes without direct communication
- **Fault Tolerant**: Automatic recovery from network failures and service restarts

### Intelligent Task Scheduling
- **Deduplication**: Multiple events for same resource are automatically deduplicated
- **Exponential Backoff**: Failed tasks retry with increasing delays (100ms → 10s max)
- **Priority Queue**: Efficient task scheduling using min-heap

### Resource Management
- **Health Monitoring**: Continuous health checks via etcd heartbeat (TTL: 10s)
- **Dynamic Allocation**: Automatic selection of healthy Mixer/Janus instances
- **Graceful Degradation**: Cordon/drain mechanism for safe service shutdown

## Architecture Highlights

### Microservices
- **Room Manager**: Room CRUD, lifecycle management, housekeeping
- **Janus Manager**: Janus room management, RTP forwarding
- **Mixer**: FFmpeg process management, HLS stream generation
- **WSGateway**: WebRTC signaling, Janus proxy
- **User Service**: JWT authentication, user state management

### Data Flow
```
Anchor → WSGateway → Janus Gateway → Mixer (FFmpeg) → Nginx → HLS Viewer
                 ↓                       ↓
               etcd (state)          Redis (locks)
```

## Project Structure

```
rtc/
├── backend/             # Backend services (Go)
│   ├── rooms/          # Room management service
│   ├── januses/        # Janus manager service
│   ├── mixeres/        # Mixer service (FFmpeg)
│   ├── wsgateway/      # WebSocket gateway
│   ├── users/          # User service
│   ├── internal/       # Internal shared code
│   │   ├── watcher/    # Generic watcher pattern implementation
│   │   ├── reswatcher/ # Watcher pattern implementation of rooms and modules
│   │   ├── scheduler/  # Task scheduler with dedup & retry
│   │   └── jsonrpc/    # JSON-RPC framework
├── frontend/           # Frontend applications (Svelte)
│   ├── anchor/         # Broadcaster UI
│   ├── audience-hls/   # HLS viewer UI
│   └── shared/         # Shared frontend code
├── docs/               # Documentation
└── docker-compose.yml  # Service orchestration for development
```

## Environment Variables

### Log Configuration

The logging system supports flexible configuration through environment variables:

#### Log Level

- `LOG_LEVEL` - Sets the default log level for all modules (default: `info`)
  - Valid values: `debug`, `info`, `warn`, `error`, `fatal`
  - Example: `LOG_LEVEL=debug`

#### Module-Specific Log Levels

You can set different log levels for specific modules using format:
- `LOG_LEVEL__{MODULE_NAME}` - one level format
- `LOG_LEVEL__{MODULE_NAME}__{MODULE_NAME}` - two level format

Module names are automatically converted to SCREAMING_SNAKE_CASE. For example, if you create a logger module named `RoomSvc`, you can set its log level using:

```bash
LOG_LEVEL__ROOM_SVC=debug
```

**Priority Order** (highest to lowest):
1. `LOG_LEVEL__{MODULE_LV1}__{MODULE_LV2}` - module specific
1. `LOG_LEVEL__{MODULE_LV1}` - upper level module
2. `LOG_LEVEL` - Global default
3. `info` - Fallback default

**Example Usage:**

```bash
# Set global level to warn, but enable debug for specific modules
export LOG_LEVEL=warn
export LOG_LEVEL_ROOM_SVC=debug
export LOG_LEVEL_RESOURCE_MGR=debug
```

#### Log Config File

- `APP_LOG_CONFIG_FILE` - Path to a JSON configuration file for advanced Zap logger configuration
  - When empty (default), uses the default console logger with color output
  - When set, loads a custom Zap configuration from the specified file
  - Example: `APP_LOG_CONFIG_FILE=/etc/rtc/log-config.json`

### Using the Logger (backend/internal/log)

#### Basic Usage

```go
import "github.com/imtaco/audio-rtc-exp/internal/log"

// Initialize the logger (usually in main.go)
logger, err := log.NewLogger(os.Getenv("APP_LOG_CONFIG_FILE"))
if err != nil {
    log.Fatal("Failed to initialize logger:", err)
}

// Use the logger
logger.Info("Application started")
logger.Debug("Debug message")
logger.Warn("Warning message")
logger.Error("Error message")
```

#### Module-Based Logging with Multiple Levels

The logger supports hierarchical module names using the `Module()` method. Each module can have its own log level configured independently:

```go
// Create module-specific loggers
rsLog := logger.Module("RoomSvc")
rmLog := logger.Module("ResourceMgr")

// Create nested module loggers (multiple levels)
jwLog := rmLog.Module("JanusWorker")
mwLog := rmLog.Module("MixerWorker")

// Use module loggers
rsLog.Info("Room created", log.String("roomId", "123"))
rmLog.Debug("Checking resource availability")
jwLog.Debug("Processing Janus instance", log.String("id", "janus-1"))
mxLog.Debug("Processing mixer instance", log.String("id", "mixer-1"))
```

#### Configuring Module-Specific Log Levels

**Important**: The global `LOG_LEVEL` only affects the default logger. Module-specific loggers use their own environment variables:

```bash
# Global level (only affects default logger, NOT modules)
export LOG_LEVEL=warn

# Module-specific levels (single level)
export LOG_LEVEL__ROOM_SVC=debug
export LOG_LEVEL__RESOURCE_MGR=info

# Multi-level module configuration
export LOG_LEVEL__RESOURCE_MGR__JANUS_WORKER=debug
export LOG_LEVEL__RESOURCE_MGR__MIXER_WORKER=warn
```

**How It Works:**

1. Module names are automatically converted to SCREAMING_SNAKE_CASE
   - `RoomSvc` → `LOG_LEVEL__ROOM_SVC`
   - `ResourceMgr.JanusWorker` → `LOG_LEVEL__RESOURCE_MGR__JANUS_WORKER`

2. Priority order for multi-level modules (highest to lowest):
   - `LOG_LEVEL__RESOURCE_MGR__JANUS_WORKER` (most specific)
   - `LOG_LEVEL__RESOURCE_MGR` (parent module)
   - `LOG_LEVEL` (global default)
   - `info` (fallback)

3. Each module logger independently checks its environment variables

**Example Scenario:**

```bash
# Set global level to warn (affects default logger only)
export LOG_LEVEL=warn

# Enable debug for resource manager
export LOG_LEVEL__RESOURCE_MGR=debug

# Disable debug for mixer worker specifically
export LOG_LEVEL__RESOURCE_MGR__MIXER_WORKER=error
```

```go
logger.Debug("This won't show (LOG_LEVEL=warn)")
logger.Warn("This will show")

resourceMgr := logger.Module("ResourceMgr")
resourceMgr.Debug("This will show (LOG_LEVEL__RESOURCE_MGR=debug)")

mixerWorker := resourceMgr.Module("MixerWorker")
mixerWorker.Debug("This won't show (LOG_LEVEL__RESOURCE_MGR__MIXER_WORKER=error)")
mixerWorker.Error("This will show")

janusWorker := resourceMgr.Module("JanusWorker")
janusWorker.Debug("This will show (inherits LOG_LEVEL__RESOURCE_MGR=debug)")
```

**Best Practices:**

- Use descriptive module names that reflect your component hierarchy
- Set global `LOG_LEVEL` to `warn` or `error` in production
- Enable `debug` only for specific modules you're troubleshooting
- Use multi-level modules to organize related components (e.g., `ResourceMgr.JanusWorker`, `ResourceMgr.MixerWorker`)

### Application Configuration

The application uses [Viper](https://github.com/spf13/viper) for configuration management with automatic environment variable binding.

#### Configuration Rules

1. **Environment Variable Format**: Configuration keys use dot notation (e.g., `app.shutdown_timeout`) and are automatically mapped to environment variables with underscores
   - Example: `app.shutdown_timeout` → `APP_SHUTDOWN_TIMEOUT`

2. **No Prefix Required**: The global environment prefix is empty (`SetEnvPrefix("")`), so use the configuration key directly
   - Example: `ETCD_ENDPOINTS=localhost:2379` (not `RTC_ETCD_ENDPOINTS`)

3. **Automatic Binding**: All configuration values can be overridden via environment variables using `AutomaticEnv()`

#### Common Configuration Variables

**Application Settings:**
- `APP_LOG_CONFIG_FILE` - Path to log configuration file (default: empty, uses default config)
- `APP_SHUTDOWN_TIMEOUT` - Graceful shutdown timeout (default: `10s`)

**HTTP Server:**
- `HTTP_ADDR` - HTTP server listen address (varies by service)
  - Room service: `0.0.0.0:3000`
  - Other services: see service-specific defaults

**etcd:**
- `ETCD_ENDPOINTS` - Comma-separated list of etcd endpoints (default: `localhost:2379`)
- `ETCD_DIAL_TIMEOUT` - Connection timeout (default: `5s`)
- `ETCD_USERNAME` - etcd username (default: empty)
- `ETCD_PASSWORD` - etcd password (default: empty)

**Service-Specific:**
- `HLS_ADV_URL` - Advertised HLS URL for room service (default: `http://localhost:8080/hls/`)
- `ETCD_PREFIX_ROOM_STORE` - etcd key prefix for room data (default: `/rooms/`)
- `ETCD_PREFIX_JANUS_STORE` - etcd key prefix for Janus data (default: `/januses/`)
- `ETCD_PREFIX_MIXER_STORE` - etcd key prefix for mixer data (default: `/mixers/`)

## Observability (Optional)

This project includes optional OpenTelemetry support for distributed tracing and metrics. By default, observability is **disabled** and the application runs without any external dependencies.

### Features

- **Distributed Tracing**: Track requests across microservices (supports Jaeger, Tempo, etc.)
- **Metrics**: Business and operational metrics (supports Prometheus, Grafana Cloud, etc.)
- **Independent Controls**: Enable tracing and metrics separately
- **Vendor Neutral**: Uses OpenTelemetry standard (OTLP), works with any compatible backend

### Quick Start

Observability is disabled by default. To enable:

```yaml
# config.yaml
otel:
  tracing_enabled: true
  metrics_enabled: true
  service_name: "mixer-service"
  endpoint: "localhost:4317"  # OpenTelemetry Collector
  insecure: true
```

## Contributing

See [Development Guide](docs/development.md) for contribution guidelines and best practices.
