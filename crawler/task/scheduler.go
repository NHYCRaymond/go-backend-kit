package task

import (
	"log/slog"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ScheduleType defines schedule types
type ScheduleType string

const (
	ScheduleTypeOnce     ScheduleType = "once"
	ScheduleTypeInterval ScheduleType = "interval"
	ScheduleTypeCron     ScheduleType = "cron"
	ScheduleTypeEvent    ScheduleType = "event"
)

// Schedule defines a task schedule
type Schedule struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         ScheduleType           `json:"type"`
	Expression   string                 `json:"expression"` // Cron expression or interval
	TaskTemplate *Task                  `json:"task_template"`
	Enabled      bool                   `json:"enabled"`
	LastRun      *time.Time             `json:"last_run,omitempty"`
	NextRun      *time.Time             `json:"next_run,omitempty"`
	RunCount     int                    `json:"run_count"`
	MaxRuns      int                    `json:"max_runs"` // 0 for unlimited
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Scheduler manages task scheduling
type Scheduler struct {
	mu         sync.RWMutex
	schedules  map[string]*Schedule
	cron       *cron.Cron
	dispatcher *Dispatcher
	logger     *slog.Logger

	// Channels
	stopChan  chan struct{}
	eventChan chan *Event

	// Metrics
	totalRuns  int64
	failedRuns int64
}

// Event represents a trigger event
type Event struct {
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// SchedulerConfig contains scheduler configuration
type SchedulerConfig struct {
	Dispatcher *Dispatcher
	Logger     *slog.Logger
}

// NewScheduler creates a new scheduler
func NewScheduler(config *SchedulerConfig) *Scheduler {
	return &Scheduler{
		schedules:  make(map[string]*Schedule),
		cron:       cron.New(cron.WithSeconds()),
		dispatcher: config.Dispatcher,
		logger:     config.Logger,
		stopChan:   make(chan struct{}),
		eventChan:  make(chan *Event, 100),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting scheduler")

	// Start cron scheduler
	s.cron.Start()

	// Start event processor
	go s.processEvents(ctx)

	// Start interval processor
	go s.processIntervals(ctx)

	// Load and register all schedules
	s.reloadSchedules()

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.logger.Info("Stopping scheduler")

	// Stop cron
	s.cron.Stop()

	// Close channels
	close(s.stopChan)

	return nil
}

// AddSchedule adds a new schedule
func (s *Scheduler) AddSchedule(schedule *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate schedule
	if err := s.validateSchedule(schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}

	// Set timestamps
	now := time.Now()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	// Calculate next run time
	if schedule.Type == ScheduleTypeCron {
		nextRun, err := s.calculateNextRun(schedule.Expression)
		if err != nil {
			return err
		}
		schedule.NextRun = &nextRun
	}

	// Register with cron if needed
	if schedule.Type == ScheduleTypeCron && schedule.Enabled {
		entryID, err := s.cron.AddFunc(schedule.Expression, func() {
			s.executeSchedule(schedule.ID)
		})
		if err != nil {
			return fmt.Errorf("failed to add cron job: %w", err)
		}
		schedule.Metadata["cron_entry_id"] = entryID
	}

	// Store schedule
	s.schedules[schedule.ID] = schedule

	s.logger.Info("Schedule added",
		"schedule_id", schedule.ID,
		"name", schedule.Name,
		"type", string(schedule.Type))

	return nil
}

// RemoveSchedule removes a schedule
func (s *Scheduler) RemoveSchedule(scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[scheduleID]
	if !ok {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	// Remove from cron if needed
	if entryID, ok := schedule.Metadata["cron_entry_id"].(cron.EntryID); ok {
		s.cron.Remove(entryID)
	}

	delete(s.schedules, scheduleID)

	s.logger.Info("Schedule removed", "schedule_id", scheduleID)

	return nil
}

// EnableSchedule enables a schedule
func (s *Scheduler) EnableSchedule(scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[scheduleID]
	if !ok {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	if schedule.Enabled {
		return nil // Already enabled
	}

	schedule.Enabled = true
	schedule.UpdatedAt = time.Now()

	// Register with cron if needed
	if schedule.Type == ScheduleTypeCron {
		entryID, err := s.cron.AddFunc(schedule.Expression, func() {
			s.executeSchedule(schedule.ID)
		})
		if err != nil {
			return err
		}
		schedule.Metadata["cron_entry_id"] = entryID
	}

	return nil
}

// DisableSchedule disables a schedule
func (s *Scheduler) DisableSchedule(scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[scheduleID]
	if !ok {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	if !schedule.Enabled {
		return nil // Already disabled
	}

	schedule.Enabled = false
	schedule.UpdatedAt = time.Now()

	// Remove from cron if needed
	if entryID, ok := schedule.Metadata["cron_entry_id"].(cron.EntryID); ok {
		s.cron.Remove(entryID)
		delete(schedule.Metadata, "cron_entry_id")
	}

	return nil
}

// TriggerSchedule manually triggers a schedule
func (s *Scheduler) TriggerSchedule(scheduleID string) error {
	return s.executeSchedule(scheduleID)
}

// TriggerEvent triggers event-based schedules
func (s *Scheduler) TriggerEvent(event *Event) {
	select {
	case s.eventChan <- event:
	default:
		s.logger.Warn("Event channel full, dropping event")
	}
}

// executeSchedule executes a scheduled task
func (s *Scheduler) executeSchedule(scheduleID string) error {
	s.mu.RLock()
	schedule, ok := s.schedules[scheduleID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("schedule not found: %s", scheduleID)
	}

	if !schedule.Enabled {
		return nil // Schedule is disabled
	}

	// Check max runs
	if schedule.MaxRuns > 0 && schedule.RunCount >= schedule.MaxRuns {
		s.logger.Info("Schedule reached max runs",
			"schedule_id", scheduleID,
			"max_runs", schedule.MaxRuns)
		s.DisableSchedule(scheduleID)
		return nil
	}

	// Create task from template
	task := schedule.TaskTemplate.Clone()
	task.SetMetadata("schedule_id", scheduleID)
	task.SetMetadata("schedule_name", schedule.Name)
	task.SetMetadata("scheduled_at", time.Now())

	// Submit task
	ctx := context.Background()
	if err := s.dispatcher.Submit(ctx, task); err != nil {
		s.logger.Error("Failed to submit scheduled task",
			"schedule_id", scheduleID,
			"error", err)
		s.failedRuns++
		return err
	}

	// Update schedule
	s.mu.Lock()
	now := time.Now()
	schedule.LastRun = &now
	schedule.RunCount++
	schedule.UpdatedAt = now
	s.totalRuns++

	// Calculate next run
	if schedule.Type == ScheduleTypeCron {
		if nextRun, err := s.calculateNextRun(schedule.Expression); err == nil {
			schedule.NextRun = &nextRun
		}
	}
	s.mu.Unlock()

	s.logger.Info("Scheduled task executed",
		"schedule_id", scheduleID,
		"task_id", task.ID,
		"run_count", schedule.RunCount)

	return nil
}

// processEvents processes trigger events
func (s *Scheduler) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case event := <-s.eventChan:
			s.handleEvent(event)
		}
	}
}

// handleEvent handles a trigger event
func (s *Scheduler) handleEvent(event *Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find event-triggered schedules
	for _, schedule := range s.schedules {
		if schedule.Type != ScheduleTypeEvent || !schedule.Enabled {
			continue
		}

		// Check if event matches schedule trigger
		if s.matchesEventTrigger(schedule, event) {
			go s.executeSchedule(schedule.ID)
		}
	}
}

// matchesEventTrigger checks if event matches schedule trigger
func (s *Scheduler) matchesEventTrigger(schedule *Schedule, event *Event) bool {
	// Check event type in schedule expression
	// Expression format: "event_type:condition"
	// Example: "task_completed:type=seed"

	if schedule.Expression == "" {
		return false
	}

	// Simple implementation - match event type
	return schedule.Expression == event.Type
}

// processIntervals processes interval-based schedules
func (s *Scheduler) processIntervals(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkIntervalSchedules()
		}
	}
}

// checkIntervalSchedules checks and executes interval schedules
func (s *Scheduler) checkIntervalSchedules() {
	s.mu.RLock()
	schedules := make([]*Schedule, 0)
	for _, schedule := range s.schedules {
		if schedule.Type == ScheduleTypeInterval && schedule.Enabled {
			schedules = append(schedules, schedule)
		}
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, schedule := range schedules {
		if schedule.NextRun != nil && now.After(*schedule.NextRun) {
			go s.executeSchedule(schedule.ID)

			// Update next run time
			interval, err := time.ParseDuration(schedule.Expression)
			if err == nil {
				s.mu.Lock()
				nextRun := now.Add(interval)
				schedule.NextRun = &nextRun
				s.mu.Unlock()
			}
		}
	}
}

// validateSchedule validates a schedule configuration
func (s *Scheduler) validateSchedule(schedule *Schedule) error {
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	if schedule.Name == "" {
		return fmt.Errorf("schedule name is required")
	}

	if schedule.TaskTemplate == nil {
		return fmt.Errorf("task template is required")
	}

	switch schedule.Type {
	case ScheduleTypeCron:
		if _, err := cron.ParseStandard(schedule.Expression); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case ScheduleTypeInterval:
		if _, err := time.ParseDuration(schedule.Expression); err != nil {
			return fmt.Errorf("invalid interval expression: %w", err)
		}
	case ScheduleTypeOnce:
		// Parse time for one-time execution
		if _, err := time.Parse(time.RFC3339, schedule.Expression); err != nil {
			return fmt.Errorf("invalid time expression: %w", err)
		}
	case ScheduleTypeEvent:
		if schedule.Expression == "" {
			return fmt.Errorf("event type is required")
		}
	default:
		return fmt.Errorf("unknown schedule type: %s", schedule.Type)
	}

	return nil
}

// calculateNextRun calculates next run time for cron expression
func (s *Scheduler) calculateNextRun(expression string) (time.Time, error) {
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return time.Time{}, err
	}

	return schedule.Next(time.Now()), nil
}

// reloadSchedules reloads all schedules
func (s *Scheduler) reloadSchedules() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, schedule := range s.schedules {
		if schedule.Type == ScheduleTypeCron && schedule.Enabled {
			s.cron.AddFunc(schedule.Expression, func() {
				s.executeSchedule(schedule.ID)
			})
		}
	}
}

