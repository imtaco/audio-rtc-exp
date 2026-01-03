# Business Flows

## 1. Room Creation Flow

1. **Create Room** (via Room Manager REST API)
   ```
   POST /api/rooms
   {roomId, pin}
   ```

2. **Room Service Processing**
   - Check if room exists
   - Write to etcd `/rooms/{roomId}/meta`
   - Return room information (including HLS URL)

3. **Start Live Stream**
   ```
   POST /api/rooms/{roomId}/start
   ```

4. **Resource Allocation** (ResourceManager) ([rooms/service/resource_manager.go](../backend/rooms/service/resource_manager.go))

   **Pick** - Select an available Mixer/Janus instance:
   - Get all healthy Mixer/Janus instances from Etcd Watcher
   - Filter pickable candidates based on:
     - Module state must be `IsPickable()`:
       - Heartbeat status = `healthy`
       - Mark label = `ready` (or empty, defaults to ready)
     - Capacity check:
       - Heartbeat must specify `capacity > 0`
       - Current stream count < capacity
       - Stream count tracked by RoomWatcher from `/rooms/*/livemeta`
       - **Note**: Due to eventual consistency, stream count may not be precise in real-time
   - Randomly select one from pickable candidates
   - Return empty string if no available Mixer/Janus

   **Other operations:**
   - Generate random nonce (for encryption)
   - Write to etcd `/rooms/{roomId}/livemeta`

## 2. Janus Manager Response Flow

**RoomWatcher detects livemeta change** ([januses/watcher/room_watcher.go](../backend/januses/watcher/room_watcher.go)):

1. **Detect Assignment** - `livemeta.janusId == own service ID && status == "onair"`

2. **Create Janus Room**
   - Generate random 6-digit janusRoomId
   - Call Janus Admin API to create AudioBridge room
   - Write to etcd `/rooms/{roomId}/janus` with status "room_created"

3. **Wait for Mixer Port**
   - Monitor `/rooms/{roomId}/mixer` appearance

4. **Create RTP Forwarder**
   - Call Janus Admin API to create RTP forwarder
   - Forward to mixer.IP:mixer.Port
   - Update status to "forwarding"

## 3. Mixer Response Flow

**RoomWatcher detects livemeta change** ([mixers/watcher/watcher.go](../backend/mixers/watcher/watcher.go)):

1. **Detect Assignment** - `livemeta.mixerId == own service ID && status == "onair"`

2. **Allocate RTP Port**
   - Get free port from port pool (PortManager)

3. **Start FFmpeg**
   - Generate encryption key URL
   - Generate SDP file (listening on RTP port)
   - Start FFmpeg process for transcoding and HLS segmentation

4. **Write Mixer Data**
   - Update etcd `/rooms/{roomId}/mixer` with allocated port

## 4. Anchor Connection Flow

**WebSocket Connection** ([wsgateway/signal/signal_server.go](../backend/wsgateway/signal/signal_server.go)):

1. **Establish Connection**
   ```
   ws://localhost:8081/ws?token={JWT}
   ```

2. **JWT Verification** ([wsgateway/signal/ws_hook.go](../backend/wsgateway/signal/ws_hook.go))
   - Extract and verify JWT token
   - Get userID and roomID

3. **Connection Lock Acquisition** (ConnLock)
   - Redis distributed lock to prevent duplicate user connections

4. **Join Room** (JSON-RPC)
   ```json
   {"method": "join", "params": {}}
   ```

5. **JanusProxy Processing**
   - Query etcd for room's Janus instance
   - Create WebSocket connection to Janus
   - Create Janus Session and Handle
   - Join AudioBridge room

6. **WebRTC Signaling Exchange**
   - Offer/Answer SDP exchange
   - ICE Candidate exchange
   - Establish WebRTC connection

## 5. Room Deletion Flow

1. **Mark for Deletion**
   - Update `/rooms/{roomId}/livemeta` status = "removing"

2. **Janus Manager Response**
   - Stop RTP Forwarder
   - Destroy Janus room
   - Delete `/rooms/{roomId}/janus`

3. **Mixer Response**
   - Stop FFmpeg process
   - Release RTP port
   - Delete `/rooms/{roomId}/mixer`

4. **Room Manager Cleanup**
   - Delete `/rooms/{roomId}/meta`
   - Delete `/rooms/{roomId}/livemeta`

## Fault Tolerance and Reliability

### Heartbeat Mechanism

All module services (Janus Manager, Mixer) periodically send heartbeats to etcd ([pkg/heartbeat/etcd/heartbeat.go](../backend/pkg/heartbeat/etcd/heartbeat.go)):
- Uses etcd Lease mechanism (TTL: 10s)
- Heartbeat contains: status, host, startedAt
- etcd automatically clears data when heartbeat fails

### Watcher Rebuild Mechanism

When service starts or reconnects to etcd:
1. **RebuildStart** - Clear memory state
2. **Get actual running state from Janus/FFmpeg**
3. **RebuildState** - Compare with etcd state, clean up inconsistent resources
4. **RebuildEnd** - Complete rebuild
5. Start normal change listening

### Housekeeping

Room Manager periodically executes cleanup tasks (30s interval) ([rooms/service/housekeeping.go](../backend/rooms/service/housekeeping.go)):

1. **CheckStaleRooms** - Clean up timed-out rooms in removing status
2. **CheckRoomModules** - Check if room's Janus/Mixer are healthy, reassign if unhealthy

### Task Scheduler

Uses task scheduler with exponential backoff for failure retry ([pkg/scheduler/scheduler.go](../backend/pkg/scheduler/scheduler.go)):
- Supports organizing tasks by key
- Automatic retry on failure with exponential backoff
- Avoids avalanche effect
