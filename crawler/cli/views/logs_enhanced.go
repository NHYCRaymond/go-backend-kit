package views

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/NHYCRaymond/go-backend-kit/logging/filters"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EnhancedLogsView represents an enhanced logs viewer using the modular logging system
type EnhancedLogsView struct {
	// UI components
	pages       *tview.Pages        // Pages for main view and modals
	view        *tview.Flex
	logsText    *tview.TextView
	statusBar   *tview.TextView
	app         *tview.Application  // Reference to trigger redraws
	
	// Logging system
	store       logging.Store
	filter      *filters.CompositeFilter
	
	// State
	logs        []*logging.LogEntry
	maxLogs     int
	autoScroll  bool
	paused      bool
	scrollRow   int  // Current scroll position
	scrollCol   int  // Current scroll column
	
	// Stats
	stats       LogStats
	
	// Filters
	nodes          []string  // List of discovered nodes
	selectedNode   string    // Currently selected node filter
	selectedLevel  string    // Currently selected level filter
	searchText     string    // Current search text
	
	// Refresh control
	refreshTicker *time.Ticker
	dirty         bool  // Flag to indicate if display needs refresh
	
	// Synchronization
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// LogStats tracks log statistics
type LogStats struct {
	Total  int
	Levels map[logging.Level]int
	Sources map[string]int
	Rate   float64
	LastUpdate time.Time
}

// EnhancedLogsConfig holds configuration
type EnhancedLogsConfig struct {
	Store      logging.Store
	MaxLogs    int
	AutoScroll bool
}

// NewEnhancedLogsView creates an enhanced logs view
func NewEnhancedLogsView(config *EnhancedLogsConfig) *EnhancedLogsView {
	ctx, cancel := context.WithCancel(context.Background())
	
	if config.MaxLogs == 0 {
		config.MaxLogs = 1000
	}
	
	v := &EnhancedLogsView{
		pages:         tview.NewPages(),
		store:         config.Store,
		filter:        filters.NewCompositeFilter("AND"),
		logs:          make([]*logging.LogEntry, 0, config.MaxLogs),
		maxLogs:       config.MaxLogs,
		autoScroll:    config.AutoScroll,
		nodes:         []string{"ALL"},  // Start with ALL option
		selectedNode:  "ALL",
		selectedLevel: "ALL",
		ctx:           ctx,
		cancel:        cancel,
		stats: LogStats{
			Levels:  make(map[logging.Level]int),
			Sources: make(map[string]int),
		},
	}
	
	v.createUI()
	v.startSubscription()
	v.loadInitialLogs()
	v.startPeriodicRefresh()  // Start the periodic refresh
	
	// Don't schedule immediate refresh - let periodic refresh handle it
	
	return v
}

// createUI creates the user interface
func (v *EnhancedLogsView) createUI() {
	// Main log display
	v.logsText = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			if v.autoScroll && !v.paused {
				v.logsText.ScrollToEnd()
			}
		})
	
	v.logsText.SetBorder(true).
		SetTitle("[cyan][ Logs ][white] Press [yellow]'n'[white] for nodes, [yellow]'l'[white] for levels")
	
	// Status bar
	v.statusBar = tview.NewTextView().
		SetDynamicColors(true)
	
	v.statusBar.SetBorder(true).
		SetTitle("[yellow][ Status ][white]")
	
	// Simple layout - no filterForm
	v.view = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.logsText, 0, 1, true).
		AddItem(v.statusBar, 3, 0, false)
	
	// Add main view to pages
	v.pages.AddPage("main", v.view, true, true)
	
	// Setup keyboard handlers
	v.setupKeyboardHandlers()
	
	// Ensure pages passes through F keys
	v.pages.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Always pass through function keys to global handler
		if event.Key() >= tcell.KeyF1 && event.Key() <= tcell.KeyF12 {
			return event
		}
		// Pass all other events through as well
		return event
	})
}

