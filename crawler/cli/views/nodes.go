package views

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/distributed"
	"github.com/rivo/tview"
)

// NodesView represents the nodes management view
type NodesView struct {
	// UI components
	view        *tview.Flex
	table       *tview.Table
	details     *tview.TextView
	commandForm *tview.Form

	// Data
	registry     *distributed.Registry
	logger       *slog.Logger
	nodes        []*distributed.NodeInfo
	selectedNode *distributed.NodeInfo
}

// NodesConfig holds nodes view configuration
type NodesConfig struct {
	Registry *distributed.Registry
	Logger   *slog.Logger
}

// NewNodesView creates a new nodes view
func NewNodesView(config *NodesConfig) *NodesView {
	v := &NodesView{
		registry: config.Registry,
		logger:   config.Logger,
		nodes:    []*distributed.NodeInfo{},
	}

	// Create components
	v.createTable()
	v.createDetails()
	v.createCommandForm()
	v.createLayout()

	return v
}

// createTable creates the nodes table
func (v *NodesView) createTable() {
	v.table = tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0).
		SetSelectable(true, false)

	// Set headers
	headers := []string{"ID", "Hostname", "IP", "Port", "Status", "Workers", "Tasks", "CPU%", "Mem%", "Started"}
	for i, header := range headers {
		v.table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	// Set selection handler
	v.table.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row <= len(v.nodes) {
			v.selectedNode = v.nodes[row-1]
			v.updateDetails()
		}
	})
}

// createDetails creates the node details view
func (v *NodesView) createDetails() {
	v.details = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	
	v.details.SetBorder(true).
		SetTitle(" Node Details ").
		SetTitleAlign(tview.AlignLeft)
}

// createCommandForm creates the command form
func (v *NodesView) createCommandForm() {
	v.commandForm = tview.NewForm()
	
	// Add command dropdown
	commands := []string{"Pause", "Resume", "Restart", "Remove"}
	v.commandForm.AddDropDown("Command", commands, 0, nil)
	
	// Add execute button
	v.commandForm.AddButton("Execute", func() {
		v.executeCommand()
	})
	
	// Add cancel button
	v.commandForm.AddButton("Cancel", func() {
		v.commandForm.Clear(false)
	})
	
	v.commandForm.SetBorder(true).
		SetTitle(" Node Commands ").
		SetTitleAlign(tview.AlignLeft)
	
	// Note: Form.SetInputCapture doesn't work (GitHub issue #181)
	// Function keys are handled at application level
}

// createLayout creates the nodes view layout
func (v *NodesView) createLayout() {
	// Right panel - details and commands
	rightPanel := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.details, 0, 2, false).
		AddItem(v.commandForm, 8, 0, false)

	// Main layout
	v.view = tview.NewFlex().
		AddItem(v.table, 0, 2, true).
		AddItem(rightPanel, 0, 1, false)
}

// GetView returns the nodes view
func (v *NodesView) GetView() tview.Primitive {
	return v.view
}

// Refresh refreshes the nodes data
func (v *NodesView) Refresh(ctx context.Context) {
	// Get all nodes from registry (not just healthy ones)
	nodes, err := v.registry.ListNodes(ctx)
	if err != nil {
		v.logger.Error("Failed to list nodes", "error", err)
		// Show error in details panel
		v.details.Clear()
		fmt.Fprintf(v.details, "[red]Error loading nodes:[white]\n%v", err)
		return
	}

	v.nodes = nodes
	v.updateTable()
	
	// If no nodes, show message
	if len(nodes) == 0 {
		v.details.Clear()
		fmt.Fprintf(v.details, "[yellow]No nodes registered[white]\n\nStart a node with:\n  go run node/main.go")
	} else {
		v.updateDetails()
	}
}

