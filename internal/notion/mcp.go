package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient клиент для работы с кастомным Notion MCP сервером
type MCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewMCPClient создает новый MCP клиент для Notion
func NewMCPClient(token string) *MCPClient {
	return &MCPClient{}
}

// Connect подключается к кастомному Notion MCP серверу через stdio
func (m *MCPClient) Connect(ctx context.Context, notionToken string) error {
	log.Printf("🔗 Connecting to custom Notion MCP server via stdio")

	// Создаем MCP клиент
	m.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot",
		Version: "1.0.0",
	}, nil)

	// Запускаем наш кастомный MCP сервер как подпроцесс
	serverPath := "./notion-mcp-server"
	if customPath := os.Getenv("NOTION_MCP_SERVER_PATH"); customPath != "" {
		serverPath = customPath
	}

	cmd := exec.CommandContext(ctx, serverPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("NOTION_TOKEN=%s", notionToken))

	transport := mcp.NewCommandTransport(cmd)

	session, err := m.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to custom MCP server: %w", err)
	}

	m.session = session
	log.Printf("✅ Connected to custom Notion MCP server")
	return nil
}

// Close закрывает соединение с MCP сервером
func (m *MCPClient) Close() error {
	if m.session != nil {
		return m.session.Close()
	}
	return nil
}

// CreateDialogSummary создает страницу с сохранением диалога через кастомный MCP
func (m *MCPClient) CreateDialogSummary(ctx context.Context, title, content, userID, username, dialogType, parentPageID string) MCPResult {
	if m.session == nil {
		return MCPResult{Success: false, Message: "MCP session not connected"}
	}

	log.Printf("📝 Creating Notion page via custom MCP: %s", title)

	// Проверяем обязательный parent_page_id
	if parentPageID == "" {
		return MCPResult{Success: false, Message: "parent_page_id is required - get it from your Notion workspace"}
	}

	// Вызываем инструмент save_dialog_to_notion
	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "save_dialog_to_notion",
		Arguments: map[string]any{
			"title":          title,
			"content":        content,
			"user_id":        userID,
			"username":       username,
			"dialog_type":    dialogType,
			"parent_page_id": parentPageID,
		},
	})

	if err != nil {
		log.Printf("❌ MCP save_dialog error: %v", err)
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return MCPResult{Success: false, Message: "Tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	var pageID string
	if result.Meta != nil {
		if id, ok := result.Meta["page_id"].(string); ok {
			pageID = id
		}
	}

	return MCPResult{
		Success: true,
		Message: responseText,
		PageID:  pageID,
		Data:    formatResultMeta(result.Meta),
	}
}

// SearchDialogSummaries ищет сохраненные диалоги через кастомный MCP
func (m *MCPClient) SearchDialogSummaries(ctx context.Context, query, userID, dialogType string) MCPResult {
	if m.session == nil {
		return MCPResult{Success: false, Message: "MCP session not connected"}
	}

	log.Printf("🔍 Searching Notion via custom MCP: query='%s'", query)

	// Вызываем инструмент search_pages
	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_pages",
		Arguments: map[string]any{
			"query": query,
			"filter": map[string]any{
				"property": "Type",
				"select": map[string]any{
					"equals": "Dialog",
				},
			},
			"page_size": 20,
		},
	})

	if err != nil {
		log.Printf("❌ MCP search error: %v", err)
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %v", err)}
	}

	if result.IsError {
		return MCPResult{Success: false, Message: "Tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return MCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// CreateFreeFormPage создает произвольную страницу через кастомный MCP
func (m *MCPClient) CreateFreeFormPage(ctx context.Context, title, content, parentPageId string, tags []string) MCPResult {
	if m.session == nil {
		return MCPResult{Success: false, Message: "MCP session not connected"}
	}

	log.Printf("📄 Creating free-form page via custom MCP: %s", title)

	// Вызываем инструмент create_page
	args := map[string]any{
		"title":   title,
		"content": content,
		"properties": map[string]any{
			"Type":    "Free-form",
			"Created": time.Now().Format("2006-01-02"),
		},
	}

	if parentPageId == "" {
		return MCPResult{Success: false, Message: "parent_page_id is required - get it from your Notion workspace"}
	}

	args["parent_page_id"] = parentPageId

	if len(tags) > 0 {
		args["properties"].(map[string]any)["Tags"] = tags
	}

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_page",
		Arguments: args,
	})

	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP error: %v", err)}
	}

	if result.IsError {
		return MCPResult{Success: false, Message: fmt.Sprintf("Tool returned error: %v", result.Content)}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	var pageID string
	if result.Meta != nil {
		if id, ok := result.Meta["page_id"].(string); ok {
			pageID = id
		}
	}

	return MCPResult{
		Success: true,
		Message: responseText,
		PageID:  strings.Replace(pageID, "-", "", -1),
		Data:    formatResultMeta(result.Meta),
	}
}

