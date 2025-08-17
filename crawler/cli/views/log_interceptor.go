package views

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// LogInterceptor intercepts fmt.Printf and similar output and redirects to log view
type LogInterceptor struct {
	mu       sync.Mutex
	view     *TaskManagerView
	original io.Writer
	buffer   []byte
}

// NewLogInterceptor creates a new log interceptor
func NewLogInterceptor(view *TaskManagerView) *LogInterceptor {
	return &LogInterceptor{
		view:   view,
		buffer: make([]byte, 0, 1024),
	}
}

// Write implements io.Writer interface
func (l *LogInterceptor) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Add to buffer
	l.buffer = append(l.buffer, p...)
	
	// Check for complete lines
	for {
		idx := indexOf(l.buffer, '\n')
		if idx == -1 {
			break
		}
		
		// Extract the line
		line := string(l.buffer[:idx])
		l.buffer = l.buffer[idx+1:]
		
		// Process the line
		l.processLine(line)
	}
	
	return len(p), nil
}

// processLine processes a single line of output
func (l *LogInterceptor) processLine(line string) {
	if l.view == nil {
		return
	}
	
	// Skip empty lines
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	
	// Determine log level based on content
	level := "debug"
	if strings.Contains(line, "ERROR") || strings.Contains(line, "error") || strings.Contains(line, "Failed") {
		level = "error"
	} else if strings.Contains(line, "WARN") || strings.Contains(line, "warn") || strings.Contains(line, "Warning") {
		level = "warn"
	} else if strings.Contains(line, "INFO") || strings.Contains(line, "info") {
		level = "info"
	} else if strings.Contains(line, "SUCCESS") || strings.Contains(line, "success") || strings.Contains(line, "✅") {
		level = "success"
	}
	
	// Log to the view
	l.view.LogEvent(level, line)
}

// Flush flushes any remaining buffer
func (l *LogInterceptor) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if len(l.buffer) > 0 {
		line := string(l.buffer)
		l.buffer = l.buffer[:0]
		l.processLine(line)
	}
}

// indexOf finds the index of a byte in a byte slice
func indexOf(data []byte, b byte) int {
	for i, v := range data {
		if v == b {
			return i
		}
	}
	return -1
}

// RedirectOutput redirects stdout/stderr to the log interceptor
func (l *LogInterceptor) RedirectOutput() {
	// Note: This would typically be done at the application level
	// For now, we'll provide methods that components can use
}

// Printf logs a formatted message (replacement for fmt.Printf)
func (l *LogInterceptor) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.processLine(msg)
}

// Println logs a message with newline (replacement for fmt.Println)
func (l *LogInterceptor) Println(args ...interface{}) {
	msg := fmt.Sprint(args...)
	l.processLine(msg)
}