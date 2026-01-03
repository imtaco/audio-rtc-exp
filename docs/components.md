# Core Components

## Service Components

| Service | Responsibilities | Key Files |
|---------|------------------|-----------|
| Room Manager | Room CRUD, lifecycle management, Housekeeping | [rooms/cmd/main.go](../backend/rooms/cmd/main.go) |
| Janus Manager | Janus room management, RTP forwarder, health monitoring | [januses/cmd/main.go](../backend/januses/cmd/main.go) |
| Mixer | FFmpeg process management, HLS stream generation, RTP port allocation | [mixers/cmd/main.go](../backend/mixers/cmd/main.go) |
| WSGateway | WebRTC signaling, Janus proxy, connection management | [wsgateway/cmd/main.go](../backend/wsgateway/cmd/main.go) |
| User Service | User state management, JWT authentication | [users/cmd/main.go](../backend/users/cmd/main.go) |
| HLS Server | HLS stream proxy and distribution | - |
| Frontend | Anchor and audience UI | [frontend/](../frontend/) |

## External Dependencies

| Service | Purpose |
|---------|---------|
| etcd | Distributed configuration storage, service state coordination, room metadata storage |
| Redis | User state cache, Redis Stream message passing, WebSocket connection management |
| Janus Gateway | WebRTC media server, AudioBridge audio mixing |
| Nginx | Static resource serving, HLS stream proxy, reverse proxy |

## Resource Management

### Module Selection Algorithm

**PickMixer() / PickJanus()** ([rooms/service/resource_manager.go](../backend/rooms/service/resource_manager.go)):

```go
func randomPickModule(watcher HealthyModuleWatcher) string {
    // 1. Get all healthy module IDs
    healthyIDs := watcher.GetAllHealthy()

    // 2. Filter out IsPickable() modules
    //    - heartbeat.status == "healthy"
    //    - mark.label == "ready" (or no mark)

    // 3. Randomly select one
    return pickableKeys[rand.Intn(len(pickableKeys))]
}
```

### RTP Port Management

**PortManager** ([mixers/watcher/port.go](../backend/mixers/watcher/port.go)):
- Maintains port pool (10000-20000)
- Allocates and reclaims RTP ports
- Uses bitmap to track port usage status

## Security

### JWT Authentication

([pkg/jwt/jwt.go](../backend/pkg/jwt/jwt.go))
- Uses HS256 algorithm
- Payload contains: userID, roomID
- Verified during WebSocket connection
- Supports Query Parameter or Authorization Header

### HLS Encryption

- FFmpeg generates AES-128 encrypted HLS streams
- Encryption keys obtained dynamically via HTTP
- Key URL contains room ID and nonce

### Room PIN Code

- Joining Janus room requires PIN code verification
- PIN stored in etcd `/rooms/{roomId}/meta`

## Frontend Architecture

### Anchor (Broadcaster)

([frontend/anchor/Anchor.svelte](../frontend/anchor/Anchor.svelte))

**AnchorClient.js**:
- Uses JSON-RPC WebSocket connection to wsgateway
- Wraps WebRTC signaling methods: join, offer, iceCandidate, leave
- Manages local media stream (getUserMedia)

### Audience (Viewer)

Provides two viewer modes:
1. **HLS Mode** ([frontend/audience-hls/](../frontend/audience-hls/)) - Uses hls.js to play HLS streams
2. **MediaSoup Mode(test only)** ([frontend/audience-mediasoup/](../frontend/audience-mediasoup/)) - Uses mediasoup-client for WebRTC receiving

### Shared Code

([frontend/shared/](../frontend/shared/))
- **ws-rpc.js** - WebSocket JSON-RPC client wrapper
- **constants.js** - Constants definition
- **logger.js** - Logging utility

## Key File Path Index

### Service Entry Points
- Rooms: [backend/rooms/cmd/main.go](../backend/rooms/cmd/main.go)
- Januses: [backend/januses/cmd/main.go](../backend/januses/cmd/main.go)
- mixers: [backend/mixers/cmd/main.go](../backend/mixers/cmd/main.go)
- WSGateway: [backend/wsgateway/cmd/main.go](../backend/wsgateway/cmd/main.go)
- Users: [backend/users/cmd/main.go](../backend/users/cmd/main.go)

### Core Packages
- JSON-RPC: [backend/pkg/jsonrpc/](../backend/pkg/jsonrpc/)
- Watcher: [backend/pkg/watcher/](../backend/pkg/watcher/)
- etcd State: [backend/internal/etcdstate/](../backend/internal/etcdstate/)
- Redis Stream: [backend/pkg/stream/redis/](../backend/pkg/stream/redis/)
- Scheduler: [backend/pkg/scheduler/](../backend/pkg/scheduler/)

### Frontend
- Anchor: [frontend/anchor/](../frontend/anchor/)
- Audience: [frontend/audience-hls/](../frontend/audience-hls/), [frontend/audience-mediasoup/](../frontend/audience-mediasoup/)
- RPC Client: [frontend/shared/ws-rpc.js](../frontend/shared/ws-rpc.js)
