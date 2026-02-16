package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthHandler_GenerateChallenge(t *testing.T) {
	auth := NewAuthHandler("test-secret")

	t.Run("should generate 32-byte challenge as hex", func(t *testing.T) {
		challenge, err := auth.GenerateChallenge()
		require.NoError(t, err)
		assert.Len(t, challenge, 64) // 32 bytes = 64 hex characters
	})

	t.Run("should generate unique challenges", func(t *testing.T) {
		challenge1, err1 := auth.GenerateChallenge()
		challenge2, err2 := auth.GenerateChallenge()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, challenge1, challenge2)
	})
}

func TestAuthHandler_VerifySignature(t *testing.T) {
	auth := NewAuthHandler("test-secret")

	t.Run("should verify valid signature", func(t *testing.T) {
		challenge, err := auth.GenerateChallenge()
		require.NoError(t, err)

		// Generate valid signature
		valid := auth.VerifySignature(challenge, computeHMAC(challenge, "test-secret"))
		assert.True(t, valid)
	})

	t.Run("should reject invalid signature", func(t *testing.T) {
		challenge, err := auth.GenerateChallenge()
		require.NoError(t, err)

		valid := auth.VerifySignature(challenge, "invalid-signature")
		assert.False(t, valid)
	})

	t.Run("should reject signature with wrong secret", func(t *testing.T) {
		challenge, err := auth.GenerateChallenge()
		require.NoError(t, err)

		wrongSignature := computeHMAC(challenge, "wrong-secret")
		valid := auth.VerifySignature(challenge, wrongSignature)
		assert.False(t, valid)
	})
}

func TestAuthHandler_HandleAuthResponse(t *testing.T) {
	auth := NewAuthHandler("test-secret")

	t.Run("should succeed with valid signature", func(t *testing.T) {
		client := &Client{
			ID:            "test-client",
			Challenge:     "test-challenge",
			AuthAttempts:  0,
			Authenticated: false,
		}

		signature := computeHMAC("test-challenge", "test-secret")
		result := auth.HandleAuthResponse(client, signature)

		assert.True(t, result.Success)
		assert.Equal(t, "auth.success", result.Event)
		assert.True(t, client.Authenticated)
		assert.Equal(t, StateAuthenticated, client.State)
		assert.Equal(t, 0, client.AuthAttempts)
		assert.Empty(t, client.Challenge)
	})

	t.Run("should fail with invalid signature", func(t *testing.T) {
		client := &Client{
			ID:            "test-client",
			Challenge:     "test-challenge",
			AuthAttempts:  0,
			Authenticated: false,
		}

		result := auth.HandleAuthResponse(client, "invalid-signature")

		assert.False(t, result.Success)
		assert.Equal(t, "auth.failure", result.Event)
		assert.False(t, client.Authenticated)
		assert.Equal(t, 1, client.AuthAttempts)
	})

	t.Run("should block after 3 failed attempts", func(t *testing.T) {
		client := &Client{
			ID:            "test-client",
			Challenge:     "test-challenge",
			AuthAttempts:  2,
			Authenticated: false,
		}

		result := auth.HandleAuthResponse(client, "invalid-signature")

		assert.False(t, result.Success)
		assert.Equal(t, "auth.failure", result.Event)
		assert.Contains(t, result.Message, "Too many failed attempts")
		assert.Equal(t, 3, client.AuthAttempts)
	})

	t.Run("should fail when no challenge exists", func(t *testing.T) {
		client := &Client{
			ID:            "test-client",
			Challenge:     "",
			AuthAttempts:  0,
			Authenticated: false,
		}

		result := auth.HandleAuthResponse(client, "any-signature")

		assert.False(t, result.Success)
		assert.Contains(t, result.Message, "No challenge found")
	})
}

// Helper function to compute HMAC-SHA256
func computeHMAC(challenge, secret string) string {
	// Generate a valid signature by using the same logic as VerifySignature
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(challenge))
	return hex.EncodeToString(h.Sum(nil))
}
