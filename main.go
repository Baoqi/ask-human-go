package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
		host           = flag.String("host", "localhost", "HTTP server host")
		port           = flag.Int("port", 3000, "HTTP server port")
		timeoutFlag    = flag.Int("timeout", 1800, "Question timeout in seconds")
		maxPending     = flag.Int("max-pending", 100, "Maximum pending questions")
		maxQuestionLen = flag.Int("max-question-length", 10240, "Maximum question length")
		maxContextLen  = flag.Int("max-context-length", 51200, "Maximum context length")
		verbose        = flag.Bool("verbose", false, "Enable verbose logging (not recommended for stdio mode)")
	)

	flag.Parse()

	if *helpFlag {
		showHelp()
		return
	}

	// In stdio mode, disable logging to stderr to avoid interfering with MCP protocol
	// unless verbose mode is explicitly enabled
	if !*httpMode && !*verbose {
		log.SetOutput(io.Discard)
	}

	// Create configuration
	config := DefaultConfig()
	config.HTTPMode = *httpMode
	config.Host = *host
	config.Port = *port
	config.Timeout = time.Duration(*timeoutFlag) * time.Second
	config.MaxPendingQuestions = *maxPending
	config.MaxQuestionLength = *maxQuestionLen
	config.MaxContextLength = *maxContextLen

	// Create server
	askServer, err := NewAskHumanServer(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}
	defer askServer.Close()

	if config.HTTPMode {
		// HTTP mode: handle signals ourselves
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			log.Println("Shutting down HTTP server...")
			askServer.Close()
		}()

		log.Printf("Ask-Human MCP Server starting in HTTP mode on %s:%d", config.Host, config.Port)
		if err := runHTTPMode(askServer, config.Host, config.Port); err != nil {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Stdio mode: let mcp-go handle signals internally
		// ServeStdio already handles SIGTERM and SIGINT
		err := runStdioMode(askServer)
		if err != nil {
			// Don't print error for normal termination cases
			errStr := err.Error()
			if errStr != "EOF" && errStr != "context canceled" {
				fmt.Fprintf(os.Stderr, "Stdio server error: %v\n", err)
			}
		}
	}
}

// runStdioMode runs the server in stdio mode for MCP clients
func runStdioMode(askServer *AskHumanServer) error {
	mcpServer := askServer.GetMCPServer()
	return server.ServeStdio(mcpServer)
}

// runHTTPMode runs the server in HTTP/SSE mode
func runHTTPMode(askServer *AskHumanServer, host string, port int) error {
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
	errChan := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	log.Printf("Server listening on http://%s:%d", host, port)
	log.Printf("SSE endpoint: http://%s:%d/sse", host, port)
	log.Printf("Health check: http://%s:%d/health", host, port)

	// Wait for shutdown signal or error
	select {
	case <-askServer.shutdownCtx.Done():
		// Graceful shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := sseServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("SSE server shutdown error: %v", err)
		}
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// showHelp displays usage information
func showHelp() {
	fmt.Printf(`Ask-Human MCP Server

A Model Context Protocol (MCP) server that allows AI agents to ask questions
to humans through zenity GUI dialogs.

USAGE:
    ask-human-go [OPTIONS]

OPTIONS:
    --help                      Show this help message
    --http                      Run in HTTP mode instead of stdio
    --host <HOST>               HTTP server host (default: localhost)
    --port <PORT>               HTTP server port (default: 3000)
    --timeout <SECONDS>         Question timeout in seconds (default: 1800)
    --max-pending <NUM>         Maximum pending questions (default: 100)
    --max-question-length <NUM> Maximum question length (default: 10240)
    --max-context-length <NUM>  Maximum context length (default: 51200)
    --verbose                   Enable verbose logging (not recommended for stdio mode)

EXAMPLES:
    # Run in stdio mode (for MCP clients like Cursor)
    ask-human-go

    # Run in HTTP mode
    ask-human-go --http --port 3000

    # Run with custom timeout
    ask-human-go --timeout 900

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
1. AI agent calls ask_human(question, context)
2. A GUI dialog box appears asking the question
3. Human types the answer directly into the dialog
4. AI agent receives the answer immediately

PROJECT: https://github.com/masonyarbrough/ask-human-go
`)
}
