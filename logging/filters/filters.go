package filters

import (
	"regexp"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/logging"
)

// LevelFilter filters by minimum log level
type LevelFilter struct {
	MinLevel logging.Level
}

func NewLevelFilter(minLevel logging.Level) *LevelFilter {
	return &LevelFilter{MinLevel: minLevel}
}

func (f *LevelFilter) Filter(entry *logging.LogEntry) bool {
	return entry.Level >= f.MinLevel
}

// SourceFilter filters by source pattern
type SourceFilter struct {
	Pattern *regexp.Regexp
	Include bool // true = include matching, false = exclude matching
}

func NewSourceFilter(pattern string, include bool) (*SourceFilter, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &SourceFilter{Pattern: re, Include: include}, nil
}

func (f *SourceFilter) Filter(entry *logging.LogEntry) bool {
	matches := f.Pattern.MatchString(entry.Source)
	return matches == f.Include
}

// MessageFilter filters by message content
type MessageFilter struct {
	Contains []string
	Excludes []string
}

func NewMessageFilter() *MessageFilter {
	return &MessageFilter{
		Contains: make([]string, 0),
		Excludes: make([]string, 0),
	}
}

func (f *MessageFilter) AddContains(s string) *MessageFilter {
	f.Contains = append(f.Contains, s)
	return f
}

func (f *MessageFilter) AddExcludes(s string) *MessageFilter {
	f.Excludes = append(f.Excludes, s)
	return f
}

func (f *MessageFilter) Filter(entry *logging.LogEntry) bool {
	msg := strings.ToLower(entry.Message)
	
	// Check excludes first
	for _, exclude := range f.Excludes {
		if strings.Contains(msg, strings.ToLower(exclude)) {
			return false
		}
	}
	
	// Check contains (if any specified)
	if len(f.Contains) > 0 {
		for _, contain := range f.Contains {
			if strings.Contains(msg, strings.ToLower(contain)) {
				return true
			}
		}
		return false
	}
	
	return true
}

// RateLimitFilter limits log rate per source
type RateLimitFilter struct {
	rates map[string]*rateLimiter
	limit int
}

type rateLimiter struct {
	tokens   int
	lastFill int64
}

func NewRateLimitFilter(tokensPerSecond int) *RateLimitFilter {
	return &RateLimitFilter{
		rates: make(map[string]*rateLimiter),
		limit: tokensPerSecond,
	}
}

func (f *RateLimitFilter) Filter(entry *logging.LogEntry) bool {
	// Simple token bucket implementation
	limiter, exists := f.rates[entry.Source]
	if !exists {
		limiter = &rateLimiter{tokens: f.limit}
		f.rates[entry.Source] = limiter
	}
	
	now := entry.Timestamp.Unix()
	if now > limiter.lastFill {
		// Refill tokens
		elapsed := now - limiter.lastFill
		limiter.tokens = min(f.limit, limiter.tokens+int(elapsed))
		limiter.lastFill = now
	}
	
	if limiter.tokens > 0 {
		limiter.tokens--
		return true
	}
	
	return false
}

// CompositeFilter combines multiple filters with AND/OR logic
type CompositeFilter struct {
	filters []logging.Filter
	mode    string // "AND" or "OR"
}

func NewCompositeFilter(mode string) *CompositeFilter {
	return &CompositeFilter{
		filters: make([]logging.Filter, 0),
		mode:    mode,
	}
}

func (f *CompositeFilter) Add(filter logging.Filter) *CompositeFilter {
	f.filters = append(f.filters, filter)
	return f
}

func (f *CompositeFilter) Filter(entry *logging.LogEntry) bool {
	if len(f.filters) == 0 {
		return true
	}
	
	if f.mode == "OR" {
		for _, filter := range f.filters {
			if filter.Filter(entry) {
				return true
			}
		}
		return false
	}
	
	// Default to AND
	for _, filter := range f.filters {
		if !filter.Filter(entry) {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}