package views

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/task"
	"github.com/gdamore/tcell/v2"
	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// TaskManagerView 任务管理视图 - 完整的任务CRUD界面
type TaskManagerView struct {
	// UI components
	view        *tview.Flex
	pages       *tview.Pages
	taskList    *tview.Table
	taskDetail  *tview.TextView
	taskForm    *tview.Form
	statusBar   *tview.TextView
	modal       *tview.Modal
	app         *tview.Application  // Reference to the app for focus management
	logView     *tview.TextView      // Log viewer panel
	
	// Navigation
	menuList    *tview.List
	
	// Data
	service     *task.Service
	redis       *redis.Client
	logger      *slog.Logger
	tasks       []*task.TaskDocument
	currentTask *task.TaskDocument
	filter      task.TaskFilter
	
	// State
	editMode     bool
	currentPage  string
	menuFocused  bool  // Track whether menu is focused
	ctrlWPressed bool  // Track if Ctrl+W was pressed
	logBuffer    []string  // Buffer for log messages
	maxLogs      int       // Maximum number of log lines to keep
}

// TaskManagerConfig 任务管理配置
type TaskManagerConfig struct {
	MongoDB *mongo.Database
	Redis   *redis.Client
	Logger  *slog.Logger
}

// NewTaskManagerView 创建任务管理视图
func NewTaskManagerView(config *TaskManagerConfig) *TaskManagerView {
	v := &TaskManagerView{
		service:     task.NewService(config.MongoDB),
		redis:       config.Redis,
		logger:      config.Logger,
		tasks:       []*task.TaskDocument{},
		currentPage: "list",
		menuFocused: false,  // Start with content focused
		logBuffer:   []string{},
		maxLogs:     100,    // Keep last 100 log lines
	}
	
	v.createComponents()
	v.createLayout()
	v.setupKeyBindings()
	
	return v
}

// createComponents 创建UI组件
func (v *TaskManagerView) createComponents() {
	// 创建任务列表
	v.createTaskList()
	
	// 创建任务详情
	v.createTaskDetail()
	
	// 创建任务表单
	v.createTaskForm()
	
	// 创建菜单
	v.createMenu()
	
	// 创建状态栏
	v.createStatusBar()
	
	// 创建日志视图
	v.createLogView()
	
	// 创建页面容器
	v.pages = tview.NewPages()
}

// createTaskList 创建任务列表
func (v *TaskManagerView) createTaskList() {
	v.taskList = tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0).
		SetSelectable(true, false)
	
	// 设置表头
	headers := []string{"Name", "Type", "Category", "Status", "Schedule", "Last Run", "Success Rate", "Actions"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		v.taskList.SetCell(0, i, cell)
	}
	
	// 设置选择处理
	v.taskList.SetSelectedFunc(func(row, col int) {
		if row > 0 && row <= len(v.tasks) {
			v.currentTask = v.tasks[row-1]
			v.showTaskDetail()
		}
	})
	
	v.taskList.SetBorder(true).
		SetTitle(" Task List [[yellow]ESC[white]]Menu [[yellow]↑↓/jk[white]]Navigate [[yellow]Enter[white]]View [[yellow]A[white]]dd [[yellow]E[white]]dit [[yellow]D[white]]elete ").
		SetTitleAlign(tview.AlignLeft)
}

// createTaskDetail 创建任务详情视图
func (v *TaskManagerView) createTaskDetail() {
	v.taskDetail = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	
	v.taskDetail.SetBorder(true).
		SetTitle(" Task Details [[yellow]Ctrl+W W[white]]Switch [[yellow]ESC/B[white]]ack [[yellow]E[white]]dit [[yellow]D[white]]elete [[yellow]R[white]]un [[yellow]T[white]]oggle ").
		SetTitleAlign(tview.AlignLeft)
}

