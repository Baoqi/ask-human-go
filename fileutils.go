package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// FileLock provides cross-platform file locking
type FileLock struct {
	filePath string
	lockFile *os.File
	mutex    sync.Mutex
}

// NewFileLock creates a new file lock for the given path
func NewFileLock(filePath string) *FileLock {
	return &FileLock{
		filePath: filePath,
	}
}

// Lock acquires the file lock
func (fl *FileLock) Lock() error {
	fl.mutex.Lock()
	defer fl.mutex.Unlock()

	lockPath := fl.filePath + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	fl.lockFile = file
	return nil
}

// Unlock releases the file lock
func (fl *FileLock) Unlock() error {
	fl.mutex.Lock()
	defer fl.mutex.Unlock()

	if fl.lockFile == nil {
		return nil
	}

	err := fl.lockFile.Close()
	fl.lockFile = nil

	lockPath := fl.filePath + ".lock"
	os.Remove(lockPath) // Best effort cleanup

	return err
}

// WithFileLock executes a function with file lock protection
func WithFileLock(filePath string, fn func() error) error {
	lock := NewFileLock(filePath)
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	return fn()
}

// ValidateInput validates and sanitizes input text
func ValidateInput(text string, maxLength int, fieldName string) (string, error) {
	if len(text) > maxLength {
		return "", fmt.Errorf("%w: %s too long: %d chars (max %d)",
			ErrInputValidation, fieldName, len(text), maxLength)
	}

	// Remove control characters except newlines and tabs
	sanitized := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1 // Remove control characters
		}
		return r
	}, text)

	return sanitized, nil
}

// NormalizeLineEndings converts line endings to Unix style
func NormalizeLineEndings(text string) string {
	// Convert Windows and old Mac line endings to Unix
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

// SafeWriteText writes text to a file safely with proper error handling
func SafeWriteText(filePath string, content string) error {
	// Normalize line endings
	content = NormalizeLineEndings(content)

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temporary file first
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, filePath); err != nil {
		os.Remove(tempFile) // Clean up on failure
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// SafeReadText reads text from a file safely with proper error handling
func SafeReadText(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Return empty string for non-existent files
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(data), nil
}

// FindAnswer searches for an answer to a specific question ID in markdown content
func FindAnswer(content, questionID string) (string, bool) {
	// Use regex for robust parsing - look for the question section and extract the answer
	pattern := fmt.Sprintf(`(?i)### %s\s*\n.*?\*\*Answer:\*\*\s*(.+?)(?=\n\n---|### |$)`,
		regexp.QuoteMeta(questionID))

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(content)

	if len(matches) > 1 {
		answer := strings.TrimSpace(matches[1])
		// Check if it's still pending (case-insensitive)
		if strings.ToLower(answer) == "pending" {
			return "", false
		}
		return answer, true
	}

	return "", false
}

// AppendQuestion adds a new question to the markdown file
func AppendQuestion(filePath, questionID, question, context, timestamp string) error {
	return WithFileLock(filePath, func() error {
		// Read existing content
		existingContent, err := SafeReadText(filePath)
		if err != nil {
			return err
		}

		// Append new question
		newContent := existingContent + fmt.Sprintf(
			"\n---\n\n### %s\n\n"+
				"**Timestamp:** %s  \n"+
				"**Question:** %s  \n"+
				"**Context:** %s  \n"+
				"**Answer:** PENDING\n\n",
			questionID, timestamp, question, context)

		return SafeWriteText(filePath, newContent)
	})
}
