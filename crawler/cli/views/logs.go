package views

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
)

// LogsView represents the logs viewer
type LogsView struct {
	// UI components
	view       *tview.Flex
	logsText   *tview.TextView
	filterForm *tview.Form
	statsText  *tview.TextView

	// Data
	redis  *redis.Client
	prefix string
	logger *slog.Logger
	logs   []LogEntry
	filter LogFilter

	// Auto-scroll
	autoScroll bool
	maxLines   int
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	NodeID    string    `json:"node_id"`
	Message   string    `json:"message"`
	Fields    map[string]interface{} `json:"fields"`
}

// LogFilter holds log filtering options
type LogFilter struct {
	Level  string
	NodeID string
	Search string
}

// LogsConfig holds logs view configuration
type LogsConfig struct {
	Redis  *redis.Client
	Prefix string
	Logger *slog.Logger
}

// NewLogsView creates a new logs view
func NewLogsView(config *LogsConfig) *LogsView {
	v := &LogsView{
		redis:      config.Redis,
		prefix:     config.Prefix,
		logger:     config.Logger,
		logs:       []LogEntry{},
		filter:     LogFilter{},
		autoScroll: true,
		maxLines:   1000,
	}

	// Create components
	v.createLogsText()
	v.createFilterForm()
	v.createStatsText()
	v.createLayout()

	return v
}

// createLogsText creates the logs text view
func (v *LogsView) createLogsText() {
	v.logsText = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			if v.autoScroll {
				v.logsText.ScrollToEnd()
			}
		})

	v.logsText.SetBorder(true).
		SetTitle(" Logs ").
		SetTitleAlign(tview.AlignLeft)
}

// createFilterForm creates the filter form
func (v *LogsView) createFilterForm() {
	v.filterForm = tview.NewForm()

	// Level filter
	levels := []string{"All", "Debug", "Info", "Warn", "Error"}
	v.filterForm.AddDropDown("Level", levels, 0, func(option string, index int) {
		if option == "All" {
			v.filter.Level = ""
		} else {
			v.filter.Level = strings.ToUpper(option)
		}
	})

	// Node filter
	v.filterForm.AddInputField("Node ID", "", 20, nil, func(text string) {
		v.filter.NodeID = text
	})

	// Search filter
	v.filterForm.AddInputField("Search", "", 30, nil, func(text string) {
		v.filter.Search = text
	})

	// Auto-scroll checkbox
	v.filterForm.AddCheckbox("Auto-scroll", v.autoScroll, func(checked bool) {
		v.autoScroll = checked
	})

	// Apply button
	v.filterForm.AddButton("Apply", func() {
		v.applyFilter()
	})

	// Clear button
	v.filterForm.AddButton("Clear Logs", func() {
		v.logsText.Clear()
		v.logs = []LogEntry{}
	})

	v.filterForm.SetBorder(true).
		SetTitle(" Filter ").
		SetTitleAlign(tview.AlignLeft)
}

// createStatsText creates the stats text view
func (v *LogsView) createStatsText() {
	v.statsText = tview.NewTextView().
		SetDynamicColors(true)

	v.statsText.SetBorder(true).
		SetTitle(" Statistics ").
		SetTitleAlign(tview.AlignLeft)
}

// createLayout creates the logs view layout
func (v *LogsView) createLayout() {
	// Right panel - filter and stats
	rightPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.filterForm, 11, 0, false).
		AddItem(v.statsText, 0, 1, false)

	// Main layout
	v.view = tview.NewFlex().
		AddItem(v.logsText, 0, 3, true).
		AddItem(rightPanel, 40, 0, false)
}

// GetView returns the logs view
func (v *LogsView) GetView() tview.Primitive {
	return v.view
}

// Refresh refreshes the logs data
func (v *LogsView) Refresh(ctx context.Context) {
	v.refreshLogs(ctx)
	v.updateStats()
}

// refreshLogs refreshes the logs from Redis
func (v *LogsView) refreshLogs(ctx context.Context) {
	// Get recent logs from Redis stream
	streamKey := fmt.Sprintf("%s:logs", v.prefix)
	
	// Read last 100 entries
	entries, err := v.redis.XRevRange(ctx, streamKey, "+", "-").Result()
	if err != nil && err != redis.Nil {
		// Try alternative: get from list
		v.refreshLogsFromList(ctx)
		return
	}

	// Parse and display logs
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		v.parseAndAddLog(entry.Values)
	}
}

// refreshLogsFromList refreshes logs from a list (fallback)
func (v *LogsView) refreshLogsFromList(ctx context.Context) {
	listKey := fmt.Sprintf("%s:logs:recent", v.prefix)
	
	// Get recent logs
	logs, err := v.redis.LRange(ctx, listKey, 0, 99).Result()
	if err != nil && err != redis.Nil {
		v.logger.Error("Failed to get logs", "error", err)
		return
	}

	// Clear existing logs if getting fresh batch
	if len(v.logs) == 0 {
		v.logsText.Clear()
	}

	// Parse and display logs
	for _, logData := range logs {
		var entry LogEntry
		if err := json.Unmarshal([]byte(logData), &entry); err != nil {
			// Try parsing as plain text
			entry = LogEntry{
				Timestamp: time.Now(),
				Level:     "INFO",
				Message:   logData,
			}
		}
		
		if v.matchesFilter(entry) {
			v.addLogEntry(entry)
		}
	}
}

