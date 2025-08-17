package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/go-redis/redis/v8"
)

// RedisConfig configures Redis log store
type RedisConfig struct {
	Client     *redis.Client
	StreamKey  string // Redis Stream key
	PubSubKey  string // Redis PubSub channel
	MaxLen     int64  // Maximum stream length (0 = unlimited)
	BufferSize int    // Async buffer size
}

// RedisStore implements log Store using Redis
type RedisStore struct {
	client    *redis.Client
	streamKey string
	pubsubKey string
	maxLen    int64
	
	// Async processing
	buffer chan *logging.LogEntry
	stopCh chan struct{}
	wg     sync.WaitGroup
	
	// Subscription management
	pubsub      *redis.PubSub
	subscribers map[string]logging.Handler
	subMu       sync.RWMutex
}

// NewRedisStore creates a new Redis-based log store
func NewRedisStore(config RedisConfig) *RedisStore {
	if config.StreamKey == "" {
		config.StreamKey = "logs:stream"
	}
	if config.PubSubKey == "" {
		config.PubSubKey = "logs:pubsub"
	}
	if config.MaxLen == 0 {
		config.MaxLen = 1000
	}
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}

	store := &RedisStore{
		client:      config.Client,
		streamKey:   config.StreamKey,
		pubsubKey:   config.PubSubKey,
		maxLen:      config.MaxLen,
		buffer:      make(chan *logging.LogEntry, config.BufferSize),
		stopCh:      make(chan struct{}),
		subscribers: make(map[string]logging.Handler),
	}

	// Start async processor
	store.wg.Add(1)
	go store.processLoop()

	return store
}

// Write implements Writer interface
func (s *RedisStore) Write(ctx context.Context, entry *logging.LogEntry) error {
	select {
	case s.buffer <- entry:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Buffer full, write directly
		return s.writeEntry(ctx, entry)
	}
}

// Read implements Reader interface
func (s *RedisStore) Read(ctx context.Context, opts logging.ReadOptions) ([]*logging.LogEntry, error) {
	// Calculate range
	start := "-"
	end := "+"
	
	if !opts.Since.IsZero() {
		start = fmt.Sprintf("%d-0", opts.Since.UnixMilli())
	}
	if !opts.Until.IsZero() {
		end = fmt.Sprintf("%d-0", opts.Until.UnixMilli())
	}

	// Read from stream
	var cmd *redis.XMessageSliceCmd
	if opts.Ascending {
		cmd = s.client.XRange(ctx, s.streamKey, start, end)
	} else {
		cmd = s.client.XRevRange(ctx, s.streamKey, end, start)
	}

	messages, err := cmd.Result()
	if err != nil {
		return nil, err
	}

	// Convert to LogEntry
	entries := make([]*logging.LogEntry, 0, len(messages))
	for _, msg := range messages {
		entry := s.parseEntry(msg.Values)
		if entry == nil {
			continue
		}

		// Apply filters
		if opts.Level > 0 && entry.Level < opts.Level {
			continue
		}
		if opts.Source != "" && entry.Source != opts.Source {
			continue
		}
		if opts.Search != "" && !contains(entry.Message, opts.Search) {
			continue
		}

		entries = append(entries, entry)
		
		if opts.Limit > 0 && len(entries) >= opts.Limit {
			break
		}
	}

	return entries, nil
}

// Subscribe implements Subscriber interface
func (s *RedisStore) Subscribe(ctx context.Context, handler logging.Handler) error {
	// Generate unique ID for this subscription
	subID := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	
	s.subMu.Lock()
	s.subscribers[subID] = handler
	
	// Start pubsub if not already started
	// Use a background context for PubSub to avoid it being cancelled
	if s.pubsub == nil {
		// Use background context so PubSub doesn't stop when one subscriber cancels
		s.pubsub = s.client.Subscribe(context.Background(), s.pubsubKey)
		go s.handlePubSub(context.Background())
	}
	s.subMu.Unlock()

	// Return a wrapped context that removes subscription on cancel
	go func() {
		<-ctx.Done()
		s.subMu.Lock()
		delete(s.subscribers, subID)
		s.subMu.Unlock()
	}()

	return nil
}

