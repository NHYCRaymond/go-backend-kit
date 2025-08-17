package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	// Create debug log file
	debugFile, err := os.Create("tui_debug.log")
	if err != nil {
		log.Fatal(err)
	}
	defer debugFile.Close()

	app := tview.NewApplication()
	
	// Create pages
	pages := tview.NewPages()
	
	// Create different views
	view1 := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[green]View 1 - Dashboard[white]\n\nPress F2 or F3 to switch views\nPress F10 to quit")
	
	view2 := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow]View 2 - Nodes[white]\n\nPress F1 or F3 to switch views\nPress F10 to quit")
	
	view3 := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[blue]View 3 - Tasks[white]\n\nPress F1 or F2 to switch views\nPress F10 to quit")
	
	// Add pages
	pages.AddPage("view1", view1, true, true)
	pages.AddPage("view2", view2, true, false)
	pages.AddPage("view3", view3, true, false)
	
	// Create menu
	menu := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[yellow]F1[white] View1 | [yellow]F2[white] View2 | [yellow]F3[white] View3 | [yellow]F10[white] Quit")
	
	// Create status bar
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetText("Status: Ready")
	
	// Main layout
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(menu, 1, 0, false).
		AddItem(pages, 0, 1, true).
		AddItem(status, 1, 0, false)
	
	// CRITICAL: Set input capture BEFORE SetRoot
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Log all key events
		logMsg := fmt.Sprintf("Key event: Key=%v, Rune=%v, Modifiers=%v\n", 
			event.Key(), event.Rune(), event.Modifiers())
		debugFile.WriteString(logMsg)
		
		// Update status to show key press
		status.SetText(fmt.Sprintf("Last key: %v", event.Key()))
		
		switch event.Key() {
		case tcell.KeyF1:
			debugFile.WriteString("F1 pressed - switching to view1\n")
			pages.SwitchToPage("view1")
			status.SetText("Switched to View 1")
			return nil // Consume event
		case tcell.KeyF2:
			debugFile.WriteString("F2 pressed - switching to view2\n")
			pages.SwitchToPage("view2")
			status.SetText("Switched to View 2")
			return nil // Consume event
		case tcell.KeyF3:
			debugFile.WriteString("F3 pressed - switching to view3\n")
			pages.SwitchToPage("view3")
			status.SetText("Switched to View 3")
			return nil // Consume event
		case tcell.KeyF10:
			debugFile.WriteString("F10 pressed - quitting\n")
			app.Stop()
			return nil // Consume event
		}
		
		return event // Pass through other keys
	})
	
	// Set root AFTER input capture
	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}
}