// parseAndAddLog parses and adds a log entry
func (v *LogsView) parseAndAddLog(values map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Fields:    make(map[string]interface{}),
	}

	// Parse fields
	for key, value := range values {
		switch key {
		case "timestamp":
			if ts, ok := value.(string); ok {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					entry.Timestamp = t
				}
			}
		case "level":
			entry.Level = fmt.Sprintf("%v", value)
		case "node_id":
			entry.NodeID = fmt.Sprintf("%v", value)
		case "message", "msg":
			entry.Message = fmt.Sprintf("%v", value)
		default:
			entry.Fields[key] = value
		}
	}

	if v.matchesFilter(entry) {
		v.addLogEntry(entry)
	}
}

// addLogEntry adds a log entry to the display
func (v *LogsView) addLogEntry(entry LogEntry) {
	// Format log line
	var color string
	switch strings.ToUpper(entry.Level) {
	case "DEBUG":
		color = "gray"
	case "INFO":
		color = "white"
	case "WARN", "WARNING":
		color = "yellow"
	case "ERROR":
		color = "red"
	default:
		color = "white"
	}

	// Format timestamp
	timestamp := entry.Timestamp.Format("15:04:05.000")
	
	// Format node ID (short)
	nodeID := entry.NodeID
	if len(nodeID) > 8 {
		nodeID = nodeID[:8]
	}
	if nodeID == "" {
		nodeID = "system"
	}

	// Format message
	message := entry.Message
	if v.filter.Search != "" && strings.Contains(strings.ToLower(message), strings.ToLower(v.filter.Search)) {
		// Highlight search term
		message = strings.ReplaceAll(message, v.filter.Search, fmt.Sprintf("[yellow]%s[%s]", v.filter.Search, color))
	}

	// Add fields if present
	if len(entry.Fields) > 0 {
		fieldStrs := []string{}
		for k, v := range entry.Fields {
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%v", k, v))
		}
		message = fmt.Sprintf("%s [gray](%s)[%s]", message, strings.Join(fieldStrs, " "), color)
	}

	// Format and add line
	line := fmt.Sprintf("[gray]%s[white] [[%s]%-5s[white]] [cyan]%-8s[white] %s\n",
		timestamp, color, strings.ToUpper(entry.Level)[:5], nodeID, message)
	
	fmt.Fprint(v.logsText, line)
	
	// Add to logs array
	v.logs = append(v.logs, entry)
	
	// Trim if too many logs
	if len(v.logs) > v.maxLines {
		v.logs = v.logs[len(v.logs)-v.maxLines:]
		// TODO: Trim text view content as well
	}
}

// matchesFilter checks if a log entry matches the current filter
func (v *LogsView) matchesFilter(entry LogEntry) bool {
	if v.filter.Level != "" && !strings.EqualFold(entry.Level, v.filter.Level) {
		return false
	}
	if v.filter.NodeID != "" && !strings.Contains(entry.NodeID, v.filter.NodeID) {
		return false
	}
	if v.filter.Search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(v.filter.Search)) {
		return false
	}
	return true
}

// applyFilter applies the current filter
func (v *LogsView) applyFilter() {
	// Clear and re-add filtered logs
	v.logsText.Clear()
	for _, entry := range v.logs {
		if v.matchesFilter(entry) {
			v.addLogEntry(entry)
		}
	}
}

// updateStats updates the statistics display
func (v *LogsView) updateStats() {
	// Count logs by level
	counts := map[string]int{
		"DEBUG": 0,
		"INFO":  0,
		"WARN":  0,
		"ERROR": 0,
	}
	
	for _, entry := range v.logs {
		level := strings.ToUpper(entry.Level)
		if _, ok := counts[level]; ok {
			counts[level]++
		}
	}

	// Update stats display
	v.statsText.Clear()
	fmt.Fprintln(v.statsText, "[yellow]Log Statistics:[white]")
	fmt.Fprintln(v.statsText, "")
	fmt.Fprintf(v.statsText, "  Total:  %d\n", len(v.logs))
	fmt.Fprintf(v.statsText, "  Debug:  %d\n", counts["DEBUG"])
	fmt.Fprintf(v.statsText, "  Info:   %d\n", counts["INFO"])
	fmt.Fprintf(v.statsText, "  Warn:   %d\n", counts["WARN"])
	fmt.Fprintf(v.statsText, "  Error:  %d\n", counts["ERROR"])
	fmt.Fprintln(v.statsText, "")
	fmt.Fprintln(v.statsText, "[yellow]Settings:[white]")
	fmt.Fprintln(v.statsText, "")
	fmt.Fprintf(v.statsText, "  Auto-scroll: %v\n", v.autoScroll)
	fmt.Fprintf(v.statsText, "  Max lines:   %d\n", v.maxLines)
}