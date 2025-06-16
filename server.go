package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AskHumanServer handles AI questions through markdown file interactions or zenity GUI
type AskHumanServer struct {
	config            *Config
	mcpServer         *server.MCPServer
	watcher           *FileWatcher
	zenityHandler     *ZenityHandler
	pendingQuestions  map[string]time.Time
	answeredQuestions map[string]bool
	mutex             sync.RWMutex
	shutdownCtx       context.Context
	shutdownCancel    context.CancelFunc
	totalQuestions    int
	totalAnswered     int
}

// NewAskHumanServer creates a new Ask-Human MCP server
func NewAskHumanServer(config *Config) (*AskHumanServer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"ask-human",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	var watcher *FileWatcher
	var zenityHandler *ZenityHandler

	if config.ZenityMode {
		// Initialize zenity handler
		zenityHandler = NewZenityHandler(config.Timeout)
	} else {
		// Create file watcher for traditional mode
		var err error
		watcher, err = NewFileWatcher(config.AskFile)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create file watcher: %w", err)
		}
	}

	askServer := &AskHumanServer{
		config:            config,
		mcpServer:         mcpServer,
		watcher:           watcher,
		zenityHandler:     zenityHandler,
		pendingQuestions:  make(map[string]time.Time),
		answeredQuestions: make(map[string]bool),
		shutdownCtx:       ctx,
		shutdownCancel:    cancel,
	}

	// Initialize the ask file (only for non-zenity mode)
	if !config.ZenityMode {
		if err := askServer.initFile(); err != nil {
			askServer.Close()
			return nil, fmt.Errorf("failed to initialize ask file: %w", err)
		}
	}

	// Register MCP tools
	askServer.registerTools()

	// Start background cleanup goroutine
	go askServer.cleanupLoop()

	return askServer, nil
}

// initFile initializes the ask file if it doesn't exist
func (s *AskHumanServer) initFile() error {
	if _, err := os.Stat(s.config.AskFile); os.IsNotExist(err) {
		initialContent := fmt.Sprintf(`# Ask Human Q&A Session

This file is used by the Ask-Human MCP server to facilitate communication between AI agents and humans.

**Instructions:**
1. AI agents will add questions below with "Answer: PENDING"
2. Replace "PENDING" with your actual answer
3. The AI will automatically pick up your response

**File:** %s
**Started:** %s

---

`, s.config.AskFile, time.Now().Format("2006-01-02 15:04:05"))

		return SafeWriteText(s.config.AskFile, initialContent)
	}
	return nil
}

// registerTools registers the MCP tools that AI can call
func (s *AskHumanServer) registerTools() {
	// Ask Human tool
	askHumanTool := mcp.NewTool(
		"ask_human",
		mcp.WithDescription("Ask the human a question and wait for them to answer"),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("What you actually want to know"),
		),
		mcp.WithString("context",
			mcp.Description("Extra info that might help (like file paths, error messages, etc.)"),
		),
	)

	s.mcpServer.AddTool(askHumanTool, s.handleAskHuman)
}

// handleAskHuman handles the ask_human tool call
func (s *AskHumanServer) handleAskHuman(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	question, err := req.RequireString("question")
	if err != nil {
		return mcp.NewToolResultError("question parameter is required and must be a string"), nil
	}

	context := req.GetString("context", "")

	answer, err := s.askQuestion(question, context)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to ask question: %v", err)), nil
	}

	return mcp.NewToolResultText(answer), nil
}

// askQuestion handles the core question asking logic
func (s *AskHumanServer) askQuestion(question, context string) (string, error) {
	// Validate inputs
	question, err := ValidateInput(question, s.config.MaxQuestionLength, "Question")
	if err != nil {
		return "", err
	}

	context, err = ValidateInput(context, s.config.MaxContextLength, "Context")
	if err != nil {
		return "", err
	}

	// Check resource limits
	s.mutex.RLock()
	pendingCount := len(s.pendingQuestions)
	s.mutex.RUnlock()

	if pendingCount >= s.config.MaxPendingQuestions {
		return "", fmt.Errorf("%w: %d pending questions (max %d)",
			ErrTooManyQuestions, pendingCount, s.config.MaxPendingQuestions)
	}

	// Generate question ID and timestamp
	questionID := fmt.Sprintf("Q%s", uuid.New().String()[:8])
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Clean up any timed out questions
	s.cleanupTimeouts()

	// Use different approaches based on mode
	if s.config.ZenityMode {
		return s.askQuestionZenity(questionID, question, context)
	} else {
		return s.askQuestionFile(questionID, question, context, timestamp)
	}
}

