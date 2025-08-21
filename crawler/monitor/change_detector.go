package monitor

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/notification"
	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"github.com/go-redis/redis/v8"
	"github.com/patrickmn/go-cache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SubscriberProvider interface for getting subscribers
type SubscriberProvider interface {
	GetSubscribers(ctx context.Context, source, timeSlot string) []types.Subscriber
}

// ChangeDetector detects changes in venue states
type ChangeDetector struct {
	mongodb      *mongo.Database
	redis        *redis.Client
	config       *types.ChangeDetectorConfig
	subClient    SubscriberProvider
	pusher       *notification.NotificationPusher
	logger       *slog.Logger
	cache        *cache.Cache
	parsers      map[string]VenueParser
	distLock     *DistributedLock
	dedup        *DeduplicationChecker
	mu           sync.RWMutex
	stopChan     chan struct{}
}

// VenueParser interface for parsing venue data from different sources
type VenueParser interface {
	Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error)
	GetSource() string
}

// NewChangeDetector creates a new change detector
func NewChangeDetector(
	mongodb *mongo.Database,
	redis *redis.Client,
	config *types.ChangeDetectorConfig,
	logger *slog.Logger,
) *ChangeDetector {
	if logger == nil {
		logger = slog.Default()
	}

	cd := &ChangeDetector{
		mongodb:  mongodb,
		redis:    redis,
		config:   config,
		logger:   logger,
		cache:    cache.New(time.Duration(config.CacheTTL)*time.Second, 2*time.Duration(config.CacheTTL)*time.Second),
		parsers:  make(map[string]VenueParser),
		distLock: NewDistributedLock(redis, "crawler:monitor"),
		dedup:    NewDeduplicationChecker(redis, "crawler:monitor", 5*time.Minute),
		stopChan: make(chan struct{}),
	}

	// Initialize subscription client
	cd.subClient = NewSubscriptionClient(config.SubscriptionAPIURL, config.SubscriptionAPITimeout)
	
	// If webhook URL is provided in config, use enhanced client for evening slots
	if webhookURL := os.Getenv("DINGTALK_WEBHOOK_URL"); webhookURL != "" {
		integration := NewCoordinatorIntegration(cd, redis, webhookURL, logger)
		cd.subClient = NewEnhancedSubscriptionClient(
			config.SubscriptionAPIURL, 
			config.SubscriptionAPITimeout,
			integration,
		)
		logger.Info("Enhanced subscription client enabled for evening slots (19:00-22:00)")
	}

	// Initialize notification pusher
	pusherConfig := &notification.PusherConfig{
		Workers:    config.Workers,
		QueueSize:  config.QueueSize,
		RetryTimes: 3,
		RetryDelay: 5,
	}
	cd.pusher = notification.NewNotificationPusher(pusherConfig, logger)

	return cd
}

// RegisterParser registers a venue parser for a specific source
func (cd *ChangeDetector) RegisterParser(source string, parser VenueParser) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.parsers[source] = parser
	cd.logger.Info("Registered venue parser", "source", source)
}

// extractBatchID extracts the batch ID from a task ID
// For example: "tennis_booking-ground-batch-20250821120000-2025-08-21" -> "tennis_booking-ground-batch-20250821120000"
func extractBatchID(taskID string) string {
	// For tennis_booking tasks with dates, remove the date suffix
	if strings.Contains(taskID, "tennis_booking") && strings.Contains(taskID, "-batch-") {
		// Find the last occurrence of date pattern (YYYY-MM-DD)
		parts := strings.Split(taskID, "-")
		if len(parts) >= 3 {
			// Check if last 3 parts form a date (YYYY-MM-DD)
			lastThree := parts[len(parts)-3:]
			if len(lastThree[0]) == 4 && len(lastThree[1]) == 2 && len(lastThree[2]) == 2 {
				// Remove the date suffix
				return strings.Join(parts[:len(parts)-3], "-")
			}
		}
	}
	
	// For other tasks, use the full task ID as batch ID
	return taskID
}