// createTaskForm 创建任务表单
func (v *TaskManagerView) createTaskForm() {
	v.taskForm = tview.NewForm()
	
	// 基本信息
	v.taskForm.AddInputField("Name", "", 40, nil, nil)
	v.taskForm.AddInputField("Description", "", 60, nil, nil)
	
	// 类型和分类
	types := []string{"http", "api", "websocket", "custom"}
	v.taskForm.AddDropDown("Type", types, 0, nil)
	
	categories := []string{"sports", "news", "e-commerce", "social", "finance", "other"}
	v.taskForm.AddDropDown("Category", categories, 0, nil)
	
	// 请求配置
	v.taskForm.AddInputField("URL", "", 60, nil, nil)
	methods := []string{"GET", "POST", "PUT", "DELETE"}
	v.taskForm.AddDropDown("Method", methods, 0, nil)
	
	// 调度配置
	scheduleTypes := []string{"manual", "once", "interval", "cron"}
	v.taskForm.AddDropDown("Schedule Type", scheduleTypes, 0, nil)
	v.taskForm.AddInputField("Schedule Expression", "", 30, nil, nil)
	
	// 配置参数
	v.taskForm.AddInputField("Timeout (s)", "30", 10, nil, nil)
	v.taskForm.AddInputField("Max Retries", "3", 10, nil, nil)
	v.taskForm.AddInputField("Priority", "5", 10, nil, nil)
	
	// 存储配置
	storageTypes := []string{"mongodb", "mysql", "redis", "file"}
	v.taskForm.AddDropDown("Storage Type", storageTypes, 0, nil)
	v.taskForm.AddInputField("Collection/Table", "", 30, nil, nil)
	
	// 按钮
	v.taskForm.AddButton("Save", func() {
		v.saveTask()
	})
	
	v.taskForm.AddButton("Cancel", func() {
		if v.currentPage == "detail" {
			v.showTaskDetail()
		} else {
			v.showTaskList()
		}
	})
	
	v.taskForm.SetBorder(true).
		SetTitle(" Task Form [[yellow]ESC[white]]Cancel [[yellow]Tab[white]]Navigate ").
		SetTitleAlign(tview.AlignLeft)
	
	// Note: Form.SetInputCapture doesn't work (GitHub issue #181)
	// ESC key handling moved to application level
}

// createMenu 创建菜单
func (v *TaskManagerView) createMenu() {
	v.menuList = tview.NewList().
		AddItem("[L] Task List", "View all tasks", 'l', func() {
			v.showTaskList()
			v.focusContent()  // 自动切换焦点到内容
		}).
		AddItem("[A] Add Task", "Create new task", 'a', func() {
			v.showTaskForm(nil)
			v.focusContent()  // 自动切换焦点到表单
		}).
		AddItem("[I] Import Task", "Import from YAML/JSON", 'i', func() {
			v.showImportDialog()
		}).
		AddItem("[E] Export Tasks", "Export to YAML/JSON", 'e', func() {
			v.showExportDialog()
		}).
		AddItem("[S] Task Stats", "View statistics", 's', func() {
			v.showStats()
		}).
		AddItem("[R] Refresh", "Refresh task list", 'r', func() {
			v.refreshTasks()
		})
	
	v.menuList.SetBorder(true).
		SetTitle(" Menu [[yellow]↑↓[white]]Navigate [[yellow]Enter[white]]Select ").
		SetTitleAlign(tview.AlignLeft)
	
	// 菜单快捷键
	v.menuList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Always pass through function keys to global handler
		if event.Key() >= tcell.KeyF1 && event.Key() <= tcell.KeyF12 {
			return event
		}
		
		// 先处理 Ctrl+W
		if result := v.handleCtrlW(event); result == nil {
			return nil
		}
		
		switch event.Key() {
		case tcell.KeyEnter:
			// Enter键执行选中的菜单项（会自动切换焦点）
			return event
		case tcell.KeyRune:
			// 快捷键直接跳转并切换焦点
			switch event.Rune() {
			case 'l', 'L':
				v.showTaskList()
				v.focusContent()
				return nil
			case 'a', 'A':
				v.showTaskForm(nil)
				v.focusContent()
				return nil
			case 'r', 'R':
				v.refreshTasks()
				// 刷新不切换焦点
				return nil
			}
		}
		return event
	})
}

// createStatusBar 创建状态栏
func (v *TaskManagerView) createStatusBar() {
	v.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
	v.updateStatus("Ready")
}

// createLogView 创建日志视图
func (v *TaskManagerView) createLogView() {
	v.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			// Auto-scroll to bottom when new content is added
			// Don't call Draw() here - tview handles redraws automatically
		})
	
	v.logView.SetBorder(true).
		SetTitle(" Logs & Events [[yellow]Ctrl+L[white]]Clear ").
		SetTitleAlign(tview.AlignLeft)
	
	// Initialize with welcome message
	v.addLog("info", "Log viewer initialized")
}