// askQuestionZenity uses zenity GUI to ask questions
func (s *AskHumanServer) askQuestionZenity(questionID, question, context string) (string, error) {
	// Track this question
	s.mutex.Lock()
	s.pendingQuestions[questionID] = time.Now()
	s.totalQuestions++
	s.mutex.Unlock()

	// Use zenity to get the answer directly
	answer, err := s.zenityHandler.AskQuestion(questionID, question, context)

	// Remove from pending and record result
	s.removePendingQuestion(questionID)

	if err != nil {
		return "", err
	}

	// Record successful answer
	s.recordAnswer(questionID)
	return answer, nil
}

// askQuestionFile uses file-based interaction to ask questions
func (s *AskHumanServer) askQuestionFile(questionID, question, context, timestamp string) (string, error) {
	// Check file size
	if info, err := os.Stat(s.config.AskFile); err == nil {
		if info.Size() > s.config.MaxFileSize {
			return "", fmt.Errorf("%w: file size %d bytes (max %d)",
				ErrFileTooLarge, info.Size(), s.config.MaxFileSize)
		}
	}

	// Register for file change notifications
	notificationChan := s.watcher.RegisterCallback(questionID)
	defer s.watcher.UnregisterCallback(questionID)

	// Write question to file
	if err := AppendQuestion(s.config.AskFile, questionID, question, context, timestamp); err != nil {
		return "", fmt.Errorf("%w: failed to write question: %v", ErrFileAccess, err)
	}

	// Check if answer already exists (race condition protection)
	if content, err := SafeReadText(s.config.AskFile); err == nil {
		if answer, found := FindAnswer(content, questionID); found {
			log.Printf("✅ Found existing answer for %s", questionID)
			s.recordAnswer(questionID)
			return answer, nil
		}
	}

	// Track this question
	s.mutex.Lock()
	s.pendingQuestions[questionID] = time.Now()
	s.totalQuestions++
	s.mutex.Unlock()

	// Log the new question
	logSafeQuestion := truncateString(question, 100)
	logSafeContext := truncateString(context, 100)

	log.Printf("🚨 New question: %s", questionID)
	log.Printf("   Question: %s", logSafeQuestion)
	log.Printf("   Context: %s", logSafeContext)
	log.Printf("   Edit %s and replace PENDING with your answer", s.config.AskFile)

	// Wait for answer with timeout
	startTime := time.Now()
	timeout := time.After(s.config.Timeout)

	for {
		select {
		case <-s.shutdownCtx.Done():
			s.removePendingQuestion(questionID)
			return "", ErrServerShutdown

		case <-timeout:
			s.removePendingQuestion(questionID)
			log.Printf("⏰ Question %s timed out after %v", questionID, s.config.Timeout)
			return "", fmt.Errorf("%w: no answer received for question %s within %v",
				ErrQuestionTimeout, questionID, s.config.Timeout)

		case <-notificationChan:
			// File changed, check for answer
			content, err := SafeReadText(s.config.AskFile)
			if err != nil {
				continue // Try again on next notification
			}

			if answer, found := FindAnswer(content, questionID); found {
				s.removePendingQuestion(questionID)
				s.recordAnswer(questionID)
				log.Printf("✅ Got answer for %s", questionID)
				return answer, nil
			}

		case <-time.After(5 * time.Second):
			// Periodic check for answer (in case notification was missed)
			content, err := SafeReadText(s.config.AskFile)
			if err != nil {
				continue
			}

			if answer, found := FindAnswer(content, questionID); found {
				s.removePendingQuestion(questionID)
				s.recordAnswer(questionID)
				log.Printf("✅ Got answer for %s", questionID)
				return answer, nil
			}

			// Check if we've exceeded timeout
			if time.Since(startTime) > s.config.Timeout {
				s.removePendingQuestion(questionID)
				log.Printf("⏰ Question %s timed out after %v", questionID, s.config.Timeout)
				return "", fmt.Errorf("%w: no answer received for question %s within %v",
					ErrQuestionTimeout, questionID, s.config.Timeout)
			}
		}
	}
}

// removePendingQuestion removes a question from pending list
func (s *AskHumanServer) removePendingQuestion(questionID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.pendingQuestions, questionID)
}

// recordAnswer records that a question was answered
func (s *AskHumanServer) recordAnswer(questionID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.answeredQuestions[questionID] = true
	s.totalAnswered++
}

// GetMCPServer returns the underlying MCP server
func (s *AskHumanServer) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}

// Close shuts down the server and cleans up resources
func (s *AskHumanServer) Close() error {
	s.shutdownCancel()
	if s.watcher != nil {
		return s.watcher.Close()
	}
	// zenityHandler doesn't need explicit cleanup
	return nil
}

// truncateString truncates a string to maxLength and adds "..." if needed
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