// GetSchedule returns a schedule by ID
func (s *Scheduler) GetSchedule(scheduleID string) (*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, ok := s.schedules[scheduleID]
	if !ok {
		return nil, fmt.Errorf("schedule not found: %s", scheduleID)
	}

	return schedule, nil
}

// ListSchedules returns all schedules
func (s *Scheduler) ListSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedules := make([]*Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		schedules = append(schedules, schedule)
	}

	return schedules
}

// GetStats returns scheduler statistics
func (s *Scheduler) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	enabledCount := 0
	typeCount := make(map[ScheduleType]int)

	for _, schedule := range s.schedules {
		if schedule.Enabled {
			enabledCount++
		}
		typeCount[schedule.Type]++
	}

	return map[string]interface{}{
		"total_schedules": len(s.schedules),
		"enabled":         enabledCount,
		"disabled":        len(s.schedules) - enabledCount,
		"by_type":         typeCount,
		"total_runs":      s.totalRuns,
		"failed_runs":     s.failedRuns,
	}
}

// ScheduleBuilder helps build schedules
type ScheduleBuilder struct {
	schedule *Schedule
}

// NewScheduleBuilder creates a new schedule builder
func NewScheduleBuilder(name string) *ScheduleBuilder {
	return &ScheduleBuilder{
		schedule: &Schedule{
			ID:       name, // Use name as ID for simplicity
			Name:     name,
			Enabled:  true,
			Metadata: make(map[string]interface{}),
		},
	}
}

