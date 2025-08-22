package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"ai-chatter/internal/vibecoding"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// VibeCodingMCPHTTPServer основной VibeCoding MCP HTTP сервер
type VibeCodingMCPHTTPServer struct {
	sessionManager *vibecoding.SessionManager
}

var vibeCodingServer *VibeCodingMCPHTTPServer

func main() {
	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	log.Printf("🚀 Starting VibeCoding HTTP MCP Server...")

	// Создаем менеджер сессий без веб-сервера (он используется только для основного бота)
	sessionManager := vibecoding.NewSessionManagerWithoutWebServer()

	// Создаем MCP сервер
	vibeCodingServer = &VibeCodingMCPHTTPServer{
		sessionManager: sessionManager,
	}

	// Create MCP server with HTTP transport
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "vibecoding-mcp-http-server",
		Version: "1.0.0",
	}, nil)

	// Register VibeCoding tools
	registerVibeCodingTools(server)

	port := os.Getenv("VIBECODING_HTTP_PORT")
	if port == "" {
		port = "8082"
	}

	// SSE handler for MCP
	handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server })
	http.Handle("/mcp", handler)

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("VibeCoding HTTP MCP Server is running"))
	})

	log.Printf("🌐 VibeCoding SSE MCP Server listening on http://localhost:%s/mcp", port)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: nil,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server failed: %v", err)
		}
	}()

	// Wait for Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	log.Println("🔌 VibeCoding HTTP MCP Server shutting down...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ Server shutdown error: %v", err)
	}
}

func registerVibeCodingTools(server *mcp.Server) {
	// List files tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_list_files",
		Description: "Lists files in the VibeCoding workspace for the specified user",
	}, vibeCodingServer.ListFiles)

	// Read file tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_read_file",
		Description: "Reads the content of a file in the VibeCoding workspace",
	}, vibeCodingServer.ReadFile)

	// Write file tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_write_file",
		Description: "Writes content to a file in the VibeCoding workspace. Set generated=true for AI-generated files.",
	}, vibeCodingServer.WriteFile)

	// Execute command tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_execute_command",
		Description: "Executes a command in the VibeCoding environment",
	}, vibeCodingServer.ExecuteCommand)

	// Validate code tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_validate_code",
		Description: "Validates code in a specific file using the VibeCoding validation system",
	}, vibeCodingServer.ValidateCode)

	// Run tests tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_run_tests",
		Description: "Runs tests for the VibeCoding project using the configured test command. Set validate_and_fix=true to automatically validate generated tests and fix failures.",
	}, vibeCodingServer.RunTests)

	// Get session info tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_get_session_info",
		Description: "Gets information about the VibeCoding session for the specified user",
	}, vibeCodingServer.GetSessionInfo)

	log.Printf("📋 Registered 7 VibeCoding HTTP MCP tools")
}

// Implementation of all MCP tools (same logic as stdio version)
// ... (I'll implement the key methods here, referencing the existing stdio implementation)

// ListFiles списки файлов в VibeCoding сессии
func (s *VibeCodingMCPHTTPServer) ListFiles(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	userID, err := vibecoding.ParseUserID(userIDArg)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ Invalid user_id format"},
			},
		}, nil
	}

	log.Printf("📁 HTTP MCP Server: Listing files for user %d", userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	// Get all files from the session
	allFiles := vibeCodingSession.GetAllFiles()
	var fileList []string
	for filename := range allFiles {
		fileList = append(fileList, filename)
	}

	result := vibecoding.FormatFileList(userID, fileList)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
		Meta: map[string]interface{}{
			"user_id":     userID,
			"total_files": len(fileList),
		},
	}, nil
}

// ReadFile читает файл из VibeCoding сессии
func (s *VibeCodingMCPHTTPServer) ReadFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	filenameArg, ok := params.Arguments["filename"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename parameter is required"},
			},
		}, nil
	}

	filename, ok := filenameArg.(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename must be a string"},
			},
		}, nil
	}

	userID, err := vibecoding.ParseUserID(userIDArg)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ Invalid user_id format"},
			},
		}, nil
	}

	log.Printf("📄 HTTP MCP Server: Reading file %s for user %d", filename, userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	content, err := vibeCodingSession.ReadFile(ctx, filename)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to read file: %v", err)},
			},
		}, nil
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: content},
		},
		Meta: map[string]interface{}{
			"user_id":  userID,
			"filename": filename,
			"size":     len(content),
		},
	}, nil
}

// WriteFile записывает файл в VibeCoding сессию
func (s *VibeCodingMCPHTTPServer) WriteFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	filenameArg, ok := params.Arguments["filename"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename parameter is required"},
			},
		}, nil
	}

	contentArg, ok := params.Arguments["content"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ content parameter is required"},
			},
		}, nil
	}

	filename, ok := filenameArg.(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename must be a string"},
			},
		}, nil
	}

	content, ok := contentArg.(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ content must be a string"},
			},
		}, nil
	}

	generated, _ := params.Arguments["generated"].(bool)

	userID, err := vibecoding.ParseUserID(userIDArg)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ Invalid user_id format"},
			},
		}, nil
	}

	log.Printf("✏️ HTTP MCP Server: Writing file %s for user %d (generated: %t)", filename, userID, generated)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	err = vibeCodingSession.WriteFile(ctx, filename, content, generated)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to write file: %v", err)},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ File written successfully\n\n**File:** %s\n**Size:** %d bytes\n**Generated:** %t", filename, len(content), generated)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":   userID,
			"filename":  filename,
			"size":      len(content),
			"generated": generated,
		},
	}, nil
}

// GetSessionInfo получает информацию о VibeCoding сессии
func (s *VibeCodingMCPHTTPServer) GetSessionInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	userID, err := vibecoding.ParseUserID(userIDArg)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ Invalid user_id format"},
			},
		}, nil
	}

	log.Printf("ℹ️ HTTP MCP Server: Getting session info for user %d", userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	sessionInfo := vibecoding.FormatSessionInfo(userID, vibeCodingSession)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: sessionInfo},
		},
		Meta: map[string]interface{}{
			"user_id":         userID,
			"project_name":    vibeCodingSession.ProjectName,
			"container_id":    vibeCodingSession.ContainerID,
			"test_command":    vibeCodingSession.TestCommand,
			"start_time":      vibeCodingSession.StartTime,
			"files_count":     len(vibeCodingSession.Files),
			"generated_count": len(vibeCodingSession.GeneratedFiles),
		},
	}, nil
}

// Stub implementations for other tools
func (s *VibeCodingMCPHTTPServer) ExecuteCommand(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "HTTP ExecuteCommand not fully implemented yet"},
		},
	}, nil
}

func (s *VibeCodingMCPHTTPServer) ValidateCode(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "HTTP ValidateCode not fully implemented yet"},
		},
	}, nil
}

func (s *VibeCodingMCPHTTPServer) RunTests(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "HTTP RunTests not fully implemented yet"},
		},
	}, nil
}