// ProcessTaskResult processes a crawler task result
func (cd *ChangeDetector) ProcessTaskResult(taskID string, result map[string]interface{}) error {
	if !cd.config.Enabled {
		return nil
	}

	ctx := context.Background()
	
	// Extract batch ID from task ID (tasks from same batch share prefix)
	batchID := extractBatchID(taskID)
	
	// Acquire processing lock to prevent duplicate processing
	lockValue, acquired := cd.distLock.AcquireProcessingLock(ctx, taskID, 30*time.Second)
	if !acquired {
		cd.logger.Debug("Task already being processed by another node", "task_id", taskID)
		return nil
	}
	defer cd.distLock.ReleaseProcessingLock(ctx, taskID, lockValue)
	
	cd.logger.Info("Processing task result for change detection", 
		"task_id", taskID,
		"batch_id", batchID)

	// Get source from result
	source, ok := result["source"].(string)
	if !ok {
		cd.logger.Warn("No source found in task result", "task_id", taskID)
		return fmt.Errorf("no source found in result")
	}

	// Get appropriate parser
	cd.mu.RLock()
	parser, exists := cd.parsers[source]
	cd.mu.RUnlock()

	if !exists {
		cd.logger.Warn("No parser found for source", "source", source)
		return fmt.Errorf("no parser found for source: %s", source)
	}

	// Parse venue data
	snapshots, err := parser.Parse(taskID, result)
	if err != nil {
		cd.logger.Error("Failed to parse venue data", 
			"error", err, 
			"source", source, 
			"task_id", taskID)
		return err
	}

	cd.logger.Info("Parsed venue snapshots", 
		"count", len(snapshots), 
		"source", source)

	// Batch get last snapshots for better performance
	lastSnapshots := cd.batchGetLastSnapshots(ctx, snapshots)
	
	// Prepare snapshots for batch save
	now := time.Now()
	snapshotsToSave := make([]interface{}, 0, len(snapshots))
	var changeEvents []types.ChangeEvent
	
	for i := range snapshots {
		snapshot := &snapshots[i]
		
		// Calculate state MD5
		stateMD5 := cd.calculateStateMD5(types.VenueState{
			Status:    snapshot.Status,
			Available: snapshot.Available,
			Price:     snapshot.Price,
		})
		snapshot.StateMD5 = stateMD5
		snapshot.CreatedAt = now
		
		// Get last snapshot from batch result
		lastKey := fmt.Sprintf("%s:%s:%s", snapshot.Source, snapshot.TimeSlot, snapshot.VenueID)
		lastSnapshot := lastSnapshots[lastKey]
		
		// Check for changes or initial state
		if lastSnapshot == nil {
			// No previous record - this is initial state
			event := cd.createInitialStateEvent(snapshot)
			event.ID = fmt.Sprintf("%s:%s:%s:initial", snapshot.Source, snapshot.TimeSlot, snapshot.VenueID)
			changeEvents = append(changeEvents, event)
			
			cd.logger.Debug("Detected initial venue state",
				"source", snapshot.Source,
				"time_slot", snapshot.TimeSlot,
				"venue", snapshot.VenueName,
				"status", snapshot.Status)
		} else if lastSnapshot.StateMD5 != stateMD5 {
			// State changed from previous
			event := cd.createChangeEvent(lastSnapshot, snapshot)
			event.ID = fmt.Sprintf("%s:%s:%s:change", snapshot.Source, snapshot.TimeSlot, snapshot.VenueID)
			changeEvents = append(changeEvents, event)
			
			cd.logger.Debug("Detected venue change",
				"source", snapshot.Source,
				"time_slot", snapshot.TimeSlot,
				"venue", snapshot.VenueName,
				"old_status", lastSnapshot.Status,
				"new_status", snapshot.Status)
		}
		
		snapshotsToSave = append(snapshotsToSave, snapshot)
	}
	
	// Batch save all snapshots
	if len(snapshotsToSave) > 0 {
		if err := cd.batchSaveSnapshots(ctx, snapshotsToSave); err != nil {
			cd.logger.Error("Failed to batch save snapshots", "error", err)
		}
	}

	// Store change events for batch tracking
	if len(changeEvents) > 0 || len(snapshotsToSave) > 0 {
		// Store events and check if batch is complete
		if cd.storeBatchEvents(ctx, batchID, taskID, changeEvents) {
			// Batch is complete, process all events
			cd.processBatchComplete(ctx, batchID)
		}
	}

	return nil
}

