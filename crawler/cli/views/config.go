package views

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
)

// ConfigView represents the configuration editor view
type ConfigView struct {
	// UI components
	view       *tview.Flex
	tree       *tview.TreeView
	editor     *tview.TextArea
	saveButton *tview.Button
	infoText   *tview.TextView

	// Data
	redis  *redis.Client
	prefix string
	logger *slog.Logger
	
	// Current config
	currentKey    string
	currentConfig map[string]interface{}
	modified      bool
}

// ConfigConfig holds config view configuration
type ConfigConfig struct {
	Redis  *redis.Client
	Prefix string
	Logger *slog.Logger
}

// NewConfigView creates a new config view
func NewConfigView(config *ConfigConfig) *ConfigView {
	v := &ConfigView{
		redis:         config.Redis,
		prefix:        config.Prefix,
		logger:        config.Logger,
		currentConfig: make(map[string]interface{}),
	}

	// Create components
	v.createTree()
	v.createEditor()
	v.createInfoText()
	v.createLayout()

	return v
}

// createTree creates the configuration tree
func (v *ConfigView) createTree() {
	root := tview.NewTreeNode("Configuration").
		SetColor(tview.Styles.SecondaryTextColor)

	v.tree = tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root)

	// Add configuration categories
	categories := []struct {
		name string
		key  string
	}{
		{"Crawler Settings", "crawler"},
		{"Node Settings", "node"},
		{"Task Settings", "task"},
		{"Fetcher Settings", "fetcher"},
		{"Pipeline Settings", "pipeline"},
		{"Redis Settings", "redis"},
		{"Monitoring", "monitoring"},
	}

	for _, cat := range categories {
		node := tview.NewTreeNode(cat.name).
			SetReference(cat.key).
			SetSelectable(true)
		root.AddChild(node)
		
		// Add sub-items
		v.addConfigItems(node, cat.key)
	}

	// Set selection handler
	v.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		ref := node.GetReference()
		if ref != nil {
			v.loadConfig(ref.(string))
		}
	})

	v.tree.SetBorder(true).
		SetTitle(" Configuration Tree ").
		SetTitleAlign(tview.AlignLeft)
}

// addConfigItems adds configuration items to a tree node
func (v *ConfigView) addConfigItems(parent *tview.TreeNode, category string) {
	items := v.getConfigItems(category)
	for _, item := range items {
		key := fmt.Sprintf("%s:%s", category, item)
		node := tview.NewTreeNode(item).
			SetReference(key).
			SetSelectable(true)
		parent.AddChild(node)
	}
}

// getConfigItems returns configuration items for a category
func (v *ConfigView) getConfigItems(category string) []string {
	switch category {
	case "crawler":
		return []string{"mode", "workers", "timeout", "retry"}
	case "node":
		return []string{"max_workers", "queue_size", "heartbeat", "capabilities"}
	case "task":
		return []string{"max_retries", "timeout", "priority", "batch_size"}
	case "fetcher":
		return []string{"user_agent", "timeout", "rate_limit", "proxy"}
	case "pipeline":
		return []string{"processors", "concurrency", "buffer_size"}
	case "redis":
		return []string{"address", "password", "db", "pool_size"}
	case "monitoring":
		return []string{"metrics", "logging", "tracing", "alerts"}
	default:
		return []string{}
	}
}

// createEditor creates the configuration editor
func (v *ConfigView) createEditor() {
	v.editor = tview.NewTextArea().
		SetPlaceholder("Select a configuration item to edit...")
	
	// Set change handler
	v.editor.SetChangedFunc(func() {
		v.modified = true
		v.updateInfo()
	})

	// Create save button
	v.saveButton = tview.NewButton("Save").
		SetSelectedFunc(func() {
			v.saveConfig()
		})

	v.editor.SetBorder(true).
		SetTitle(" Editor ").
		SetTitleAlign(tview.AlignLeft)
}

// createInfoText creates the info text view
func (v *ConfigView) createInfoText() {
	v.infoText = tview.NewTextView().
		SetDynamicColors(true)

	v.infoText.SetBorder(true).
		SetTitle(" Information ").
		SetTitleAlign(tview.AlignLeft)

	v.updateInfo()
}

// createLayout creates the config view layout
func (v *ConfigView) createLayout() {
	// Right panel - editor and info
	rightPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.editor, 0, 2, true).
		AddItem(v.saveButton, 3, 0, false).
		AddItem(v.infoText, 10, 0, false)

	// Main layout
	v.view = tview.NewFlex().
		AddItem(v.tree, 40, 0, true).
		AddItem(rightPanel, 0, 1, false)
}

// GetView returns the config view
func (v *ConfigView) GetView() tview.Primitive {
	return v.view
}

