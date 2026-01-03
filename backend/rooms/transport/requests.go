package transport

// CreateRoomRequest represents the request to create a room
type CreateRoomRequest struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - optional
	RoomID string `json:"roomId,omitempty" binding:"omitempty,roomid"`
	// Pin: exactly 6 alphanumeric characters (optional)
	Pin string `json:"pin,omitempty" binding:"omitempty,len=6,alphanum"`
	// MaxAnchors: optional, min 1, max 5
	MaxAnchors int `json:"maxAnchors,omitempty" binding:"omitempty,min=1,max=5"`
}

// GetRoomRequest represents the request to get a room (from URL param)
type GetRoomRequest struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - required
	RoomID string `uri:"roomId" binding:"required,roomid"`
}

// DeleteRoomRequest represents the request to delete a room (from URL param)
type DeleteRoomRequest struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - required
	RoomID string `uri:"roomId" binding:"required,roomid"`
}

// ModuleMarkURI represents the URI parameters for module mark operations
type ModuleMarkURI struct {
	// ModuleType: "mixers" or "januses"
	ModuleType string `uri:"moduleType" binding:"required,modules"`
	// ModuleID: module identifier
	ModuleID string `uri:"moduleId" binding:"required,moduleid"`
}

// SetModuleMarkBody represents the request body for setting a module mark label
type SetModuleMarkBody struct {
	// Label: mark label (ready, cordon, draining, drained, unready)
	Label string `json:"label" binding:"required,label"`
	// TTL: time to live in seconds (optional, 0 means no expiration)
	TTL int64 `json:"ttl" binding:"omitempty,min=0,max=86400"`
}
