package views

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Dashboard represents the main dashboard view
type Dashboard struct {
	// UI components
	view        *tview.Flex
	statsView   *tview.TextView
	nodesTable  *tview.Table
	tasksTable  *tview.Table
	metricsView *tview.TextView

	// Data sources
	registry *distributed.Registry
	redis    *redis.Client
	mongodb  *mongo.Database
	logger   *slog.Logger

	// Statistics
	lastUpdate time.Time
}

// DashboardConfig holds dashboard configuration
type DashboardConfig struct {
	Registry *distributed.Registry
	Redis    *redis.Client
	MongoDB  *mongo.Database
	Logger   *slog.Logger
}

// NewDashboard creates a new dashboard view
func NewDashboard(config *DashboardConfig) *Dashboard {
	d := &Dashboard{
		registry: config.Registry,
		redis:    config.Redis,
		mongodb:  config.MongoDB,
		logger:   config.Logger,
	}

	// Create components
	d.createStatsView()
	d.createNodesTable()
	d.createTasksTable()
	d.createMetricsView()
	d.createLayout()

	return d
}

// createStatsView creates the statistics view
func (d *Dashboard) createStatsView() {
	d.statsView = tview.NewTextView().
		SetDynamicColors(true) // Enable colors for text content

	d.statsView.SetBorder(true).
		SetTitle("[yellow][ Cluster Statistics ][white]")
}

// createNodesTable creates the nodes table
func (d *Dashboard) createNodesTable() {
	d.nodesTable = tview.NewTable().
		SetBorders(false). // Remove internal borders for cleaner look
		SetFixed(1, 0).
		SetSelectable(false, false)

	d.nodesTable.SetBorder(true).
		SetTitle("[cyan][ Nodes ][white]")

	// Headers for crawler nodes - adjusted widths
	headers := []struct {
		name  string
		width int
		align int
	}{
		{"Node ID", 30, tview.AlignLeft},
		{"Hostname", 28, tview.AlignLeft},
		{"Status", 12, tview.AlignCenter},
		{"Workers", 10, tview.AlignCenter},
		{"CPU%", 7, tview.AlignRight},
		{"Mem%", 7, tview.AlignRight},
		{"Tasks", 8, tview.AlignRight},
		{"Failed", 7, tview.AlignRight},
		{"Rate", 8, tview.AlignRight},
		{"Uptime", 10, tview.AlignRight},
	}

	for i, h := range headers {
		// Orange text, no background
		cell := tview.NewTableCell("[orange]" + h.name).
			SetAlign(h.align).
			SetExpansion(0).
			SetMaxWidth(h.width)
		d.nodesTable.SetCell(0, i, cell)
	}
}

// createTasksTable creates the tasks table
func (d *Dashboard) createTasksTable() {
	d.tasksTable = tview.NewTable().
		SetBorders(false).          // Disable borders to avoid header background
		SetFixed(1, 0).             // Fix header row
		SetSelectable(false, false) // Completely disable selection - this is just a display panel

	d.tasksTable.SetBorder(true).
		SetTitle("[green][ Recent Tasks ][white]")

	// Headers for crawl tasks - simplified without width constraints
	headers := []string{
		"Time",
		"URL/Target",
		"Status",
		"Node",
		"Extracted",
		"Size",
		"Duration",
		"Depth",
	}

	for i, h := range headers {
		// Orange text with bold, no background
		cell := tview.NewTableCell("[orange::b]" + h).
			SetAlign(tview.AlignCenter).
			SetSelectable(false) // Make header non-selectable
		d.tasksTable.SetCell(0, i, cell)
	}
}

// createMetricsView creates the metrics view
func (d *Dashboard) createMetricsView() {
	d.metricsView = tview.NewTextView().
		SetDynamicColors(true) // Enable colors for text content

	d.metricsView.SetBorder(true).
		SetTitle("[magenta][ System Metrics ][white]")
}

