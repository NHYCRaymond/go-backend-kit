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
		SetText("Press F1, F2, F3 to test\nPress Q to quit\n\nWaiting for key press...")
	
	// Set input capture at APPLICATION level BEFORE SetRoot
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		text := textView.GetText(false)
		
		switch event.Key() {
		case tcell.KeyF1:
			textView.SetText(text + fmt.Sprintf("\nF1 pressed!"))
			return nil
		case tcell.KeyF2:
			textView.SetText(text + fmt.Sprintf("\nF2 pressed!"))
			return nil
		case tcell.KeyF3:
			textView.SetText(text + fmt.Sprintf("\nF3 pressed!"))
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'q' || event.Rune() == 'Q' {
				app.Stop()
				return nil
			}
		}
		
		textView.SetText(text + fmt.Sprintf("\nOther key: %v", event.Key()))
		return event
	})
	
	// Set root AFTER SetInputCapture
	if err := app.SetRoot(textView, true).Run(); err != nil {
		panic(err)
	}
}