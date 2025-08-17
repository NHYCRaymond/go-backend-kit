package pipeline

// Config contains extended pipeline configuration
type Config struct {
	// Basic configuration
	Name        string `json:"name"`
	Concurrency int    `json:"concurrency"`
	
	// Processor flags
	EnableCleaner      bool `json:"enable_cleaner"`
	EnableValidator    bool `json:"enable_validator"`
	EnableDeduplicator bool `json:"enable_deduplicator"`
	EnableTransformer  bool `json:"enable_transformer"`
	EnableEnricher     bool `json:"enable_enricher"`
	
	// Processor configurations
	CleanerConfig      CleanerConfig    `json:"cleaner_config,omitempty"`
	ValidationRules    []ValidationRule `json:"validation_rules,omitempty"`
	DedupeKey          string           `json:"dedupe_key,omitempty"`
	Transforms         []Transform      `json:"transforms,omitempty"`
	
	// Advanced settings
	ErrorHandling      string `json:"error_handling"` // stop, skip, retry
	MaxRetries         int    `json:"max_retries"`
	BatchSize          int    `json:"batch_size"`
	EnableMetrics      bool   `json:"enable_metrics"`
	EnableProfiling    bool   `json:"enable_profiling"`
}

// DefaultConfig returns default pipeline configuration
func DefaultConfig() *Config {
	return &Config{
		Name:               "default",
		Concurrency:        1,
		EnableCleaner:      true,
		EnableValidator:    false,
		EnableDeduplicator: true,
		ErrorHandling:      "skip",
		MaxRetries:         3,
		BatchSize:          100,
		EnableMetrics:      true,
		CleanerConfig: CleanerConfig{
			TrimSpace:      true,
			RemoveEmpty:    true,
			NormalizeSpace: true,
		},
	}
}