// createLayout creates the dashboard layout
func (d *Dashboard) createLayout() {
	// Top section - statistics and metrics side by side
	topSection := tview.NewFlex().
		AddItem(d.statsView, 0, 2, false).  // 2/3 width for stats
		AddItem(d.metricsView, 0, 1, false) // 1/3 width for metrics

	// Middle section - nodes table (full width)
	middleSection := d.nodesTable

	// Bottom section - tasks table (full width)
	bottomSection := d.tasksTable

	// Main layout with better proportions
	d.view = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(topSection, 12, 0, false).   // 12 lines for top section
		AddItem(middleSection, 0, 1, false). // 1/2 of remaining for nodes
		AddItem(bottomSection, 0, 1, false)  // 1/2 of remaining for tasks
}

// GetView returns the dashboard view
func (d *Dashboard) GetView() tview.Primitive {
	return d.view
}

// Refresh refreshes the dashboard data
func (d *Dashboard) Refresh(ctx context.Context) {
	d.refreshStats(ctx)
	d.refreshNodes(ctx)
	d.refreshTasks(ctx)
	d.refreshMetrics(ctx)
	d.lastUpdate = time.Now()
}

// refreshStats refreshes statistics
func (d *Dashboard) refreshStats(ctx context.Context) {
	d.statsView.Clear()

	// Get cluster stats
	clusterStats, err := d.registry.GetClusterStats(ctx)
	if err != nil {
		fmt.Fprintln(d.statsView, "Failed to get cluster statistics")
		return
	}

	// Display crawler-specific statistics
	fmt.Fprintln(d.statsView, "")
	fmt.Fprintln(d.statsView, " [yellow]▶ Cluster Overview[white]")
	fmt.Fprintf(d.statsView, "  Nodes:     [cyan]%d[white] active / %d total\n",
		clusterStats.ActiveNodes, clusterStats.TotalNodes)
	fmt.Fprintf(d.statsView, "  Workers:   [cyan]%d[white] active / %d total\n",
		clusterStats.ActiveWorkers, clusterStats.TotalWorkers)

	fmt.Fprintln(d.statsView, "")
	fmt.Fprintln(d.statsView, " [yellow]▶ Crawl Progress[white]")
	fmt.Fprintf(d.statsView, "  Processed: [green]%d[white] pages\n", clusterStats.TasksProcessed)
	fmt.Fprintf(d.statsView, "  Failed:    [red]%d[white] pages\n", clusterStats.TasksFailed)

	// Calculate and display success rate
	if clusterStats.TasksProcessed > 0 {
		successRate := float64(clusterStats.TasksProcessed-clusterStats.TasksFailed) /
			float64(clusterStats.TasksProcessed) * 100
		color := "green"
		if successRate < 80 {
			color = "yellow"
		}
		if successRate < 50 {
			color = "red"
		}
		fmt.Fprintf(d.statsView, "  Success:   [%s]%.1f%%[white]\n", color, successRate)
	}

	fmt.Fprintln(d.statsView, "")
	fmt.Fprintln(d.statsView, " [yellow]▶ Resource Usage[white]")
	fmt.Fprintf(d.statsView, "  CPU:       %.1f%% avg\n", clusterStats.AvgCPUUsage)
	fmt.Fprintf(d.statsView, "  Memory:    %.1f%% avg\n", clusterStats.AvgMemoryUsage)
}