// calculateStateMD5 calculates MD5 hash of venue state
func (cd *ChangeDetector) calculateStateMD5(state types.VenueState) string {
	// Only include fields that users care about
	data := fmt.Sprintf("%s|%d|%.2f", 
		state.Status, 
		state.Available, 
		state.Price)
	
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// getLastSnapshot retrieves the last snapshot for a venue
func (cd *ChangeDetector) getLastSnapshot(ctx context.Context, source, timeSlot, venueID string) (*types.VenueSnapshot, error) {
	filter := bson.M{
		"source":    source,
		"time_slot": timeSlot,
		"venue_id":  venueID,
	}

	opts := options.FindOne().SetSort(bson.D{{"crawled_at", -1}})

	var snapshot types.VenueSnapshot
	err := cd.mongodb.Collection("venue_snapshots").FindOne(ctx, filter, opts).Decode(&snapshot)
	
	return &snapshot, err
}

// batchGetLastSnapshots batch retrieves last snapshots for multiple venues
func (cd *ChangeDetector) batchGetLastSnapshots(ctx context.Context, snapshots []types.VenueSnapshot) map[string]*types.VenueSnapshot {
	result := make(map[string]*types.VenueSnapshot)
	
	if len(snapshots) == 0 {
		return result
	}
	
	// Build aggregation pipeline for batch query
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"$or": cd.buildBatchFilter(snapshots),
			},
		},
		{
			"$sort": bson.M{"crawled_at": -1},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"source":    "$source",
					"time_slot": "$time_slot",
					"venue_id":  "$venue_id",
				},
				"doc": bson.M{"$first": "$$ROOT"},
			},
		},
	}
	
	cursor, err := cd.mongodb.Collection("venue_snapshots").Aggregate(ctx, pipeline)
	if err != nil {
		cd.logger.Error("Failed to batch get last snapshots", "error", err)
		return result
	}
	defer cursor.Close(ctx)
	
	for cursor.Next(ctx) {
		var item struct {
			ID  bson.M         `bson:"_id"`
			Doc types.VenueSnapshot `bson:"doc"`
		}
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		
		key := fmt.Sprintf("%s:%s:%s", 
			item.Doc.Source, 
			item.Doc.TimeSlot, 
			item.Doc.VenueID)
		result[key] = &item.Doc
	}
	
	return result
}

// buildBatchFilter builds OR filter for batch query
func (cd *ChangeDetector) buildBatchFilter(snapshots []types.VenueSnapshot) []bson.M {
	filters := make([]bson.M, 0, len(snapshots))
	seen := make(map[string]bool)
	
	for _, snapshot := range snapshots {
		key := fmt.Sprintf("%s:%s:%s", snapshot.Source, snapshot.TimeSlot, snapshot.VenueID)
		if !seen[key] {
			seen[key] = true
			filters = append(filters, bson.M{
				"source":    snapshot.Source,
				"time_slot": snapshot.TimeSlot,
				"venue_id":  snapshot.VenueID,
			})
		}
	}
	
	return filters
}

// batchSaveSnapshots batch saves multiple snapshots
func (cd *ChangeDetector) batchSaveSnapshots(ctx context.Context, snapshots []interface{}) error {
	if len(snapshots) == 0 {
		return nil
	}
	
	// Use InsertMany for batch insert
	_, err := cd.mongodb.Collection("venue_snapshots").InsertMany(ctx, snapshots)
	if err != nil {
		return fmt.Errorf("batch insert failed: %w", err)
	}
	
	cd.logger.Debug("Batch saved snapshots", "count", len(snapshots))
	return nil
}

// createInitialStateEvent creates an event for initial venue state
func (cd *ChangeDetector) createInitialStateEvent(snapshot *types.VenueSnapshot) types.ChangeEvent {
	return types.ChangeEvent{
		ID:           primitive.NewObjectID().Hex(),
		Source:       snapshot.Source,
		TimeSlot:     snapshot.TimeSlot,
		VenueID:      snapshot.VenueID,
		VenueName:    snapshot.VenueName,
		OldStatus:    "", // No previous state
		NewStatus:    snapshot.Status,
		OldAvailable: 0,  // No previous state
		NewAvailable: snapshot.Available,
		OldPrice:     0,  // No previous state
		NewPrice:     snapshot.Price,
		Timestamp:    snapshot.CrawledAt,
		Details:      snapshot.RawData,
		IsInitial:    true, // Mark as initial state
	}
}

