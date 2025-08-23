package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ai-chatter/internal/vibecoding"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// VibeCodingMCPServer основной VibeCoding MCP сервер
type VibeCodingMCPServer struct {
	sessionManager *vibecoding.SessionManager
}

// NewVibeCodingMCPServer создает новый VibeCoding MCP сервер
func NewVibeCodingMCPServer() *VibeCodingMCPServer {
	log.Printf("🔧 Initializing VibeCoding MCP Server")

	sessionManager := vibecoding.NewSessionManager()

	return &VibeCodingMCPServer{
		sessionManager: sessionManager,
	}
}

// ListFiles получает список файлов в VibeCoding сессии
func (s *VibeCodingMCPServer) ListFiles(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("📁 MCP Server: Listing files for user %d", userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	files, err := vibeCodingSession.ListFiles(ctx)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to list files: %v", err)},
			},
		}, nil
	}

	var resultMessage string
	if len(files) == 0 {
		resultMessage = "📁 No files found in VibeCoding workspace"
	} else {
		resultMessage = fmt.Sprintf("📁 Found %d files in VibeCoding workspace:\n\n", len(files))
		for i, file := range files {
			resultMessage += fmt.Sprintf("%d. %s\n", i+1, file)
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":     userID,
			"files":       files,
			"total_files": len(files),
			"success":     true,
		},
	}, nil
}

// ReadFile читает файл из VibeCoding сессии
func (s *VibeCodingMCPServer) ReadFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	filename, ok := params.Arguments["filename"].(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename parameter is required and must be a string"},
			},
		}, nil
	}

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("📄 MCP Server: Reading file %s for user %d", filename, userID)

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

	resultMessage := fmt.Sprintf("📄 Content of file %s:\n\n```\n%s\n```", filename, content)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":  userID,
			"filename": filename,
			"size":     len(content),
			"success":  true,
		},
	}, nil
}

// WriteFile записывает файл в VibeCoding сессию
func (s *VibeCodingMCPServer) WriteFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	filename, ok := params.Arguments["filename"].(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename parameter is required and must be a string"},
			},
		}, nil
	}

	content, ok := params.Arguments["content"].(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ content parameter is required and must be a string"},
			},
		}, nil
	}

	generated, _ := params.Arguments["generated"].(bool)

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("✏️ MCP Server: Writing file %s for user %d (generated: %t)", filename, userID, generated)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	err := vibeCodingSession.WriteFile(ctx, filename, content, generated)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to write file: %v", err)},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Successfully wrote file %s (%d bytes)", filename, len(content))

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":   userID,
			"filename":  filename,
			"size":      len(content),
			"generated": generated,
			"success":   true,
		},
	}, nil
}

// ExecuteCommand выполняет команду в VibeCoding сессии
func (s *VibeCodingMCPServer) ExecuteCommand(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	command, ok := params.Arguments["command"].(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ command parameter is required and must be a string"},
			},
		}, nil
	}

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("⚡ MCP Server: Executing command '%s' for user %d", command, userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	result, err := vibeCodingSession.ExecuteCommand(ctx, command)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to execute command: %v", err)},
			},
		}, nil
	}

	var status string
	if result.Success {
		status = "✅ Success"
	} else {
		status = "❌ Failed"
	}

	resultMessage := fmt.Sprintf("%s Command execution completed\n\n**Command:** %s\n**Exit Code:** %d\n**Output:**\n```\n%s\n```",
		status, command, result.ExitCode, result.Output)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":   userID,
			"command":   command,
			"success":   result.Success,
			"exit_code": result.ExitCode,
			"output":    result.Output,
		},
	}, nil
}

// ValidateCode валидирует код в VibeCoding сессии
func (s *VibeCodingMCPServer) ValidateCode(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	filename, ok := params.Arguments["filename"].(string)
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ filename parameter is required and must be a string"},
			},
		}, nil
	}

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("🔍 MCP Server: Validating code in file %s for user %d", filename, userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	// Читаем содержимое файла
	content, err := vibeCodingSession.ReadFile(ctx, filename)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to read file for validation: %v", err)},
			},
		}, nil
	}

	// Валидируем код через VibeCoding сессию
	result, err := vibeCodingSession.ValidateCode(ctx, content, filename)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to validate code: %v", err)},
			},
		}, nil
	}

	var status string
	if result.Success {
		status = "✅ Validation Passed"
	} else {
		status = "❌ Validation Failed"
	}

	resultMessage := fmt.Sprintf("%s Code validation completed\n\n**File:** %s\n**Exit Code:** %d\n**Output:**\n```\n%s\n```",
		status, filename, result.ExitCode, result.Output)

	if len(result.Errors) > 0 {
		resultMessage += "\n\n**Errors:**\n"
		for _, err := range result.Errors {
			resultMessage += fmt.Sprintf("- %s\n", err)
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":   userID,
			"filename":  filename,
			"success":   result.Success,
			"exit_code": result.ExitCode,
			"errors":    result.Errors,
		},
	}, nil
}