// showNodeSelector shows a popup modal to select node
func (v *EnhancedLogsView) showNodeSelector() {
	// Update nodes list
	v.updateNodesList()
	
	// Create buttons for each node
	buttons := make([]string, len(v.nodes))
	for i, node := range v.nodes {
		if node == v.selectedNode {
			buttons[i] = fmt.Sprintf("[yellow]▶ %s[white]", node)
		} else {
			buttons[i] = node
		}
	}
	buttons = append(buttons, "Cancel")
	
	// Create modal
	modal := tview.NewModal().
		SetText("Select Node to View Logs:").
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex < len(v.nodes) {
				// Node selected
				v.selectedNode = v.nodes[buttonIndex]
				v.clearLogs()
				v.updateFilters()
			}
			// Remove modal and return focus to main view
			v.pages.RemovePage("modal")
			if v.app != nil {
				v.app.SetFocus(v.logsText)
			}
		})
	
	// Show modal as overlay
	v.pages.AddPage("modal", modal, true, true)
	if v.app != nil {
		v.app.SetFocus(modal)
	}
}

// showLevelSelector shows a popup modal to select log level
func (v *EnhancedLogsView) showLevelSelector() {
	levels := []string{"ALL", "DEBUG", "INFO", "WARN", "ERROR"}
	
	// Create buttons for each level
	buttons := make([]string, len(levels))
	for i, level := range levels {
		if level == v.selectedLevel {
			buttons[i] = fmt.Sprintf("[yellow]▶ %s[white]", level)
		} else {
			buttons[i] = level
		}
	}
	buttons = append(buttons, "Cancel")
	
	// Create modal
	modal := tview.NewModal().
		SetText("Select Log Level:").
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex < len(levels) {
				// Level selected
				v.selectedLevel = levels[buttonIndex]
				v.updateFilters()
			}
			// Remove modal and return focus to main view
			v.pages.RemovePage("modal")
			if v.app != nil {
				v.app.SetFocus(v.logsText)
			}
		})
	
	// Show modal as overlay
	v.pages.AddPage("modal", modal, true, true)
	if v.app != nil {
		v.app.SetFocus(modal)
	}
}

// updateNodesList updates the list of available nodes from logs
func (v *EnhancedLogsView) updateNodesList() {
	// Get unique nodes from logs
	nodeMap := make(map[string]bool)
	v.mu.RLock()
	for _, log := range v.logs {
		if nodeID, ok := log.Fields["node_id"].(string); ok && nodeID != "" {
			nodeMap[nodeID] = true
		}
	}
	v.mu.RUnlock()
	
	// Reset nodes list
	v.nodes = []string{"ALL"}
	for node := range nodeMap {
		v.nodes = append(v.nodes, node)
	}
}

// updateFilters updates the active filters
func (v *EnhancedLogsView) updateFilters() {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Clear existing filters
	v.filter = filters.NewCompositeFilter("AND")
	
	// Apply level filter
	if v.selectedLevel != "ALL" && v.selectedLevel != "" {
		level := logging.ParseLevel(v.selectedLevel)
		v.filter.Add(filters.NewLevelFilter(level))
	}
	
	// Apply search filter (using MessageFilter)
	if v.searchText != "" {
		msgFilter := filters.NewMessageFilter()
		msgFilter.AddContains(v.searchText)
		v.filter.Add(msgFilter)
	}
	
	// Note: Node filtering will be done in the display logic
	// since there's no built-in field filter
	
	// Trigger refresh
	v.dirty = true
}

// setupKeyboardHandlers sets up keyboard shortcuts
func (v *EnhancedLogsView) setupKeyboardHandlers() {
	// Don't use v.view.SetInputCapture as it blocks global hotkeys
	// Instead, set input capture only on specific components
	
	// Specific keyboard handler for logs text
	v.logsText.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Always pass through function keys to global handler
		if event.Key() >= tcell.KeyF1 && event.Key() <= tcell.KeyF12 {
			return event
		}
		
		// Handle component-specific keys only
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'n', 'N':
				// Show node selector popup
				v.showNodeSelector()
				return nil
			case 'l', 'L':
				// Show level selector popup
				v.showLevelSelector()
				return nil
			case ' ':
				v.togglePause()
				return nil
			case 'c', 'C':
				v.clearLogs()
				return nil
			case 'g':
				v.logsText.ScrollToBeginning()
				return nil
			case 'G':
				v.logsText.ScrollToEnd()
				return nil
			case '/':
				// TODO: Implement search popup
				return nil
			}
		}
		// Pass through all other keys (including function keys)
		return event
	})
	// We need to preserve that to ensure function keys pass through
}

