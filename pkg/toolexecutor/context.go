package toolexecutor

import "context"

type execContextKey struct{}

// ContextWithExecContext attaches the execution context to a context.Context for tool handlers.
func ContextWithExecContext(ctx context.Context, execCtx *ExecutionContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if execCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, execContextKey{}, execCtx)
}

// ExecContextFromContext extracts the execution context from a context.Context.
func ExecContextFromContext(ctx context.Context) *ExecutionContext {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(execContextKey{}); v != nil {
		if execCtx, ok := v.(*ExecutionContext); ok {
			return execCtx
		}
	}
	return nil
}
