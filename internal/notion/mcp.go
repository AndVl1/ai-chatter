package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// MCPClient клиент для работы с официальным Notion MCP сервером
type MCPClient struct {
	httpClient *http.Client
	baseURL    string
	sessionID  string
}

// NewMCPClient создает новый MCP клиент для Notion
func NewMCPClient(token string) *MCPClient {
	host := os.Getenv("MCP_HOST")
	baseURL := fmt.Sprintf("http://%s:3000/mcp", host) // Локальный MCP сервер

	// Проверяем переменную окружения для custom URL
	if customURL := os.Getenv("NOTION_MCP_URL"); customURL != "" {
		baseURL = customURL
	}

	return &MCPClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// Connect подключается к локальному Notion MCP серверу
func (m *MCPClient) Connect(ctx context.Context, notionToken string) error {
	log.Printf("🔗 Connecting to local Notion MCP server: %s", m.baseURL)

	// Инициализируем MCP сессию
	initReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"clientInfo": map[string]interface{}{
				"name":    "ai-chatter-bot",
				"version": "1.0.0",
			},
		},
	}

	response, err := m.sendMCPRequest(ctx, initReq)
	if err != nil {
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	// Проверяем успешную инициализацию
	if response.Error != nil {
		return fmt.Errorf("MCP initialization error: %s", response.Error.Message)
	}

	log.Printf("✅ Successfully connected to Notion MCP server")
	return nil
}

// Close закрывает соединение с MCP сервером
func (m *MCPClient) Close() error {
	// HTTP соединения закрываются автоматически
	return nil
}

// CreateDialogSummary создает страницу с сохранением диалога через официальный MCP
func (m *MCPClient) CreateDialogSummary(ctx context.Context, title, content, userID, username, dialogType string) MCPResult {
	log.Printf("📝 Creating Notion page via MCP: %s", title)

	// Получаем список доступных инструментов
	toolsReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}

	toolsResponse, err := m.sendMCPRequest(ctx, toolsReq)
	if err != nil {
		log.Printf("❌ Failed to get tools list: %v", err)
		return MCPResult{Success: false, Message: fmt.Sprintf("Failed to get tools: %v", err)}
	}

	log.Printf("📋 Available tools: %+v", toolsResponse.Result)

	// Вызываем инструмент создания страницы
	createReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "create_page",
			"arguments": map[string]interface{}{
				"title":   title,
				"content": formatDialogContent(title, content, userID, username, dialogType),
				"properties": map[string]interface{}{
					"Type":       "Dialog",
					"User":       username,
					"UserID":     userID,
					"Created":    time.Now().Format("2006-01-02"),
					"DialogType": dialogType,
				},
			},
		},
	}

	response, err := m.sendMCPRequest(ctx, createReq)
	if err != nil {
		log.Printf("❌ MCP create_page error: %v", err)
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if response.Error != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP tool error: %s", response.Error.Message)}
	}

	return m.parseToolResult(response.Result, "Dialog saved successfully to Notion")
}

// SearchDialogSummaries ищет сохраненные диалоги через официальный MCP
func (m *MCPClient) SearchDialogSummaries(ctx context.Context, query, userID, dialogType string) MCPResult {
	log.Printf("🔍 Searching Notion via MCP: query='%s'", query)

	searchReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "search",
			"arguments": map[string]interface{}{
				"query": query,
				"filter": map[string]interface{}{
					"property": "Type",
					"select": map[string]interface{}{
						"equals": "Dialog",
					},
				},
				"page_size": 20,
			},
		},
	}

	response, err := m.sendMCPRequest(ctx, searchReq)
	if err != nil {
		log.Printf("❌ MCP search error: %v", err)
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %v", err)}
	}

	if response.Error != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %s", response.Error.Message)}
	}

	return m.parseSearchResult(response.Result, query)
}

// CreateFreeFormPage создает произвольную страницу через официальный MCP
func (m *MCPClient) CreateFreeFormPage(ctx context.Context, title, content, parentPageName string, tags []string) MCPResult {
	log.Printf("📄 Creating free-form page via MCP: %s", title)

	toolParams := map[string]interface{}{
		"title":   title,
		"content": content,
		"properties": map[string]interface{}{
			"Type":    "Free-form",
			"Created": time.Now().Format("2006-01-02"),
		},
	}

	// Добавляем parent если указан
	if parentPageName != "" {
		toolParams["parent"] = map[string]interface{}{
			"type":      "page_name",
			"page_name": parentPageName,
		}
	}

	// Добавляем теги если указаны
	if len(tags) > 0 {
		toolParams["properties"].(map[string]interface{})["Tags"] = tags
	}

	createReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "create_page",
			"arguments": toolParams,
		},
	}

	response, err := m.sendMCPRequest(ctx, createReq)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if response.Error != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP tool error: %s", response.Error.Message)}
	}

	return m.parseToolResult(response.Result, "Free-form page created successfully")
}