// startSubscription starts real-time log subscription
func (v *EnhancedLogsView) startSubscription() {
	go func() {
		err := v.store.Subscribe(v.ctx, func(entry *logging.LogEntry) {
			// Add to UI in a separate goroutine to avoid blocking
			if !v.paused {
				v.addLog(entry)
			}
		})
		if err != nil {
			// Handle error - maybe show in status bar
			fmt.Printf("Subscribe error: %v\n", err)
		}
	}()
}

// startPeriodicRefresh starts a ticker to refresh the display periodically
func (v *EnhancedLogsView) startPeriodicRefresh() {
	v.refreshTicker = time.NewTicker(1 * time.Second)
	
	go func() {
		for {
			select {
			case <-v.refreshTicker.C:
				// Check if we need to refresh without holding lock
				v.mu.RLock()
				needRefresh := v.app != nil && v.dirty
				v.mu.RUnlock()
				
				if needRefresh {
					// Do the refresh in QueueUpdateDraw to avoid concurrency issues
					if v.app != nil {
						v.app.QueueUpdateDraw(func() {
							v.mu.Lock()
							defer v.mu.Unlock()
							
							// Save scroll position
							row, col := v.logsText.GetScrollOffset()
							v.scrollRow = row
							v.scrollCol = col
							
							// Refresh display
							v.refreshLogsDisplay()
							
							// Restore scroll position if not auto-scrolling
							if !v.autoScroll || v.paused {
								v.logsText.ScrollTo(v.scrollRow, v.scrollCol)
							} else {
								v.logsText.ScrollToEnd()
							}
							
							// Update status bar
							v.updateStatusBar()
							
							// Clear dirty flag
							v.dirty = false
						})
					}
				}
				
			case <-v.ctx.Done():
				return
			}
		}
	}()
}

// loadInitialLogs loads recent logs from store
func (v *EnhancedLogsView) loadInitialLogs() {
	go func() {
		opts := logging.ReadOptions{
			Limit:     v.maxLogs,
			Ascending: false,
		}
		
		logs, err := v.store.Read(v.ctx, opts)
		if err != nil {
			fmt.Printf("Error loading initial logs: %v\n", err)
			return
		}
		
		// Add all logs in batch (in reverse order - oldest first)
		v.mu.Lock()
		for i := len(logs) - 1; i >= 0; i-- {
			entry := logs[i]
			// Add to buffer
			v.logs = append(v.logs, entry)
			// Update node list
			v.updateNodeList(entry.Source)
			// Update stats
			v.updateStats(entry)
		}
		// Trim if needed
		if len(v.logs) > v.maxLogs {
			v.logs = v.logs[len(v.logs)-v.maxLogs:]
		}
		v.dirty = true
		v.mu.Unlock()
		// Don't try to refresh here - let the periodic refresh handle it
	}()
}

// addLog adds a log entry to the display
func (v *EnhancedLogsView) addLog(entry *logging.LogEntry) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Always add to buffer (filters are applied only during display)
	v.logs = append(v.logs, entry)
	if len(v.logs) > v.maxLogs {
		// Remove oldest entries
		v.logs = v.logs[len(v.logs)-v.maxLogs:]
	}
	
	// Update node list if we see a new node
	v.updateNodeList(entry.Source)
	
	// Update stats (always count all logs)
	v.updateStats(entry)
	
	// Mark as dirty for next refresh cycle
	v.dirty = true
}

// formatLogLine formats a log entry for display
func (v *EnhancedLogsView) formatLogLine(entry *logging.LogEntry) string {
	// Color based on level
	levelColor := v.getLevelColor(entry.Level)
	
	// Format timestamp - use local time
	timestamp := entry.Timestamp.Local().Format("15:04:05.000")
	
	// Format source (truncate if needed)
	source := entry.Source
	if len(source) > 15 {
		source = "..." + source[len(source)-12:]
	}
	
	// Format message
	message := entry.Message
	if len(entry.Fields) > 0 {
		// Add fields as dimmed text
		var fieldParts []string
		for k, v := range entry.Fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		message += fmt.Sprintf(" [dim]%s[white]", strings.Join(fieldParts, " "))
	}
	
	// Build formatted line
	return fmt.Sprintf("[gray]%s[white] [%s]%-5s[white] [cyan]%-15s[white] %s\n",
		timestamp,
		levelColor,
		entry.Level.String(),
		source,
		message,
	)
}