// createLayout 创建布局
func (v *TaskManagerView) createLayout() {
	// 创建内容区域（左侧）和日志区域（右侧）的水平布局
	
	// 列表页面内容
	listContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.taskList, 0, 1, true).
		AddItem(v.statusBar, 1, 0, false)
	
	// 列表页面 - 水平分割，日志在右侧
	listPage := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(listContent, 0, 3, true).     // 主内容占3/4宽度
		AddItem(v.logView, 0, 1, false)       // 日志视图占1/4宽度
	
	// 详情页面内容
	detailContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.taskDetail, 0, 1, false).
		AddItem(v.createDetailButtons(), 3, 0, true).
		AddItem(v.statusBar, 1, 0, false)
		
	// 详情页面 - 水平分割，日志在右侧
	detailPage := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(detailContent, 0, 3, true).   // 主内容占3/4宽度
		AddItem(v.logView, 0, 1, false)       // 日志视图占1/4宽度
	
	// 表单页面内容
	formContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.taskForm, 0, 1, false).
		AddItem(v.statusBar, 1, 0, false)
	
	// 表单页面 - 水平分割，日志在右侧
	formPage := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(formContent, 0, 3, false).     // 主内容占3/4宽度
		AddItem(v.logView, 0, 1, false)       // 日志视图占1/4宽度
	
	// 添加页面
	v.pages.AddPage("list", listPage, true, true)
	v.pages.AddPage("detail", detailPage, true, false)
	v.pages.AddPage("form", formPage, true, false)
	
	// 主布局
	v.view = tview.NewFlex().
		AddItem(v.menuList, 30, 0, false).  // 菜单栏
		AddItem(v.pages, 0, 1, true)        // 内容区域 - 初始焦点
	
	// 设置全局快捷键处理
	v.setupGlobalKeyBindings()
}

// createDetailButtons 创建详情页按钮
func (v *TaskManagerView) createDetailButtons() *tview.Form {
	form := tview.NewForm()
	
	// 添加按钮到表单，这样可以用Tab键导航
	form.AddButton("[yellow::b]B[-::-]ack", func() {
		v.showTaskList()
	})
	
	form.AddButton("[yellow::b]E[-::-]dit", func() {
		if v.currentTask != nil {
			v.showTaskForm(v.currentTask)
		}
	})
	
	form.AddButton("[yellow::b]D[-::-]elete", func() {
		if v.currentTask != nil {
			v.confirmDelete()
		}
	})
	
	form.AddButton("[yellow::b]R[-::-]un Now", func() {
		if v.currentTask != nil {
			v.runTask()
		}
	})
	
	form.AddButton("[yellow::b]T[-::-]oggle Enable", func() {
		if v.currentTask != nil {
			v.toggleTask()
		}
	})
	
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetBorderPadding(0, 0, 1, 1)
	
	// 添加键盘快捷键处理
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Always pass through function keys to global handler
		if event.Key() >= tcell.KeyF1 && event.Key() <= tcell.KeyF12 {
			return event
		}
		
		// 先处理 Ctrl+W
		if result := v.handleCtrlW(event); result == nil {
			return nil
		}
		
		switch event.Key() {
		case tcell.KeyEsc:
			v.showTaskList()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'b', 'B':
				v.showTaskList()
				return nil
			case 'e', 'E':
				if v.currentTask != nil {
					v.showTaskForm(v.currentTask)
				}
				return nil
			case 'd', 'D':
				if v.currentTask != nil {
					v.confirmDelete()
				}
				return nil
			case 'r', 'R':
				if v.currentTask != nil {
					v.runTask()
				}
				return nil
			case 't', 'T':
				if v.currentTask != nil {
					v.toggleTask()
				}
				return nil
			}
		}
		return event
	})
	
	return form
}

// setupGlobalKeyBindings 设置全局快捷键
func (v *TaskManagerView) setupGlobalKeyBindings() {
	// 暂时不设置全局快捷键，让子组件处理
	// 因为 v.view.SetInputCapture 会拦截所有事件
}

// toggleFocus 切换焦点
func (v *TaskManagerView) toggleFocus() {
	// 切换焦点状态
	if v.menuFocused {
		v.focusContent()
	} else {
		v.focusMenu()
	}
}

// focusMenu 聚焦到菜单
func (v *TaskManagerView) focusMenu() {
	if v.app == nil {
		return
	}
	v.menuFocused = true
	v.app.SetFocus(v.menuList)
	v.updateStatus("Focus: Menu (Use arrow keys to navigate)")
}

// focusContent 聚焦到内容区域
func (v *TaskManagerView) focusContent() {
	if v.app == nil {
		return
	}
	v.menuFocused = false
	
	switch v.currentPage {
	case "list":
		v.app.SetFocus(v.taskList)
		v.updateStatus("Focus: Task List")
	case "detail":
		// 详情页面的焦点应该在按钮表单上
		// 但我们不能直接访问它，因为它是动态创建的
		// 所以让页面自己处理焦点
		v.updateStatus("Focus: Task Details")
	case "form":
		v.app.SetFocus(v.taskForm)
		v.updateStatus("Focus: Task Form")
	default:
		v.app.SetFocus(v.taskList)
		v.updateStatus("Focus: Content")
	}
}

