package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Parse command line flags
	var (
		helpFlag       = flag.Bool("help", false, "Show help message")
		httpMode       = flag.Bool("http", false, "Run in HTTP mode instead of stdio")
		zenityMode     = flag.Bool("zenity", true, "Use zenity GUI dialogs instead of file-based interaction")
		noZenityMode   = flag.Bool("no-zenity", false, "Disable zenity GUI dialogs and use file-based interaction")
		host           = flag.String("host", "localhost", "HTTP server host")
		port           = flag.Int("port", 3000, "HTTP server port")
		timeoutFlag    = flag.Int("timeout", 1800, "Question timeout in seconds")
		askFile        = flag.String("file", "", "Path to ask file (default: platform-specific)")
		maxPending     = flag.Int("max-pending", 100, "Maximum pending questions")
		maxQuestionLen = flag.Int("max-question-length", 10240, "Maximum question length")
		maxContextLen  = flag.Int("max-context-length", 51200, "Maximum context length")
		maxFileSize    = flag.Int64("max-file-size", 104857600, "Maximum file size")
		rotationSize   = flag.Int64("rotation-size", 52428800, "File rotation size")
	)

	flag.Parse()

	if *helpFlag {
		showHelp()
		return
	}

	// Create configuration
	config := DefaultConfig()
	config.HTTPMode = *httpMode
	config.ZenityMode = *zenityMode && !*noZenityMode // Disable zenity if no-zenity flag is set
	config.Host = *host
	config.Port = *port
	config.Timeout = time.Duration(*timeoutFlag) * time.Second
	config.MaxPendingQuestions = *maxPending
	config.MaxQuestionLength = *maxQuestionLen
	config.MaxContextLength = *maxContextLen
	config.MaxFileSize = *maxFileSize
	config.RotationSize = *rotationSize

	if *askFile != "" {
		config.AskFile = *askFile
	}

	// Create server
	askServer, err := NewAskHumanServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer askServer.Close()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down server...")
		cancel()
	}()

	log.Printf("Ask-Human MCP Server starting...")
	if config.ZenityMode {
		log.Printf("Mode: Zenity GUI dialogs")
	} else {
		log.Printf("Mode: File-based interaction")
		log.Printf("Ask file: %s", config.AskFile)
	}
	log.Printf("Timeout: %v", config.Timeout)
	log.Printf("Max pending questions: %d", config.MaxPendingQuestions)

	if config.HTTPMode {
		if err := runHTTPMode(ctx, askServer, config.Host, config.Port); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	} else {
		if err := runStdioMode(ctx, askServer); err != nil {
			log.Fatalf("Stdio server error: %v", err)
		}
	}

	log.Println("Server stopped")
}

// runStdioMode runs the server in stdio mode for MCP clients
func runStdioMode(ctx context.Context, askServer *AskHumanServer) error {
	log.Println("Running in stdio mode...")

	mcpServer := askServer.GetMCPServer()

	// Run the stdio transport
	return server.ServeStdio(mcpServer)
}

// runHTTPMode runs the server in HTTP/SSE mode
func runHTTPMode(ctx context.Context, askServer *AskHumanServer, host string, port int) error {
	log.Printf("Running in HTTP mode on %s:%d", host, port)

	mcpServer := askServer.GetMCPServer()

	// Create SSE server
	sseServer := server.NewSSEServer(mcpServer)

	// Create HTTP server with SSE endpoint
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","service":"ask-human-mcp"}`)
	})

	// SSE endpoint for MCP communication
	mux.Handle("/sse", sseServer.SSEHandler())

	// Message endpoint for MCP communication
	mux.Handle("/message", sseServer.MessageHandler())

	httpServer := &http.Server{
		Addr:    host + ":" + strconv.Itoa(port),
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Printf("Server listening on http://%s:%d", host, port)
	log.Printf("SSE endpoint: http://%s:%d/sse", host, port)
	log.Printf("Message endpoint: http://%s:%d/message", host, port)
	log.Printf("Health check: http://%s:%d/health", host, port)

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown SSE server first
	if err := sseServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("SSE server shutdown error: %v", err)
	}

	return httpServer.Shutdown(shutdownCtx)
}

// showHelp displays usage information
func showHelp() {
	fmt.Printf(`Ask-Human MCP Server

A Model Context Protocol (MCP) server that allows AI agents to ask questions
to humans through a markdown file interface.

USAGE:
    ask-human-go [OPTIONS]

OPTIONS:
    --help                      Show this help message
    --http                      Run in HTTP mode instead of stdio
    --zenity                    Use zenity GUI dialogs (default: true)
    --no-zenity                 Disable zenity GUI and use file-based interaction
    --host <HOST>               HTTP server host (default: localhost)
    --port <PORT>               HTTP server port (default: 3000)
    --timeout <SECONDS>         Question timeout in seconds (default: 1800)
    --file <PATH>               Path to ask file (default: platform-specific, ignored in zenity mode)
    --max-pending <NUM>         Maximum pending questions (default: 100)
    --max-question-length <NUM> Maximum question length (default: 10240)
    --max-context-length <NUM>  Maximum context length (default: 51200)
    --max-file-size <NUM>       Maximum file size (default: 104857600, ignored in zenity mode)
    --rotation-size <NUM>       File rotation size (default: 52428800, ignored in zenity mode)

EXAMPLES:
    # Run in stdio mode with file-based interaction (for local MCP clients like Cursor)
    ask-human-go

    # Run with zenity GUI dialogs (no file needed)
    ask-human-go --zenity

    # Run in HTTP mode
    ask-human-go --http --port 3000

    # Run with zenity GUI and custom timeout
    ask-human-go --zenity --timeout 900

    # Traditional file mode with custom timeout and file location
    ask-human-go --timeout 900 --file /path/to/questions.md

MCP CLIENT CONFIGURATION:

For Cursor (.cursor/mcp.json):
    {
      "mcpServers": {
        "ask-human": {
          "command": "ask-human-go"
        }
      }
    }

For HTTP mode:
    {
      "mcpServers": {
        "ask-human": {
          "url": "http://localhost:3000/sse"
        }
      }
    }

WORKFLOW:

File mode:
1. AI agent calls ask_human(question, context)
2. Question appears in the markdown file with "Answer: PENDING"
3. Human edits the file and replaces "PENDING" with the answer
4. AI agent receives the answer and continues

Zenity mode (--zenity):
1. AI agent calls ask_human(question, context)
2. A GUI dialog box appears asking the question
3. Human types the answer directly into the dialog
4. AI agent receives the answer immediately

PROJECT: https://github.com/masonyarbrough/ask-human-go
`)
}
