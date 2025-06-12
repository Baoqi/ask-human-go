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
	// The regex pattern matches the question header, then non-greedily matches any content,
	// then the "**Answer:**" prefix, and then captures the answer non-greedily until
	// it hits either a double newline followed by "---", or "### " (for the next question), or the end of the string.
	// Since Go's regexp package (RE2) does not support lookaheads directly,
	// we use a non-capturing group `(?:...)` to match the delimiters at the end of the answer,
	// ensuring the captured answer does not include them.

	// `(?is)`:
	//   `i` makes the match case-insensitive.
	//   `s` makes `.` match newlines (dotall mode).
	// `### %s\s*\n`: Matches the question ID header. %s is replaced by the quoted questionID.
	// `.*?`: Non-greedy match for any characters (including newlines due to `s` flag).
	// `\*\*Answer:\*\*\s*`: Matches "**Answer:**" followed by optional whitespace.
	// `(.*?)`: This is the first capturing group (`matches[1]`), which captures the answer itself.
	//          It's non-greedy, so it stops at the first occurrence of the following patterns.
	// `(?:\n{2,}---|### |$)`: This is a non-capturing group for the delimiters that mark the end of the answer.
	//          `\n{2,}` matches two or more newlines.
	//          `---|### `: Matches "---" or "### ".
	//          `|`: OR operator.
	//          `$`: Matches the end of the string (if the answer is at the end of the file).
	pattern := fmt.Sprintf(`(?is)### %s\s*\n.*?\*\*Answer:\*\*\s*(.*?)(?:\n{2,}---|### |$)`,
		regexp.QuoteMeta(questionID))

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(content)

	if len(matches) > 1 {
		answer := strings.TrimSpace(matches[1]) // matches[1] contains the captured answer
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
