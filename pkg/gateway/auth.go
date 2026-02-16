package gateway

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// AuthHandler manages challenge-response authentication
type AuthHandler struct {
	sharedSecret string
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(sharedSecret string) *AuthHandler {
	return &AuthHandler{
		sharedSecret: sharedSecret,
	}
}

// GenerateChallenge generates a cryptographically random 32-byte challenge
func (a *AuthHandler) GenerateChallenge() (string, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return "", fmt.Errorf("failed to generate challenge: %w", err)
	}
	return hex.EncodeToString(challenge), nil
}

// VerifySignature verifies an HMAC-SHA256 signature against a challenge
func (a *AuthHandler) VerifySignature(challenge, signature string) bool {
	// Compute expected signature
	h := hmac.New(sha256.New, []byte(a.sharedSecret))
	h.Write([]byte(challenge))
	expected := hex.EncodeToString(h.Sum(nil))

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// HandleAuthResponse processes an authentication response from a client
func (a *AuthHandler) HandleAuthResponse(client *Client, signature string) AuthResult {
	// Check if client has a challenge
	if client.Challenge == "" {
		return AuthResult{
			Event:   "auth.failure",
			Success: false,
			Message: "No challenge found",
		}
	}

	// Verify signature
	if !a.VerifySignature(client.Challenge, signature) {
		client.AuthAttempts++

		// Block after 3 failed attempts
		if client.AuthAttempts >= 3 {
			return AuthResult{
				Event:   "auth.failure",
				Success: false,
				Message: "Too many failed attempts",
			}
		}

		return AuthResult{
			Event:   "auth.failure",
			Success: false,
			Message: "Invalid signature",
		}
	}

	// Authentication successful
	client.Authenticated = true
	client.State = StateAuthenticated
	client.AuthAttempts = 0
	client.Challenge = "" // Clear challenge

	return AuthResult{
		Event:   "auth.success",
		Success: true,
	}
}