// refreshNodes refreshes nodes table
func (d *Dashboard) refreshNodes(ctx context.Context) {
	// Get healthy nodes
	nodes, err := d.registry.GetHealthyNodes(ctx)
	if err != nil {
		if d.logger != nil {
			d.logger.Error("Failed to list nodes", "error", err)
		}
		return
	}

	// Clear table (except header)
	for i := d.nodesTable.GetRowCount() - 1; i > 0; i-- {
		d.nodesTable.RemoveRow(i)
	}

	// Add nodes
	for i, node := range nodes {
		if i >= 10 { // Limit to 10 nodes
			break
		}

		row := i + 1

		// Node ID (show more characters)
		nodeID := node.ID
		if len(nodeID) > 30 {
			nodeID = nodeID[:27] + "..."
		}

		// Hostname (show full if possible)
		hostname := node.Hostname
		if hostname == "" {
			hostname = node.IP
		}
		if len(hostname) > 25 {
			hostname = hostname[:22] + "..."
		}

		// Uptime
		uptime := "-"
		if !node.StartedAt.IsZero() {
			uptime = formatDuration(time.Since(node.StartedAt))
		}

		// Status with color
		statusText := node.Status
		switch node.Status {
		case "active":
			statusText = "[green]● active[white]"
		case "paused":
			statusText = "[yellow]● paused[white]"
		case "disconnected":
			statusText = "[red]● offline[white]"
		case "error":
			statusText = "[red]● error[white]"
		default:
			statusText = "[gray]● " + node.Status + "[white]"
		}

		// Calculate task rate (tasks per minute for better readability)
		var taskRate string
		if !node.StartedAt.IsZero() {
			duration := time.Since(node.StartedAt).Minutes()
			if duration > 0 {
				rate := float64(node.TasksProcessed) / duration
				taskRate = fmt.Sprintf("%.0f/m", rate)
			} else {
				taskRate = "0/m"
			}
		} else {
			taskRate = "0/m"
		}

		// Set cells with crawler-specific data
		d.nodesTable.SetCell(row, 0, tview.NewTableCell(nodeID).SetAlign(tview.AlignLeft).SetMaxWidth(30))
		d.nodesTable.SetCell(row, 1, tview.NewTableCell(hostname).SetAlign(tview.AlignLeft).SetMaxWidth(28))
		d.nodesTable.SetCell(row, 2, tview.NewTableCell(statusText).SetAlign(tview.AlignCenter).SetMaxWidth(12))
		d.nodesTable.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d/%d",
			node.ActiveWorkers, node.MaxWorkers)).SetAlign(tview.AlignCenter).SetMaxWidth(10))
		d.nodesTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%.0f%%", node.CPUUsage)).SetAlign(tview.AlignRight).SetMaxWidth(7))
		d.nodesTable.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%.0f%%", node.MemoryUsage)).SetAlign(tview.AlignRight).SetMaxWidth(7))
		d.nodesTable.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d", node.TasksProcessed)).SetAlign(tview.AlignRight).SetMaxWidth(8))
		d.nodesTable.SetCell(row, 7, tview.NewTableCell(fmt.Sprintf("%d", node.TasksFailed)).SetAlign(tview.AlignRight).SetMaxWidth(7))
		d.nodesTable.SetCell(row, 8, tview.NewTableCell(taskRate).SetAlign(tview.AlignRight).SetMaxWidth(8))
		d.nodesTable.SetCell(row, 9, tview.NewTableCell(uptime).SetAlign(tview.AlignRight).SetMaxWidth(10))
	}

	// Add empty message if no nodes
	if len(nodes) == 0 {
		d.nodesTable.SetCell(1, 0, tview.NewTableCell("No active nodes").
			SetAlign(tview.AlignCenter).
			SetExpansion(7))
	}
}