// handleCtrlW 处理 Ctrl+W 组合键
func (v *TaskManagerView) handleCtrlW(event *tcell.EventKey) *tcell.EventKey {
	// Ctrl+W 组合键处理
	if event.Key() == tcell.KeyCtrlW {
		v.ctrlWPressed = true
		return nil
	}
	
	// 如果已经按下了 Ctrl+W，等待第二个键
	if v.ctrlWPressed {
		v.ctrlWPressed = false
		switch event.Rune() {
		case 'w', 'W':
			v.toggleFocus()
			return nil
		case 'h', 'H':
			v.focusMenu()
			return nil
		case 'l', 'L':
			v.focusContent()
			return nil
		}
	}
	
	return event
}

// setupKeyBindings 设置快捷键
func (v *TaskManagerView) setupKeyBindings() {
	// 任务列表快捷键
	v.taskList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Always pass through function keys to global handler
		if event.Key() >= tcell.KeyF1 && event.Key() <= tcell.KeyF12 {
			return event
		}
		
		// 先处理 Ctrl+W
		if result := v.handleCtrlW(event); result == nil {
			return nil
		}
		
		// 处理 Ctrl+L 清除日志
		if event.Key() == tcell.KeyCtrlL {
			v.clearLogs()
			return nil
		}
		
		switch event.Key() {
		case tcell.KeyEsc:
			// ESC 返回到菜单
			v.focusMenu()
			return nil
		case tcell.KeyEnter:
			row, _ := v.taskList.GetSelection()
			if row > 0 && row <= len(v.tasks) {
				v.currentTask = v.tasks[row-1]
				v.showTaskDetail()
			}
			return nil
		case tcell.KeyRune:
			row, col := v.taskList.GetSelection()
			switch event.Rune() {
			// vim 风格导航
			case 'j', 'J':
				// 向下移动
				if row < v.taskList.GetRowCount()-1 {
					v.taskList.Select(row+1, col)
				}
				return nil
			case 'k', 'K':
				// 向上移动
				if row > 1 { // 保持在数据行，跳过表头
					v.taskList.Select(row-1, col)
				}
				return nil
			case 'g':
				// gg - 跳到第一行
				v.taskList.Select(1, col)
				return nil
			case 'G':
				// G - 跳到最后一行
				if v.taskList.GetRowCount() > 1 {
					v.taskList.Select(v.taskList.GetRowCount()-1, col)
				}
				return nil
			// 操作快捷键
			case 'a', 'A':
				v.showTaskForm(nil)
				return nil
			case 'd', 'D':
				if row > 0 && row <= len(v.tasks) {
					v.currentTask = v.tasks[row-1]
					v.confirmDelete()
				}
				return nil
			case 'e', 'E':
				if row > 0 && row <= len(v.tasks) {
					v.currentTask = v.tasks[row-1]
					v.showTaskForm(v.currentTask)
				}
				return nil
			case 'r', 'R':
				v.refreshTasks()
				return nil
			case 't', 'T':
				if row > 0 && row <= len(v.tasks) {
					v.currentTask = v.tasks[row-1]
					v.toggleTask()
				}
				return nil
			}
		}
		return event
	})
	
	// 详情视图不需要键盘处理，因为 TextView 不接收焦点
	// 键盘处理已经移到 createDetailButtons 的表单中
}

// showTaskList 显示任务列表
func (v *TaskManagerView) showTaskList() {
	v.pages.SwitchToPage("list")
	v.currentPage = "list"
	v.refreshTasks()
	// 自动聚焦到任务列表
	if v.app != nil {
		v.app.SetFocus(v.taskList)
		v.menuFocused = false
		v.updateStatus("Focus: Task List")
	}
}

