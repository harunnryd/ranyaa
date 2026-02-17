package sandbox

import "errors"

var (
	// ErrInvalidRuntime is returned when the sandbox runtime is invalid
	ErrInvalidRuntime = errors.New("invalid sandbox runtime")

	// ErrInvalidMode is returned when the sandbox mode is invalid
	ErrInvalidMode = errors.New("invalid sandbox mode")

	// ErrInvalidScope is returned when the sandbox scope is invalid
	ErrInvalidScope = errors.New("invalid sandbox scope")

	// ErrInvalidCPULimit is returned when the CPU limit is invalid
	ErrInvalidCPULimit = errors.New("invalid CPU limit (must be 0-100)")

	// ErrInvalidMemoryLimit is returned when the memory limit is invalid
	ErrInvalidMemoryLimit = errors.New("invalid memory limit (must be >= 0)")

	// ErrInvalidProcessLimit is returned when the process limit is invalid
	ErrInvalidProcessLimit = errors.New("invalid process limit (must be >= 0)")

	// ErrInvalidTimeout is returned when the timeout is invalid
	ErrInvalidTimeout = errors.New("invalid timeout (must be >= 0)")

	// ErrSandboxNotRunning is returned when the sandbox is not running
	ErrSandboxNotRunning = errors.New("sandbox is not running")

	// ErrSandboxAlreadyRunning is returned when the sandbox is already running
	ErrSandboxAlreadyRunning = errors.New("sandbox is already running")

	// ErrExecutionTimeout is returned when execution times out
	ErrExecutionTimeout = errors.New("execution timed out")

	// ErrResourceLimitExceeded is returned when a resource limit is exceeded
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")

	// ErrFilesystemAccessDenied is returned when filesystem access is denied
	ErrFilesystemAccessDenied = errors.New("filesystem access denied")

	// ErrNetworkAccessDenied is returned when network access is denied
	ErrNetworkAccessDenied = errors.New("network access denied")

	// ErrDockerImageRequired is returned when Docker runtime is enabled without an image
	ErrDockerImageRequired = errors.New("docker image is required for docker runtime")
)
