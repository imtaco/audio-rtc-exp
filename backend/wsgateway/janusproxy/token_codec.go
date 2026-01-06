package janusproxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/imtaco/audio-rtc-exp/wsgateway"
)

func NewJanusTokenCodec(key []byte) (wsgateway.JanusTokenCodec, error) {
	if len(key) != 32 {
		return nil, errors.Errorf("key must be 32 bytes (AES-256), got %d", len(key))
	}
	return &janusIDCodec{
		key: key,
	}, nil
}

type JanusToken struct {
	UserID    string
	SessionID int64
	HandleID  int64
}

type janusIDCodec struct {
	key []byte
}

// AES-256-GCM encrypts two int64 packed into 16 bytes.
// Output token: standard Base64 of nonce(12) || ciphertext+tag
func (c *janusIDCodec) Encode(roomKey string, sessionID, handleID int64) (string, error) {
	plain := make([]byte, 18)
	plain[0] = 'J'
	plain[1] = 'T'
	binary.BigEndian.PutUint64(plain[2:10], uint64(sessionID))   // #nosec G115 -- sessionID is int64, conversion to uint64 is safe for binary encoding
	binary.BigEndian.PutUint64(plain[10:18], uint64(handleID)) // #nosec G115 -- handleID is int64, conversion to uint64 is safe for binary encoding

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Bind ciphertext to this userId (prevents swapping token across users)
	aad := []byte(roomKey)
	ciphertext := gcm.Seal(nil, nonce, plain, aad)

	raw := nonce
	raw = append(raw, ciphertext...)
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (c *janusIDCodec) Decode(roomKey string, token string) (int64, int64, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, 0, err
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return 0, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, 0, err
	}

	ns := gcm.NonceSize()
	if len(raw) < ns+1 {
		return 0, 0, errors.New("token too short")
	}
	nonce := raw[:ns]
	ciphertext := raw[ns:]
	aad := []byte(roomKey)

	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return 0, 0, err
	}
	if len(plain) != 18 {
		return 0, 0, errors.New("unexpected plaintext length")
	}
	if plain[0] != 'J' || plain[1] != 'T' {
		return 0, 0, errors.New("invalid janus token prefix")
	}

	sessionID := int64(binary.BigEndian.Uint64(plain[2:10]))  // #nosec G115 -- uint64 to int64 conversion is safe, values come from our own encoding
	handleID := int64(binary.BigEndian.Uint64(plain[10:18])) // #nosec G115 -- uint64 to int64 conversion is safe, values come from our own encoding
	return sessionID, handleID, nil
}
