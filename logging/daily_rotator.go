package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyRotator implements io.WriteCloser with daily log rotation
type DailyRotator struct {
	baseFilePath string
	currentFile  *os.File
	currentDate  string
	maxAge       int // days to keep old files
	mu           sync.Mutex
}

// NewDailyRotator creates a new daily log rotator
func NewDailyRotator(baseFilePath string, maxAge int) (*DailyRotator, error) {
	dr := &DailyRotator{
		baseFilePath: baseFilePath,
		maxAge:       maxAge,
	}

	// Open initial file
	if err := dr.rotate(); err != nil {
		return nil, err
	}

	// Start cleanup routine
	go dr.cleanupRoutine()

	return dr, nil
}

// Write implements io.Writer
func (dr *DailyRotator) Write(p []byte) (n int, err error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	// Check if we need to rotate
	currentDate := time.Now().Format("2006-01-02")
	if currentDate != dr.currentDate {
		if err := dr.rotate(); err != nil {
			return 0, err
		}
	}

	return dr.currentFile.Write(p)
}

// Close implements io.Closer
func (dr *DailyRotator) Close() error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.currentFile != nil {
		return dr.currentFile.Close()
	}
	return nil
}

// rotate creates a new log file for the current date
func (dr *DailyRotator) rotate() error {
	// Close current file if exists
	if dr.currentFile != nil {
		dr.currentFile.Close()
	}

	// Generate new filename
	currentDate := time.Now().Format("2006-01-02")
	dir := filepath.Dir(dr.baseFilePath)
	base := filepath.Base(dr.baseFilePath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	// Create directory if needed
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open new file
	fileName := filepath.Join(dir, fmt.Sprintf("%s.%s%s", name, currentDate, ext))
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	dr.currentFile = file
	dr.currentDate = currentDate

	// Create symlink to current log
	linkPath := dr.baseFilePath
	os.Remove(linkPath) // Remove old symlink
	if err := os.Symlink(fileName, linkPath); err != nil {
		// Log error but don't fail
		fmt.Fprintf(os.Stderr, "Failed to create symlink: %v\n", err)
	}

	return nil
}

// cleanupRoutine runs daily to clean up old log files
func (dr *DailyRotator) cleanupRoutine() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		dr.cleanup()
	}
}

// cleanup removes log files older than maxAge days
func (dr *DailyRotator) cleanup() {
	if dr.maxAge <= 0 {
		return
	}

	dir := filepath.Dir(dr.baseFilePath)
	base := filepath.Base(dr.baseFilePath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	// Get cutoff date
	cutoff := time.Now().AddDate(0, 0, -dr.maxAge)

	// List all files in directory
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read log directory: %v\n", err)
		return
	}

	// Check each file
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if it matches our log pattern
		fileName := file.Name()
		if !matchesLogPattern(fileName, name, ext) {
			continue
		}

		// Extract date from filename
		dateStr := extractDateFromFilename(fileName, name, ext)
		if dateStr == "" {
			continue
		}

		// Parse date
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		// Remove if older than cutoff
		if fileDate.Before(cutoff) {
			filePath := filepath.Join(dir, fileName)
			if err := os.Remove(filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to remove old log file %s: %v\n", filePath, err)
			}
		}
	}
}

// matchesLogPattern checks if filename matches our log file pattern
func matchesLogPattern(fileName, baseName, ext string) bool {
	expectedPrefix := baseName + "."
	expectedSuffix := ext

	return len(fileName) > len(expectedPrefix)+len(expectedSuffix) &&
		fileName[:len(expectedPrefix)] == expectedPrefix &&
		fileName[len(fileName)-len(expectedSuffix):] == expectedSuffix
}

// extractDateFromFilename extracts the date part from log filename
func extractDateFromFilename(fileName, baseName, ext string) string {
	expectedPrefix := baseName + "."
	expectedSuffix := ext

	if !matchesLogPattern(fileName, baseName, ext) {
		return ""
	}

	dateStart := len(expectedPrefix)
	dateEnd := len(fileName) - len(expectedSuffix)

	return fileName[dateStart:dateEnd]
}

// MultiWriter creates a writer that duplicates writes to all provided writers
func MultiWriter(writers ...io.Writer) io.Writer {
	return io.MultiWriter(writers...)
}