// Refresh refreshes the config data
func (v *ConfigView) Refresh(ctx context.Context) {
	// Reload current config if selected
	if v.currentKey != "" {
		v.loadConfig(v.currentKey)
	}
}

// loadConfig loads a configuration item
func (v *ConfigView) loadConfig(key string) {
	ctx := context.Background()
	v.currentKey = key
	
	// Get config from Redis
	configKey := fmt.Sprintf("%s:config:%s", v.prefix, key)
	data, err := v.redis.Get(ctx, configKey).Result()
	if err != nil && err != redis.Nil {
		v.logger.Error("Failed to load config", "key", key, "error", err)
		v.editor.SetText("# Error loading configuration", false)
		return
	}

	// If no data, show default template
	if err == redis.Nil || data == "" {
		data = v.getDefaultConfig(key)
	}

	// Parse JSON for validation
	if err := json.Unmarshal([]byte(data), &v.currentConfig); err != nil {
		// If not valid JSON, treat as raw text
		v.currentConfig = map[string]interface{}{
			"raw": data,
		}
	}

	// Set editor text
	v.editor.SetText(data, false)
	v.modified = false
	v.updateInfo()
}

// getDefaultConfig returns default configuration for a key
func (v *ConfigView) getDefaultConfig(key string) string {
	defaults := map[string]string{
		"crawler:mode": `{
  "mode": "distributed",
  "description": "Crawler operation mode"
}`,
		"node:max_workers": `{
  "value": 10,
  "min": 1,
  "max": 100,
  "description": "Maximum number of concurrent workers"
}`,
		"task:max_retries": `{
  "value": 3,
  "min": 0,
  "max": 10,
  "description": "Maximum retry attempts for failed tasks"
}`,
		"fetcher:user_agent": `{
  "value": "Mozilla/5.0 (compatible; Crawler/1.0)",
  "description": "User agent string for HTTP requests"
}`,
	}

	if config, ok := defaults[key]; ok {
		return config
	}

	// Generic default
	return fmt.Sprintf(`{
  "key": "%s",
  "value": null,
  "description": "Configuration for %s"
}`, key, key)
}

// saveConfig saves the current configuration
func (v *ConfigView) saveConfig() {
	if !v.modified || v.currentKey == "" {
		return
	}

	ctx := context.Background()
	configKey := fmt.Sprintf("%s:config:%s", v.prefix, v.currentKey)
	
	// Get editor text
	text := v.editor.GetText()
	
	// Validate JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(text), &config); err != nil {
		v.updateInfo()
		v.logger.Error("Invalid JSON", "error", err)
		return
	}

	// Save to Redis
	if err := v.redis.Set(ctx, configKey, text, 0).Err(); err != nil {
		v.logger.Error("Failed to save config", "key", v.currentKey, "error", err)
		return
	}

	v.currentConfig = config
	v.modified = false
	v.updateInfo()
	
	// Publish config change event
	eventKey := fmt.Sprintf("%s:events:config", v.prefix)
	event := map[string]interface{}{
		"type": "config_changed",
		"key":  v.currentKey,
		"data": config,
	}
	eventData, _ := json.Marshal(event)
	v.redis.Publish(ctx, eventKey, eventData)
}

// updateInfo updates the info display
func (v *ConfigView) updateInfo() {
	v.infoText.Clear()
	
	fmt.Fprintln(v.infoText, "[yellow]Current Configuration:[white]")
	fmt.Fprintln(v.infoText, "")
	
	if v.currentKey != "" {
		fmt.Fprintf(v.infoText, "  Key:      %s\n", v.currentKey)
		fmt.Fprintf(v.infoText, "  Modified: %v\n", v.modified)
		fmt.Fprintln(v.infoText, "")
		
		// Validate JSON
		text := v.editor.GetText()
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(text), &config); err != nil {
			fmt.Fprintln(v.infoText, "[red]JSON Validation Error:[white]")
			fmt.Fprintf(v.infoText, "  %v\n", err)
		} else {
			fmt.Fprintln(v.infoText, "[green]JSON Valid[white]")
			fmt.Fprintln(v.infoText, "")
			fmt.Fprintln(v.infoText, "[yellow]Fields:[white]")
			for key := range config {
				fmt.Fprintf(v.infoText, "  - %s\n", key)
			}
		}
	} else {
		fmt.Fprintln(v.infoText, "  No configuration selected")
	}
	
	fmt.Fprintln(v.infoText, "")
	fmt.Fprintln(v.infoText, "[yellow]Keyboard Shortcuts:[white]")
	fmt.Fprintln(v.infoText, "")
	fmt.Fprintln(v.infoText, "  Ctrl+S - Save configuration")
	fmt.Fprintln(v.infoText, "  Tab    - Switch focus")
	fmt.Fprintln(v.infoText, "  F5     - Refresh")
}