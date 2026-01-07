package cryptoutil

import (
	"crypto/rand"
	"crypto/sha256"
)

// GenerateAESKey generates a deterministic AES-128 key from roomID and nonce
func GenerateAESKey(roomID, nonce string) []byte {
	hash := sha256.New()
	hash.Write([]byte(roomID))
	hash.Write([]byte(nonce))
	sum := hash.Sum(nil)
	return sum[:16] // AES-128 uses 16 bytes
}

// GenerateIV generates a random 16-byte IV for AES encryption
func GenerateIV() ([]byte, error) {
	iv := make([]byte, 16)
	_, err := rand.Read(iv)
	if err != nil {
		return nil, err
	}
	return iv, nil
}
