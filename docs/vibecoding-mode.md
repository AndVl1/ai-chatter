# VibeCoding Mode Documentation

## Overview

VibeCoding Mode is an interactive development session that allows users to upload code archives, get real-time analysis, generate tests, and iteratively improve their projects with AI assistance. It provides a comprehensive development environment with automated test fixing, environment setup, and project visualization.

## Table of Contents

1. [Architecture](#architecture)
2. [Core Components](#core-components)
3. [Session Management](#session-management)
4. [LLM Integration](#llm-integration)
5. [Test System](#test-system)
6. [Web Interface](#web-interface)
7. [Usage Guide](#usage-guide)
8. [API Reference](#api-reference)
9. [Configuration](#configuration)
10. [Troubleshooting](#troubleshooting)

## Architecture

VibeCoding mode is built with a modular architecture consisting of several key components:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Telegram Bot  │────│  VibeCoding     │────│   LLM Client    │
│                 │    │   Handler       │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                    ┌─────────┼─────────┐
                    │                   │
          ┌─────────────────┐    ┌─────────────────┐
          │ Session Manager │    │  Web Server     │
          │                 │    │                 │
          └─────────────────┘    └─────────────────┘
                    │                   │
          ┌─────────────────┐    ┌─────────────────┐
          │ Docker Adapter  │    │  File Viewer    │
          │                 │    │                 │
          └─────────────────┘    └─────────────────┘
```

### Key Features

- **Interactive Development**: Real-time code analysis and improvement suggestions
- **Automated Environment Setup**: Docker-based isolated environments with LLM-guided configuration
- **Intelligent Test Generation**: Automatic test creation with iterative fixing based on execution results
- **Web Interface**: Browser-based project visualization and file exploration
- **MCP Integration**: Model Context Protocol for direct LLM access to project files
- **Multi-Language Support**: Python, JavaScript/TypeScript, Go, Java, and more

## Core Components

### 1. VibeCodingSession (`session.go`)

The central data structure representing an active coding session.

```go
type VibeCodingSession struct {
    UserID         int64                              // Telegram user ID
    ChatID         int64                              // Telegram chat ID
    ProjectName    string                             // Project name
    StartTime      time.Time                          // Session start time
    Files          map[string]string                  // Original project files
    GeneratedFiles map[string]string                  // AI-generated files
    ContainerID    string                             // Docker container ID
    Analysis       *codevalidation.CodeAnalysisResult // Project analysis
    TestCommand    string                             // Test execution command
    Docker         *DockerAdapter                     // Docker interface
    LLMClient      llm.Client                         // LLM client
}
```

**Key Methods:**
- `SetupEnvironment(ctx)`: Configures Docker environment with up to 3 retry attempts
- `ExecuteCommand(ctx, command)`: Runs commands in the container
- `AddGeneratedFile(filename, content)`: Adds AI-generated files
- `GetAllFiles()`: Returns combined original and generated files
- `Cleanup()`: Releases resources when session ends

### 2. SessionManager (`session.go`)

Manages multiple active VibeCoding sessions across users.

```go
type SessionManager struct {
    sessions  map[int64]*VibeCodingSession // Active sessions by UserID
    mutex     sync.RWMutex                 // Thread safety
    webServer *WebServer                   // Web interface
}
```

**Key Methods:**
- `CreateSession(userID, chatID, projectName, files, llmClient)`: Creates new session
- `GetSession(userID)`: Retrieves active session
- `EndSession(userID)`: Terminates session and cleanup
- `HasActiveSession(userID)`: Checks if user has active session

### 3. VibeCodingHandler (`commands.go`)

Handles Telegram commands and user interactions.

**Supported Commands:**
- `/vibecoding_info`: Session information
- `/vibecoding_test`: Run tests with auto-fixing
- `/vibecoding_generate_tests`: Generate new tests
- `/vibecoding_auto`: Autonomous AI work
- `/vibecoding_end`: End session and export results

### 4. Docker Integration (`docker_adapter.go`)

Provides isolated execution environments for each project.

```go
type DockerAdapter struct {
    dockerManager codevalidation.DockerManager
}
```

**Features:**
- Automatic language detection and appropriate Docker image selection
- Dependency installation based on project analysis
- Secure isolated execution environment
- Container lifecycle management

## Session Management

### Session Lifecycle

1. **Creation**: User uploads archive → System extracts files → Creates Docker container
2. **Environment Setup**: LLM analyzes project → Installs dependencies → Configures environment
3. **Interactive Phase**: User asks questions, generates code, runs tests
4. **Termination**: Cleanup resources → Export results as archive

### Environment Setup Process

The environment setup process is sophisticated and includes multiple retry attempts:

```go
func (s *VibeCodingSession) SetupEnvironment(ctx context.Context) error {
    maxAttempts := 3
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        // 1. Analyze project with LLM
        if err := s.analyzeProject(ctx); err != nil { continue }
        
        // 2. Create Docker container
        containerID, err := s.Docker.CreateContainer(ctx, s.Analysis)
        if err != nil { continue }
        
        // 3. Copy files to container
        if err := s.Docker.CopyFilesToContainer(ctx, containerID, s.Files); err != nil { continue }
        
        // 4. Install dependencies
        if err := s.Docker.InstallDependencies(ctx, containerID, s.Analysis); err != nil {
            // Try to fix configuration with LLM
            fixedAnalysis, fixErr := s.analyzeAndFixError(ctx, err, s.Analysis, attempt)
            if fixErr == nil {
                s.Analysis = fixedAnalysis
            }
            continue
        }
        
        // 5. Generate test command
        s.TestCommand = s.generateTestCommand()
        
        return nil // Success
    }
    
    return fmt.Errorf("setup failed after %d attempts", maxAttempts)
}
```

## LLM Integration

### JSON Protocol (`llm_protocol.go`)

VibeCoding uses a structured JSON protocol for LLM communication:

```go
type VibeCodingRequest struct {
    Action  string                 `json:"action"`  // Request type
    Context VibeCodingContext      `json:"context"` // Session context
    Query   string                 `json:"query"`   // User query
    Options map[string]interface{} `json:"options"` // Additional options
}

type VibeCodingResponse struct {
    Status      string                 `json:"status"`      // "success", "error", "partial"
    Response    string                 `json:"response"`    // Main response
    Code        map[string]string      `json:"code"`        // Generated code files
    Suggestions []string               `json:"suggestions"` // Next step suggestions
    Error       string                 `json:"error"`       // Error message
    Metadata    map[string]interface{} `json:"metadata"`    // Additional data
}
```

### Supported Actions

- **`analyze`**: Project analysis and environment setup
- **`generate_code`**: Code generation (tests, features, fixes)
- **`answer_question`**: Interactive Q&A about the project
- **`autonomous_work`**: Multi-step autonomous development
- **`analyze_error`**: Error analysis and fixing suggestions

### MCP Integration (`mcp_client.go`)

Model Context Protocol provides direct LLM access to project files:

```go
type VibeCodingMCPClient struct {
    client  *mcp.Client
    session *mcp.ClientSession
}
```

**Available Tools:**
- `vibe_list_files`: List project files
- `vibe_read_file`: Read file contents
- `vibe_write_file`: Write/update files
- `vibe_delete_file`: Delete files
- `vibe_execute_command`: Run commands in container
- `vibe_validate_code`: Validate code syntax
- `vibe_run_tests`: Execute tests
- `vibe_get_session_info`: Get session information

## Test System

### Automatic Test Generation

The test generation system creates comprehensive tests with multiple validation layers:

```go
func (h *VibeCodingHandler) generateTests(ctx context.Context, session *VibeCodingSession) (map[string]string, error) {
    maxAttempts := 5
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        // 1. Generate tests via LLM
        tests, err := h.generateTestsOnce(ctx, session, attempt)
        if err != nil { continue }
        
        // 2. Validate generated tests
        validationResult, err := h.validateGeneratedTests(ctx, session, tests)
        if err != nil { continue }
        
        if validationResult.Success {
            return validationResult.ValidTests, nil
        }
        
        // 3. Fix issues if validation fails
        if attempt < maxAttempts {
            fixedTests, err := h.fixTestIssues(ctx, session, tests, validationResult)
            if err == nil {
                tests = fixedTests
            }
        }
    }
    
    // Fallback to basic tests
    return h.generateTestsBasic(session), lastError
}
```

### Smart Test Execution with Auto-Fixing

When running tests, the system automatically attempts to fix failures:

```go
func (h *VibeCodingHandler) handleTestCommand(ctx context.Context, chatID int64, session *VibeCodingSession) error {
    maxAttempts := 3
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        result, err := session.ExecuteCommand(ctx, session.TestCommand)
        
        if err != nil {
            // Fix execution issues (missing dependencies, etc.)
            if attempt < maxAttempts {
                h.fixTestExecutionIssues(ctx, session, err)
                continue
            }
        }
        
        if result.Success {
            break // Tests passed
        }
        
        // Fix failing tests
        if attempt < maxAttempts {
            h.fixFailingTests(ctx, session, result)
        }
    }
}
```

### Test File Detection

The system intelligently identifies test files across multiple languages:

```go
func (h *VibeCodingHandler) isTestFile(filename string) bool {
    filename = strings.ToLower(filename)
    
    // Check prefixes and suffixes
    if strings.HasPrefix(filename, "test_") || 
       strings.HasSuffix(filename, "_test.py") ||
       strings.HasSuffix(filename, "_test.go") ||
       strings.HasSuffix(filename, ".test.js") ||
       strings.HasSuffix(filename, ".spec.js") ||
       strings.Contains(filename, "test") {
        return true
    }
    
    // Check directories
    if strings.Contains(filename, "/test/") || 
       strings.Contains(filename, "/tests/") {
        return true
    }
    
    return false
}
```

## Web Interface

### Project Visualization (`webserver.go`)

The web server provides a real-time view of the project structure and files:

- **URL Pattern**: `http://localhost:8080/vibe_{userID}`
- **Auto-refresh**: Updates every 30 seconds
- **File Tree**: Interactive file browser with expand/collapse
- **File Viewer**: Syntax-highlighted code display
- **Session Stats**: Real-time session information

### Key Endpoints

- `GET /vibe_{userID}`: Main project page
- `GET /api/vibe_{userID}`: JSON session data
- `GET /api/vibe_{userID}/file/{filepath}`: File content
- `GET /static/...`: Static assets (CSS, JS)

### Features

- **Dark Theme**: IDE-style interface
- **File Type Icons**: Visual file type identification
- **Generated File Highlighting**: Distinguishes AI-generated files
- **Responsive Design**: Works on various screen sizes
- **Real-time Updates**: Reflects changes as they happen

## Usage Guide

### Starting a VibeCoding Session

1. **Prepare Archive**: Create a .zip/.tar.gz archive of your project
2. **Upload**: Send the archive to the Telegram bot without any caption
3. **Wait for Setup**: The system will automatically:
   - Extract files
   - Analyze the project with LLM
   - Set up Docker environment
   - Install dependencies
4. **Receive Confirmation**: Get session details and web interface URL

### Example Session Flow

```
User: [Uploads Python project archive]
Bot: 🔥 Сессия вайбкодинга готова!
     Проект: my-python-app
     Язык: Python
     🌐 Веб-интерфейс: http://localhost:8080/vibe_123

User: /vibecoding_generate_tests
Bot: 🧠 Генерация тестов...
     ✅ Тесты сгенерированы и сохранены в проект
     📝 Сгенерированный файл: test_main.py

User: /vibecoding_test
Bot: 🧪 Запуск тестов...
     ✅ Тесты выполнены успешно

User: "Add error handling to the main function"
Bot: [Provides code improvements and explanations]

User: /vibecoding_end
Bot: 🔥 Сессия завершена
     [Sends archive with all original and generated files]
```

### Interactive Commands

1. **Information Commands**:
   - `/vibecoding_info`: Show session details
   
2. **Test Commands**:
   - `/vibecoding_generate_tests`: Create comprehensive tests
   - `/vibecoding_test`: Run tests with auto-fixing
   
3. **Development Commands**:
   - `/vibecoding_auto`: Start autonomous AI development
   - Text messages: Ask questions, request changes
   
4. **Session Management**:
   - `/vibecoding_end`: End session and export results

### Best Practices

1. **Archive Preparation**:
   - Include all source files and configuration files
   - Keep archive size reasonable (< 50 files, < 2MB per file)
   - Exclude build artifacts and temporary files

2. **Interaction**:
   - Be specific in your requests
   - Ask one question or request one change at a time
   - Use the web interface to monitor file changes

3. **Testing**:
   - Generate tests early in the session
   - Run tests frequently to catch issues
   - Let the system auto-fix failing tests

## API Reference

### Core Classes

#### VibeCodingSession

```go
// Create and setup environment
func (s *VibeCodingSession) SetupEnvironment(ctx context.Context) error

// Execute commands in container
func (s *VibeCodingSession) ExecuteCommand(ctx context.Context, command string) (*ValidationResult, error)

// Add generated files
func (s *VibeCodingSession) AddGeneratedFile(filename, content string)

// Get all files (original + generated)
func (s *VibeCodingSession) GetAllFiles() map[string]string

// Get session information
func (s *VibeCodingSession) GetSessionInfo() map[string]interface{}

// Cleanup resources
func (s *VibeCodingSession) Cleanup() error
```

#### SessionManager

```go
// Create new session
func (sm *SessionManager) CreateSession(userID, chatID int64, projectName string, files map[string]string, llmClient llm.Client) (*VibeCodingSession, error)

// Get existing session
func (sm *SessionManager) GetSession(userID int64) (*VibeCodingSession, bool)

// End session
func (sm *SessionManager) EndSession(userID int64) error

// Check if session exists
func (sm *SessionManager) HasActiveSession(userID int64) bool
```

#### VibeCodingHandler

```go
// Handle archive upload
func (h *VibeCodingHandler) HandleArchiveUpload(ctx context.Context, userID, chatID int64, archiveData []byte, archiveName, caption string) error

// Handle commands
func (h *VibeCodingHandler) HandleVibeCodingCommand(ctx context.Context, userID, chatID int64, command string) error

// Handle text messages
func (h *VibeCodingHandler) HandleVibeCodingMessage(ctx context.Context, userID, chatID int64, messageText string) error
```

### LLM Protocol

#### Request Format

```json
{
  "action": "generate_code|analyze|answer_question|autonomous_work",
  "context": {
    "project_name": "my-project",
    "language": "Python",
    "files": {
      "main.py": "def main(): pass"
    },
    "generated_files": {
      "test_main.py": "import unittest"
    },
    "session_duration": "5m30s"
  },
  "query": "Generate unit tests for the main function",
  "options": {
    "task_type": "test_generation",
    "language": "Python"
  }
}
```

#### Response Format

```json
{
  "status": "success|error|partial",
  "response": "Generated comprehensive unit tests...",
  "code": {
    "test_main.py": "import unittest\n\nclass TestMain(unittest.TestCase):\n    def test_main(self):\n        # Test implementation\n        pass"
  },
  "suggestions": [
    "Run the generated tests to verify they work",
    "Consider adding integration tests"
  ],
  "metadata": {
    "execution_log": ["Created test file", "Added test cases"],
    "install_commands": ["pip install pytest"]
  }
}
```

## Configuration

### Environment Variables

```bash
# Docker configuration
DOCKER_HOST=unix:///var/run/docker.sock

# VibeCoding MCP server path
VIBECODING_MCP_SERVER_PATH=./vibecoding-mcp-server

# Web server port (default: 8080)
VIBECODING_WEB_PORT=8080

# LLM configuration
LLM_PROVIDER=openai
LLM_MODEL=gpt-4
LLM_API_KEY=your-api-key
```

### Docker Images

The system automatically selects appropriate Docker images based on project language:

- **Python**: `python:3.11-slim`
- **Node.js**: `node:18-alpine`
- **Go**: `golang:1.22`
- **Java**: `openjdk:17-slim`
- **Generic**: `ubuntu:22.04`

### File Limits

```go
const (
    MaxArchiveFiles = 50          // Maximum files per archive
    MaxFileSize     = 2 * 1024 * 1024  // 2MB per file
    MaxArchiveSize  = 50 * 1024 * 1024 // 50MB total archive
)
```

## Troubleshooting

### Common Issues

1. **Archive Upload Fails**
   - Check file size limits
   - Ensure archive contains valid project files
   - Remove binary files and build artifacts

2. **Environment Setup Fails**
   - Check Docker is running
   - Verify project has valid configuration files (package.json, requirements.txt, etc.)
   - Check logs for specific error messages

3. **Tests Don't Generate**
   - Ensure project has clear structure
   - Add comments to explain complex code
   - Try smaller, focused requests

4. **Web Interface Not Accessible**
   - Check if port 8080 is available
   - Verify session is active
   - Try refreshing the page

### Debug Commands

```bash
# Check Docker status
docker ps

# View container logs
docker logs <container_id>

# Check web server
curl http://localhost:8080/

# View session status
curl http://localhost:8080/vibe_<user_id>
```

### Log Messages

The system provides detailed logging with emoji indicators:

- 🔥 Session events
- 🧪 Test operations
- 🐳 Docker operations
- 🌐 Web server events
- 🔧 Fixing operations
- ✅ Success operations
- ❌ Error operations

### Error Recovery

The system includes automatic error recovery mechanisms:

1. **Environment Setup**: Up to 3 retry attempts with LLM-guided fixes
2. **Test Execution**: Automatic fixing of failing tests
3. **Container Issues**: Automatic container recreation
4. **Resource Cleanup**: Automatic cleanup on session end

## Performance Considerations

- **Memory**: Each session uses 100-500MB depending on project size
- **CPU**: LLM requests are the main performance bottleneck
- **Storage**: Temporary files are cleaned up automatically
- **Network**: Docker image downloads only occur once per image

## Security

- **Isolation**: Each session runs in a separate Docker container
- **File Access**: Limited to uploaded project files
- **Network**: No external network access from containers
- **Cleanup**: All resources are cleaned up on session end

---

For more information or support, please refer to the main project documentation or create an issue in the repository.