// refreshTasks refreshes tasks table
func (d *Dashboard) refreshTasks(ctx context.Context) {
	// Clear table (except header)
	for i := d.tasksTable.GetRowCount() - 1; i > 0; i-- {
		d.tasksTable.RemoveRow(i)
	}

	// Try to get recent task results from Redis
	// First try to get from result keys
	resultKeys, err := d.redis.Keys(ctx, "crawler:result:*").Result()
	if err == nil && len(resultKeys) > 0 {
		// Get all results first, then sort by time
		type TaskInfo struct {
			Time   time.Time
			URL    string
			Status string
			NodeID string
			Size   int64
			TaskID string
		}
		var taskInfos []TaskInfo

		for _, key := range resultKeys {
			resultData, err := d.redis.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			// Try to parse as TaskResult first
			var result distributed.TaskResult
			if err := json.Unmarshal([]byte(resultData), &result); err == nil {
				// Check if this is a valid TaskResult with metadata
				if result.TaskID != "" && !result.StartTime.IsZero() {
					info := TaskInfo{
						Time:   result.EndTime,
						Status: result.Status,
						NodeID: result.NodeID,
						Size:   result.BytesRead,
						TaskID: result.TaskID,
					}
					if info.Time.IsZero() {
						info.Time = result.StartTime
					}

					// Extract URL from Data
					if len(result.Data) > 0 {
						// Try to get URL from the data
						if urlVal, ok := result.Data[0]["url"]; ok {
							info.URL = fmt.Sprintf("%v", urlVal)
						}
					}

					taskInfos = append(taskInfos, info)
				}
			} else {
				// This might be raw data from old format, skip it
				continue
			}
		}

		// Sort by time (newest first)
		sort.Slice(taskInfos, func(i, j int) bool {
			return taskInfos[i].Time.After(taskInfos[j].Time)
		})

		// Show more results (up to 50)
		if len(taskInfos) > 50 {
			taskInfos = taskInfos[:50]
		}

		// Display results
		for i, info := range taskInfos {
			row := i + 1

			// Time
			timeStr := info.Time.Format("15:04:05")
			d.tasksTable.SetCell(row, 0, tview.NewTableCell(timeStr).
				SetAlign(tview.AlignCenter))

			// URL/Target
			url := info.URL

			// If no URL found, try to get it from MongoDB task document
			if url == "" {
				// Extract parent task ID from instance ID (format: objectid_timestamp)
				parts := strings.Split(info.TaskID, "_")
				if len(parts) >= 1 && d.mongodb != nil {
					// Try to get task from MongoDB
					objectID, err := primitive.ObjectIDFromHex(parts[0])
					if err == nil {
						var taskDoc task.TaskDocument
						collection := d.mongodb.Collection("crawler_tasks")
						if err := collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&taskDoc); err == nil {
							url = taskDoc.Request.URL
						}
					}
				}

				// Final fallback - show task ID
				if url == "" {
					if len(parts) >= 2 {
						objectID := parts[0]
						if len(objectID) > 12 {
							objectID = objectID[:12]
						}
						url = fmt.Sprintf("Task %s", objectID)
					} else {
						url = fmt.Sprintf("Task %s", info.TaskID)
					}
				}
			}

			// Truncate long URLs to fit reasonably
			if len(url) > 70 {
				url = url[:67] + "..."
			}
			d.tasksTable.SetCell(row, 1, tview.NewTableCell(url).
				SetAlign(tview.AlignLeft))

			// Status with color (shorter format)
			statusIcon := "[green]✓[white]"
			if info.Status == "failed" {
				statusIcon = "[red]✗[white]"
			} else if info.Status == "running" {
				statusIcon = "[yellow]↻[white]"
			}
			d.tasksTable.SetCell(row, 2, tview.NewTableCell(statusIcon).
				SetAlign(tview.AlignCenter))

			// Node (show more of the ID)
			nodeStr := info.NodeID
			if len(nodeStr) > 30 {
				nodeStr = nodeStr[:27] + "..."
			}
			d.tasksTable.SetCell(row, 3, tview.NewTableCell(nodeStr).
				SetAlign(tview.AlignLeft))

			// Items extracted
			itemStr := "1"
			d.tasksTable.SetCell(row, 4, tview.NewTableCell(itemStr).
				SetAlign(tview.AlignRight))

			// Size
			sizeStr := formatBytes(info.Size)
			d.tasksTable.SetCell(row, 5, tview.NewTableCell(sizeStr).
				SetAlign(tview.AlignRight))

			// Duration (calculate from task ID timestamp)
			durationStr := "0s"
			taskParts := strings.Split(info.TaskID, "_")
			if len(taskParts) >= 2 {
				if startTimestamp, err := strconv.ParseInt(taskParts[1], 10, 64); err == nil {
					startTime := time.Unix(startTimestamp, 0)
					duration := info.Time.Sub(startTime)
					if duration > 0 && duration < 1*time.Hour {
						durationStr = formatDuration(duration)
					}
				}
			}
			d.tasksTable.SetCell(row, 6, tview.NewTableCell(durationStr).
				SetAlign(tview.AlignRight))

			// Depth
			depth := 0
			d.tasksTable.SetCell(row, 7, tview.NewTableCell(fmt.Sprintf("%d", depth)).
				SetAlign(tview.AlignCenter))
		}

		return
	}

	// If no real data available, show a message
	d.tasksTable.SetCell(1, 0, tview.NewTableCell("No recent task results available").
		SetAlign(tview.AlignCenter).
		SetExpansion(8).
		SetTextColor(tview.Styles.SecondaryTextColor))
}

