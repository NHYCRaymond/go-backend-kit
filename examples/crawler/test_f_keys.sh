#!/bin/bash

echo "Testing F keys in TUI..."
echo "================================================"
echo "Instructions:"
echo "1. Run this script in one terminal"
echo "2. Open another terminal to watch the log: tail -f tui-debug.log"
echo "3. Press F1, F2, F3 to test view switching"
echo "4. Check if the status bar shows 'F key pressed: KeyF1' etc."
echo "5. Check if views actually switch"
echo "================================================"
echo ""
echo "Starting TUI with debug logging..."

# Clear old log
rm -f tui-debug.log

# Run TUI with debug config
go run tui/main.go -config config/tui-debug.yaml

echo ""
echo "TUI exited. Check tui-debug.log for debug information."