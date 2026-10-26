package worker

import "context"

type cachedAuditKey struct{}

// WithCachedAudit keeps analysis offline after an isolated service refreshes the advisory cache.
func WithCachedAudit(ctx context.Context) context.Context {
	return context.WithValue(ctx, cachedAuditKey{}, true)
}
