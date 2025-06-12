# ask-human-go

A Go implementation of an MCP (Model Context Protocol) server that prevents AI hallucinations by enabling AI agents to ask humans questions through a markdown file interface.

## Overview

`ask-human-go` allows AI agents to request human input when they are uncertain or need clarification. When an AI calls the `ask_human` tool, the question appears in a `ask_human.md` file with "Answer: PENDING" status. Humans can then edit the file to provide answers, and the AI continues with the correct information.

## Features

- **File-based Communication**: Uses markdown files for human-AI interaction
- **File Watching**: Automatically detects when humans provide answers
- **Timeout Handling**: Configurable timeouts to prevent indefinite waiting
- **Concurrent Questions**: Supports multiple pending questions simultaneously
- **Cross-platform**: Works on Windows, macOS, and Linux
- **Security Limits**: Configurable limits on question length and frequency
- **Statistics**: Track question/answer metrics

## Installation

### Prerequisites
- Go 1.24 or later

### Build from Source
```bash
git clone https://github.com/yourusername/ask-human-go.git
cd ask-human-go
go mod download
go build -o ask-human-go .
```

## Usage

### Basic Usage (stdio mode)
```bash
./ask-human-go
```

### HTTP Server Mode
```bash
./ask-human-go -transport http -port 8080
```

### Configuration Options
```bash
./ask-human-go -help
```

Available flags:
- `-transport`: Transport mode (stdio or http, default: stdio)
- `-port`: Port for HTTP server (default: 8080)
- `-file`: Path to the markdown file (default: ask_human.md)
- `-timeout`: Question timeout duration (default: 30m)
- `-max-length`: Maximum question length (default: 10240)
- `-max-pending`: Maximum pending questions (default: 10)

## MCP Tools

The server provides three MCP tools:

### 1. ask_human
Ask a human a question and wait for their response.

**Parameters:**
- `question` (required): The question to ask
- `context` (optional): Additional context for the question

**Example:**
```json
{
  "method": "tools/call",
  "params": {
    "name": "ask_human",
    "arguments": {
      "question": "What is the preferred coding style for this project?",
      "context": "I'm about to refactor the authentication module"
    }
  }
}
```

### 2. list_pending_questions
List all currently pending questions.

**Example:**
```json
{
  "method": "tools/call",
  "params": {
    "name": "list_pending_questions",
    "arguments": {}
  }
}
```

### 3. get_qa_stats
Get statistics about questions and answers.

**Example:**
```json
{
  "method": "tools/call",
  "params": {
    "name": "get_qa_stats",
    "arguments": {}
  }
}
```

## How It Works

1. AI agent calls `ask_human` tool with a question
2. Question is written to `ask_human.md` with "Answer: PENDING" status
3. Human opens the file and replaces "PENDING" with their answer
4. File watcher detects the change and returns the answer to the AI
5. AI continues with the human-provided information

## File Format

The `ask_human.md` file uses a simple format:

```markdown
## Question 1 (ID: abc123)
**Context:** Optional context information
**Question:** What is the preferred coding style for this project?
**Answer:** PENDING

## Question 2 (ID: def456)
**Question:** Should we use a database for this feature?
**Answer:** Yes, use PostgreSQL for better performance.
```

## Security

- Questions are limited in length to prevent abuse
- Maximum number of pending questions is configurable
- File access is safely handled with proper locking
- Input validation prevents malicious content

## Configuration

Default configuration can be customized:

```go
type Config struct {
    QuestionTimeout    time.Duration // 30 minutes
    MaxQuestionLength  int          // 10KB
    MaxPendingQuestions int         // 10
    MarkdownFile       string      // "ask_human.md"
    CleanupInterval    time.Duration // 5 minutes
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Based on the original Python implementation of `ask-human-mcp`
- Built using the `mark3labs/mcp-go` library
- Uses `fsnotify` for cross-platform file watching 