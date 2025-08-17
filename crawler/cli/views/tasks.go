package views

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
)

// TasksView represents the tasks management view
type TasksView struct {
	// UI components
	view       *tview.Flex
	table      *tview.Table
	queueList  *tview.List
	addForm    *tview.Form
	filterForm *tview.Form

	// Data
	redis   *redis.Client
	prefix  string
	logger  *slog.Logger
	tasks   []*task.Task
	queues  []string
	filter  TaskFilter
}

// TaskFilter holds task filtering options
type TaskFilter struct {
	Status   string
	Type     string
	Priority int
	NodeID   string
}

// TasksConfig holds tasks view configuration
type TasksConfig struct {
	Redis  *redis.Client
	Prefix string
	Logger *slog.Logger
}

// NewTasksView creates a new tasks view
func NewTasksView(config *TasksConfig) *TasksView {
	v := &TasksView{
		redis:  config.Redis,
		prefix: config.Prefix,
		logger: config.Logger,
		tasks:  []*task.Task{},
		queues: []string{},
		filter: TaskFilter{},
	}

	// Create components
	v.createTable()
	v.createQueueList()
	v.createAddForm()
	v.createFilterForm()
	v.createLayout()

	return v
}

// createTable creates the tasks table
func (v *TasksView) createTable() {
	v.table = tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0).
		SetSelectable(true, false)

	// Set headers
	headers := []string{"ID", "URL", "Type", "Priority", "Status", "Node", "Retries", "Created"}
	for i, header := range headers {
		v.table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}
}

// createQueueList creates the queue list
func (v *TasksView) createQueueList() {
	v.queueList = tview.NewList().
		ShowSecondaryText(true)
	
	v.queueList.SetBorder(true).
		SetTitle(" Task Queues ").
		SetTitleAlign(tview.AlignLeft)
	
	// Set selection handler
	v.queueList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		v.loadQueueTasks(context.Background(), mainText)
	})
}

// createAddForm creates the add task form
func (v *TasksView) createAddForm() {
	v.addForm = tview.NewForm()
	
	// URL input
	v.addForm.AddInputField("URL", "", 40, nil, nil)
	
	// Type dropdown
	types := []string{"list", "detail", "download"}
	v.addForm.AddDropDown("Type", types, 1, nil)
	
	// Priority dropdown
	priorities := []string{"Low", "Normal", "High", "Urgent"}
	v.addForm.AddDropDown("Priority", priorities, 1, nil)
	
	// Add button
	v.addForm.AddButton("Add Task", func() {
		v.addTask()
	})
	
	// Clear button
	v.addForm.AddButton("Clear", func() {
		v.addForm.Clear(true)
		v.createAddForm() // Recreate form
	})
	
	v.addForm.SetBorder(true).
		SetTitle(" Add Task ").
		SetTitleAlign(tview.AlignLeft)
	
	// Note: Form.SetInputCapture doesn't work (GitHub issue #181)
	// Function keys are handled at application level
}

// createFilterForm creates the filter form
func (v *TasksView) createFilterForm() {
	v.filterForm = tview.NewForm()
	
	// Status filter
	statuses := []string{"All", "Pending", "Running", "Completed", "Failed"}
	v.filterForm.AddDropDown("Status", statuses, 0, func(option string, index int) {
		if option == "All" {
			v.filter.Status = ""
		} else {
			v.filter.Status = strings.ToLower(option)
		}
	})
	
	// Type filter
	types := []string{"All", "List", "Detail", "Download"}
	v.filterForm.AddDropDown("Type", types, 0, func(option string, index int) {
		if option == "All" {
			v.filter.Type = ""
		} else {
			v.filter.Type = strings.ToLower(option)
		}
	})
	
	// Apply button
	v.filterForm.AddButton("Apply", func() {
		v.applyFilter()
	})
	
	// Reset button
	v.filterForm.AddButton("Reset", func() {
		v.filter = TaskFilter{}
		v.Refresh(context.Background())
	})
	
	v.filterForm.SetBorder(true).
		SetTitle(" Filter ").
		SetTitleAlign(tview.AlignLeft)
	
	// Note: Form.SetInputCapture doesn't work (GitHub issue #181)
	// Function keys are handled at application level
}