// SearchWorkspace выполняет поиск по workspace через кастомный MCP
func (m *MCPClient) SearchWorkspace(ctx context.Context, query, pageType string, tags []string) MCPResult {
	if m.session == nil {
		return MCPResult{Success: false, Message: "MCP session not connected"}
	}

	args := map[string]any{
		"query":     query,
		"page_size": 50,
	}

	// Добавляем фильтр по типу если указан
	if pageType != "" {
		args["filter"] = map[string]any{
			"property": "Type",
			"select": map[string]any{
				"equals": pageType,
			},
		}
	}

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_pages",
		Arguments: args,
	})

	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("MCP search error: %v", err)}
	}

	if result.IsError {
		return MCPResult{Success: false, Message: "Tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return MCPResult{
		Success: true,
		Message: responseText,
		Data:    formatResultMeta(result.Meta),
	}
}

// SearchPagesWithID ищет страницы в Notion и возвращает их ID, название и URL
func (m *MCPClient) SearchPagesWithID(ctx context.Context, query string, limit int, exactMatch bool) MCPPageSearchResult {
	if m.session == nil {
		return MCPPageSearchResult{Success: false, Message: "MCP session not connected"}
	}

	args := map[string]any{
		"query": query,
	}

	if limit > 0 {
		args["limit"] = limit
	}

	if exactMatch {
		args["exact_match"] = exactMatch
	}

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_pages_with_id",
		Arguments: args,
	})

	if err != nil {
		return MCPPageSearchResult{Success: false, Message: fmt.Sprintf("MCP search error: %v", err)}
	}

	if result.IsError {
		return MCPPageSearchResult{Success: false, Message: "Tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные с результатами
	var pages []MCPPageResult
	var totalFound int

	if result.Meta != nil {
		// Извлекаем total_found
		if count, ok := result.Meta["total_found"].(float64); ok {
			totalFound = int(count)
		}

		// Извлекаем результаты
		if resultsData, ok := result.Meta["results"].([]any); ok {
			for _, item := range resultsData {
				if pageData, ok := item.(map[string]any); ok {
					page := MCPPageResult{}
					if id, ok := pageData["id"].(string); ok {
						page.ID = id
					}
					if title, ok := pageData["title"].(string); ok {
						page.Title = title
					}
					if url, ok := pageData["url"].(string); ok {
						page.URL = url
					}
					pages = append(pages, page)
				}
			}
		}
	}

	return MCPPageSearchResult{
		Success:    true,
		Message:    responseText,
		Pages:      pages,
		TotalFound: totalFound,
	}
}

// ListAvailablePages получает список доступных страниц в Notion workspace
func (m *MCPClient) ListAvailablePages(ctx context.Context, limit int, pageType string, parentOnly bool) MCPAvailablePagesResult {
	if m.session == nil {
		return MCPAvailablePagesResult{Success: false, Message: "MCP session not connected"}
	}

	args := map[string]any{}

	if limit > 0 {
		args["limit"] = limit
	}

	if pageType != "" {
		args["page_type"] = pageType
	}

	if parentOnly {
		args["parent_only"] = parentOnly
	}

	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_available_pages",
		Arguments: args,
	})

	if err != nil {
		return MCPAvailablePagesResult{Success: false, Message: fmt.Sprintf("MCP list pages error: %v", err)}
	}

	if result.IsError {
		return MCPAvailablePagesResult{Success: false, Message: "Tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные с результатами
	var pages []MCPAvailablePageResult
	var totalFound int

	if result.Meta != nil {
		// Извлекаем total_found
		if count, ok := result.Meta["total_found"].(float64); ok {
			totalFound = int(count)
		}

		// Извлекаем результаты
		if pagesData, ok := result.Meta["pages"].([]any); ok {
			for _, item := range pagesData {
				if pageData, ok := item.(map[string]any); ok {
					page := MCPAvailablePageResult{}
					if id, ok := pageData["id"].(string); ok {
						page.ID = id
					}
					if title, ok := pageData["title"].(string); ok {
						page.Title = title
					}
					if url, ok := pageData["url"].(string); ok {
						page.URL = url
					}
					if canBeParent, ok := pageData["can_be_parent"].(bool); ok {
						page.CanBeParent = canBeParent
					}
					if pageType, ok := pageData["type"].(string); ok {
						page.Type = pageType
					}
					pages = append(pages, page)
				}
			}
		}
	}

	return MCPAvailablePagesResult{
		Success:    true,
		Message:    responseText,
		Pages:      pages,
		TotalFound: totalFound,
	}
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

// MCPPageSearchResult результат поиска страниц с ID
type MCPPageSearchResult struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	Pages      []MCPPageResult `json:"pages"`
	TotalFound int             `json:"total_found"`
}

// MCPPageResult информация о найденной странице
type MCPPageResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// MCPAvailablePagesResult результат получения списка доступных страниц
type MCPAvailablePagesResult struct {
	Success    bool                     `json:"success"`
	Message    string                   `json:"message"`
	Pages      []MCPAvailablePageResult `json:"pages"`
	TotalFound int                      `json:"total_found"`
}

// MCPAvailablePageResult информация о доступной странице
type MCPAvailablePageResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	CanBeParent bool   `json:"can_be_parent"`
	Type        string `json:"type,omitempty"`
}
