# Development Guide

## Technology Stack

### Backend Stack
- **Language**: Go 1.24
- **Web Framework**: Gin (HTTP REST API)
- **WebSocket**: Gorilla WebSocket
- **Distributed Coordination**: etcd v3.5.11 (state storage and service discovery)
- **Cache/Message Queue**: Redis 8.4 (user state, message streams)
- **WebRTC Media Server**: Janus Gateway (AudioBridge plugin)
- **Audio Transcoding**: FFmpeg (HLS stream generation)
- **Configuration Management**: Viper
- **Logging**: Zap
- **JWT**: golang-jwt/jwt v5

### Frontend Stack
- **Framework**: Svelte 4.2.0
- **Build Tool**: Vite 5.2.0
- **HLS Player**: hls.js 1.6.15
- **RPC Communication**: rpc-websockets 9.3.2

### Infrastructure
- **Containerization**: Docker, Docker Compose
- **Reverse Proxy**: Nginx 1.28

## System Characteristics

### Strengths
1. **Microservices Architecture** - Components decoupled, easy to scale and maintain
2. **Reactive Design** - Uses Watcher pattern for event-driven architecture
3. **Strong Fault Tolerance** - Heartbeat, rebuild mechanism, task retry
4. **Type Safety** - Go generics, unified data models
5. **Test Coverage** - Extensive unit tests and mocks

### Architectural Highlights
1. Uses etcd as Single Source of Truth
2. Custom JSON-RPC framework supports multiple transport layers
3. Generic Watcher pattern simplifies state synchronization logic
4. Clear state machines and resource lifecycle management

## Use Cases
- Multi-person online audio conferencing
- Online radio/podcasting
- Collaborative music creation platforms
- Online education voice interaction

## TODO

### High Priority
1. **Janus Restart Handling** - Detect and handle Janus restart events
2. **Room Migration** - Support room migration when Janus nodes fail
3. ~~**Capacity Limits** - Implement capacity-based module selection algorithm~~
4. **Anchor Notification** - Notify anchors to reconnect during Janus restart or migration
5. **Better Error Hanlding** - Refine error codes and messages (retryable)

### Medium Priority
1. ~~**Room Size Limits** - Limit number of participants per room~~
2. **Periodic Participant Sync** - Use version numbers to sync participant state
3. ~~**Input Validation** - REST API input validation~~
4. **Error Messages** - Don't expose detailed error messages to end users
5. **Throttling** - Message rate limiting and blocking users reconnect too frequently
6. ~~**Observability** - Support OpenTelemetry~~

### Low Priority
1. **Recording Feature** - Implement recording and upload to cloud storage
2. **HLS Gateway** - Proxy HLS requests to corresponding Mixer
3. **Separate Signing Service** - Separate signature server
4. **Store IV in etcd** - Share the same IV to ease merging recordings
5. **Upload to external storage** - Keep uploading HLS to S3/GCS/NFS while streaming