// createChangeEvent creates a change event from two snapshots
func (cd *ChangeDetector) createChangeEvent(old, new *types.VenueSnapshot) types.ChangeEvent {
	return types.ChangeEvent{
		ID:           primitive.NewObjectID().Hex(),
		Source:       new.Source,
		TimeSlot:     new.TimeSlot,
		VenueID:      new.VenueID,
		VenueName:    new.VenueName,
		OldStatus:    old.Status,
		NewStatus:    new.Status,
		OldAvailable: old.Available,
		NewAvailable: new.Available,
		OldPrice:     old.Price,
		NewPrice:     new.Price,
		Timestamp:    new.CrawledAt,
		Details:      new.RawData,
		IsInitial:    false, // Mark as change event
	}
}

// createAggregatedEvent creates an aggregated event from multiple time slot events
func (cd *ChangeDetector) createAggregatedEvent(events []types.ChangeEvent) types.ChangeEvent {
	if len(events) == 0 {
		return types.ChangeEvent{}
	}
	
	// Use first event as base
	first := events[0]
	aggregated := types.ChangeEvent{
		ID:        primitive.NewObjectID().Hex(),
		Source:    first.Source,
		VenueID:   first.VenueID,
		VenueName: first.VenueName,
		Timestamp: time.Now(),
		IsInitial: first.IsInitial,
		Details:   make(map[string]interface{}),
	}
	
	// Aggregate all time slot changes
	timeSlotChanges := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		change := map[string]interface{}{
			"time_slot":     event.TimeSlot,
			"old_status":    event.OldStatus,
			"new_status":    event.NewStatus,
			"old_available": event.OldAvailable,
			"new_available": event.NewAvailable,
			"old_price":     event.OldPrice,
			"new_price":     event.NewPrice,
		}
		timeSlotChanges = append(timeSlotChanges, change)
	}
	
	// Store aggregated changes in details
	aggregated.Details["time_slot_changes"] = timeSlotChanges
	aggregated.Details["total_changes"] = len(events)
	
	// Set a summary in TimeSlot field
	aggregated.TimeSlot = fmt.Sprintf("%d个时段变化", len(events))
	
	return aggregated
}

// processChangeEvents processes detected change events
func (cd *ChangeDetector) processChangeEvents(events []types.ChangeEvent) {
	ctx := context.Background()
	
	if len(events) == 0 {
		return
	}
	
	// Sort events for consistent MD5 calculation
	sortedEvents := cd.sortEventsForMD5(events)
	
	// Calculate MD5 for this batch of changes
	batchMD5 := cd.calculateBatchMD5(sortedEvents)
	
	// Check if we've already sent notification for this exact set of changes
	notificationKey := "crawler:monitor:batch_notification"
	lastMD5, _ := cd.redis.Get(ctx, notificationKey).Result()
	
	if lastMD5 == batchMD5 {
		cd.logger.Info("Batch changes already notified, skipping",
			"batch_md5", batchMD5,
			"events_count", len(events))
		return
	}
	
	// Collect all unique subscribers from all time slots
	allSubscribers := make(map[string]types.Subscriber)
	for _, event := range events {
		subscribers := cd.subClient.GetSubscribers(ctx, event.Source, event.TimeSlot)
		for _, sub := range subscribers {
			allSubscribers[sub.UserID] = sub
		}
	}
	
	if len(allSubscribers) == 0 {
		cd.logger.Debug("No subscribers for batch changes")
		return
	}
	
	// Convert map to slice
	subscriberList := make([]types.Subscriber, 0, len(allSubscribers))
	for _, sub := range allSubscribers {
		subscriberList = append(subscriberList, sub)
	}
	
	cd.logger.Info("Sending batch notification",
		"total_changes", len(events),
		"subscriber_count", len(subscriberList),
		"batch_md5", batchMD5)
	
	// Create batch notification event
	batchEvent := cd.createBatchNotificationEvent(sortedEvents)
	
	// Send notification
	cd.pusher.Push(subscriberList, batchEvent)
	
	// Store MD5 to prevent duplicate notifications (with 30 minutes TTL)
	cd.redis.SetEX(ctx, notificationKey, batchMD5, 30*time.Minute)
}

// groupEventsBySourceAndVenue groups events by source and venue
func (cd *ChangeDetector) groupEventsBySourceAndVenue(events []types.ChangeEvent) map[string][]types.ChangeEvent {
	grouped := make(map[string][]types.ChangeEvent)
	
	for _, event := range events {
		key := fmt.Sprintf("%s:%s", event.Source, event.VenueID)
		grouped[key] = append(grouped[key], event)
	}
	
	return grouped
}

