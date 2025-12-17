package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ncruces/zenity"
)

// ZenityHandler handles GUI interactions using zenity
type ZenityHandler struct {
	timeout time.Duration
}

// NewZenityHandler creates a new zenity handler
func NewZenityHandler(timeout time.Duration) *ZenityHandler {
	return &ZenityHandler{
		timeout: timeout,
	}
}

// AskQuestion shows a zenity dialog to ask the user a question and returns the answer
func (z *ZenityHandler) AskQuestion(parentCtx context.Context, questionID, question, contextInfo string) (string, error) {
	// Build the dialog text
	var dialogText strings.Builder

	// Show workspace folder paths if available to help identify the source project
	if workspacePaths := os.Getenv("WORKSPACE_FOLDER_PATHS"); workspacePaths != "" {
		dialogText.WriteString(fmt.Sprintf("Project: %s\n", workspacePaths))
	}

	dialogText.WriteString(fmt.Sprintf("%s\n", question))
	if contextInfo != "" {
		dialogText.WriteString(fmt.Sprintf("Context: %s\n", contextInfo))
	}
	dialogText.WriteString("Please provide your answer:")

	// Create a context with timeout, derived from parent context
	ctx, cancel := context.WithTimeout(parentCtx, z.timeout)
	defer cancel()

	// Reduce multiple newlines to one newline
	content := strings.ReplaceAll(dialogText.String(), "\n\n", "\n")

	// Show the text entry dialog
	answer, err := zenity.Entry(content,
		zenity.Title("Ask-Human MCP Server - Question ID: "+questionID),
		zenity.Width(600),
		zenity.Height(400),
		zenity.Context(ctx),
	)

	if err != nil {
		if err == zenity.ErrCanceled {
			return "", fmt.Errorf("user canceled the question dialog")
		}
		if err == context.DeadlineExceeded || err == context.Canceled {
			return "", fmt.Errorf("dialog timeout or canceled")
		}
		return "", fmt.Errorf("zenity dialog error: %v", err)
	}

	// Validate answer
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("empty answer provided")
	}

	return answer, nil
}