// refreshMetrics refreshes metrics view
func (d *Dashboard) refreshMetrics(ctx context.Context) {
	d.metricsView.Clear()

	// Get additional metrics from Redis
	pipe := d.redis.Pipeline()
	completedTasks := pipe.Get(ctx, "crawler:stats:completed_tasks")
	failedTasks := pipe.Get(ctx, "crawler:stats:failed_tasks")
	pendingTasks := pipe.LLen(ctx, "crawler:queue:pending")

	_, _ = pipe.Exec(ctx)

	fmt.Fprintln(d.metricsView, "")
	fmt.Fprintln(d.metricsView, "[yellow]▶ Queue Status[white]")

	pendingCount := pendingTasks.Val()
	if pendingCount > 0 {
		color := "green"
		if pendingCount > 100 {
			color = "yellow"
		}
		if pendingCount > 1000 {
			color = "red"
		}
		fmt.Fprintf(d.metricsView, "  Pending:   [%s]%d[white]\n", color, pendingCount)
	} else {
		fmt.Fprintln(d.metricsView, "  Pending:   0")
	}

	if val, _ := completedTasks.Int64(); val > 0 {
		fmt.Fprintf(d.metricsView, "  Complete:  [green]%d[white]\n", val)
	}
	if val, _ := failedTasks.Int64(); val > 0 {
		fmt.Fprintf(d.metricsView, "  Failed:    [red]%d[white]\n", val)
	}

	fmt.Fprintln(d.metricsView, "")
	fmt.Fprintln(d.metricsView, "[yellow]▶ Performance[white]")

	// Get cluster stats for rate calculation
	clusterStats, err := d.registry.GetClusterStats(ctx)
	if err == nil && clusterStats != nil {
		// Calculate crawl rate
		if !d.lastUpdate.IsZero() {
			duration := time.Since(d.lastUpdate)
			if duration.Seconds() > 0 && clusterStats.TasksProcessed > 0 {
				rate := float64(clusterStats.TasksProcessed) / duration.Seconds()
				fmt.Fprintf(d.metricsView, "  Rate:      %.1f pages/s\n", rate)
			}
		}
	}

	fmt.Fprintln(d.metricsView, "")
	fmt.Fprintln(d.metricsView, "[yellow]▶ Controls[white]")
	fmt.Fprintln(d.metricsView, "  [cyan]F2[white] Nodes")
	fmt.Fprintln(d.metricsView, "  [cyan]F3[white] Tasks")
	fmt.Fprintln(d.metricsView, "  [cyan]F4[white] Logs")
	fmt.Fprintln(d.metricsView, "  [cyan]P[white]  Pause All")
	fmt.Fprintln(d.metricsView, "  [cyan]R[white]  Resume All")
}

// formatDuration formats duration for display
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d %= time.Hour
	m := d / time.Minute
	d %= time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatBytes formats bytes for display
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
