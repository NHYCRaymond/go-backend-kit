package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	app := tview.NewApplication()
	
	// Create a simple text view
	textView := tview.NewTextView().
		SetText("Press F1-F10 to test function keys\nPress ESC to quit").
		SetDynamicColors(true)
	
	// Set up global key handler
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Log all key events
		text := textView.GetText(false)
		
		switch event.Key() {
		case tcell.KeyF1:
			textView.SetText(text + "\n[green]F1 pressed![white]")
			return nil
		case tcell.KeyF2:
			textView.SetText(text + "\n[green]F2 pressed![white]")
			return nil
		case tcell.KeyF3:
			textView.SetText(text + "\n[green]F3 pressed![white]")
			return nil
		case tcell.KeyF4:
			textView.SetText(text + "\n[green]F4 pressed![white]")
			return nil
		case tcell.KeyF5:
			textView.SetText(text + "\n[green]F5 pressed![white]")
			return nil
		case tcell.KeyF6:
			textView.SetText(text + "\n[green]F6 pressed![white]")
			return nil
		case tcell.KeyF10:
			textView.SetText(text + "\n[red]F10 pressed - Exiting![white]")
			app.Stop()
			return nil
		case tcell.KeyEsc:
			app.Stop()
			return nil
		default:
			textView.SetText(text + fmt.Sprintf("\n[yellow]Key: %v[white]", event.Key()))
		}
		
		return event
	})
	
	// Run the app
	app.SetRoot(textView, true)
	if err := app.Run(); err != nil {
		panic(err)
	}
}