package transport

// CreateUserURI represents the URI parameters for creating a user
type CreateUserURI struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - required
	RoomID string `uri:"roomId" binding:"required,roomid"`
}

// CreateUserBody represents the request body for creating a user
type CreateUserBody struct {
	// Role: must be host, guest, or anchor (optional)
	Role string `json:"role,omitempty" binding:"omitempty,role"`
}

// DeleteUserURI represents the URI parameters for deleting a user
type DeleteUserURI struct {
	RoomID string `uri:"roomId" binding:"required,roomid"`
	// UserID: must be valid UUID v4 format
	UserID string `uri:"userId" binding:"required,userid"`
}
