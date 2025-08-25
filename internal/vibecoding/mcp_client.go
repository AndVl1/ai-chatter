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
	client     *mcp.Client
	session    *mcp.ClientSession
	httpServer *VibeCodingMCPHTTPServer
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
	serverPath := "./bin/vibecoding-mcp-server"
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

// ConnectWebSocket подключается к VibeCoding MCP серверу через WebSocket (неактивно)
func (m *VibeCodingMCPClient) ConnectWebSocket(ctx context.Context, serverURL string) error {
	log.Printf("⚠️  WebSocket transport not implemented - falling back to stdio")
	return fmt.Errorf("WebSocket transport not available")
}

// ConnectHTTP подключается к VibeCoding MCP серверу через HTTP SSE
func (m *VibeCodingMCPClient) ConnectHTTP(ctx context.Context, sessionManager *SessionManager) error {
	log.Printf("🌐 Attempting to connect to VibeCoding MCP server via HTTP SSE")

	// Try SSE HTTP connection
	sseURL := "http://localhost:8082/mcp"
	if customURL := os.Getenv("VIBECODING_SSE_URL"); customURL != "" {
		sseURL = customURL
	}

	if err := m.ConnectSSE(ctx, sseURL); err != nil {
		log.Printf("⚠️ SSE connection failed: %v - trying WebSocket fallback", err)

		// Try WebSocket connection as fallback
		websocketURL := "ws://localhost:8081/ws"
		if customURL := os.Getenv("VIBECODING_WEBSOCKET_URL"); customURL != "" {
			websocketURL = customURL
		}

		if err := m.ConnectWebSocket(ctx, websocketURL); err != nil {
			log.Printf("⚠️ WebSocket connection also failed: %v - falling back to stdio", err)
			return m.Connect(ctx, sessionManager)
		}
	}

	return nil
}

// ConnectSSE подключается к VibeCoding MCP серверу через Server-Sent Events
func (m *VibeCodingMCPClient) ConnectSSE(ctx context.Context, sseURL string) error {
	log.Printf("🌐 Connecting to VibeCoding MCP server via SSE: %s", sseURL)

	// Создаем MCP клиент
	m.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot-vibecoding-sse",
		Version: "1.0.0",
	}, nil)

	// Создаем SSE транспорт
	transport := mcp.NewSSEClientTransport(sseURL, nil)

	// Подключаемся через MCP клиент
	session, err := m.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to VibeCoding MCP server via SSE: %w", err)
	}

	m.session = session
	log.Printf("✅ Connected to VibeCoding MCP server via SSE")
	return nil
}

// Close закрывает соединение с VibeCoding MCP сервером
func (m *VibeCodingMCPClient) Close() error {
	var err error
	if m.session != nil {
		err = m.session.Close()
	}
	if m.httpServer != nil {
		if stopErr := m.httpServer.Stop(context.Background()); stopErr != nil && err == nil {
			err = stopErr
		}
	}
	return err
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
	return m.RunTestsWithValidation(ctx, userID, testFile, false)
}

// RunTestsWithValidation запускает тесты с опциональной валидацией и исправлением
func (m *VibeCodingMCPClient) RunTestsWithValidation(ctx context.Context, userID int64, testFile string, validateAndFix bool) VibeCodingMCPResult {
	if m.session == nil {
		return VibeCodingMCPResult{Success: false, Message: "VibeCoding MCP session not connected"}
	}

	log.Printf("🧪 Running tests via MCP for user %d (validate_and_fix: %t)", userID, validateAndFix)

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "vibe_run_tests",
		Arguments: map[string]any{
			"user_id":          userID,
			"test_file":        testFile,
			"validate_and_fix": validateAndFix,
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

// GetAvailableTools получает список доступных MCP тулов
func (m *VibeCodingMCPClient) GetAvailableTools(ctx context.Context) ([]string, error) {
	if m.session == nil {
		return nil, fmt.Errorf("VibeCoding MCP session not connected")
	}

	log.Printf("🔧 Getting available MCP tools...")

	// Запрашиваем список тулов у MCP сервера
	toolsResult, err := m.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP tools: %w", err)
	}

	toolNames := make([]string, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	log.Printf("✅ Found %d MCP tools: %v", len(toolNames), toolNames)
	return toolNames, nil
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
