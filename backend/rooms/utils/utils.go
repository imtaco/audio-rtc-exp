package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateRandomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func IsExceed(timestamp time.Time, duration time.Duration) bool {
	if timestamp.IsZero() {
		return false
	}
	return time.Since(timestamp) > duration
}

func ParseTime(timestamp string) (time.Time, error) {
	if timestamp == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	return time.Parse(time.RFC3339, timestamp)
}
