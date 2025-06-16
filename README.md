# ask-human-go

A Go implementation of an MCP (Model Context Protocol) server that prevents AI hallucinations by enabling AI agents to ask humans questions through either a markdown file interface or GUI dialogs.

## Overview

`ask-human-go` allows AI agents to request human input when they are uncertain or need clarification. The server supports two interaction modes:

1. **File Mode (Default)**: Questions appear in a `ask_human.md` file with "Answer: PENDING" status. Humans edit the file to provide answers.
2. **Zenity Mode (New)**: Questions appear as GUI dialog boxes. Humans type answers directly into the dialogs.

## Features

- **Dual Interaction Modes**: Choose between file-based or GUI-based interaction
- **Zenity GUI Support**: Modern cross-platform dialogs using zenity
- **File-based Communication**: Traditional markdown file interface for automation
- **File Watching**: Automatically detects when humans provide answers (file mode)
- **Timeout Handling**: Configurable timeouts to prevent indefinite waiting
- **Concurrent Questions**: Supports multiple pending questions simultaneously
- **Cross-platform**: Works on Windows, macOS, and Linux
- **Security Limits**: Configurable limits on question length and frequency
- **Statistics**: Track question/answer metrics

## Installation

### Prerequisites
- Go 1.24 or later
- For GUI mode: zenity-compatible dialogs (automatically handled by the ncruces/zenity library)

### Build from Source
```bash
git clone https://github.com/yourusername/ask-human-go.git
cd ask-human-go
go mod download
go build -o ask-human-go .
```

## Usage

### Zenity GUI Mode (Default)
```bash
# Run with GUI dialog boxes (default mode)
./ask-human-go
```

### File Mode
```bash
# Run with traditional file-based interaction
./ask-human-go --no-zenity
```

### HTTP Server Mode
```bash
# File mode over HTTP
./ask-human-go --http --port 3000

# Zenity mode over HTTP
./ask-human-go --zenity --http --port 3000
```

### Configuration Options
```bash
./ask-human-go --help
```

Available flags:
- `--zenity`: Use zenity GUI dialogs (default: true)
- `--no-zenity`: Disable zenity GUI and use file-based interaction
- `--http`: Run in HTTP mode instead of stdio
- `--host <HOST>`: HTTP server host (default: localhost)
- `--port <PORT>`: HTTP server port (default: 3000)
- `--file <PATH>`: Path to the markdown file (ignored in zenity mode)
- `--timeout <SECONDS>`: Question timeout in seconds (default: 1800)
- `--max-pending <NUM>`: Maximum pending questions (default: 100)
- `--max-question-length <NUM>`: Maximum question length (default: 10240)
- `--max-context-length <NUM>`: Maximum context length (default: 51200)

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

### File Mode
1. AI agent calls `ask_human` tool with a question
2. Question is written to `ask_human.md` with "Answer: PENDING" status
3. Human opens the file and replaces "PENDING" with their answer
4. File watcher detects the change and returns the answer to the AI
5. AI continues with the human-provided information

### Zenity Mode  
1. AI agent calls `ask_human` tool with a question
2. A GUI dialog box appears with the question
3. Human types the answer directly into the dialog
4. AI receives the answer immediately and continues

## File Format (File Mode Only)

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

Note: In zenity mode, no file is created as interactions happen through GUI dialogs.

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
- GUI dialogs powered by `ncruces/zenity` for cross-platform native dialogs 