// groupEventsBySourceAndSlot groups events by source and time slot
func (cd *ChangeDetector) groupEventsBySourceAndSlot(events []types.ChangeEvent) map[string][]types.ChangeEvent {
	grouped := make(map[string][]types.ChangeEvent)
	
	for _, event := range events {
		key := fmt.Sprintf("%s:%s", event.Source, event.TimeSlot)
		grouped[key] = append(grouped[key], event)
	}
	
	return grouped
}

// parseGroupKey parses a group key into source and time slot
func (cd *ChangeDetector) parseGroupKey(key string) (source, timeSlot string) {
	// Simple split by colon
	// Note: This assumes source names don't contain colons
	parts := []byte(key)
	for i := 0; i < len(parts); i++ {
		if parts[i] == ':' {
			return string(parts[:i]), string(parts[i+1:])
		}
	}
	return key, ""
}

// sortEventsForMD5 sorts events for consistent MD5 calculation
func (cd *ChangeDetector) sortEventsForMD5(events []types.ChangeEvent) []types.ChangeEvent {
	sorted := make([]types.ChangeEvent, len(events))
	copy(sorted, events)
	
	// Sort by source, venue, time slot for consistency
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Source != sorted[j].Source {
			return sorted[i].Source < sorted[j].Source
		}
		if sorted[i].VenueID != sorted[j].VenueID {
			return sorted[i].VenueID < sorted[j].VenueID
		}
		return sorted[i].TimeSlot < sorted[j].TimeSlot
	})
	
	return sorted
}

