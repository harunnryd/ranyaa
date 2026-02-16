package webhook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifySignatureSHA256(t *testing.T) {
	body := `{"test": "data"}`
	secret := "my-secret-key"

	// Compute valid signature
	validSignature := computeHMACSHA256(body, secret)

	// Test valid signature
	assert.True(t, verifySignature(body, validSignature, secret, "sha256"))

	// Test invalid signature
	assert.False(t, verifySignature(body, "sha256=invalid", secret, "sha256"))

	// Test wrong secret
	assert.False(t, verifySignature(body, validSignature, "wrong-secret", "sha256"))

	// Test wrong body
	assert.False(t, verifySignature("different body", validSignature, secret, "sha256"))
}

func TestVerifySignatureSHA1(t *testing.T) {
	body := `{"test": "data"}`
	secret := "my-secret-key"

	// Compute valid signature
	validSignature := computeHMACSHA1(body, secret)

	// Test valid signature
	assert.True(t, verifySignature(body, validSignature, secret, "sha1"))

	// Test invalid signature
	assert.False(t, verifySignature(body, "sha1=invalid", secret, "sha1"))

	// Test wrong secret
	assert.False(t, verifySignature(body, validSignature, "wrong-secret", "sha1"))

	// Test wrong body
	assert.False(t, verifySignature("different body", validSignature, secret, "sha1"))
}

func TestComputeHMACSHA256(t *testing.T) {
	body := "test body"
	secret := "secret"

	signature := computeHMACSHA256(body, secret)

	// Should have sha256= prefix
	assert.Contains(t, signature, "sha256=")

	// Should be deterministic
	signature2 := computeHMACSHA256(body, secret)
	assert.Equal(t, signature, signature2)

	// Different body should produce different signature
	signature3 := computeHMACSHA256("different body", secret)
	assert.NotEqual(t, signature, signature3)
}

func TestComputeHMACSHA1(t *testing.T) {
	body := "test body"
	secret := "secret"

	signature := computeHMACSHA1(body, secret)

	// Should have sha1= prefix
	assert.Contains(t, signature, "sha1=")

	// Should be deterministic
	signature2 := computeHMACSHA1(body, secret)
	assert.Equal(t, signature, signature2)

	// Different body should produce different signature
	signature3 := computeHMACSHA1("different body", secret)
	assert.NotEqual(t, signature, signature3)
}

func TestVerifySignatureInvalidAlgorithm(t *testing.T) {
	body := "test"
	signature := "test"
	secret := "secret"

	// Invalid algorithm should return false
	assert.False(t, verifySignature(body, signature, secret, "invalid"))
}

func TestParseSignatureHeader(t *testing.T) {
	// Test with algorithm prefix
	assert.Equal(t, "sha256=abc123", parseSignatureHeader("sha256=abc123"))

	// Test without prefix
	assert.Equal(t, "abc123", parseSignatureHeader("abc123"))

	// Test empty
	assert.Equal(t, "", parseSignatureHeader(""))
}