// RunTests запускает тесты в VibeCoding сессии
func (s *VibeCodingMCPServer) RunTests(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	testFile, _ := params.Arguments["test_file"].(string)

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("🧪 MCP Server: Running tests for user %d (test_file: %s)", userID, testFile)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	// Получаем опциональный параметр validate_and_fix
	validateAndFix, _ := params.Arguments["validate_and_fix"].(bool)

	if validateAndFix {
		// Получаем список сгенерированных тестовых файлов для валидации
		var testFilesToValidate []string
		for filename := range vibeCodingSession.GeneratedFiles {
			if strings.Contains(strings.ToLower(filename), "test") {
				testFilesToValidate = append(testFilesToValidate, filename)
			}
		}

		if len(testFilesToValidate) > 0 {
			log.Printf("🧪 Running test validation for %d generated test files", len(testFilesToValidate))

			// Запускаем валидацию и исправление тестов
			if err := vibeCodingSession.ValidateAndFixTests(ctx, testFilesToValidate); err != nil {
				return &mcp.CallToolResultFor[any]{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("❌ Test validation failed: %v", err)},
					},
				}, nil
			}

			// Возвращаем результат валидации
			resultMessage := fmt.Sprintf("✅ Test Validation Completed\n\n**Validated Files:** %d\n**All tests passed after validation and fixes**",
				len(testFilesToValidate))

			return &mcp.CallToolResultFor[any]{
				Content: []mcp.Content{
					&mcp.TextContent{Text: resultMessage},
				},
				Meta: map[string]interface{}{
					"user_id":         userID,
					"test_file":       testFile,
					"validated_files": testFilesToValidate,
					"success":         true,
					"validation_mode": true,
				},
			}, nil
		}
	}

	// Стандартный режим - просто запускаем тесты
	var testCommand string
	if vibeCodingSession.TestCommand != "" {
		testCommand = vibeCodingSession.TestCommand
	} else {
		testCommand = "echo 'No test command configured'"
	}

	result, err := vibeCodingSession.ExecuteCommand(ctx, testCommand)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to run tests: %v", err)},
			},
		}, nil
	}

	var status string
	if result.Success {
		status = "✅ Tests Passed"
	} else {
		status = "❌ Tests Failed"
	}

	resultMessage := fmt.Sprintf("%s Test execution completed\n\n**Test Command:** %s\n**Exit Code:** %d\n**Output:**\n```\n%s\n```",
		status, testCommand, result.ExitCode, result.Output)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":      userID,
			"test_file":    testFile,
			"test_command": testCommand,
			"success":      result.Success,
			"exit_code":    result.ExitCode,
		},
	}, nil
}

// GetSessionInfo получает информацию о VibeCoding сессии
func (s *VibeCodingMCPServer) GetSessionInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	userIDArg, ok := params.Arguments["user_id"]
	if !ok {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id parameter is required"},
			},
		}, nil
	}

	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return &mcp.CallToolResultFor[any]{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("❌ Invalid user_id format: %v", err)},
				},
			}, nil
		}
		userID = parsed
	default:
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ user_id must be a number"},
			},
		}, nil
	}

	log.Printf("ℹ️ MCP Server: Getting session info for user %d", userID)

	vibeCodingSession := s.sessionManager.GetSession(userID)
	if vibeCodingSession == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ No VibeCoding session found for user"},
			},
		}, nil
	}

	status := "Active"
	if vibeCodingSession.ContainerID == "" {
		status = "No Container"
	}

	resultMessage := fmt.Sprintf("ℹ️ VibeCoding Session Information\n\n**User ID:** %d\n**Status:** %s\n**Container ID:** %s\n**Test Command:** %s\n**Created:** %s",
		userID, status, vibeCodingSession.ContainerID, vibeCodingSession.TestCommand, vibeCodingSession.CreatedAt().Format(time.RFC3339))

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"user_id":      userID,
			"status":       status,
			"container_id": vibeCodingSession.ContainerID,
			"test_command": vibeCodingSession.TestCommand,
			"created_at":   vibeCodingSession.CreatedAt(),
			"success":      true,
		},
	}, nil
}

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	log.Printf("🚀 Starting VibeCoding MCP Server")

	// Создаем VibeCoding сервер
	vibeCodingServer := NewVibeCodingMCPServer()

	// Создаем MCP сервер
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-chatter-vibecoding-mcp",
		Version: "1.0.0",
	}, nil)

	// Регистрируем все VibeCoding инструменты
	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_list_files",
		Description: "Lists all files in the VibeCoding workspace for the specified user",
	}, vibeCodingServer.ListFiles)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_read_file",
		Description: "Reads the content of a specific file from the VibeCoding workspace",
	}, vibeCodingServer.ReadFile)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_write_file",
		Description: "Writes content to a file in the VibeCoding workspace",
	}, vibeCodingServer.WriteFile)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_execute_command",
		Description: "Executes a shell command in the VibeCoding session container",
	}, vibeCodingServer.ExecuteCommand)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_validate_code",
		Description: "Validates code in a specific file using the VibeCoding validation system",
	}, vibeCodingServer.ValidateCode)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_run_tests",
		Description: "Runs tests for the VibeCoding project using the configured test command. Set validate_and_fix=true to automatically validate generated tests and fix failures.",
	}, vibeCodingServer.RunTests)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vibe_get_session_info",
		Description: "Gets information about the VibeCoding session for the specified user",
	}, vibeCodingServer.GetSessionInfo)

	log.Printf("📋 Registered 7 VibeCoding MCP tools:")
	log.Printf("   - vibe_list_files: Lists files in workspace")
	log.Printf("   - vibe_read_file: Reads file content")
	log.Printf("   - vibe_write_file: Writes file content")
	log.Printf("   - vibe_execute_command: Executes commands")
	log.Printf("   - vibe_validate_code: Validates code")
	log.Printf("   - vibe_run_tests: Runs tests")
	log.Printf("   - vibe_get_session_info: Gets session info")
	log.Printf("🔗 Starting VibeCoding MCP server on stdin/stdout...")

	// Запускаем сервер через stdin/stdout
	transport := mcp.NewStdioTransport()
	if err := server.Run(context.Background(), transport); err != nil {
		log.Fatalf("❌ VibeCoding MCP Server failed: %v", err)
	}
}