// showTaskDetail 显示任务详情
func (v *TaskManagerView) showTaskDetail() {
	if v.currentTask == nil {
		return
	}
	
	// Task status is now displayed in the detail view
	
	// 格式化详情文本
	detail := fmt.Sprintf(`[yellow]Task Details[white]
─────────────────────────────────────────

[cyan]Basic Information:[white]
  ID:          %s
  Name:        %s
  Type:        %s
  Category:    %s
  Description: %s
  Version:     %s
  Created:     %s
  Updated:     %s

[cyan]Status:[white]
  Enabled:      %v
  Last Run:     %s
  Next Run:     %s
  Run Count:    %d
  Success:      %d
  Failed:       %d
  Success Rate: %.2f%%

[cyan]Configuration:[white]
  Priority:     %d
  Timeout:      %d seconds
  Max Retries:  %d
  Concurrency:  %d

[cyan]Request:[white]
  URL:     %s
  Method:  %s
  
[cyan]Schedule:[white]
  Type:       %s
  Expression: %s
  Timezone:   %s

[cyan]Storage:[white]
  %+v

[cyan]Tags:[white]
  %s
`,
		v.currentTask.ID.Hex(),
		v.currentTask.Name,
		v.currentTask.Type,
		v.currentTask.Category,
		v.currentTask.Description,
		v.currentTask.Version,
		v.currentTask.CreatedAt.Format("2006-01-02 15:04:05"),
		v.currentTask.UpdatedAt.Format("2006-01-02 15:04:05"),
		v.currentTask.Status.Enabled,
		formatTime(v.currentTask.Status.LastRun),
		formatTime(v.currentTask.Status.NextRun),
		v.currentTask.Status.RunCount,
		v.currentTask.Status.SuccessCount,
		v.currentTask.Status.FailureCount,
		calculateSuccessRate(v.currentTask.Status),
		v.currentTask.Config.Priority,
		v.currentTask.Config.Timeout,
		v.currentTask.Config.MaxRetries,
		v.currentTask.Config.Concurrency,
		v.currentTask.Request.URL,
		v.currentTask.Request.Method,
		v.currentTask.Schedule.Type,
		v.currentTask.Schedule.Expression,
		v.currentTask.Schedule.Timezone,
		v.currentTask.Storage,
		strings.Join(v.currentTask.Tags, ", "),
	)
	
	v.taskDetail.SetText(detail)
	v.pages.SwitchToPage("detail")
	v.currentPage = "detail"
	// 焦点会自动在按钮表单上（通过布局设置）
	v.menuFocused = false
	v.updateStatus("Task Details - Use B/ESC to go back, E to edit, D to delete")
}

// showTaskForm 显示任务表单
func (v *TaskManagerView) showTaskForm(task *task.TaskDocument) {
	v.taskForm.Clear(true)
	
	if task != nil {
		// 编辑模式 - 填充现有数据
		v.editMode = true
		v.currentTask = task
		
		v.taskForm.AddInputField("Name", task.Name, 40, nil, nil)
		v.taskForm.AddInputField("Description", task.Description, 60, nil, nil)
		// ... 填充其他字段
	} else {
		// 新建模式
		v.editMode = false
		v.currentTask = nil
		v.createTaskForm() // 重新创建空表单
	}
	
	v.pages.SwitchToPage("form")
	v.currentPage = "form"
	
	// 自动聚焦到表单
	if v.app != nil {
		v.app.SetFocus(v.taskForm)
		v.menuFocused = false
		v.updateStatus("Focus: Task Form")
	}
}

// refreshTasks 刷新任务列表
func (v *TaskManagerView) refreshTasks() {
	v.LogInfo("Refreshing task list...")
	
	// 获取任务列表
	tasks, err := v.service.ListTasks(v.filter)
	if err != nil {
		v.logger.Error("Failed to list tasks", "error", err)
		v.LogError("Failed to list tasks: %v", err)
		v.updateStatus(fmt.Sprintf("Error: %v", err))
		return
	}
	
	v.tasks = tasks
	
	// 清空表格（保留表头）
	for i := v.taskList.GetRowCount() - 1; i > 0; i-- {
		v.taskList.RemoveRow(i)
	}
	
	// 填充任务数据
	for i, task := range tasks {
		row := i + 1
		
		// 状态颜色
		statusText := "Disabled"
		statusColor := tcell.ColorRed
		if task.Status.Enabled {
			statusText = "Enabled"
			statusColor = tcell.ColorGreen
		}
		
		// 成功率
		successRate := calculateSuccessRate(task.Status)
		
		// 添加行
		v.taskList.SetCell(row, 0, tview.NewTableCell(task.Name))
		v.taskList.SetCell(row, 1, tview.NewTableCell(task.Type))
		v.taskList.SetCell(row, 2, tview.NewTableCell(task.Category))
		v.taskList.SetCell(row, 3, tview.NewTableCell(statusText).SetTextColor(statusColor))
		v.taskList.SetCell(row, 4, tview.NewTableCell(task.Schedule.Type))
		v.taskList.SetCell(row, 5, tview.NewTableCell(formatTime(task.Status.LastRun)))
		v.taskList.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%.1f%%", successRate)))
		v.taskList.SetCell(row, 7, tview.NewTableCell("[Edit] [Delete] [Run]"))
	}
	
	v.LogSuccess("Loaded %d tasks", len(tasks))
	v.updateStatus(fmt.Sprintf("Loaded %d tasks", len(tasks)))
}

