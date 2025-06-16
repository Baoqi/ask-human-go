package main

import (
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