// WithCron sets cron expression
func (sb *ScheduleBuilder) WithCron(expression string) *ScheduleBuilder {
	sb.schedule.Type = ScheduleTypeCron
	sb.schedule.Expression = expression
	return sb
}

// WithInterval sets interval
func (sb *ScheduleBuilder) WithInterval(interval time.Duration) *ScheduleBuilder {
	sb.schedule.Type = ScheduleTypeInterval
	sb.schedule.Expression = interval.String()
	return sb
}

// WithOnce sets one-time execution
func (sb *ScheduleBuilder) WithOnce(at time.Time) *ScheduleBuilder {
	sb.schedule.Type = ScheduleTypeOnce
	sb.schedule.Expression = at.Format(time.RFC3339)
	sb.schedule.NextRun = &at
	return sb
}

// WithEvent sets event trigger
func (sb *ScheduleBuilder) WithEvent(eventType string) *ScheduleBuilder {
	sb.schedule.Type = ScheduleTypeEvent
	sb.schedule.Expression = eventType
	return sb
}

// WithTask sets task template
func (sb *ScheduleBuilder) WithTask(task *Task) *ScheduleBuilder {
	sb.schedule.TaskTemplate = task
	return sb
}

// WithMaxRuns sets maximum runs
func (sb *ScheduleBuilder) WithMaxRuns(maxRuns int) *ScheduleBuilder {
	sb.schedule.MaxRuns = maxRuns
	return sb
}

// Build builds the schedule
func (sb *ScheduleBuilder) Build() *Schedule {
	return sb.schedule
}
