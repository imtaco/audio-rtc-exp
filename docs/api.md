# API Documentation

This document describes the RESTful API endpoints for the RTC system.

## Table of Contents

- [Rooms API](#rooms-api)
- [Users API](#users-api)
- [HLS Server API](#hls-server-api)

---

## Rooms API

The Rooms API manages room creation, retrieval, deletion, and module marking operations.

**Base Path**: `/api`

### Endpoints

#### Create Room

Creates a new room with optional configuration.

- **URL**: `/api/rooms`
- **Method**: `POST`
- **Content-Type**: `application/json`

**Request Body**:

```json
{
  "roomId": "optional-room-id",
  "pin": "abc123",
  "maxAnchors": 3
}
```

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `roomId` | string | No | 3-32 chars, alphanumeric with hyphens/underscores | Custom room identifier. Auto-generated if not provided. |
| `pin` | string | No | Exactly 6 alphanumeric characters | Room PIN. Auto-generated if not provided. |
| `maxAnchors` | integer | No | Min: 1, Max: 5 | Maximum number of anchors. Defaults to 3. |

**Success Response** (201 Created):

```json
{
  "success": true,
  "room": {
    "roomId": "generated-room-id",
    "pin": "abc123",
    "maxAnchors": 3,
    "createdAt": "2026-01-07T12:00:00Z"
  }
}
```

**Error Responses**:

- **400 Bad Request**: Validation failed
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId must be 3-32 characters"]
  }
  ```

- **409 Conflict**: Room already exists
  ```json
  {
    "success": false,
    "error": "Room already exists"
  }
  ```

- **500 Internal Server Error**: Failed to create room
  ```json
  {
    "success": false,
    "error": "Failed to create room"
  }
  ```

**Implementation**: [router.go:80](../backend/rooms/transport/router.go#L80)

---

#### Get Room

Retrieves information about a specific room.

- **URL**: `/api/rooms/:roomId`
- **Method**: `GET`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |

**Success Response** (200 OK):

```json
{
  "success": true,
  "room": {
    "roomId": "my-room-123",
    "pin": "abc123",
    "maxAnchors": 3,
    "createdAt": "2026-01-07T12:00:00Z"
  }
}
```

**Error Responses**:

- **400 Bad Request**: Invalid room ID format
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId is required"]
  }
  ```

- **404 Not Found**: Room not found
  ```json
  {
    "success": false,
    "error": "Room not found"
  }
  ```

- **500 Internal Server Error**: Failed to get room
  ```json
  {
    "success": false,
    "error": "Failed to get room"
  }
  ```

**Implementation**: [router.go:160](../backend/rooms/transport/router.go#L160)

---

#### List Rooms

Retrieves a list of all rooms.

- **URL**: `/api/rooms`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "success": true,
  "count": 2,
  "rooms": [
    {
      "roomId": "room-1",
      "pin": "abc123",
      "maxAnchors": 3
    },
    {
      "roomId": "room-2",
      "pin": "def456",
      "maxAnchors": 5
    }
  ]
}
```

**Error Responses**:

- **500 Internal Server Error**: Failed to list rooms
  ```json
  {
    "success": false,
    "error": "Failed to list rooms"
  }
  ```

**Implementation**: [router.go:199](../backend/rooms/transport/router.go#L199)

---

#### Delete Room

Deletes a specific room.

- **URL**: `/api/rooms/:roomId`
- **Method**: `DELETE`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |

**Success Response** (200 OK):

```json
{
  "success": true,
  "message": "Room deleted successfully"
}
```

**Error Responses**:

- **400 Bad Request**: Invalid room ID format
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId is required"]
  }
  ```

- **404 Not Found**: Room not found
  ```json
  {
    "success": false,
    "error": "Room not found"
  }
  ```

- **500 Internal Server Error**: Failed to delete room
  ```json
  {
    "success": false,
    "error": "Failed to delete room"
  }
  ```

**Implementation**: [router.go:219](../backend/rooms/transport/router.go#L219)

---

#### Set Module Mark

Sets a mark label on a module (mixer or janus).

- **URL**: `/api/modules/:moduleType/:moduleId/mark`
- **Method**: `PUT`
- **Content-Type**: `application/json`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `moduleType` | string | Yes | "mixers" or "januses" | Type of module |
| `moduleId` | string | Yes | Valid module identifier | Module identifier |

**Request Body**:

```json
{
  "label": "ready",
  "ttl": 3600
}
```

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `label` | string | Yes | One of: "ready", "cordon", "draining", "drained", "unready" | Mark label |
| `ttl` | integer | No | Min: 0, Max: 86400 (24 hours) | Time to live in seconds. 0 means no expiration. |

**Success Response** (200 OK):

```json
{
  "success": true,
  "message": "Module mark set successfully",
  "module": {
    "type": "mixers",
    "id": "mixer-1",
    "label": "ready",
    "ttl": 3600
  }
}
```

**Error Responses**:

- **400 Bad Request**: Validation failed or invalid label
  ```json
  {
    "success": false,
    "error": "Invalid label value"
  }
  ```

- **500 Internal Server Error**: Failed to set module mark
  ```json
  {
    "success": false,
    "error": "Failed to set module mark"
  }
  ```

**Implementation**: [router.go:284](../backend/rooms/transport/router.go#L284)

---

#### Delete Module Mark

Removes a mark label from a module.

- **URL**: `/api/modules/:moduleType/:moduleId/mark`
- **Method**: `DELETE`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `moduleType` | string | Yes | "mixers" or "januses" | Type of module |
| `moduleId` | string | Yes | Valid module identifier | Module identifier |

**Success Response** (200 OK):

```json
{
  "success": true,
  "message": "Module mark deleted successfully",
  "module": {
    "type": "mixers",
    "id": "mixer-1"
  }
}
```

**Error Responses**:

- **400 Bad Request**: Validation failed
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["moduleType is required"]
  }
  ```

- **500 Internal Server Error**: Failed to delete module mark
  ```json
  {
    "success": false,
    "error": "Failed to delete module mark"
  }
  ```

**Implementation**: [router.go:352](../backend/rooms/transport/router.go#L352)

---

#### Get Stats

Retrieves room statistics.

- **URL**: `/api/stats`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "success": true,
  "stats": {
    "totalRooms": 10,
    "activeRooms": 5
  }
}
```

**Error Responses**:

- **500 Internal Server Error**: Failed to get stats
  ```json
  {
    "success": false,
    "error": "Failed to get stats"
  }
  ```

**Implementation**: [router.go:257](../backend/rooms/transport/router.go#L257)

---

#### Health Check

Checks the health status of the rooms service.

- **URL**: `/health`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "status": "ok",
  "service": "rooms",
  "timestamp": 1704636000
}
```

**Implementation**: [router.go:276](../backend/rooms/transport/router.go#L276)

---

## Users API

The Users API manages user creation and deletion within rooms.

**Base Path**: `/api`

### Endpoints

#### Create User

Creates a new user in a room and returns a JWT token.

- **URL**: `/api/rooms/:roomId/users`
- **Method**: `POST`
- **Content-Type**: `application/json`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |

**Request Body**:

```json
{
  "role": "host"
}
```

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `role` | string | No | Only support "anchor" now | User role. Optional. |

**Success Response** (200 OK):

```json
{
  "userID": "550e8400-e29b-41d4-a716-446655440000",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Error Responses**:

- **400 Bad Request**: Validation failed
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId is required"]
  }
  ```

- **500 Internal Server Error**: Failed to create user
  ```json
  {
    "success": false,
    "error": "Failed to create user"
  }
  ```

**Implementation**: [router.go:56](../backend/users/transport/router.go#L56)

---

#### Delete User

Deletes a user from a room.

- **URL**: `/api/rooms/:roomId/users/:userId`
- **Method**: `DELETE`

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |
| `userId` | string | Yes | Valid UUID v4 format | User identifier |

**Success Response** (200 OK):

```json
{}
```

**Error Responses**:

- **400 Bad Request**: Validation failed
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["userId must be valid UUID v4"]
  }
  ```

- **500 Internal Server Error**: Failed to delete user
  ```json
  {
    "error": "Failed to delete user"
  }
  ```

**Implementation**: [router.go:107](../backend/users/transport/router.go#L107)

---

#### Health Check

Checks the health status of the users service.

- **URL**: `/health`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "status": "ok",
  "timestamp": 1704636000
}
```

**Implementation**: [router.go:134](../backend/users/transport/router.go#L134)

---

## HLS Server API

The HLS Server API provides token generation and encryption key serving for HLS streaming.

**Base Path**: `/api` (Token Router) and `/hls` (Key Router)

### Token Router

Handles JWT token generation for HLS access.

#### Generate Token

Generates a JWT token for accessing HLS streams.

- **URL**: `/api/token`
- **Method**: `POST`
- **Content-Type**: `application/json`

**Request Body**:

```json
{
  "roomId": "my-room-123"
}
```

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |

**Success Response** (200 OK):

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Error Responses**:

- **400 Bad Request**: Validation failed
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId is required"]
  }
  ```

- **500 Internal Server Error**: Failed to generate token
  ```json
  {
    "success": false,
    "error": "Failed to generate token"
  }
  ```

**Implementation**: [router.go:67](../backend/hlsserver/transport/router.go#L67)

---

#### Health Check (Token Router)

Checks the health status of the token server.

- **URL**: `/health`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "status": "ok"
}
```

**Implementation**: [router.go:104](../backend/hlsserver/transport/router.go#L104)

---

### Key Router

Handles encryption key serving for HLS streams.

#### Get Encryption Key

Retrieves the encryption key for a specific room's HLS stream.

- **URL**: `/hls/rooms/:roomId/enc.key`
- **Method**: `GET`
- **Headers**: `Authorization: Bearer <token>` (required)

**URL Parameters**:

| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `roomId` | string | Yes | 3-32 chars, alphanumeric with hyphens/underscores | Room identifier |

**Request Headers**:

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes | Bearer token obtained from the token endpoint |

**Success Response** (200 OK):

- **Content-Type**: `application/octet-stream`
- **Body**: Binary encryption key data

**Response Headers**:

```
Cache-Control: no-cache, no-store, must-revalidate
Pragma: no-cache
Expires: 0
```

**Error Responses**:

- **400 Bad Request**: Invalid room ID format
  ```json
  {
    "success": false,
    "error": "Validation failed",
    "details": ["roomId is required"]
  }
  ```

- **401 Unauthorized**: Missing authorization header
  ```
  Authorization header required
  ```

- **403 Forbidden**: Invalid token, room ID mismatch, or room not found
  ```
  Access denied
  ```

**Implementation**: [router.go:156](../backend/hlsserver/transport/router.go#L156)

**Authentication Flow**:

1. Client obtains JWT token from `/api/token` endpoint
2. Client includes token in `Authorization` header when requesting encryption key
3. Server validates token and verifies room ID matches
4. Server checks if room is active via room watcher
5. Server generates or retrieves cached encryption key
6. Server returns binary key data

**Caching**:

- Encryption keys are cached using an LRU cache (max 100 entries)
- Cache hits and misses are tracked via metrics
- Keys are generated using the room ID and live metadata nonce

---

#### Health Check (Key Router)

Checks the health status of the key server.

- **URL**: `/health`
- **Method**: `GET`

**Success Response** (200 OK):

```json
{
  "status": "ok"
}
```

**Implementation**: [router.go:238](../backend/hlsserver/transport/router.go#L238)

---

## Common Patterns

### Validation

All endpoints use consistent validation patterns:

- Room IDs: 3-32 characters, alphanumeric with hyphens/underscores
- User IDs: UUID v4 format
- PINs: Exactly 6 alphanumeric characters
- Roles: "host", "guest", or "moderator"
- Module types: "mixers" or "januses"
- Mark labels: "ready", "cordon", "draining", "drained", "unready"

### Error Responses

All error responses follow a consistent structure:

```json
{
  "success": false,
  "error": "Error message",
  "details": ["Optional validation details"]
}
```

### Authentication

- **Users API**: Returns JWT tokens for user authentication
- **HLS Server API**: Requires JWT tokens in Authorization header for encryption key access

### Observability

All services include:

- OpenTelemetry middleware for automatic HTTP tracing
- Request logging with method and URL
- Health check endpoints

### CORS

The Key Router includes CORS configuration:

- Allowed origins: `*` (all)
- Allowed methods: `GET`, `POST`, `OPTIONS`
- Allowed headers: `Authorization`, `Content-Type`
