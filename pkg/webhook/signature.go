package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// verifySignature verifies a webhook signature using HMAC
func verifySignature(body string, signature string, secret string, algorithm string) bool {
	var expected string

	switch algorithm {
	case "sha256":
		expected = computeHMACSHA256(body, secret)
	case "sha1":
		expected = computeHMACSHA1(body, secret)
	default:
		return false
	}

	// Timing-safe comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

// computeHMACSHA256 computes HMAC-SHA256 signature
func computeHMACSHA256(body string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(body))
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(h.Sum(nil)))
}

// computeHMACSHA1 computes HMAC-SHA1 signature
func computeHMACSHA1(body string, secret string) string {
	h := hmac.New(sha1.New, []byte(secret))
	h.Write([]byte(body))
	return fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
}

// parseSignatureHeader extracts the signature from various header formats
func parseSignatureHeader(header string) string {
	// Handle formats like "sha256=abc123" or just "abc123"
	if strings.Contains(header, "=") {
		parts := strings.SplitN(header, "=", 2)
		if len(parts) == 2 {
			return header // Return full format
		}
	}
	return header
}
