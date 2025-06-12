package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

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

// listPendingQuestions returns a formatted list of pending questions
func (s *AskHumanServer) listPendingQuestions() (string, error) {
	content, err := SafeReadText(s.config.AskFile)
	if err != nil {
		return "", err
	}

	s.mutex.RLock()
	pendingQuestions := make(map[string]time.Time)
	for id, timestamp := range s.pendingQuestions {
		pendingQuestions[id] = timestamp
	}
	s.mutex.RUnlock()

	if len(pendingQuestions) == 0 {
		return "No pending questions", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Pending Questions (%d):\n\n", len(pendingQuestions)))

	for questionID, timestamp := range pendingQuestions {
		duration := time.Since(timestamp)
		questionText := s.getQuestionText(content, questionID)

		result.WriteString(fmt.Sprintf("**%s** (waiting %v)\n", questionID, duration.Round(time.Second)))
		if questionText != "" {
			truncated := truncateString(questionText, 200)
			result.WriteString(fmt.Sprintf("  Question: %s\n", truncated))
		}
		result.WriteString("\n")
	}

	result.WriteString(fmt.Sprintf("Edit %s to provide answers.", s.config.AskFile))
	return result.String(), nil
}

// getQuestionText extracts the question text for a given question ID
func (s *AskHumanServer) getQuestionText(content, questionID string) string {
	lines := strings.Split(content, "\n")
	inQuestion := false

	for _, line := range lines {
		if strings.Contains(line, "### "+questionID) {
			inQuestion = true
			continue
		}

		if inQuestion {
			if strings.HasPrefix(line, "**Question:**") {
				return strings.TrimSpace(strings.TrimPrefix(line, "**Question:**"))
			}
			if strings.HasPrefix(line, "###") || strings.HasPrefix(line, "---") {
				break
			}
		}
	}

	return ""
}

// getStats returns formatted statistics about the Q&A session
func (s *AskHumanServer) getStats() (string, error) {
	s.mutex.RLock()
	totalQuestions := s.totalQuestions
	totalAnswered := s.totalAnswered
	pendingCount := len(s.pendingQuestions)
	s.mutex.RUnlock()

	// Get file size
	var fileSize int64
	if info, err := os.Stat(s.config.AskFile); err == nil {
		fileSize = info.Size()
	}

	var result strings.Builder
	result.WriteString("📊 Ask-Human Q&A Statistics\n\n")
	result.WriteString(fmt.Sprintf("**Total Questions Asked:** %d\n", totalQuestions))
	result.WriteString(fmt.Sprintf("**Questions Answered:** %d\n", totalAnswered))
	result.WriteString(fmt.Sprintf("**Currently Pending:** %d\n", pendingCount))

	if totalQuestions > 0 {
		answerRate := float64(totalAnswered) / float64(totalQuestions) * 100
		result.WriteString(fmt.Sprintf("**Answer Rate:** %.1f%%\n", answerRate))
	}

	result.WriteString(fmt.Sprintf("**Ask File:** %s\n", s.config.AskFile))
	result.WriteString(fmt.Sprintf("**File Size:** %.2f KB\n", float64(fileSize)/1024))
	result.WriteString(fmt.Sprintf("**Max Pending:** %d\n", s.config.MaxPendingQuestions))
	result.WriteString(fmt.Sprintf("**Timeout:** %v\n", s.config.Timeout))

	if pendingCount > 0 {
		result.WriteString(fmt.Sprintf("\n💡 You have %d questions waiting for answers!\n", pendingCount))
		result.WriteString(fmt.Sprintf("Edit %s to provide responses.", s.config.AskFile))
	}

	return result.String(), nil
}
