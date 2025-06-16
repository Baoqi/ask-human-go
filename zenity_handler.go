package main

import (
	"context"
	"fmt"
	"log"
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
func (z *ZenityHandler) AskQuestion(questionID, question, contextInfo string) (string, error) {
	// Build the dialog text
	var dialogText strings.Builder

	dialogText.WriteString(fmt.Sprintf("Question ID: %s\n\n", questionID))
	dialogText.WriteString(fmt.Sprintf("Question: %s\n\n", question))

	if contextInfo != "" {
		dialogText.WriteString(fmt.Sprintf("Context: %s\n\n", contextInfo))
	}

	dialogText.WriteString("Please provide your answer:")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), z.timeout)
	defer cancel()

	// Log the question
	log.Printf("🚨 Showing zenity dialog for question: %s", questionID)
	log.Printf("   Question: %s", truncateString(question, 100))
	if contextInfo != "" {
		log.Printf("   Context: %s", truncateString(contextInfo, 100))
	}

	// Show the text entry dialog
	answer, err := zenity.Entry(dialogText.String(),
		zenity.Title("Ask-Human MCP Server - Question"),
		zenity.Width(600),
		zenity.Height(400),
		zenity.Context(ctx),
	)

	if err != nil {
		if err == zenity.ErrCanceled {
			log.Printf("❌ User canceled dialog for question: %s", questionID)
			return "", fmt.Errorf("user canceled the question dialog")
		}
		if err == context.DeadlineExceeded {
			log.Printf("⏰ Dialog timeout for question: %s", questionID)
			return "", fmt.Errorf("dialog timeout after %v", z.timeout)
		}
		log.Printf("❌ Zenity error for question %s: %v", questionID, err)
		return "", fmt.Errorf("zenity dialog error: %v", err)
	}

	// Validate answer
	answer = strings.TrimSpace(answer)
	if answer == "" {
		log.Printf("❌ Empty answer provided for question: %s", questionID)
		return "", fmt.Errorf("empty answer provided")
	}

	log.Printf("✅ Got answer for question %s: %s", questionID, truncateString(answer, 100))
	return answer, nil
}

// ShowNotification shows a notification using zenity
func (z *ZenityHandler) ShowNotification(title, message string) error {
	return zenity.Notify(message,
		zenity.Title(title),
		zenity.InfoIcon,
	)
}

// ShowInfo shows an info dialog
func (z *ZenityHandler) ShowInfo(title, message string) error {
	return zenity.Info(message,
		zenity.Title(title),
	)
}

// ShowError shows an error dialog
func (z *ZenityHandler) ShowError(title, message string) error {
	return zenity.Error(message,
		zenity.Title(title),
	)
}

// ShowQuestion shows a yes/no question dialog
func (z *ZenityHandler) ShowQuestion(title, message string) (bool, error) {
	err := zenity.Question(message,
		zenity.Title(title),
		zenity.QuestionIcon,
	)

	if err == zenity.ErrCanceled {
		return false, nil
	}

	return err == nil, err
}