// getLevelColor returns color for log level
func (v *EnhancedLogsView) getLevelColor(level logging.Level) string {
	switch level {
	case logging.LevelDebug:
		return "gray"
	case logging.LevelInfo:
		return "white"
	case logging.LevelWarn:
		return "yellow"
	case logging.LevelError:
		return "red"
	case logging.LevelFatal:
		return "red::b"
	default:
		return "white"
	}
}

// updateStats updates statistics
func (v *EnhancedLogsView) updateStats(entry *logging.LogEntry) {
	v.stats.Total++
	v.stats.Levels[entry.Level]++
	v.stats.Sources[entry.Source]++
	
	// Calculate rate
	now := time.Now()
	if v.stats.LastUpdate.IsZero() {
		v.stats.LastUpdate = now
	} else {
		elapsed := now.Sub(v.stats.LastUpdate).Seconds()
		if elapsed > 0 {
			v.stats.Rate = 1.0 / elapsed
		}
		v.stats.LastUpdate = now
	}
}

// refreshLogsDisplay refreshes the entire logs display
func (v *EnhancedLogsView) refreshLogsDisplay() {
	// Note: This method should be called with the lock already held
	
	// Clear the text view
	v.logsText.Clear()
	
	if len(v.logs) == 0 {
		fmt.Fprint(v.logsText, "[gray]No logs in buffer. Waiting for logs...[white]\n")
	}
	
	// Re-add all logs that pass the filter
	displayedCount := 0
	for _, entry := range v.logs {
		passes := v.filter.Filter(entry)
		if passes {
			line := v.formatLogLine(entry)
			fmt.Fprint(v.logsText, line)
			displayedCount++
		}
	}
	
	// Update title with filter info and total count
	title := fmt.Sprintf("[cyan][ Logs - Buffer: %d ][white]", len(v.logs))
	if v.selectedNode != "" && v.selectedNode != "ALL" {
		title = fmt.Sprintf("[cyan][ Logs - Node: %s - Buffer: %d ][white]", v.selectedNode, len(v.logs))
	}
	if displayedCount < len(v.logs) {
		title += fmt.Sprintf(" [yellow](Showing: %d)[white]", displayedCount)
	}
	v.logsText.SetTitle(title)
}

// updateStatusBar updates the status bar display
func (v *EnhancedLogsView) updateStatusBar() {
	v.statusBar.Clear()
	
	// Display stats
	fmt.Fprintln(v.statusBar, "[yellow]Statistics:[white]")
	fmt.Fprintf(v.statusBar, "  Total: %d\n", v.stats.Total)
	fmt.Fprintf(v.statusBar, "  Rate:  %.1f/s\n\n", v.stats.Rate)
	
	fmt.Fprintln(v.statusBar, "[yellow]Levels:[white]")
	for level := logging.LevelDebug; level <= logging.LevelFatal; level++ {
		count := v.stats.Levels[level]
		if count > 0 {
			color := v.getLevelColor(level)
			fmt.Fprintf(v.statusBar, "  [%s]%-5s[white]: %d\n", 
				color, level.String(), count)
		}
	}
	
	fmt.Fprintln(v.statusBar, "\n[yellow]Top Sources:[white]")
	// Show top 5 sources
	topSources := v.getTopSources(5)
	for _, src := range topSources {
		fmt.Fprintf(v.statusBar, "  %-12s: %d\n", 
			truncate(src.source, 12), src.count)
	}
	
	// Display status
	fmt.Fprintln(v.statusBar, "\n[yellow]Status:[white]")
	if v.paused {
		fmt.Fprintln(v.statusBar, "  [red]PAUSED[white]")
	} else {
		fmt.Fprintln(v.statusBar, "  [green]LIVE[white]")
	}
	fmt.Fprintf(v.statusBar, "  Buffer: %d/%d\n", len(v.logs), v.maxLogs)
	
	// Display shortcuts
	fmt.Fprintln(v.statusBar, "\n[yellow]Shortcuts:[white]")
	fmt.Fprintln(v.statusBar, "  [cyan]Tab[white]    - Switch focus")
	fmt.Fprintln(v.statusBar, "  [cyan]Space[white]  - Pause/Resume")
	fmt.Fprintln(v.statusBar, "  [cyan]c[white]      - Clear logs")
	fmt.Fprintln(v.statusBar, "  [cyan]g/G[white]    - Top/Bottom")
	fmt.Fprintln(v.statusBar, "  [cyan]/[white]      - Search")
}