// saveTask 保存任务
func (v *TaskManagerView) saveTask() {
	// 从表单收集数据
	name := v.taskForm.GetFormItemByLabel("Name").(*tview.InputField).GetText()
	description := v.taskForm.GetFormItemByLabel("Description").(*tview.InputField).GetText()
	
	if name == "" {
		v.updateStatus("Error: Task name is required")
		return
	}
	
	if v.editMode && v.currentTask != nil {
		// 更新现有任务
		update := map[string]interface{}{
			"name":        name,
			"description": description,
			// ... 其他字段
		}
		
		err := v.service.UpdateTask(v.currentTask.ID.Hex(), update)
		if err != nil {
			v.updateStatus(fmt.Sprintf("Error updating task: %v", err))
			return
		}
		v.updateStatus("Task updated successfully")
	} else {
		// 创建新任务
		newTask := &task.TaskDocument{
			Name:        name,
			Description: description,
			Type:        "http",
			Category:    "general",
			Status: task.TaskStatus{
				Enabled: true,
			},
			Config: task.TaskConfiguration{
				Priority:   5,
				Timeout:    30,
				MaxRetries: 3,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		
		_, err := v.service.CreateTask(newTask)
		if err != nil {
			v.updateStatus(fmt.Sprintf("Error creating task: %v", err))
			return
		}
		v.updateStatus("Task created successfully")
	}
	
	v.showTaskList()
}

// confirmDelete 确认删除
func (v *TaskManagerView) confirmDelete() {
	if v.currentTask == nil {
		return
	}
	
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete task '%s'?", v.currentTask.Name)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Delete" {
				v.deleteTask()
			}
			v.pages.RemovePage("confirm")
		})
	
	v.pages.AddPage("confirm", modal, true, true)
}

// deleteTask 删除任务
func (v *TaskManagerView) deleteTask() {
	if v.currentTask == nil {
		return
	}
	
	err := v.service.DeleteTask(v.currentTask.ID.Hex())
	if err != nil {
		v.updateStatus(fmt.Sprintf("Error deleting task: %v", err))
		return
	}
	
	v.updateStatus("Task deleted successfully")
	v.currentTask = nil
	v.showTaskList()
}

// runTask 立即运行任务
func (v *TaskManagerView) runTask() {
	if v.currentTask == nil {
		return
	}
	
	ctx := context.Background()
	
	// Log to the log view
	v.LogInfo("Submitting task '%s' for execution...", v.currentTask.Name)
	v.updateStatus(fmt.Sprintf("Submitting task '%s' for execution...", v.currentTask.Name))
	
	// Convert TaskDocument to executable task
	execTask := v.convertToExecutableTask(v.currentTask)
	v.LogDebug("Task ID: %s, URL: %s", execTask.ID, v.currentTask.Request.URL)
	
	// Serialize task
	data, err := json.Marshal(execTask)
	if err != nil {
		v.LogError("Failed to marshal task: %v", err)
		v.updateStatus(fmt.Sprintf("❌ Error: Failed to marshal task: %v", err))
		return
	}
	
	// Push to Redis queue
	queueKey := "crawler:queue:pending"
	v.LogDebug("Pushing task to Redis queue: %s", queueKey)
	if err := v.redis.LPush(ctx, queueKey, data).Err(); err != nil {
		v.LogError("Failed to submit task to Redis: %v", err)
		v.updateStatus(fmt.Sprintf("❌ Error: Failed to submit task: %v", err))
		return
	}
	
	// Update task status in MongoDB
	v.updateTaskLastRun(ctx, v.currentTask.ID)
	v.LogDebug("Updated task last_run in MongoDB")
	
	// Log success
	v.LogSuccess("Task '%s' submitted successfully!", v.currentTask.Name)
	v.LogInfo("Task ID: %s", execTask.ID)
	v.LogInfo("Queue: %s", queueKey)
	v.updateStatus(fmt.Sprintf("✅ Task '%s' submitted successfully!", v.currentTask.Name))
	
	// Update detail view with success info
	successMsg := fmt.Sprintf(`[green]Task Submitted Successfully![white]

Task: %s
ID: %s
URL: %s
Queue: %s
Time: %s

The task has been added to the Redis queue and will be processed by available worker nodes.

To monitor execution:
1. Check worker node logs
2. View Redis queue: crawler:queue:pending
3. Check results in MongoDB

Press [yellow]ESC[white] to return to list view.`,
		v.currentTask.Name,
		execTask.ID,
		v.currentTask.Request.URL,
		queueKey,
		time.Now().Format("2006-01-02 15:04:05"))
	
	v.taskDetail.SetText(successMsg)
	
	// Keep focus on taskDetail to allow ESC to work properly
	v.app.SetFocus(v.taskDetail)
}

