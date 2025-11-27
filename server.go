package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AskHumanServer handles AI questions through zenity GUI dialogs
type AskHumanServer struct {
	config           *Config
	mcpServer        *server.MCPServer
	zenityHandler    *ZenityHandler
	pendingQuestions map[string]time.Time
	mutex            sync.RWMutex
	shutdownCtx      context.Context // Exported for HTTP mode shutdown coordination
	shutdownCancel   context.CancelFunc
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

	askServer := &AskHumanServer{
		config:           config,
		mcpServer:        mcpServer,
		zenityHandler:    NewZenityHandler(config.Timeout),
		pendingQuestions: make(map[string]time.Time),
		shutdownCtx:      ctx,
		shutdownCancel:   cancel,
	}

	// Register MCP tools
	askServer.registerTools()

	// Start background cleanup goroutine
	go askServer.cleanupLoop()

	return askServer, nil
}

// registerTools registers the MCP tools that AI can call
func (s *AskHumanServer) registerTools() {
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

	contextInfo := req.GetString("context", "")

	answer, err := s.askQuestion(ctx, question, contextInfo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to ask question: %v", err)), nil
	}

	return mcp.NewToolResultText(answer), nil
}

// askQuestion handles the core question asking logic
func (s *AskHumanServer) askQuestion(ctx context.Context, question, contextInfo string) (string, error) {
	// Validate inputs
	if len(question) > s.config.MaxQuestionLength {
		return "", fmt.Errorf("question too long: %d chars (max %d)", len(question), s.config.MaxQuestionLength)
	}
	if len(contextInfo) > s.config.MaxContextLength {
		return "", fmt.Errorf("context too long: %d chars (max %d)", len(contextInfo), s.config.MaxContextLength)
	}

	// Check resource limits
	s.mutex.RLock()
	pendingCount := len(s.pendingQuestions)
	s.mutex.RUnlock()

	if pendingCount >= s.config.MaxPendingQuestions {
		return "", fmt.Errorf("too many pending questions: %d (max %d)", pendingCount, s.config.MaxPendingQuestions)
	}

	// Generate question ID
	questionID := fmt.Sprintf("Q%s", uuid.New().String()[:8])

	// Track this question
	s.mutex.Lock()
	s.pendingQuestions[questionID] = time.Now()
	s.mutex.Unlock()

	// Use zenity to get the answer
	answer, err := s.zenityHandler.AskQuestion(ctx, questionID, question, contextInfo)

	// Remove from pending
	s.mutex.Lock()
	delete(s.pendingQuestions, questionID)
	s.mutex.Unlock()

	return answer, err
}

// cleanupLoop runs periodic cleanup of timed out questions
func (s *AskHumanServer) cleanupLoop() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCtx.Done():
			return
		case <-ticker.C:
			s.cleanupTimeouts()
		}
	}
}

// cleanupTimeouts removes questions that have timed out
func (s *AskHumanServer) cleanupTimeouts() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	for questionID, startTime := range s.pendingQuestions {
		if now.Sub(startTime) > s.config.Timeout {
			delete(s.pendingQuestions, questionID)
		}
	}
}

// GetMCPServer returns the underlying MCP server
func (s *AskHumanServer) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}

// Close shuts down the server and cleans up resources
func (s *AskHumanServer) Close() error {
	s.shutdownCancel()
	return nil
}