// Helper methods

func (v *EnhancedLogsView) updateNodeList(source string) {
	// Check if this node is already in the list
	if source == "" {
		return
	}
	
	found := false
	for _, node := range v.nodes {
		if node == source {
			found = true
			break
		}
	}
	
	if !found {
		// Add new node to the list
		v.nodes = append(v.nodes, source)
	}
}

func (v *EnhancedLogsView) rebuildFilters() {
	// Rebuild the composite filter based on current selections
	v.filter = filters.NewCompositeFilter("AND")
	
	// Apply node filter
	if v.selectedNode != "" && v.selectedNode != "ALL" {
		// Use exact match pattern for source filter
		pattern := "^" + regexp.QuoteMeta(v.selectedNode) + "$"
		if filter, err := filters.NewSourceFilter(pattern, true); err == nil {
			v.filter.Add(filter)
		}
	}
	
	// Apply level filter
	if v.selectedLevel != "" && v.selectedLevel != "ALL" {
		minLevel := logging.ParseLevel(v.selectedLevel)
		v.filter.Add(filters.NewLevelFilter(minLevel))
	}
	
	// Apply search filter
	if v.searchText != "" {
		v.filter.Add(filters.NewMessageFilter().AddContains(v.searchText))
	}
	
	// Reapply filters to existing logs
	v.refreshLogsDisplay()
}

func (v *EnhancedLogsView) updateNodeFilter(node string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.selectedNode = node
	v.rebuildFilters()
	v.dirty = true
}

func (v *EnhancedLogsView) updateLevelFilter(level string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.selectedLevel = level
	v.rebuildFilters()
	v.dirty = true
}

func (v *EnhancedLogsView) updateSearchFilter(search string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.searchText = search
	v.rebuildFilters()
	v.dirty = true
}

func (v *EnhancedLogsView) clearLogs() {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.logs = v.logs[:0]
	v.logsText.Clear()
	v.stats = LogStats{
		Levels:  make(map[logging.Level]int),
		Sources: make(map[string]int),
	}
}

func (v *EnhancedLogsView) togglePause() {
	v.paused = !v.paused
	v.updateStatusBar()
}

func (v *EnhancedLogsView) exportLogs() {
	// TODO: Implement log export
}

type sourceCount struct {
	source string
	count  int
}

func (v *EnhancedLogsView) getTopSources(n int) []sourceCount {
	var sources []sourceCount
	for src, count := range v.stats.Sources {
		sources = append(sources, sourceCount{src, count})
	}
	
	// Sort by count
	for i := 0; i < len(sources)-1; i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[j].count > sources[i].count {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}
	
	if len(sources) > n {
		sources = sources[:n]
	}
	
	return sources
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// GetView returns the view primitive
func (v *EnhancedLogsView) GetView() tview.Primitive {
	return v.pages
}

// SetApp sets the application reference for redraws
func (v *EnhancedLogsView) SetApp(app *tview.Application) {
	v.app = app
	// Mark as dirty so the next periodic refresh will update the display
	v.mu.Lock()
	v.dirty = true
	v.mu.Unlock()
}

// Stop stops the logs view
func (v *EnhancedLogsView) Stop() {
	if v.refreshTicker != nil {
		v.refreshTicker.Stop()
	}
	v.cancel()
	if v.store != nil {
		v.store.Close()
	}
}