// convertToExecutableTask converts TaskDocument to executable task
func (v *TaskManagerView) convertToExecutableTask(doc *task.TaskDocument) *task.Task {
	// Generate unique task instance ID
	instanceID := fmt.Sprintf("%s_%d", doc.ID.Hex(), time.Now().Unix())
	
	// Create executable task
	t := &task.Task{
		ID:       instanceID,
		ParentID: doc.ID.Hex(),
		Name:     doc.Name,
		Type:     doc.Type,
		Priority: doc.Config.Priority,
		
		// Request details
		URL:     doc.Request.URL,
		Method:  doc.Request.Method,
		Headers: doc.Request.Headers,
		Body:    v.convertRequestBody(doc.Request.Body),
		
		// Configuration
		Timeout:    time.Duration(doc.Config.Timeout) * time.Second,
		MaxRetries: doc.Config.MaxRetries,
		RetryDelay: time.Duration(doc.Config.RetryDelay) * time.Second,
		
		// Metadata
		CreatedAt: time.Now(),
		Status:    task.StatusPending,
		
		// Storage configuration
		StorageConf: v.buildStorageConfig(doc.Storage),
	}
	
	// Add extract rules from extraction config
	if doc.Extraction.Rules != nil && len(doc.Extraction.Rules) > 0 {
		t.ExtractRules = make([]task.ExtractRule, len(doc.Extraction.Rules))
		for i, ext := range doc.Extraction.Rules {
			t.ExtractRules[i] = task.ExtractRule{
				Field:     ext.Field,
				Selector:  ext.Path,  // Path is used as selector
				Type:      ext.Type,
				Required:  ext.Required,
				Default:   ext.Default,
			}
		}
	}
	
	return t
}

// updateTaskLastRun updates the last run time in MongoDB
func (v *TaskManagerView) updateTaskLastRun(ctx context.Context, taskID primitive.ObjectID) {
	update := bson.M{
		"$set": bson.M{
			"status.last_run": time.Now(),
			"updated_at":      time.Now(),
		},
		"$inc": bson.M{
			"status.run_count": 1,
		},
	}
	
	if _, err := v.service.UpdateTaskRaw(taskID, update); err != nil {
		v.logger.Error("Failed to update task last run", "error", err)
	}
}

// toggleTask 启用/禁用任务
func (v *TaskManagerView) toggleTask() {
	if v.currentTask == nil {
		return
	}
	
	if v.currentTask.Status.Enabled {
		err := v.service.DisableTask(v.currentTask.ID.Hex())
		if err != nil {
			v.updateStatus(fmt.Sprintf("Error disabling task: %v", err))
			return
		}
		v.updateStatus("Task disabled")
	} else {
		err := v.service.EnableTask(v.currentTask.ID.Hex())
		if err != nil {
			v.updateStatus(fmt.Sprintf("Error enabling task: %v", err))
			return
		}
		v.updateStatus("Task enabled")
	}
	
	v.refreshTasks()
}

// showImportDialog 显示导入对话框
func (v *TaskManagerView) showImportDialog() {
	// TODO: 实现导入功能
	v.updateStatus("Import feature coming soon")
}

// showExportDialog 显示导出对话框
func (v *TaskManagerView) showExportDialog() {
	// TODO: 实现导出功能
	v.updateStatus("Export feature coming soon")
}

// showStats 显示统计信息
func (v *TaskManagerView) showStats() {
	// TODO: 实现统计视图
	v.updateStatus("Statistics feature coming soon")
}

// updateStatus 更新状态栏
func (v *TaskManagerView) updateStatus(message string) {
	timestamp := time.Now().Format("15:04:05")
	v.statusBar.SetText(fmt.Sprintf("[gray]%s[white] | %s", timestamp, message))
}

// GetView 获取视图
func (v *TaskManagerView) GetView() tview.Primitive {
	return v.view
}

// SetApp 设置应用程序引用
func (v *TaskManagerView) SetApp(app *tview.Application) {
	v.app = app
}