// calculateBatchMD5 calculates MD5 for a batch of changes
func (cd *ChangeDetector) calculateBatchMD5(events []types.ChangeEvent) string {
	// Build a consistent string representation of all changes
	var builder strings.Builder
	
	for _, event := range events {
		// Include only core fields that represent the actual change
		builder.WriteString(fmt.Sprintf("%s|%s|%s|%s|%s|%.2f\n",
			event.Source,
			event.VenueID,
			event.VenueName,
			event.TimeSlot,
			event.NewStatus,
			event.NewPrice,
		))
	}
	
	hash := md5.Sum([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}

// createBatchNotificationEvent creates a single event for batch notification
func (cd *ChangeDetector) createBatchNotificationEvent(events []types.ChangeEvent) types.ChangeEvent {
	if len(events) == 0 {
		return types.ChangeEvent{}
	}
	
	// Group changes by venue for better organization
	venueChanges := make(map[string][]map[string]interface{})
	crawlDate := time.Now().Format("2006-01-02")
	
	for _, event := range events {
		change := map[string]interface{}{
			"time_slot":  event.TimeSlot,
			"old_status": event.OldStatus,
			"new_status": event.NewStatus,
			"old_price":  event.OldPrice,
			"new_price":  event.NewPrice,
		}
		
		// Extract date if available
		if event.Details != nil {
			if dateVal, ok := event.Details["date"]; ok {
				if date := getStringValue(dateVal); date != "" {
					crawlDate = date
				}
			}
		}
		
		venueChanges[event.VenueName] = append(venueChanges[event.VenueName], change)
	}
	
	// Create batch event
	batchEvent := types.ChangeEvent{
		ID:        primitive.NewObjectID().Hex(),
		Source:    events[0].Source,
		TimeSlot:  fmt.Sprintf("批次通知 - %d个时段变化", len(events)),
		VenueName: fmt.Sprintf("%s 场地预订状态更新", crawlDate),
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"date":           crawlDate,
			"total_changes":  len(events),
			"venue_count":    len(venueChanges),
			"venue_changes":  venueChanges,
		},
	}
	
	return batchEvent
}

// getStringValue safely extracts string value from interface{}
func getStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Start starts the change detector
func (cd *ChangeDetector) Start() error {
	cd.logger.Info("Starting change detector")
	
	// Start notification pusher
	if err := cd.pusher.Start(); err != nil {
		return fmt.Errorf("failed to start pusher: %w", err)
	}

	// Create indexes
	if err := cd.createIndexes(); err != nil {
		cd.logger.Error("Failed to create indexes", "error", err)
	}

	cd.logger.Info("Change detector started")
	return nil
}

// Stop stops the change detector
func (cd *ChangeDetector) Stop() error {
	cd.logger.Info("Stopping change detector")
	
	close(cd.stopChan)
	
	// Stop notification pusher
	if err := cd.pusher.Stop(); err != nil {
		cd.logger.Error("Failed to stop pusher", "error", err)
	}

	cd.logger.Info("Change detector stopped")
	return nil
}

// createIndexes creates MongoDB indexes
func (cd *ChangeDetector) createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := cd.mongodb.Collection("venue_snapshots")
	
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{"source", 1},
				{"time_slot", 1},
				{"venue_id", 1},
				{"crawled_at", -1},
			},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys: bson.D{
				{"task_id", 1},
			},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys: bson.D{
				{"created_at", 1},
			},
			Options: options.Index().
				SetBackground(true).
				SetExpireAfterSeconds(30 * 24 * 60 * 60), // 30 days TTL
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// GetStats returns statistics about change detection
func (cd *ChangeDetector) GetStats(ctx context.Context) (map[string]interface{}, error) {
	collection := cd.mongodb.Collection("venue_snapshots")
	
	// Count total snapshots
	totalCount, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	// Count today's snapshots
	todayStart := time.Now().Truncate(24 * time.Hour)
	todayCount, err := collection.CountDocuments(ctx, bson.M{
		"created_at": bson.M{"$gte": todayStart},
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_snapshots": totalCount,
		"today_snapshots": todayCount,
		"cache_items":     cd.cache.ItemCount(),
		"parsers":         len(cd.parsers),
	}, nil
}

// storeBatchEvents stores change events for a batch and returns true if batch is complete
func (cd *ChangeDetector) storeBatchEvents(ctx context.Context, batchID, taskID string, events []types.ChangeEvent) bool {
	// Store events in Redis with batch key
	batchKey := fmt.Sprintf("crawler:batch:events:%s", batchID)
	taskKey := fmt.Sprintf("crawler:batch:tasks:%s", batchID)
	
	// Store events as JSON
	if len(events) > 0 {
		eventsJSON, err := json.Marshal(events)
		if err != nil {
			cd.logger.Error("Failed to marshal events", "error", err)
			return false
		}
		
		// Store events for this task
		cd.redis.HSet(ctx, batchKey, taskID, eventsJSON)
		cd.redis.Expire(ctx, batchKey, 10*time.Minute) // TTL for cleanup
	}
	
	// Mark task as complete
	cd.redis.HSet(ctx, taskKey, taskID, time.Now().Unix())
	cd.redis.Expire(ctx, taskKey, 10*time.Minute)
	
	// Check if this is a multi-date batch (tennis_booking with dates)
	if strings.Contains(batchID, "tennis_booking") && strings.Contains(taskID, "-2025-") {
		// For tennis_booking, we expect 4 tasks (4 dates)
		taskCount, _ := cd.redis.HLen(ctx, taskKey).Result()
		
		cd.logger.Debug("Batch task progress", 
			"batch_id", batchID,
			"completed_tasks", taskCount,
			"expected_tasks", 4)
		
		// Return true when all 4 dates are complete
		return taskCount >= 4
	}
	
	// For other sources, process immediately
	return true
}

// processBatchComplete processes all events when a batch is complete
func (cd *ChangeDetector) processBatchComplete(ctx context.Context, batchID string) {
	cd.logger.Info("Processing complete batch", "batch_id", batchID)
	
	// Collect all events from the batch
	batchKey := fmt.Sprintf("crawler:batch:events:%s", batchID)
	eventsMap, err := cd.redis.HGetAll(ctx, batchKey).Result()
	if err != nil {
		cd.logger.Error("Failed to get batch events", "error", err, "batch_id", batchID)
		return
	}
	
	// Merge all events
	var allEvents []types.ChangeEvent
	for taskID, eventsJSON := range eventsMap {
		var events []types.ChangeEvent
		if err := json.Unmarshal([]byte(eventsJSON), &events); err != nil {
			cd.logger.Error("Failed to unmarshal events", "error", err, "task_id", taskID)
			continue
		}
		allEvents = append(allEvents, events...)
	}
	
	// Clean up batch data
	cd.redis.Del(ctx, batchKey)
	cd.redis.Del(ctx, fmt.Sprintf("crawler:batch:tasks:%s", batchID))
	
	// Process all events as a single batch
	if len(allEvents) > 0 {
		cd.logger.Info("Processing batch change events", 
			"batch_id", batchID,
			"total_events", len(allEvents))
		go cd.processChangeEvents(allEvents)
	} else {
		cd.logger.Debug("No change events in batch", "batch_id", batchID)
	}
}