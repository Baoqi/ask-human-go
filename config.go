package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Config holds configuration for the Ask-Human MCP server
type Config struct {
	AskFile             string        // Path to the markdown file
	Timeout             time.Duration // Question timeout
	MaxQuestionLength   int           // Maximum question length in bytes
	MaxContextLength    int           // Maximum context length in bytes
	MaxPendingQuestions int           // Maximum concurrent pending questions
	MaxFileSize         int64         // Maximum file size in bytes
	CleanupInterval     time.Duration // Cleanup interval for timeouts
	RotationSize        int64         // File size at which to rotate
	Host                string        // HTTP server host
	Port                int           // HTTP server port
	HTTPMode            bool          // Whether to run in HTTP mode
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		AskFile:             GetDefaultAskFile(),
		Timeout:             30 * time.Minute,
		MaxQuestionLength:   10240, // 10KB
		MaxContextLength:    51200, // 50KB
		MaxPendingQuestions: 100,
		MaxFileSize:         104857600, // 100MB
		CleanupInterval:     5 * time.Minute,
		RotationSize:        52428800, // 50MB
		Host:                "localhost",
		Port:                3000,
		HTTPMode:            false,
	}
}

// GetDefaultAskFile returns platform-appropriate default location for the ask file
func GetDefaultAskFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "ask_human.md"
	}

	var result string
	if runtime.GOOS == "windows" {
		// Use user's Documents folder on Windows
		documentsDir := filepath.Join(homeDir, "Documents")
		if _, err := os.Stat(documentsDir); err == nil {
			result = filepath.Join(documentsDir, "ask_human.md")
		} else {
			result = filepath.Join(homeDir, "ask_human.md")
		}
	} else {
		// Use home directory on Unix-like systems
		result = filepath.Join(homeDir, "ask_human.md")
	}

	return result
}

// Custom error types
var (
	ErrQuestionTimeout  = errors.New("question timeout")
	ErrFileAccess       = errors.New("file access error")
	ErrInputValidation  = errors.New("input validation error")
	ErrTooManyQuestions = errors.New("too many pending questions")
	ErrFileTooLarge     = errors.New("file too large")
	ErrServerShutdown   = errors.New("server is shutting down")
)

// QuestionStatus represents the status of a question
type QuestionStatus string

const (
	StatusPending  QuestionStatus = "PENDING"
	StatusAnswered QuestionStatus = "ANSWERED"
	StatusTimeout  QuestionStatus = "TIMEOUT"
	StatusError    QuestionStatus = "ERROR"
)
