package main

import (
	"time"
)

// Config holds configuration for the Ask-Human MCP server
type Config struct {
	Timeout             time.Duration // Question timeout
	MaxQuestionLength   int           // Maximum question length in bytes
	MaxContextLength    int           // Maximum context length in bytes
	MaxPendingQuestions int           // Maximum concurrent pending questions
	CleanupInterval     time.Duration // Cleanup interval for timeouts
	Host                string        // HTTP server host
	Port                int           // HTTP server port
	HTTPMode            bool          // Whether to run in HTTP mode
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:             30 * time.Minute,
		MaxQuestionLength:   10240, // 10KB
		MaxContextLength:    51200, // 50KB
		MaxPendingQuestions: 100,
		CleanupInterval:     5 * time.Minute,
		Host:                "localhost",
		Port:                3000,
		HTTPMode:            false,
	}
}
