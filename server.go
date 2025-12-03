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

// Default interval for sending progress notifications to keep the connection alive
const progressNotificationInterval = 30 * time.Second

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

	// Get progress token from request metadata (if provided by client)
	var progressToken mcp.ProgressToken
	if req.Params.Meta != nil && req.Params.Meta.ProgressToken != nil {
		progressToken = req.Params.Meta.ProgressToken
	}

	answer, err := s.askQuestion(ctx, question, contextInfo, progressToken)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to ask question: %v", err)), nil
	}

	return mcp.NewToolResultText(answer), nil
}

// askQuestion handles the core question asking logic
func (s *AskHumanServer) askQuestion(ctx context.Context, question, contextInfo string, progressToken mcp.ProgressToken) (string, error) {
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

	// Create a channel to receive the answer from zenity
	type zenityResult struct {
		answer string
		err    error
	}
	resultChan := make(chan zenityResult, 1)

	// Start zenity dialog in a goroutine
	go func() {
		answer, err := s.zenityHandler.AskQuestion(ctx, questionID, question, contextInfo)
		resultChan <- zenityResult{answer: answer, err: err}
	}()

	// Start progress notification goroutine to keep the connection alive
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	go s.sendProgressNotifications(progressCtx, progressToken, questionID)

	// Wait for zenity result
	result := <-resultChan

	// Remove from pending
	s.mutex.Lock()
	delete(s.pendingQuestions, questionID)
	s.mutex.Unlock()

	return result.answer, result.err
}

// sendProgressNotifications sends periodic progress notifications to keep the MCP connection alive
// This prevents the client from timing out while waiting for user input
func (s *AskHumanServer) sendProgressNotifications(ctx context.Context, progressToken mcp.ProgressToken, questionID string) {
	// Get client session from context to send notifications
	session := server.ClientSessionFromContext(ctx)
	if session == nil || !session.Initialized() {
		// No session available, cannot send progress notifications
		return
	}

	// If no progress token provided, generate one for internal use
	// Note: The client may not process these if it didn't request progress,
	// but sending them keeps the connection active
	if progressToken == nil {
		progressToken = questionID
	}

	ticker := time.NewTicker(progressNotificationInterval)
	defer ticker.Stop()

	startTime := time.Now()
	notificationCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			notificationCount++

			// Calculate elapsed time for the progress message
			elapsed := time.Since(startTime)
			message := fmt.Sprintf("Waiting for human response... (%s elapsed)", elapsed.Round(time.Second))

			// Create progress notification using the helper function
			progressNotif := mcp.NewProgressNotification(progressToken, float64(notificationCount), nil, &message)

			// Create JSONRPCNotification with the progress params
			// We need to manually construct this because ProgressNotification has its own Params type
			jsonrpcNotif := mcp.JSONRPCNotification{
				JSONRPC: "2.0",
				Notification: mcp.Notification{
					Method: progressNotif.Notification.Method,
					Params: mcp.NotificationParams{
						AdditionalFields: map[string]any{
							"progressToken": progressToken,
							"progress":      float64(notificationCount),
							"message":       message,
						},
					},
				},
			}

			// Send notification through session channel (non-blocking)
			select {
			case session.NotificationChannel() <- jsonrpcNotif:
				// Notification sent successfully
			default:
				// Channel full or closed, stop sending notifications
				return
			}
		}
	}
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