// updateTable updates the nodes table
func (v *NodesView) updateTable() {
	// Clear table (except header)
	for i := v.table.GetRowCount() - 1; i > 0; i-- {
		v.table.RemoveRow(i)
	}

	// Add nodes
	for i, node := range v.nodes {
		row := i + 1
		
		// Format status with color
		statusCell := tview.NewTableCell(node.Status)
		switch node.Status {
		case "active":
			statusCell.SetTextColor(tview.Styles.TertiaryTextColor)
		case "paused":
			statusCell.SetTextColor(tview.Styles.PrimaryTextColor)
		case "error":
			statusCell.SetTextColor(tview.Styles.ContrastSecondaryTextColor)
		default:
			statusCell.SetTextColor(tview.Styles.SecondaryTextColor)
		}

		// Format uptime
		uptime := "-"
		if !node.StartedAt.IsZero() {
			uptime = formatDuration(time.Since(node.StartedAt))
		}

		// Handle short IDs gracefully
		shortID := node.ID
		if len(node.ID) > 8 {
			shortID = node.ID[:8]
		}
		v.table.SetCell(row, 0, tview.NewTableCell(shortID))
		v.table.SetCell(row, 1, tview.NewTableCell(node.Hostname))
		v.table.SetCell(row, 2, tview.NewTableCell(node.IP))
		v.table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", node.Port)))
		v.table.SetCell(row, 4, statusCell)
		v.table.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%d/%d", node.ActiveWorkers, node.MaxWorkers)))
		v.table.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d", node.TasksProcessed)))
		v.table.SetCell(row, 7, tview.NewTableCell(fmt.Sprintf("%.1f", node.CPUUsage)))
		v.table.SetCell(row, 8, tview.NewTableCell(fmt.Sprintf("%.1f", node.MemoryUsage)))
		v.table.SetCell(row, 9, tview.NewTableCell(uptime))
	}
}

// updateDetails updates the node details view
func (v *NodesView) updateDetails() {
	v.details.Clear()
	
	if v.selectedNode == nil {
		fmt.Fprintln(v.details, "[gray]No node selected[white]")
		return
	}

	node := v.selectedNode
	
	// Basic information
	fmt.Fprintln(v.details, "[yellow]Basic Information:[white]")
	fmt.Fprintf(v.details, "  ID:         %s\n", node.ID)
	fmt.Fprintf(v.details, "  Hostname:   %s\n", node.Hostname)
	fmt.Fprintf(v.details, "  Address:    %s:%d\n", node.IP, node.Port)
	fmt.Fprintf(v.details, "  Status:     %s\n", node.Status)
	fmt.Fprintln(v.details, "")

	// Performance
	fmt.Fprintln(v.details, "[yellow]Performance:[white]")
	fmt.Fprintf(v.details, "  Workers:    %d / %d\n", node.ActiveWorkers, node.MaxWorkers)
	fmt.Fprintf(v.details, "  Tasks:      %d processed\n", node.TasksProcessed)
	fmt.Fprintf(v.details, "  CPU Usage:  %.2f%%\n", node.CPUUsage)
	fmt.Fprintf(v.details, "  Memory:     %.2f%%\n", node.MemoryUsage)
	fmt.Fprintln(v.details, "")

	// Capabilities
	fmt.Fprintln(v.details, "[yellow]Capabilities:[white]")
	for _, cap := range node.Capabilities {
		fmt.Fprintf(v.details, "  - %s\n", cap)
	}
	fmt.Fprintln(v.details, "")

	// Timing
	fmt.Fprintln(v.details, "[yellow]Timing:[white]")
	fmt.Fprintf(v.details, "  Started:    %s\n", node.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(v.details, "  Updated:    %s\n", node.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(v.details, "  Uptime:     %s\n", formatDuration(time.Since(node.StartedAt)))
}

// executeCommand executes a command on the selected node
func (v *NodesView) executeCommand() {
	if v.selectedNode == nil {
		return
	}

	// Get selected command
	_, command := v.commandForm.GetFormItem(0).(*tview.DropDown).GetCurrentOption()
	
	ctx := context.Background()
	
	switch command {
	case "Pause":
		// Send pause command to node
		v.logger.Info("Pausing node", "node_id", v.selectedNode.ID)
		// TODO: Implement command sending via Redis or gRPC
		
	case "Resume":
		// Send resume command to node
		v.logger.Info("Resuming node", "node_id", v.selectedNode.ID)
		// TODO: Implement command sending
		
	case "Restart":
		// Send restart command to node
		v.logger.Info("Restarting node", "node_id", v.selectedNode.ID)
		// TODO: Implement command sending
		
	case "Remove":
		// Remove node from registry
		v.logger.Info("Removing node", "node_id", v.selectedNode.ID)
		if err := v.registry.UnregisterNode(ctx, v.selectedNode.ID); err != nil {
			v.logger.Error("Failed to remove node", "error", err)
		}
		v.Refresh(ctx)
	}
}

