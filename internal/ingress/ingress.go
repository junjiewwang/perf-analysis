// Package ingress provides task ingestion endpoints that receive external tasks
// and persist them to the database for later processing by the scheduler.
package ingress

import "context"

// TaskIngress defines the interface for task ingestion components.
// Implementations receive tasks from external sources and write them to the database,
// decoupling task submission from task processing (scheduler).
type TaskIngress interface {
	// Start starts the ingress component.
	Start(ctx context.Context) error

	// Stop gracefully stops the ingress component.
	Stop(ctx context.Context) error
}
