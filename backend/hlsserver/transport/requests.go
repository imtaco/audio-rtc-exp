package transport

// GenerateTokenRequest represents the request to generate a token
type GenerateTokenRequest struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - required
	RoomID string `json:"roomId" binding:"required,roomid"`
}

// GetEncryptionKeyRequest represents the request to get encryption key (from URL param)
type GetEncryptionKeyRequest struct {
	// RoomID: 3-32 characters (letters, numbers, hyphens, underscores) - required
	RoomID string `uri:"roomId" binding:"required,roomid"`
}