// createLayout creates the tasks view layout
func (v *TasksView) createLayout() {
	// Left panel - queues
	leftPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.queueList, 0, 1, false).
		AddItem(v.addForm, 10, 0, false)

	// Right panel - filter and table
	rightPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.filterForm, 7, 0, false).
		AddItem(v.table, 0, 1, true)

	// Main layout
	v.view = tview.NewFlex().
		AddItem(leftPanel, 50, 0, false).
		AddItem(rightPanel, 0, 1, true)
}

// GetView returns the tasks view
func (v *TasksView) GetView() tview.Primitive {
	return v.view
}

// Refresh refreshes the tasks data
func (v *TasksView) Refresh(ctx context.Context) {
	v.refreshQueues(ctx)
	v.refreshTasks(ctx)
}

// refreshQueues refreshes the queue list
func (v *TasksView) refreshQueues(ctx context.Context) {
	// Get all queue keys
	pattern := fmt.Sprintf("%s:queue:*", v.prefix)
	keys, err := v.redis.Keys(ctx, pattern).Result()
	if err != nil {
		v.logger.Error("Failed to get queue keys", "error", err)
		return
	}

	v.queueList.Clear()
	v.queues = []string{}

	// Add queues to list
	for _, key := range keys {
		// Extract queue name from key
		parts := strings.Split(key, ":")
		if len(parts) < 3 {
			continue
		}
		queueName := parts[2]
		
		// Get queue size
		size, _ := v.redis.LLen(ctx, key).Result()
		
		v.queueList.AddItem(queueName, fmt.Sprintf("%d tasks", size), 0, nil)
		v.queues = append(v.queues, queueName)
	}
	
	// Add default queues if empty
	if len(v.queues) == 0 {
		defaultQueues := []string{"tasks", "priority", "retry"}
		for _, queue := range defaultQueues {
			v.queueList.AddItem(queue, "0 tasks", 0, nil)
			v.queues = append(v.queues, queue)
		}
	}
}

// refreshTasks refreshes the tasks table
func (v *TasksView) refreshTasks(ctx context.Context) {
	// Get recent tasks from results
	results, err := v.redis.LRange(ctx, fmt.Sprintf("%s:results:recent", v.prefix), 0, 49).Result()
	if err != nil && err != redis.Nil {
		v.logger.Error("Failed to get recent results", "error", err)
		return
	}

	// Clear table (except header)
	for i := v.table.GetRowCount() - 1; i > 0; i-- {
		v.table.RemoveRow(i)
	}

	// Parse and display tasks
	v.tasks = []*task.Task{}
	for i, result := range results {
		if i >= 50 { // Limit display
			break
		}
		
		// Try to parse as task
		var t task.Task
		if err := json.Unmarshal([]byte(result), &t); err != nil {
			continue
		}
		
		// Apply filter
		if !v.matchesFilter(&t) {
			continue
		}
		
		v.tasks = append(v.tasks, &t)
		row := len(v.tasks)
		
		// Format status with color
		statusCell := tview.NewTableCell(string(t.Status))
		switch t.Status {
		case task.StatusCompleted:
			statusCell.SetTextColor(tview.Styles.TertiaryTextColor)
		case task.StatusRunning:
			statusCell.SetTextColor(tview.Styles.PrimaryTextColor)
		case task.StatusFailed:
			statusCell.SetTextColor(tview.Styles.ContrastSecondaryTextColor)
		}

		// Add row
		v.table.SetCell(row, 0, tview.NewTableCell(t.ID[:8]))
		v.table.SetCell(row, 1, tview.NewTableCell(truncateString(t.URL, 40)))
		v.table.SetCell(row, 2, tview.NewTableCell(string(t.Type)))
		v.table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", t.Priority)))
		v.table.SetCell(row, 4, statusCell)
		v.table.SetCell(row, 5, tview.NewTableCell(t.NodeID[:8]))
		v.table.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d/%d", t.RetryCount, t.MaxRetries)))
		v.table.SetCell(row, 7, tview.NewTableCell(t.CreatedAt.Format("15:04:05")))
	}
}

