package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/cli/views"
	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/NHYCRaymond/go-backend-kit/logging/store"
	"github.com/gdamore/tcell/v2"
	"github.com/go-redis/redis/v8"
	"github.com/rivo/tview"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// App represents the TUI application
type App struct {
	// UI components
	app        *tview.Application
	pages      *tview.Pages
	mainLayout *tview.Flex

	// Views
	dashboard   *views.Dashboard
	nodes       *views.NodesView
	tasks       *views.TasksView
	taskManager *views.TaskManagerView
	logs        *views.EnhancedLogsView
	config      *views.ConfigView

	// Status bar
	statusBar *tview.TextView

	// Data sources
	redisClient *redis.Client
	mongoDB     *mongo.Database
	registry    *distributed.Registry
	logger      *slog.Logger

	// Context
	ctx    context.Context
	cancel context.CancelFunc

	// Update ticker
	updateTicker *time.Ticker
}

// Config holds application configuration
type Config struct {
	RedisAddr   string
	RedisPrefix string
	MongoURI    string
	RefreshRate time.Duration
	Logger      *slog.Logger
}

// NewApp creates a new TUI application
func NewApp(config *Config) *App {
	ctx, cancel := context.WithCancel(context.Background())

	// Create Redis client with timeouts to prevent blocking
	redisClient := redis.NewClient(&redis.Options{
		Addr:         config.RedisAddr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Create MongoDB client if URI provided with timeouts
	var mongoDB *mongo.Database
	if config.MongoURI != "" {
		mongoOpts := options.Client().
			ApplyURI(config.MongoURI).
			SetConnectTimeout(5 * time.Second).
			SetSocketTimeout(5 * time.Second).
			SetServerSelectionTimeout(5 * time.Second)
		
		mongoClient, err := mongo.Connect(ctx, mongoOpts)
		if err == nil {
			// Test connection with timeout
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err = mongoClient.Ping(pingCtx, readpref.Primary())
			cancel()
			if err == nil {
				mongoDB = mongoClient.Database("crawler")
			} else {
				config.Logger.Warn("MongoDB ping failed", "error", err)
			}
		} else {
			config.Logger.Warn("MongoDB connection failed", "error", err)
		}
	}

	// Create registry
	registry := distributed.NewRegistry(redisClient, config.RedisPrefix)

	// Create application
	app := &App{
		app:          tview.NewApplication(),
		pages:        tview.NewPages(),
		redisClient:  redisClient,
		mongoDB:     mongoDB,
		registry:     registry,
		logger:       config.Logger,
		ctx:          ctx,
		cancel:       cancel,
		updateTicker: time.NewTicker(config.RefreshRate),
	}

	// Initialize views first
	app.initViews()

	// Setup layout
	app.setupLayout()

	// Start background updates
	go app.startUpdates()

	return app
}

// initViews initializes all views
func (a *App) initViews() {
	// Dashboard view
	a.dashboard = views.NewDashboard(&views.DashboardConfig{
		Registry: a.registry,
		Redis:    a.redisClient,
		MongoDB:  a.mongoDB,
		Logger:   a.logger,
	})

	// Nodes view
	a.nodes = views.NewNodesView(&views.NodesConfig{
		Registry: a.registry,
		Logger:   a.logger,
	})

	// Tasks view (Redis queue view)
	a.tasks = views.NewTasksView(&views.TasksConfig{
		Redis:   a.redisClient,
		Prefix:  "crawler",
		Logger:  a.logger,
	})

	// Task Manager view (MongoDB task management)
	if a.mongoDB != nil {
		a.taskManager = views.NewTaskManagerView(&views.TaskManagerConfig{
			MongoDB: a.mongoDB,
			Redis:   a.redisClient,
			Logger:  a.logger,
		})
		a.taskManager.SetApp(a.app)  // Set app reference for focus management
		a.logger.Info("Task Manager initialized with MongoDB")
	} else {
		a.logger.Warn("MongoDB not connected, Task Manager disabled")
	}

	// Enhanced Logs view with modular logging system
	logStore := store.NewRedisStore(store.RedisConfig{
		Client:     a.redisClient,
		StreamKey:  "crawler:logs:stream",
		PubSubKey:  "crawler:logs:pubsub",
		MaxLen:     1000,
		BufferSize: 100,
	})
	
	a.logs = views.NewEnhancedLogsView(&views.EnhancedLogsConfig{
		Store:      logStore,
		MaxLogs:    1000,
		AutoScroll: true,
	})
	// Set app reference for redraws
	a.logs.SetApp(a.app)

	// Config view
	a.config = views.NewConfigView(&views.ConfigConfig{
		Redis:  a.redisClient,
		Prefix: "crawler",
		Logger: a.logger,
	})

	// Status bar
	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.updateStatus("Ready")
}

// setupLayout sets up the main layout
func (a *App) setupLayout() {
	// Create menu
	menuText := "[yellow]F1[white] Dashboard | [yellow]F2[white] Nodes | [yellow]F3[white] Queue | [yellow]F4[white] Logs | [yellow]F5[white] Config"
	if a.taskManager != nil {
		menuText += " | [yellow]F6[white] Tasks"
	}
	menuText += " | [yellow]F10[white] Quit"
	
	menu := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(menuText)

	// Add pages
	a.pages.AddPage("dashboard", a.dashboard.GetView(), true, true)
	a.pages.AddPage("nodes", a.nodes.GetView(), true, false)
	a.pages.AddPage("tasks", a.tasks.GetView(), true, false)
	a.pages.AddPage("logs", a.logs.GetView(), true, false)
	a.pages.AddPage("config", a.config.GetView(), true, false)
	
	// Add task manager if MongoDB is available
	if a.taskManager != nil {
		a.pages.AddPage("taskmanager", a.taskManager.GetView(), true, false)
	}

	// Create main layout
	a.mainLayout = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(menu, 1, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(a.statusBar, 1, 0, false)
	
	// DON'T set root here - we'll do it in Run() after SetInputCapture
}

// setupKeyBindings sets up global key bindings
func (a *App) setupKeyBindings() {
	// IMPORTANT: We use SetInputCapture at the APPLICATION level
	// This intercepts ALL keys before they reach any component
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Handle function keys globally - ALWAYS consume them
		switch event.Key() {
		case tcell.KeyF1:
			a.switchToView("dashboard")
			return nil  // Consume the event - don't pass to components
		case tcell.KeyF2:
			a.switchToView("nodes")
			return nil  // Consume the event
		case tcell.KeyF3:
			a.switchToView("tasks")
			return nil  // Consume the event
		case tcell.KeyF4:
			a.switchToView("logs")
			return nil  // Consume the event
		case tcell.KeyF5:
			a.switchToView("config")
			return nil  // Consume the event
		case tcell.KeyF6:
			if a.taskManager != nil {
				a.switchToView("taskmanager")
			}
			return nil  // Consume the event
		case tcell.KeyF10:
			a.Stop()
			return nil  // Consume the event
		case tcell.KeyCtrlR:
			a.refresh()
			return nil  // Consume the event
		}
		// Pass through all other keys to components
		return event
	})
}

// switchToView switches to a specific view
func (a *App) switchToView(name string) {
	a.pages.SwitchToPage(name)
	a.updateStatus(fmt.Sprintf("Switched to %s view", name))
	// Don't call Draw() here - tview will automatically redraw after event handling
}

// refresh refreshes the current view - runs in goroutine to avoid blocking
func (a *App) refresh() {
	page, _ := a.pages.GetFrontPage()
	
	// Run refresh in goroutine to avoid blocking the UI
	go func() {
		// Create timeout context for refresh operations
		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		
		switch page {
		case "dashboard":
			a.dashboard.Refresh(ctx)
		case "nodes":
			a.nodes.Refresh(ctx)
		case "tasks":
			a.tasks.Refresh(ctx)
		case "taskmanager":
			if a.taskManager != nil {
				a.taskManager.Refresh(ctx)
			}
		case "logs":
			// Enhanced logs view auto-refreshes via subscription
		case "config":
			a.config.Refresh(ctx)
		}
		
		// Update status in UI thread
		a.app.QueueUpdateDraw(func() {
			a.updateStatus("Refreshed")
		})
	}()
}

// startUpdates starts background updates
func (a *App) startUpdates() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.updateTicker.C:
			// Don't wrap refresh in QueueUpdateDraw since refresh is now async
			// refresh() will handle its own QueueUpdateDraw for UI updates
			a.refresh()
		}
	}
}

// updateStatus updates the status bar
func (a *App) updateStatus(message string) {
	timestamp := time.Now().Format("15:04:05")
	a.statusBar.SetText(fmt.Sprintf("[gray]%s[white] | %s", timestamp, message))
}

// Run starts the TUI application
func (a *App) Run() error {
	a.updateStatus("Starting...")
	
	// Setup key bindings BEFORE SetRoot
	a.setupKeyBindings()
	
	// Set the root and focus
	a.app.SetRoot(a.mainLayout, true).SetFocus(a.mainLayout)
	
	// Perform initial refresh (async to not block startup)
	go a.refresh()
	
	// Run the application
	return a.app.Run()
}

// Stop stops the TUI application
func (a *App) Stop() {
	a.cancel()
	a.updateTicker.Stop()
	if a.logs != nil {
		a.logs.Stop()
	}
	a.redisClient.Close()
	a.app.Stop()
}