// Refresh 刷新视图
func (v *TaskManagerView) Refresh(ctx context.Context) {
	switch v.currentPage {
	case "list":
		v.refreshTasks()
	case "detail":
		if v.currentTask != nil {
			// 重新加载任务详情
			task, err := v.service.GetTask(v.currentTask.ID.Hex())
			if err == nil {
				v.currentTask = task
				v.showTaskDetail()
			}
		}
	}
}

// RefreshAndFocus 刷新并设置焦点到任务列表
func (v *TaskManagerView) RefreshAndFocus() {
	v.showTaskList()
	v.focusContent()
}

// Helper functions

// convertRequestBody converts interface{} body to []byte
func (v *TaskManagerView) convertRequestBody(body interface{}) []byte {
	if body == nil {
		return nil
	}
	
	switch val := body.(type) {
	case []byte:
		return val
	case string:
		return []byte(val)
	default:
		// Try to marshal as JSON
		data, _ := json.Marshal(body)
		return data
	}
}

// buildStorageConfig builds storage config from StorageConfiguration
func (v *TaskManagerView) buildStorageConfig(storage task.StorageConfiguration) task.StorageConfig {
	config := task.StorageConfig{
		Options: make(map[string]interface{}),
	}
	
	// Use first target if available
	if len(storage.Targets) > 0 {
		target := storage.Targets[0]
		config.Type = target.Type
		
		// Extract common config fields
		if target.Config != nil {
			if db, ok := target.Config["database"].(string); ok {
				config.Database = db
			}
			if coll, ok := target.Config["collection"].(string); ok {
				config.Collection = coll
			}
			if table, ok := target.Config["table"].(string); ok {
				config.Table = table
			}
			
			// Copy all config as options
			for k, v := range target.Config {
				config.Options[k] = v
			}
		}
	}
	
	return config
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	return t.Format("2006-01-02 15:04")
}

func calculateSuccessRate(status task.TaskStatus) float64 {
	total := status.SuccessCount + status.FailureCount
	if total == 0 {
		return 0
	}
	return float64(status.SuccessCount) / float64(total) * 100
}

// Log management methods

// addLog adds a log message to the log view
func (v *TaskManagerView) addLog(level string, message string) {
	timestamp := time.Now().Format("15:04:05.000")
	
	// Color code based on level
	var colorTag string
	switch level {
	case "error":
		colorTag = "[red]ERROR[white]"
	case "warn":
		colorTag = "[yellow]WARN[white]"
	case "info":
		colorTag = "[cyan]INFO[white]"
	case "debug":
		colorTag = "[gray]DEBUG[white]"
	case "success":
		colorTag = "[green]SUCCESS[white]"
	default:
		colorTag = "[white]LOG[white]"
	}
	
	// Format log message
	logLine := fmt.Sprintf("[gray]%s[white] %s %s", timestamp, colorTag, message)
	
	// Add to buffer
	v.logBuffer = append(v.logBuffer, logLine)
	
	// Keep only last maxLogs lines
	if len(v.logBuffer) > v.maxLogs {
		v.logBuffer = v.logBuffer[len(v.logBuffer)-v.maxLogs:]
	}
	
	// Update log view
	v.updateLogView()
}

// updateLogView updates the log view with current buffer
func (v *TaskManagerView) updateLogView() {
	if v.logView == nil {
		return
	}
	
	// Join all log lines
	content := strings.Join(v.logBuffer, "\n")
	v.logView.SetText(content)
	
	// Scroll to bottom
	v.logView.ScrollToEnd()
}

// clearLogs clears the log view
func (v *TaskManagerView) clearLogs() {
	v.logBuffer = []string{}
	v.updateLogView()
	v.addLog("info", "Log cleared")
}

// LogEvent logs an event to the log view (public method for external use)
func (v *TaskManagerView) LogEvent(level string, message string) {
	v.addLog(level, message)
}

// LogDebug logs a debug message
func (v *TaskManagerView) LogDebug(format string, args ...interface{}) {
	v.addLog("debug", fmt.Sprintf(format, args...))
}

// LogInfo logs an info message
func (v *TaskManagerView) LogInfo(format string, args ...interface{}) {
	v.addLog("info", fmt.Sprintf(format, args...))
}

// LogWarn logs a warning message
func (v *TaskManagerView) LogWarn(format string, args ...interface{}) {
	v.addLog("warn", fmt.Sprintf(format, args...))
}

// LogError logs an error message
func (v *TaskManagerView) LogError(format string, args ...interface{}) {
	v.addLog("error", fmt.Sprintf(format, args...))
}

// LogSuccess logs a success message
func (v *TaskManagerView) LogSuccess(format string, args ...interface{}) {
	v.addLog("success", fmt.Sprintf(format, args...))
}