// loadQueueTasks loads tasks from a specific queue
func (v *TasksView) loadQueueTasks(ctx context.Context, queueName string) {
	key := fmt.Sprintf("%s:queue:%s", v.prefix, queueName)
	
	// Get tasks from queue
	tasks, err := v.redis.LRange(ctx, key, 0, 49).Result()
	if err != nil {
		v.logger.Error("Failed to get queue tasks", "error", err)
		return
	}

	// Clear table (except header)
	for i := v.table.GetRowCount() - 1; i > 0; i-- {
		v.table.RemoveRow(i)
	}

	// Display tasks
	v.tasks = []*task.Task{}
	for i, taskData := range tasks {
		var t task.Task
		if err := json.Unmarshal([]byte(taskData), &t); err != nil {
			continue
		}
		
		v.tasks = append(v.tasks, &t)
		row := i + 1
		
		v.table.SetCell(row, 0, tview.NewTableCell(t.ID[:8]))
		v.table.SetCell(row, 1, tview.NewTableCell(truncateString(t.URL, 40)))
		v.table.SetCell(row, 2, tview.NewTableCell(string(t.Type)))
		v.table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", t.Priority)))
		v.table.SetCell(row, 4, tview.NewTableCell(string(t.Status)))
		v.table.SetCell(row, 5, tview.NewTableCell("-"))
		v.table.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d/%d", t.RetryCount, t.MaxRetries)))
		v.table.SetCell(row, 7, tview.NewTableCell(t.CreatedAt.Format("15:04:05")))
	}
}

// addTask adds a new task
func (v *TasksView) addTask() {
	// Get form values
	url := v.addForm.GetFormItemByLabel("URL").(*tview.InputField).GetText()
	_, taskType := v.addForm.GetFormItemByLabel("Type").(*tview.DropDown).GetCurrentOption()
	priorityIndex, _ := v.addForm.GetFormItemByLabel("Priority").(*tview.DropDown).GetCurrentOption()
	
	if url == "" {
		return
	}

	// Map priority
	priorities := []task.Priority{
		task.PriorityLow,
		task.PriorityNormal,
		task.PriorityHigh,
		task.PriorityUrgent,
	}
	priority := priorities[priorityIndex]
	
	// Map type
	var tType task.Type
	switch taskType {
	case "list":
		tType = task.TypeList
	case "detail":
		tType = task.TypeDetail
	default:
		tType = task.TypeDetail
	}

	// Create task
	t := task.NewTask(url, string(tType), int(priority))
	
	// Add to queue
	ctx := context.Background()
	queue := task.NewRedisQueue(v.redis, v.prefix)
	if err := queue.Push(ctx, t); err != nil {
		v.logger.Error("Failed to add task", "error", err)
		return
	}

	// Clear form and refresh
	v.addForm.GetFormItemByLabel("URL").(*tview.InputField).SetText("")
	v.Refresh(ctx)
}

// applyFilter applies the current filter
func (v *TasksView) applyFilter() {
	ctx := context.Background()
	v.refreshTasks(ctx)
}

// matchesFilter checks if a task matches the current filter
func (v *TasksView) matchesFilter(t *task.Task) bool {
	if v.filter.Status != "" && string(t.Status) != v.filter.Status {
		return false
	}
	if v.filter.Type != "" && string(t.Type) != v.filter.Type {
		return false
	}
	if v.filter.NodeID != "" && t.NodeID != v.filter.NodeID {
		return false
	}
	return true
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}