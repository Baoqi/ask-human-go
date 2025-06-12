package main

import (
	"context"
	"log"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher monitors file changes and notifies waiting callbacks
type FileWatcher struct {
	filePath  string
	watcher   *fsnotify.Watcher
	callbacks map[string]chan struct{}
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewFileWatcher creates a new file watcher for the specified file
func NewFileWatcher(filePath string) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		filePath:  filePath,
		watcher:   watcher,
		callbacks: make(map[string]chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Watch the parent directory since the file might not exist yet
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		cancel()
		return nil, err
	}

	go fw.watchLoop()

	return fw, nil
}

// RegisterCallback registers a callback for a specific question ID
func (fw *FileWatcher) RegisterCallback(questionID string) <-chan struct{} {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	ch := make(chan struct{}, 1)
	fw.callbacks[questionID] = ch
	return ch
}

// UnregisterCallback removes a callback for a question ID
func (fw *FileWatcher) UnregisterCallback(questionID string) {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	if ch, exists := fw.callbacks[questionID]; exists {
		close(ch)
		delete(fw.callbacks, questionID)
	}
}

// NotifyAll notifies all registered callbacks
func (fw *FileWatcher) NotifyAll() {
	fw.mutex.RLock()
	defer fw.mutex.RUnlock()

	for _, ch := range fw.callbacks {
		select {
		case ch <- struct{}{}:
		default:
			// Channel is full, skip notification
		}
	}
}

// watchLoop runs the file watching loop
func (fw *FileWatcher) watchLoop() {
	defer fw.watcher.Close()

	for {
		select {
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Check if the event is for our target file
			if event.Name == fw.filePath {
				if event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Create == fsnotify.Create {
					fw.NotifyAll()
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

// Close stops the file watcher
func (fw *FileWatcher) Close() error {
	fw.cancel()
	return fw.watcher.Close()
}
