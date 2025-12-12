package source

import (
	"context"
	"sync"

	"github.com/perf-analysis/pkg/utils"
)

// Aggregator aggregates multiple TaskSources into a single unified task channel.
// It starts all sources in parallel and forwards their tasks to a single output channel.
type Aggregator struct {
	sources    []TaskSource
	sourceMap  map[string]TaskSource // key: "type:name"
	outputChan chan *TaskEvent
	bufferSize int
	logger     utils.Logger

	mu      sync.RWMutex
	running bool
	wg      sync.WaitGroup
	stopCh  chan struct{}
}

// NewAggregator creates a new Aggregator with the given sources.
func NewAggregator(sources []TaskSource, bufferSize int, logger utils.Logger) *Aggregator {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	// Build source map for quick lookup
	sourceMap := make(map[string]TaskSource)
	for _, src := range sources {
		key := buildSourceKey(src.Type(), src.Name())
		sourceMap[key] = src
	}

	return &Aggregator{
		sources:    sources,
		sourceMap:  sourceMap,
		outputChan: make(chan *TaskEvent, bufferSize),
		bufferSize: bufferSize,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// buildSourceKey creates a unique key for source lookup.
func buildSourceKey(sourceType SourceType, name string) string {
	return string(sourceType) + ":" + name
}

// Start starts all sources and begins forwarding tasks.
func (a *Aggregator) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = true
	a.mu.Unlock()

	a.logger.Info("Starting aggregator with %d sources", len(a.sources))

	// Start each source and its forwarder
	for _, src := range a.sources {
		if err := src.Start(ctx); err != nil {
			a.logger.Error("Failed to start source %s/%s: %v", src.Type(), src.Name(), err)
			// Stop already started sources
			a.Stop()
			return err
		}

		a.logger.Info("Started source: %s/%s", src.Type(), src.Name())

		// Start forwarder goroutine for this source
		a.wg.Add(1)
		go a.forward(ctx, src)
	}

	return nil
}

// forward forwards tasks from a single source to the aggregated output channel.
func (a *Aggregator) forward(ctx context.Context, src TaskSource) {
	defer a.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case event, ok := <-src.Tasks():
			if !ok {
				a.logger.Info("Source %s/%s channel closed", src.Type(), src.Name())
				return
			}

			// Ensure source info is set
			event.SourceType = src.Type()
			event.SourceName = src.Name()

			// Forward to output channel
			select {
			case a.outputChan <- event:
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			}
		}
	}
}

// Stop stops all sources and the aggregator.
func (a *Aggregator) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false
	a.mu.Unlock()

	a.logger.Info("Stopping aggregator...")

	// Signal all forwarders to stop
	close(a.stopCh)

	// Stop all sources
	for _, src := range a.sources {
		if err := src.Stop(); err != nil {
			a.logger.Error("Failed to stop source %s/%s: %v", src.Type(), src.Name(), err)
		}
	}

	// Wait for all forwarders to complete
	a.wg.Wait()

	// Close output channel
	close(a.outputChan)

	a.logger.Info("Aggregator stopped")
	return nil
}

// Tasks returns the aggregated task channel.
func (a *Aggregator) Tasks() <-chan *TaskEvent {
	return a.outputChan
}

// GetSource retrieves a specific source by type and name.
func (a *Aggregator) GetSource(sourceType SourceType, name string) TaskSource {
	a.mu.RLock()
	defer a.mu.RUnlock()

	key := buildSourceKey(sourceType, name)
	return a.sourceMap[key]
}

// GetSourceForEvent retrieves the source that produced the given event.
func (a *Aggregator) GetSourceForEvent(event *TaskEvent) TaskSource {
	return a.GetSource(event.SourceType, event.SourceName)
}

// Ack acknowledges a task event by delegating to the appropriate source.
func (a *Aggregator) Ack(ctx context.Context, event *TaskEvent) error {
	src := a.GetSourceForEvent(event)
	if src == nil {
		return nil
	}
	return src.Ack(ctx, event)
}

// Nack rejects a task event by delegating to the appropriate source.
func (a *Aggregator) Nack(ctx context.Context, event *TaskEvent, reason string) error {
	src := a.GetSourceForEvent(event)
	if src == nil {
		return nil
	}
	return src.Nack(ctx, event, reason)
}

// HealthCheck performs health checks on all sources.
func (a *Aggregator) HealthCheck(ctx context.Context) error {
	for _, src := range a.sources {
		if err := src.HealthCheck(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Sources returns all registered sources.
func (a *Aggregator) Sources() []TaskSource {
	return a.sources
}

// SourceCount returns the number of sources.
func (a *Aggregator) SourceCount() int {
	return len(a.sources)
}
