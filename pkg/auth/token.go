// Package auth provides authentication utilities for the perf-analysis service.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// GenerateViewToken generates an HMAC-SHA256 token for URL authentication.
func GenerateViewToken(secret, taskID string, expireAt int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(taskID + strconv.FormatInt(expireAt, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateViewToken validates an HMAC-SHA256 token.
func ValidateViewToken(secret, taskID string, expireAt int64, token string) bool {
	if time.Now().Unix() > expireAt {
		return false
	}
	expected := GenerateViewToken(secret, taskID, expireAt)
	return hmac.Equal([]byte(expected), []byte(token))
}
