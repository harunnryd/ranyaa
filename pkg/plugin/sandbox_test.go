package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSandboxContext(t *testing.T) {
	t.Run("creates sandbox with permissions", func(t *testing.T) {
		perms := []Permission{
			PermissionFilesystemRead,
			PermissionNetworkHTTP,
		}

		sandbox := NewSandboxContext(perms)

		assert.True(t, sandbox.CheckPermission(PermissionFilesystemRead))
		assert.True(t, sandbox.CheckPermission(PermissionNetworkHTTP))
		assert.False(t, sandbox.CheckPermission(PermissionFilesystemWrite))
	})

	t.Run("requires permission successfully", func(t *testing.T) {
		perms := []Permission{PermissionFilesystemRead}
		sandbox := NewSandboxContext(perms)

		err := sandbox.RequirePermission(PermissionFilesystemRead)
		assert.NoError(t, err)
	})

	t.Run("denies missing permission", func(t *testing.T) {
		perms := []Permission{PermissionFilesystemRead}
		sandbox := NewSandboxContext(perms)

		err := sandbox.RequirePermission(PermissionFilesystemWrite)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("returns all permissions", func(t *testing.T) {
		perms := []Permission{
			PermissionFilesystemRead,
			PermissionNetworkHTTP,
			PermissionDatabaseRead,
		}

		sandbox := NewSandboxContext(perms)
		returned := sandbox.GetPermissions()

		assert.Len(t, returned, 3)
		assert.Contains(t, returned, PermissionFilesystemRead)
		assert.Contains(t, returned, PermissionNetworkHTTP)
		assert.Contains(t, returned, PermissionDatabaseRead)
	})

	t.Run("checks any permission", func(t *testing.T) {
		perms := []Permission{PermissionFilesystemRead}
		sandbox := NewSandboxContext(perms)

		assert.True(t, sandbox.HasAnyPermission(
			PermissionFilesystemRead,
			PermissionFilesystemWrite,
		))

		assert.False(t, sandbox.HasAnyPermission(
			PermissionFilesystemWrite,
			PermissionNetworkHTTP,
		))
	})

	t.Run("checks all permissions", func(t *testing.T) {
		perms := []Permission{
			PermissionFilesystemRead,
			PermissionFilesystemWrite,
		}
		sandbox := NewSandboxContext(perms)

		assert.True(t, sandbox.HasAllPermissions(
			PermissionFilesystemRead,
			PermissionFilesystemWrite,
		))

		assert.False(t, sandbox.HasAllPermissions(
			PermissionFilesystemRead,
			PermissionNetworkHTTP,
		))
	})

	t.Run("creates sandbox with no permissions", func(t *testing.T) {
		sandbox := NewSandboxContext([]Permission{})

		assert.False(t, sandbox.CheckPermission(PermissionFilesystemRead))
		assert.Empty(t, sandbox.GetPermissions())
	})

	t.Run("validates all permission types", func(t *testing.T) {
		allPerms := []Permission{
			PermissionFilesystemRead,
			PermissionFilesystemWrite,
			PermissionNetworkHTTP,
			PermissionNetworkWebSocket,
			PermissionProcessSpawn,
			PermissionDatabaseRead,
			PermissionDatabaseWrite,
			PermissionGatewayRegister,
		}

		sandbox := NewSandboxContext(allPerms)

		for _, perm := range allPerms {
			assert.True(t, sandbox.CheckPermission(perm))
		}
	})
}