// Unsubscribe implements Subscriber interface
func (s *RedisStore) Unsubscribe() error {
	if s.pubsub != nil {
		return s.pubsub.Close()
	}
	return nil
}

// Close implements Store interface
func (s *RedisStore) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	
	if s.pubsub != nil {
		s.pubsub.Close()
	}
	
	return nil
}

// processLoop handles async log processing
func (s *RedisStore) processLoop() {
	defer s.wg.Done()
	
	ctx := context.Background()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]*logging.LogEntry, 0, 10)

	for {
		select {
		case entry := <-s.buffer:
			batch = append(batch, entry)
			
			// Process batch if full
			if len(batch) >= 10 {
				s.writeBatch(ctx, batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Flush batch periodically
			if len(batch) > 0 {
				s.writeBatch(ctx, batch)
				batch = batch[:0]
			}

		case <-s.stopCh:
			// Flush remaining entries
			if len(batch) > 0 {
				s.writeBatch(ctx, batch)
			}
			return
		}
	}
}

// writeBatch writes multiple entries efficiently
func (s *RedisStore) writeBatch(ctx context.Context, entries []*logging.LogEntry) {
	pipe := s.client.Pipeline()
	
	for _, entry := range entries {
		s.addToStream(pipe, entry)
		s.publishEntry(pipe, entry)
	}
	
	pipe.Exec(ctx)
}

// writeEntry writes a single entry
func (s *RedisStore) writeEntry(ctx context.Context, entry *logging.LogEntry) error {
	pipe := s.client.Pipeline()
	s.addToStream(pipe, entry)
	s.publishEntry(pipe, entry)
	_, err := pipe.Exec(ctx)
	return err
}

// addToStream adds entry to Redis Stream
func (s *RedisStore) addToStream(pipe redis.Pipeliner, entry *logging.LogEntry) {
	values := map[string]interface{}{
		"timestamp": entry.Timestamp.UnixMilli(),
		"level":     entry.Level.String(),
		"message":   entry.Message,
	}
	
	if entry.Source != "" {
		values["source"] = entry.Source
	}
	
	if len(entry.Fields) > 0 {
		if data, err := json.Marshal(entry.Fields); err == nil {
			values["fields"] = string(data)
		}
	}

	pipe.XAdd(context.Background(), &redis.XAddArgs{
		Stream: s.streamKey,
		MaxLen: s.maxLen,
		Approx: true,
		Values: values,
	})
}

// publishEntry publishes to PubSub
func (s *RedisStore) publishEntry(pipe redis.Pipeliner, entry *logging.LogEntry) {
	if data, err := json.Marshal(entry); err == nil {
		pipe.Publish(context.Background(), s.pubsubKey, data)
	}
}

// handlePubSub handles incoming pubsub messages
func (s *RedisStore) handlePubSub(ctx context.Context) {
	ch := s.pubsub.Channel()
	
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed
				return
			}
			var entry logging.LogEntry
			if err := json.Unmarshal([]byte(msg.Payload), &entry); err == nil {
				s.notifySubscribers(&entry)
			}
		case <-ctx.Done():
			return
		case <-s.stopCh:
			// Stop when store is closed
			return
		}
	}
}

// notifySubscribers notifies all subscribers
func (s *RedisStore) notifySubscribers(entry *logging.LogEntry) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	
	for _, handler := range s.subscribers {
		handler(entry)
	}
}

// parseEntry parses Redis Stream values to LogEntry
func (s *RedisStore) parseEntry(values map[string]interface{}) *logging.LogEntry {
	entry := &logging.LogEntry{
		Fields: make(map[string]interface{}),
	}

	for key, val := range values {
		strVal := fmt.Sprintf("%v", val)
		
		switch key {
		case "timestamp":
			if ts, err := parseInt64(strVal); err == nil {
				// UnixMilli returns UTC time, no need to convert here
				entry.Timestamp = time.UnixMilli(ts)
			}
		case "level":
			entry.Level = logging.ParseLevel(strVal)
		case "source", "node_id":  // Support both source and node_id fields
			entry.Source = strVal
		case "message":
			entry.Message = strVal
		case "fields":
			json.Unmarshal([]byte(strVal), &entry.Fields)
		}
	}

	// Default timestamp if not found
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	return entry
}

// Helper functions
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && s[:len(substr)] == substr)
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}