package gateway

import "context"

type ctxKey string

const clientIDKey ctxKey = "clientID"

func withClientID(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, clientIDKey, clientID)
}

func clientIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(clientIDKey).(string); ok {
		return value
	}
	return ""
}
