package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/NHYCRaymond/go-backend-kit/errors"
	"github.com/NHYCRaymond/go-backend-kit/logging"
)

// Pipeline represents a data processing pipeline
type Pipeline struct {
	name       string
	processors []Processor
	logger     *slog.Logger
	mu         sync.RWMutex

	// Metrics
	processed int64
	failed    int64
	skipped   int64
}

// Processor processes data in the pipeline
type Processor interface {
	// Name returns processor name
	Name() string
	// Process processes the data
	Process(ctx context.Context, data interface{}) (interface{}, error)
	// CanProcess checks if processor can handle the data
	CanProcess(data interface{}) bool
}

// PipelineConfig contains pipeline configuration
type PipelineConfig struct {
	Name   string
	Logger *slog.Logger
}

// NewPipeline creates a new pipeline
func NewPipeline(config *PipelineConfig) *Pipeline {
	if config.Logger == nil {
		config.Logger = logging.GetLogger()
	}

	return &Pipeline{
		name:       config.Name,
		processors: make([]Processor, 0),
		logger:     config.Logger,
	}
}

// AddProcessor adds a processor to the pipeline
func (p *Pipeline) AddProcessor(processor Processor) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.processors = append(p.processors, processor)
	p.logger.Debug("Processor added to pipeline",
		"pipeline", p.name,
		"processor", processor.Name())

	return p
}

// Process runs data through the pipeline
func (p *Pipeline) Process(ctx context.Context, data interface{}) (interface{}, error) {
	p.mu.RLock()
	processors := p.processors
	p.mu.RUnlock()

	if len(processors) == 0 {
		return data, nil
	}

	result := data
	for _, processor := range processors {
		// Check if processor can handle the data
		if !processor.CanProcess(result) {
			p.logger.Debug("Processor skipped",
				"processor", processor.Name(),
				"reason", "cannot process data type")
			p.skipped++
			continue
		}

		// Process data
		processed, err := processor.Process(ctx, result)
		if err != nil {
			p.failed++
			p.logger.Error("Processor failed",
				"processor", processor.Name(),
				"error", err)

			// Check if error is recoverable
			if errors.IsRecoverable(err) {
				continue // Skip this processor but continue pipeline
			}
			return nil, fmt.Errorf("pipeline failed at %s: %w", processor.Name(), err)
		}

		result = processed
		p.processed++
	}

	return result, nil
}

// ProcessBatch processes multiple items
func (p *Pipeline) ProcessBatch(ctx context.Context, items []interface{}) ([]interface{}, error) {
	results := make([]interface{}, 0, len(items))

	for _, item := range items {
		result, err := p.Process(ctx, item)
		if err != nil {
			p.logger.Error("Failed to process item", "error", err)
			continue // Skip failed items
		}
		if result != nil {
			results = append(results, result)
		}
	}

	return results, nil
}

// GetStats returns pipeline statistics
func (p *Pipeline) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]interface{}{
		"name":       p.name,
		"processors": len(p.processors),
		"processed":  p.processed,
		"failed":     p.failed,
		"skipped":    p.skipped,
	}
}

// Clear clears all processors
func (p *Pipeline) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.processors = make([]Processor, 0)
}

// BaseProcessor provides common processor functionality
type BaseProcessor struct {
	name string
}

// NewBaseProcessor creates a base processor
func NewBaseProcessor(name string) *BaseProcessor {
	return &BaseProcessor{name: name}
}

// Name returns processor name
func (bp *BaseProcessor) Name() string {
	return bp.name
}

// CanProcess checks if processor can handle the data
func (bp *BaseProcessor) CanProcess(data interface{}) bool {
	return data != nil
}

// ConditionalProcessor wraps a processor with a condition
type ConditionalProcessor struct {
	processor Processor
	condition func(interface{}) bool
}

// NewConditionalProcessor creates a conditional processor
func NewConditionalProcessor(processor Processor, condition func(interface{}) bool) *ConditionalProcessor {
	return &ConditionalProcessor{
		processor: processor,
		condition: condition,
	}
}

// Name returns processor name
func (cp *ConditionalProcessor) Name() string {
	return fmt.Sprintf("conditional:%s", cp.processor.Name())
}

// Process processes the data if condition is met
func (cp *ConditionalProcessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	if cp.condition(data) {
		return cp.processor.Process(ctx, data)
	}
	return data, nil
}

// CanProcess checks if processor can handle the data
func (cp *ConditionalProcessor) CanProcess(data interface{}) bool {
	return cp.processor.CanProcess(data)
}

// ParallelPipeline runs multiple pipelines in parallel
type ParallelPipeline struct {
	pipelines []*Pipeline
	logger    *slog.Logger
}

// NewParallelPipeline creates a parallel pipeline
func NewParallelPipeline(logger *slog.Logger) *ParallelPipeline {
	return &ParallelPipeline{
		pipelines: make([]*Pipeline, 0),
		logger:    logger,
	}
}

// AddPipeline adds a pipeline
func (pp *ParallelPipeline) AddPipeline(pipeline *Pipeline) {
	pp.pipelines = append(pp.pipelines, pipeline)
}

// Process runs all pipelines in parallel
func (pp *ParallelPipeline) Process(ctx context.Context, data interface{}) ([]interface{}, error) {
	if len(pp.pipelines) == 0 {
		return []interface{}{data}, nil
	}

	results := make([]interface{}, len(pp.pipelines))
	errChan := make(chan error, len(pp.pipelines))
	var wg sync.WaitGroup

	for i, pipeline := range pp.pipelines {
		wg.Add(1)
		go func(idx int, p *Pipeline) {
			defer wg.Done()

			result, err := p.Process(ctx, data)
			if err != nil {
				errChan <- err
				return
			}
			results[idx] = result
		}(i, pipeline)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// Builder helps build pipelines
type Builder struct {
	pipeline *Pipeline
}

// NewBuilder creates a new pipeline builder
func NewBuilder(name string, logger *slog.Logger) *Builder {
	return &Builder{
		pipeline: NewPipeline(&PipelineConfig{
			Name:   name,
			Logger: logger,
		}),
	}
}

// Add adds a processor
func (b *Builder) Add(processor Processor) *Builder {
	b.pipeline.AddProcessor(processor)
	return b
}

// AddConditional adds a conditional processor
func (b *Builder) AddConditional(processor Processor, condition func(interface{}) bool) *Builder {
	b.pipeline.AddProcessor(NewConditionalProcessor(processor, condition))
	return b
}

// Build returns the built pipeline
func (b *Builder) Build() *Pipeline {
	return b.pipeline
}
