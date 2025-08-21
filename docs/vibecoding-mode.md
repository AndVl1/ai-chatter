# VibeCoding Mode Documentation

## Overview

VibeCoding Mode is an interactive development session that allows users to upload code archives, get real-time analysis, generate tests, and iteratively improve their projects with AI assistance. It provides a comprehensive development environment with automated test fixing, environment setup, project visualization, and now includes a full MCP (Model Context Protocol) server architecture with external web interface.

## Table of Contents

1. [Architecture](#architecture)
2. [Core Components](#core-components)
3. [MCP Server Architecture](#mcp-server-architecture)
4. [External Web Interface](#external-web-interface)
5. [Session Management](#session-management)
6. [LLM Integration](#llm-integration)
7. [Test System](#test-system)
8. [Web Interface](#web-interface)
9. [Usage Guide](#usage-guide)
10. [API Reference](#api-reference)
11. [Configuration](#configuration)
12. [Troubleshooting](#troubleshooting)

## Architecture

VibeCoding mode is built with a modular architecture consisting of several key components:

```
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────┐
│   Telegram Bot      │────│  VibeCoding         │────│   LLM Client    │
│                     │    │   Handler           │    │                 │
└─────────────────────┘    └─────────────────────┘    └─────────────────┘
                                      │
                        ┌─────────────┼─────────────┐
                        │                           │
            ┌─────────────────────┐    ┌─────────────────────┐
            │ Session Manager     │    │  Internal Web       │
            │                     │    │  Server             │
            └─────────────────────┘    └─────────────────────┘
                        │
              ┌─────────┼─────────┐
              │                   │
    ┌─────────────────┐    ┌─────────────────────┐
    │ Docker Adapter  │    │ VibeCoding MCP      │
    │                 │    │ Server              │
    └─────────────────┘    └─────────────────────┘
              │                   │
    ┌─────────────────┐           │ MCP Protocol
    │ Docker          │           │ (stdin/stdout)
    │ Containers      │           │
    │ + MCP Server    │           │
    └─────────────────┘           │
                                  │
                      ┌─────────────────────┐
                      │ External Web        │
                      │ Interface           │
                      │ (localhost:3000)    │
                      └─────────────────────┘
```

### Key Features

- **Interactive Development**: Real-time code analysis and improvement suggestions
- **Automated Environment Setup**: Docker-based isolated environments with LLM-guided configuration
- **Intelligent Test Generation**: Automatic test creation with iterative fixing based on execution results
- **Dual Web Interface**: 
  - Internal web server for basic project visualization
  - External web interface with full MCP integration for advanced interaction
- **Full MCP Server**: Complete Model Context Protocol server running inside containers
- **External MCP Client**: Dedicated web interface communicating via MCP protocol
- **Multi-Language Support**: Python, JavaScript/TypeScript, Go, Java, and more
- **Container Orchestration**: Docker Compose setup for scalable deployment
- **LLM-Based Architecture**: Removed all hardcoded language patterns in favor of unified LLM approach

## MCP Server Architecture

### VibeCoding MCP Server (`cmd/vibecoding-mcp-server/main.go`)

The VibeCoding MCP Server is a complete Model Context Protocol implementation that provides programmatic access to VibeCoding sessions. It follows the same architecture as Gmail and Notion MCP servers.

```go
type VibeCodingMCPServer struct {
    sessionManager *vibecoding.SessionManager
}
```

**Registered MCP Tools:**

1. **`vibe_list_files`** - List all files in workspace
   - Parameters: `user_id`
   - Returns: Array of filenames with metadata

2. **`vibe_read_file`** - Read file content
   - Parameters: `user_id`, `filename`
   - Returns: File content and metadata

3. **`vibe_write_file`** - Write/update file
   - Parameters: `user_id`, `filename`, `content`, `generated`
   - Returns: Success status and file info

4. **`vibe_execute_command`** - Execute shell command
   - Parameters: `user_id`, `command`
   - Returns: Command output, exit code, success status

5. **`vibe_validate_code`** - Validate code syntax/compilation
   - Parameters: `user_id`, `filename`
   - Returns: Validation results with errors/warnings

6. **`vibe_run_tests`** - Execute test suite
   - Parameters: `user_id`, `test_file` (optional)
   - Returns: Test results and output

7. **`vibe_get_session_info`** - Get session metadata
   - Parameters: `user_id`
   - Returns: Session status, container info, timestamps

### MCP Communication Protocol

The server communicates via standard stdin/stdout JSON-RPC 2.0:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "vibe_read_file",
    "arguments": {
      "user_id": 123456,
      "filename": "main.py"
    }
  }
}
```

### Docker Integration

The MCP server is automatically deployed inside each coding container:

1. **Container Creation**: When a VibeCoding session starts, the system creates a Docker container
2. **MCP Server Deployment**: The `vibecoding-mcp-server` binary is copied into the container
3. **Auto-Start**: The MCP server starts automatically as a background process
4. **External Access**: External clients can connect to the MCP server for direct file/command access

## External Web Interface

### Architecture (`docker/vibecoding-web/`)

The external web interface runs in a separate Docker container and communicates with VibeCoding sessions through the internal HTTP API.

```
External Web Container (Node.js + Express)
    ↓ HTTP API
Web UI (HTML/CSS/JavaScript)
    ↓ RESTful calls
VibeCoding HTTP API Client
    ↓ HTTP requests (port 8080)
VibeCoding Internal API
    ↓ Direct API calls
VibeCoding Session Manager
```

### Web Interface Features

1. **Session Management**
   - Load sessions by User ID
   - Display session information and status
   - Real-time connection status monitoring

2. **File Management**
   - Interactive file browser with tree view
   - Syntax-highlighted code editor
   - Save changes directly to containers
   - Distinguish between original and generated files

3. **Terminal Interface**
   - Execute commands in real-time
   - Command history and auto-completion
   - Scrollable output display
   - Background command execution

4. **Test Runner**
   - One-click test execution
   - Test result visualization
   - Pass/fail status indicators
   - Test output analysis

### API Endpoints

```javascript
// File operations
GET    /api/files/:userId              // List all files (HTTP API)
GET    /api/files/:userId/:filename    // Read file content (HTTP API)
POST   /api/files/:userId/:filename    // Write file content (stub)

// Command execution
POST   /api/execute/:userId           // Execute shell command (stub)
POST   /api/test/:userId              // Run tests (stub)

// Session management
GET    /api/session/:userId           // Get session info (HTTP API)
GET    /api/status                    // Server status and HTTP API connection
```

### Docker Compose Deployment

The system now supports full containerization:

```yaml
# docker-compose.vibecoding.yml
services:
  vibecoding-mcp:
    build: docker/vibecoding-mcp/
    volumes:
      - /tmp/vibecoding-mcp:/tmp/vibecoding-mcp
    
  vibecoding-web:
    build: docker/vibecoding-web/
    ports:
      - "3000:3000"
    depends_on:
      - vibecoding-mcp
```

**Startup Script:**

```bash
./scripts/start-vibecoding-web.sh
```

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
- `SetupEnvironment(ctx)`: Configures Docker environment with up to 3 retry attempts and auto-starts MCP server
- `ExecuteCommand(ctx, command)`: Runs commands in the container
- `ListFiles(ctx)`: Returns list of all files in session
- `ReadFile(ctx, filename)`: Reads content of a specific file
- `WriteFile(ctx, filename, content, generated)`: Writes file to session and container
- `ValidateCode(ctx, code, filename)`: Validates code using container environment
- `AddGeneratedFile(filename, content)`: Adds AI-generated files
- `GetAllFiles()`: Returns combined original and generated files
- `CreatedAt()`: Returns session creation time for MCP compatibility
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
- `GetSession(userID)`: Retrieves active session (now returns pointer only)
- `EndSession(userID)`: Terminates session and cleanup
- `HasActiveSession(userID)`: Checks if user has active session
- `GetActiveSessions()`: Returns count of active sessions

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

### LLM-Based Test Detection

**DEPRECATED**: Hardcoded test file detection has been completely removed and replaced with LLM-based analysis for maximum flexibility and accuracy across all programming languages.

The system now uses LLM analysis to:
- Identify test files with context and confidence scoring
- Determine test command compatibility with specific files
- Adapt test commands for different file types
- Generate appropriate test commands based on project structure

This unified approach eliminates language-specific hardcoded patterns and provides better support for:
- Custom testing frameworks
- Non-standard file naming conventions
- Multi-language projects
- Emerging programming languages

## Web Interface

VibeCoding now provides two complementary web interfaces:

### 1. Internal Web Server (`webserver.go`)

Basic project visualization integrated with the Telegram bot:

- **URL Pattern**: `http://localhost:8080/api/vibe/{userID}` (internal API)
- **Purpose**: Quick project overview and file browsing
- **Features**: File tree, basic file viewer, session stats
- **Auto-refresh**: Updates every 30 seconds

### 2. External Web Interface (HTTP API-Based)

Advanced web interface with HTTP API integration:

- **URL**: `http://localhost:3000`
- **Purpose**: Complete project management and development environment
- **Architecture**: Separate Docker container communicating via HTTP API
- **Features**: 
  - Interactive file browser and viewer
  - Session management dashboard
  - Live connection status monitoring
  - File loading and display (read-only currently)

### Key Endpoints (Internal)

- `GET /vibe_{userID}`: Main project page
- `GET /api/vibe_{userID}`: JSON session data
- `GET /api/vibe_{userID}/file/{filepath}`: File content
- `GET /static/...`: Static assets (CSS, JS)

### Key Endpoints (External HTTP API)

- `GET /api/files/:userId`: List files via HTTP API
- `GET /api/files/:userId/:filename`: Read file via HTTP API
- `POST /api/files/:userId/:filename`: Write file (stub)
- `POST /api/execute/:userId`: Execute commands (stub)
- `POST /api/test/:userId`: Run tests (stub)
- `GET /api/session/:userId`: Session info via HTTP API
- `GET /api/status`: HTTP API connection status

### Technology Stack

**Internal Server:**
- Go + HTML templates
- WebSocket for real-time updates
- Static file serving

**External Interface:**
- Node.js + Express backend
- Vanilla JavaScript frontend
- HTTP API client for internal communication
- Docker containerization

## Usage Guide

### Starting a VibeCoding Session

#### Method 1: Traditional Telegram Bot Workflow

1. **Prepare Archive**: Create a .zip/.tar.gz archive of your project
2. **Upload**: Send the archive to the Telegram bot without any caption
3. **Wait for Setup**: The system will automatically:
   - Extract files
   - Analyze the project with LLM
   - Set up Docker environment with MCP server
   - Install dependencies
   - Deploy VibeCoding MCP server inside container
4. **Receive Confirmation**: Get session details and both web interface URLs

#### Method 2: External Web Interface (New)

1. **Start VibeCoding System**: Run `./scripts/start-vibecoding-web.sh`
2. **Access Web Interface**: Open `http://localhost:3000`
3. **Load Session**: Enter User ID and click "Load Session"
4. **Interactive Development**: Use the full-featured web interface for:
   - File editing and management
   - Command execution
   - Test running
   - Real-time project monitoring

### Example Session Flow

```
User: [Uploads Python project archive]
Bot: 🔥 Сессия вайбкодинга готова!
     Проект: my-python-app
     Язык: Python
     🔧 MCP сервер запущен в контейнере
     🌐 Веб-интерфейс: http://localhost:3000
     🌐 Внешний интерфейс: http://localhost:3000 (User ID: 123)

User: [Opens external web interface at localhost:3000]
      [Enters User ID: 123 and clicks "Load Session"]
Web:  ✅ Session loaded successfully
      📁 Files: main.py, requirements.txt, README.md
      📊 Status: Active, Container: abc123

User: [Clicks "Run Tests" in web interface]
Web:  🧪 Running tests...
      ✅ All tests passed (3/3)

User: [Edits main.py in web editor, adds error handling]
      [Clicks "Save"]
Web:  💾 File saved successfully

User: [Executes "python main.py" in web terminal]
Web:  $ python main.py
      Hello World with error handling!
      $ 

User: /vibecoding_end (in Telegram)
Bot:  🔥 Сессия завершена
      📦 MCP сервер остановлен
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
// Create and setup environment with MCP server auto-start
func (s *VibeCodingSession) SetupEnvironment(ctx context.Context) error

// Execute commands in container
func (s *VibeCodingSession) ExecuteCommand(ctx context.Context, command string) (*ValidationResult, error)

// MCP-compatible file operations
func (s *VibeCodingSession) ListFiles(ctx context.Context) ([]string, error)
func (s *VibeCodingSession) ReadFile(ctx context.Context, filename string) (string, error)
func (s *VibeCodingSession) WriteFile(ctx context.Context, filename, content string, generated bool) error
func (s *VibeCodingSession) ValidateCode(ctx context.Context, code, filename string) (*ValidationResult, error)

// Add generated files
func (s *VibeCodingSession) AddGeneratedFile(filename, content string)

// Get all files (original + generated)
func (s *VibeCodingSession) GetAllFiles() map[string]string

// Get session information
func (s *VibeCodingSession) GetSessionInfo() map[string]interface{}

// MCP compatibility
func (s *VibeCodingSession) CreatedAt() time.Time

// Cleanup resources
func (s *VibeCodingSession) Cleanup() error
```

#### SessionManager

```go
// Create new session
func (sm *SessionManager) CreateSession(userID, chatID int64, projectName string, files map[string]string, llmClient llm.Client) (*VibeCodingSession, error)

// Get existing session (returns pointer only)
func (sm *SessionManager) GetSession(userID int64) *VibeCodingSession

// End session
func (sm *SessionManager) EndSession(userID int64) error

// Check if session exists
func (sm *SessionManager) HasActiveSession(userID int64) bool

// Get active session count
func (sm *SessionManager) GetActiveSessions() int
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

### MCP Server API

#### VibeCodingMCPServer

```go
// MCP tool implementations
func (s *VibeCodingMCPServer) ListFiles(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) ReadFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) WriteFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) ExecuteCommand(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) ValidateCode(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) RunTests(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)

func (s *VibeCodingMCPServer) GetSessionInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error)
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

# VibeCoding MCP server configuration
VIBECODING_MCP_SERVER_PATH=./cmd/vibecoding-mcp-server/vibecoding-mcp-server
MCP_SOCKET_PATH=/tmp/vibecoding-mcp

# Web server ports
VIBECODING_WEB_PORT=8080    # Internal web server
PORT=3000                   # External web interface

# LLM configuration
LLM_PROVIDER=openai
LLM_MODEL=gpt-4
LLM_API_KEY=your-api-key

# Docker Compose configuration
COMPOSE_PROJECT_NAME=vibecoding
COMPOSE_FILE=docker-compose.vibecoding.yml
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
   - Ensure VibeCoding MCP server binary is built

3. **MCP Server Issues**
   - Verify MCP server is built: `go build -o ./cmd/vibecoding-mcp-server/vibecoding-mcp-server ./cmd/vibecoding-mcp-server/`
   - Check container has MCP server: `docker exec <container_id> ls -la /workspace/vibecoding-mcp-server`
   - View MCP server logs: `docker exec <container_id> cat /tmp/mcp-server.log`

4. **External Web Interface Issues**
   - Check MCP connection status: `curl http://localhost:3000/api/status`
   - Verify containers are running: `docker-compose -f docker-compose.vibecoding.yml ps`
   - Check web interface logs: `docker-compose -f docker-compose.vibecoding.yml logs vibecoding-web`

5. **Internal Web Interface Not Accessible**
   - Check if port 8080 is available
   - Verify session is active
   - Try refreshing the page

### Debug Commands

```bash
# Check Docker status
docker ps

# View container logs
docker logs <container_id>

# Check internal web server
curl http://localhost:8080/

# Check external web interface
curl http://localhost:3000/api/status

# View session status (internal API)
curl http://localhost:8080/api/vibe/<user_id>

# View session status (external MCP)
curl http://localhost:3000/api/session/<user_id>

# VibeCoding system status
docker-compose -f docker-compose.vibecoding.yml ps
docker-compose -f docker-compose.vibecoding.yml logs

# MCP server debug
docker exec <container_id> ps aux | grep vibecoding-mcp-server
docker exec <container_id> netstat -tlnp | grep 8090
```

### Log Messages

The system provides detailed logging with emoji indicators:

- 🔥 Session events
- 🧪 Test operations
- 🐳 Docker operations
- 🌐 Web server events
- 🔧 Fixing operations
- 📤 MCP requests/responses
- 🔗 MCP connection events
- 💻 Terminal operations
- 📁 File operations
- ✅ Success operations
- ❌ Error operations

### Error Recovery

The system includes automatic error recovery mechanisms:

1. **Environment Setup**: Up to 3 retry attempts with LLM-guided fixes
2. **Test Execution**: Automatic fixing of failing tests
3. **Container Issues**: Automatic container recreation
4. **MCP Connection**: Automatic reconnection and retry logic
5. **File Operations**: Graceful handling of file access errors
6. **Resource Cleanup**: Automatic cleanup on session end

## Performance Considerations

- **Memory**: Each session uses 100-500MB depending on project size
- **CPU**: LLM requests are the main performance bottleneck  
- **Storage**: Temporary files are cleaned up automatically
- **Network**: Docker image downloads only occur once per image
- **MCP Overhead**: Minimal additional overhead for protocol communication
- **Scalability**: External web interface allows multiple concurrent sessions

## Security

- **Isolation**: Each session runs in a separate Docker container
- **File Access**: Limited to uploaded project files
- **Network**: Controlled external network access from containers
- **MCP Security**: Communication isolated to containers and authorized clients
- **External Interface**: Runs in separate container with no direct file system access
- **Cleanup**: All resources are cleaned up on session end

## Recent Changes

### Version 2.0 - MCP Architecture Update

- ✅ **Full MCP Server**: Complete Model Context Protocol implementation
- ✅ **External Web Interface**: Separate containerized web interface with MCP communication
- ✅ **Docker Compose**: Orchestrated deployment with `docker-compose.vibecoding.yml`
- ✅ **LLM-Based Architecture**: Removed all hardcoded language patterns
- ✅ **7 MCP Tools**: Complete set of tools for file/command/test operations
- ✅ **Auto MCP Deployment**: Automatic MCP server deployment in containers
- ✅ **Dual Interface**: Both internal (port 8080) and external (port 3000) interfaces
- ✅ **Startup Scripts**: Automated deployment with `./scripts/start-vibecoding-web.sh`

---

For more information or support, please refer to:
- Main project documentation
- `README-VIBECODING-MCP.md` for detailed MCP architecture guide
- Create an issue in the repository for bugs or feature requests