package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// VibeCodingMCPClient клиент для работы с VibeCoding MCP сервером
type VibeCodingMCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewVibeCodingMCPClient создает новый VibeCoding MCP клиент
func NewVibeCodingMCPClient() *VibeCodingMCPClient {
	return &VibeCodingMCPClient{}
}

// Connect подключается к VibeCoding MCP серверу через stdio
func (m *VibeCodingMCPClient) Connect(ctx context.Context, sessionManager *SessionManager) error {
	log.Printf("🔗 Connecting to VibeCoding MCP server via stdio")

	// Создаем MCP клиент
	m.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot-vibecoding",
		Version: "1.0.0",
	}, nil)

	// Запускаем VibeCoding MCP сервер как подпроцесс
	serverPath := "./vibecoding-mcp-server"
	if customPath := os.Getenv("VIBECODING_MCP_SERVER_PATH"); customPath != "" {
		serverPath = customPath
	}

	cmd := exec.CommandContext(ctx, serverPath)
	cmd.Env = append(os.Environ())

	transport := mcp.NewCommandTransport(cmd)

	session, err := m.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to VibeCoding MCP server: %w", err)
	}

	m.session = session
	log.Printf("✅ Connected to VibeCoding MCP server")
	return nil
}

// Close закрывает соединение с VibeCoding MCP сервером
func (m *VibeCodingMCPClient) Close() error {
	if m.session != nil {
		return m.session.Close()
	}
	return nil
}

// ListFiles получает список файлов в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) ListFiles(ctx context.Context, userID int64) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("📁 Listing files via MCP for user: %d", userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_list_files",
		Arguments: map[string]any{
			"user_id": userID,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP list files error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "List files tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные
	var totalFiles int
	if result.Meta != nil {
		if count, ok := result.Meta["total_files"].(float64); ok {
			totalFiles = int(count)
		}
	}

	return VibeCodingMCPResult{
		Success:    true,
		Message:    responseText,
		TotalFiles: totalFiles,
		Data:       formatResultMeta(result.Meta),
	}
}

// ReadFile читает файл в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) ReadFile(ctx context.Context, userID int64, filename string) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("📄 Reading file via MCP: %s for user %d", filename, userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_read_file",
		Arguments: map[string]any{
			"user_id":  userID,
			"filename": filename,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP read file error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Read file tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// WriteFile записывает файл в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) WriteFile(ctx context.Context, userID int64, filename, content string, generated bool) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("✏️ Writing file via MCP: %s for user %d", filename, userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_write_file",
		Arguments: map[string]any{
			"user_id":   userID,
			"filename":  filename,
			"content":   content,
			"generated": generated,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP write file error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Write file tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// ExecuteCommand выполняет команду в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) ExecuteCommand(ctx context.Context, userID int64, command string) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("⚡ Executing command via MCP: %s for user %d", command, userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_execute_command",
		Arguments: map[string]any{
			"user_id": userID,
			"command": command,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP execute command error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Execute command tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// RunTests запускает тесты в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) RunTests(ctx context.Context, userID int64, testFile string) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("🧪 Running tests via MCP for user %d", userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_run_tests",
		Arguments: map[string]any{
			"user_id":   userID,
			"test_file": testFile,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP run tests error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Run tests tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// ValidateCode валидирует код в VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) ValidateCode(ctx context.Context, userID int64, filename string) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("🔍 Validating code via MCP for user %d", userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_validate_code",
		Arguments: map[string]any{
			"user_id":  userID,
			"filename": filename,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP validate code error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Validate code tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// GetSessionInfo получает информацию о VibeCoding сессии через MCP
func (m *VibeCodingMCPClient) GetSessionInfo(ctx context.Context, userID int64) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("ℹ️ Getting session info via MCP for user %d", userID)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_get_session_info",
		Arguments: map[string]any{
			"user_id": userID,
		},
	})

	if err != nil {
		log.Printf("❌ VibeCoding MCP get session info error: %v", err)
		return VibeCodingMCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return VibeCodingMCPResult{Success: false, Message: "Get session info tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return VibeCodingMCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// VibeCodingMCPResult результат VibeCoding MCP операции
type VibeCodingMCPResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	Data       string `json:"data,omitempty"`
	TotalFiles int    `json:"total_files,omitempty"`
}

// formatResultMeta форматирует метаданные результата в JSON строку
func formatResultMeta(meta any) string {
	if meta == nil {
		return ""
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return string(data)
}
