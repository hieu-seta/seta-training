package events

import "context"

// Publisher is the port: services depend on this, not on NATS directly.
type Publisher interface {
	// Publish serializes payload as JSON, attaches Nats-Msg-Id header for dedup.
	// Returns nil on success. Errors are non-fatal at the call site
	// (publish-after-commit pattern) — services log + continue.
	Publish(ctx context.Context, subject string, payload any, msgID string) error
}

// Noop is a Publisher that swallows everything — useful for tests that don't care.
type Noop struct{}

// Publish always returns nil.
func (Noop) Publish(_ context.Context, _ string, _ any, _ string) error { return nil }