// SearchWorkspace выполняет поиск по workspace через официальный MCP
func (m *MCPClient) SearchWorkspace(ctx context.Context, query, pageType string, tags []string) MCPResult {
	toolParams := map[string]interface{}{
		"query":     query,
		"page_size": 50,
	}

	// Добавляем фильтр по типу если указан
	if pageType != "" {
		toolParams["filter"] = map[string]interface{}{
			"property": "Type",
			"select": map[string]interface{}{
				"equals": pageType,
			},
		}
	}

	searchReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "search",
			"arguments": toolParams,
		},
	}

	response, err := m.sendMCPRequest(ctx, searchReq)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %v", err)}
	}

	if response.Error != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %s", response.Error.Message)}
	}

	return m.parseSearchResult(response.Result, query)
}

// sendMCPRequest отправляет JSON-RPC запрос к MCP серверу
func (m *MCPClient) sendMCPRequest(ctx context.Context, req MCPRequest) (*MCPResponse, error) {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("🔄 MCP Request: %s", string(reqJSON))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", m.baseURL, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("🔄 MCP Response: %s", string(respBody))

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &mcpResp, nil
}

// formatDialogContent форматирует содержимое диалога для Notion
func formatDialogContent(title, content, userID, username, dialogType string) string {
	return fmt.Sprintf(`# %s

**Пользователь:** %s (%s)  
**Тип:** %s  
**Создано:** %s

---

%s`, title, username, userID, dialogType, time.Now().Format("2006-01-02 15:04:05"), content)
}

// parseToolResult парсит результат вызова MCP инструмента
func (m *MCPClient) parseToolResult(result interface{}, successMessage string) MCPResult {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("Failed to parse result: %v", err)}
	}

	var parsedResult map[string]interface{}
	if err := json.Unmarshal(resultJSON, &parsedResult); err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("Failed to unmarshal result: %v", err)}
	}

	// Извлекаем информацию о странице
	var pageID, pageURL string
	if content, ok := parsedResult["content"].([]interface{}); ok && len(content) > 0 {
		if textContent, ok := content[0].(map[string]interface{}); ok {
			if text, ok := textContent["text"].(string); ok {
				successMessage = text
			}
		}
	}

	if id, ok := parsedResult["id"].(string); ok {
		pageID = id
	}
	if url, ok := parsedResult["url"].(string); ok {
		pageURL = url
	}

	// Создаем данные для ответа
	data := map[string]interface{}{
		"page_id": pageID,
		"url":     pageURL,
		"result":  parsedResult,
	}
	dataJSON, _ := json.Marshal(data)

	return MCPResult{
		Success: true,
		Message: successMessage,
		PageID:  pageID,
		Data:    string(dataJSON),
	}
}

// parseSearchResult парсит результат поиска
func (m *MCPClient) parseSearchResult(result interface{}, query string) MCPResult {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("Failed to parse search result: %v", err)}
	}

	var searchResult map[string]interface{}
	if err := json.Unmarshal(resultJSON, &searchResult); err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("Failed to unmarshal search result: %v", err)}
	}

	// Попробуем извлечь результаты из content
	var message string
	if content, ok := searchResult["content"].([]interface{}); ok && len(content) > 0 {
		if textContent, ok := content[0].(map[string]interface{}); ok {
			if text, ok := textContent["text"].(string); ok {
				message = text
			}
		}
	}

	if message == "" {
		message = fmt.Sprintf("Поиск выполнен для запроса '%s'", query)
	}

	return MCPResult{
		Success: true,
		Message: message,
		Data:    string(resultJSON),
	}
}

// MCP структуры для JSON-RPC протокола

// MCPRequest представляет JSON-RPC запрос к MCP серверу
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// MCPResponse представляет JSON-RPC ответ от MCP сервера
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError представляет ошибку в MCP протоколе
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPResult представляет результат MCP вызова
type MCPResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
	PageID  string `json:"page_id,omitempty"`
}

// MCPCreatePagesResult результат создания страниц через MCP
type MCPCreatePagesResult struct {
	Pages []MCPPageInfo `json:"pages"`
}

// MCPPageInfo информация о созданной странице
type MCPPageInfo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// MCPSearchResult результат поиска через MCP
type MCPSearchResult struct {
	Results []MCPSearchItem `json:"results"`
	Type    string          `json:"type"`
}

// MCPSearchItem элемент результата поиска
type MCPSearchItem struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Type      string `json:"type"`
	Highlight string `json:"highlight"`
	Timestamp string `json:"timestamp"`
	ID        string `json:"id"`
}
