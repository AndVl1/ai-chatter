package main

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

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreatePageParams параметры для создания страницы в Notion
type CreatePageParams struct {
	Title        string                 `json:"title" mcp:"the title of the page to create"`
	Content      string                 `json:"content" mcp:"the content of the page in markdown format"`
	Properties   map[string]interface{} `json:"properties,omitempty" mcp:"page properties (Type, User, etc.)"`
	ParentPageID string                 `json:"parent_page_id" mcp:"parent page ID (required - get from Notion workspace)"`
}

// SaveDialogParams параметры для сохранения диалога
type SaveDialogParams struct {
	Title        string `json:"title" mcp:"the title for the dialog summary"`
	Content      string `json:"content" mcp:"the dialog content to save"`
	UserID       string `json:"user_id" mcp:"ID of the user"`
	Username     string `json:"username" mcp:"username of the user"`
	DialogType   string `json:"dialog_type,omitempty" mcp:"Type of dialog (e.g., 'support', 'chat')"`
	ParentPageID string `json:"parent_page_id" mcp:"parent page ID (required - get from Notion workspace)"`
}

// SearchParams параметры для поиска в Notion
type SearchParams struct {
	Query    string                 `json:"query" mcp:"search query to find pages"`
	Filter   map[string]interface{} `json:"filter,omitempty" mcp:"optional filter for search"`
	PageSize int                    `json:"page_size,omitempty" mcp:"number of results to return (default: 20)"`
}

// SearchPagesParams параметры для поиска страниц с возвратом ID
type SearchPagesParams struct {
	Query      string `json:"query" mcp:"search query to find pages by title"`
	Limit      int    `json:"limit,omitempty" mcp:"maximum number of results to return (default: 10, max: 50)"`
	ExactMatch bool   `json:"exact_match,omitempty" mcp:"if true, only return exact title matches"`
}

// PageSearchResult результат поиска страницы
type PageSearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ListPagesParams параметры для получения списка доступных страниц
type ListPagesParams struct {
	Limit      int    `json:"limit,omitempty" mcp:"maximum number of pages to return (default: 20, max: 100)"`
	PageType   string `json:"page_type,omitempty" mcp:"filter by page type (optional)"`
	ParentOnly bool   `json:"parent_only,omitempty" mcp:"if true, return only pages that can be parents (default: false)"`
}

// AvailablePageResult информация о доступной странице
type AvailablePageResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	CanBeParent bool   `json:"can_be_parent"`
	Type        string `json:"type,omitempty"`
}

// NotionMCPServer кастомный MCP сервер для Notion
type NotionMCPServer struct {
	notionClient *NotionAPIClient
}

// NotionAPIClient клиент для прямой работы с Notion REST API
type NotionAPIClient struct {
	token      string
	baseURL    string
	apiVersion string
	httpClient *http.Client
}

// NewNotionAPIClient создает новый клиент Notion API
func NewNotionAPIClient(token string) *NotionAPIClient {
	return &NotionAPIClient{
		token:      token,
		baseURL:    "https://api.notion.com/v1",
		apiVersion: "2022-06-28",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// doNotionRequest выполняет HTTP запрос к Notion API
func (c *NotionAPIClient) doNotionRequest(ctx context.Context, method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", c.apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Notion API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// createPage создает страницу в Notion
func (c *NotionAPIClient) createPage(ctx context.Context, title, content, parentPageID string, properties map[string]interface{}) (string, error) {
	// Создание страницы согласно Notion API
	pageData := map[string]interface{}{
		"parent": map[string]interface{}{
			"type":    "page_id",
			"page_id": parentPageID,
		},
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"title": []map[string]interface{}{
					{
						"text": map[string]interface{}{
							"content": title,
						},
					},
				},
			},
		},
		"children": []map[string]interface{}{
			{
				"object": "block",
				"type":   "paragraph",
				"paragraph": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{
							"type": "text",
							"text": map[string]interface{}{
								"content": content,
							},
						},
					},
				},
			},
		},
	}

	respBody, err := c.doNotionRequest(ctx, "POST", "/pages", pageData)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if id, ok := result["id"].(string); ok {
		return id, nil
	}

	return "", fmt.Errorf("no page ID in response")
}

// searchPages ищет страницы в Notion
func (c *NotionAPIClient) searchPages(ctx context.Context, query string) ([]map[string]interface{}, error) {
	searchData := map[string]interface{}{
		"query":     query,
		"page_size": 20,
	}

	respBody, err := c.doNotionRequest(ctx, "POST", "/search", searchData)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if results, ok := result["results"].([]interface{}); ok {
		var pages []map[string]interface{}
		for _, r := range results {
			if page, ok := r.(map[string]interface{}); ok {
				pages = append(pages, page)
			}
		}
		return pages, nil
	}

	return nil, fmt.Errorf("no results in response")
}

// NewNotionMCPServer создает новый MCP сервер для Notion
func NewNotionMCPServer(notionToken string) *NotionMCPServer {
	return &NotionMCPServer{
		notionClient: NewNotionAPIClient(notionToken),
	}
}

// CreatePage создает новую страницу в Notion через MCP
func (s *NotionMCPServer) CreatePage(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[CreatePageParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📝 MCP Server: Creating Notion page '%s' in parent %s", args.Title, args.ParentPageID)

	// Проверяем обязательный parent_page_id
	if args.ParentPageID == "" {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ parent_page_id is required - get it from your Notion workspace"},
			},
		}, nil
	}

	// Создаем страницу через прямой API вызов
	pageID, err := s.notionClient.createPage(ctx, args.Title, args.Content, args.ParentPageID, args.Properties)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to create page: %v", err)},
			},
		}, nil
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("✅ Successfully created page '%s' in Notion", args.Title)},
		},
		Meta: map[string]interface{}{
			"page_id": pageID,
			"title":   args.Title,
			"success": true,
		},
	}, nil
}

// SearchPages ищет страницы в Notion через MCP
func (s *NotionMCPServer) SearchPages(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("🔍 MCP Server: Searching Notion for '%s'", args.Query)

	// Ищем страницы через прямой API вызов
	pages, err := s.notionClient.searchPages(ctx, args.Query)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Search failed: %v", err)},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Found %d pages for query '%s'", len(pages), args.Query)
	for i, page := range pages {
		if i >= 5 { // Показываем только первые 5 результатов
			break
		}
		if title, ok := page["properties"].(map[string]interface{}); ok {
			if titleProp, ok := title["title"].(map[string]interface{}); ok {
				if titleArray, ok := titleProp["title"].([]interface{}); ok && len(titleArray) > 0 {
					if titleText, ok := titleArray[0].(map[string]interface{}); ok {
						if text, ok := titleText["text"].(map[string]interface{}); ok {
							if content, ok := text["content"].(string); ok {
								resultMessage += fmt.Sprintf("\n- %s", content)
							}
						}
					}
				}
			}
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"query":      args.Query,
			"page_count": len(pages),
			"success":    true,
		},
	}, nil
}

// SearchPagesWithID ищет страницы в Notion и возвращает ID, название и URL
func (s *NotionMCPServer) SearchPagesWithID(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchPagesParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("🔍 MCP Server: Searching pages with ID for query '%s'", args.Query)

	// Устанавливаем лимит по умолчанию
	limit := args.Limit
	if limit <= 0 {
		limit = 5 // Уменьшили с 10 до 5 для предотвращения длинных ответов
	}
	if limit > 20 { // Уменьшили с 50 до 20
		limit = 20
	}

	// Ищем страницы через прямой API вызов
	pages, err := s.notionClient.searchPages(ctx, args.Query)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Search failed: %v", err)},
			},
		}, nil
	}

	var results []PageSearchResult

	for _, page := range pages {
		if len(results) >= limit {
			break
		}

		// Извлекаем ID страницы
		pageID, hasID := page["id"].(string)
		if !hasID {
			continue
		}

		// Извлекаем URL страницы
		pageURL, _ := page["url"].(string)

		// Извлекаем название страницы
		title := "Untitled"
		if properties, ok := page["properties"].(map[string]interface{}); ok {
			if titleProp, ok := properties["title"].(map[string]interface{}); ok {
				if titleArray, ok := titleProp["title"].([]interface{}); ok && len(titleArray) > 0 {
					if titleText, ok := titleArray[0].(map[string]interface{}); ok {
						if text, ok := titleText["text"].(map[string]interface{}); ok {
							if content, ok := text["content"].(string); ok {
								title = content
							}
						}
					}
				}
			}
		}

		// Если запрос точного совпадения, проверяем
		if args.ExactMatch {
			if title != args.Query {
				continue
			}
		}

		results = append(results, PageSearchResult{
			ID:    pageID,
			Title: title,
			URL:   pageURL,
		})
	}

	// Формируем ответ
	var resultMessage string
	if len(results) == 0 {
		resultMessage = fmt.Sprintf("🔍 No pages found for query '%s'", args.Query)
	} else {
		resultMessage = fmt.Sprintf("🔍 Found %d pages for query '%s':", len(results), args.Query)
		for i, result := range results {
			resultMessage += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, result.Title, result.ID)
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"query":       args.Query,
			"results":     results,
			"total_found": len(results),
			"exact_match": args.ExactMatch,
			"success":     true,
		},
	}, nil
}

// ListAvailablePages возвращает список доступных страниц для создания подстраниц
func (s *NotionMCPServer) ListAvailablePages(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[ListPagesParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📋 MCP Server: Listing available pages (limit: %d, parent_only: %t)", args.Limit, args.ParentOnly)

	// Устанавливаем лимит по умолчанию
	limit := args.Limit
	if limit <= 0 {
		limit = 10 // Уменьшили с 20 до 10 для предотвращения длинных ответов
	}
	if limit > 25 { // Уменьшили с 100 до 25
		limit = 25
	}

	// Получаем страницы через поиск (пустой запрос вернёт все доступные)
	pages, err := s.notionClient.searchPages(ctx, "")
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to get available pages: %v", err)},
			},
		}, nil
	}

	var results []AvailablePageResult

	for _, page := range pages {
		if len(results) >= limit {
			break
		}

		// Извлекаем ID страницы
		pageID, hasID := page["id"].(string)
		if !hasID {
			continue
		}

		// Извлекаем URL страницы
		pageURL, _ := page["url"].(string)

		// Извлекаем название страницы
		title := "Untitled"
		if properties, ok := page["properties"].(map[string]interface{}); ok {
			if titleProp, ok := properties["title"].(map[string]interface{}); ok {
				if titleArray, ok := titleProp["title"].([]interface{}); ok && len(titleArray) > 0 {
					if titleText, ok := titleArray[0].(map[string]interface{}); ok {
						if text, ok := titleText["text"].(map[string]interface{}); ok {
							if content, ok := text["content"].(string); ok {
								title = content
							}
						}
					}
				}
			}
		}

		// Определяем может ли страница быть родителем (все страницы в Notion могут быть родителями)
		canBeParent := true

		// Извлекаем тип страницы (если есть)
		pageType := "page"
		if properties, ok := page["properties"].(map[string]interface{}); ok {
			if typeProp, ok := properties["Type"].(map[string]interface{}); ok {
				if typeSelect, ok := typeProp["select"].(map[string]interface{}); ok {
					if typeName, ok := typeSelect["name"].(string); ok {
						pageType = typeName
					}
				}
			}
		}

		// Фильтрация по типу страницы
		if args.PageType != "" && pageType != args.PageType {
			continue
		}

		// Фильтрация по возможности быть родителем
		if args.ParentOnly && !canBeParent {
			continue
		}

		results = append(results, AvailablePageResult{
			ID:          pageID,
			Title:       title,
			URL:         pageURL,
			CanBeParent: canBeParent,
			Type:        pageType,
		})
	}

	// Формируем ответ
	var resultMessage string
	if len(results) == 0 {
		resultMessage = "📋 No available pages found"
	} else {
		resultMessage = fmt.Sprintf("📋 Found %d available pages:", len(results))
		for i, result := range results {
			resultMessage += fmt.Sprintf("\n%d. %s (ID: %s)", i+1, result.Title, result.ID)
			if result.CanBeParent {
				resultMessage += " ✅"
			}
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"pages":       results,
			"total_found": len(results),
			"limit":       limit,
			"parent_only": args.ParentOnly,
			"page_type":   args.PageType,
			"success":     true,
		},
	}, nil
}

// SaveDialog сохраняет диалог в Notion через MCP
func (s *NotionMCPServer) SaveDialog(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[SaveDialogParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("💾 MCP Server: Saving dialog '%s' for user %s in parent %s", args.Title, args.Username, args.ParentPageID)

	// Проверяем обязательный parent_page_id
	if args.ParentPageID == "" {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "❌ parent_page_id is required for saving dialogs - get it from your Notion workspace"},
			},
		}, nil
	}

	// Формируем контент диалога
	dialogContent := fmt.Sprintf("# %s\n\n**User:** %s\n**Type:** %s\n**Date:** %s\n\n## Content\n\n%s",
		args.Title, args.Username, args.DialogType, time.Now().Format("2006-01-02 15:04:05"), args.Content)

	// Создаем свойства для страницы
	properties := map[string]interface{}{
		"Type":       "Dialog",
		"User":       args.Username,
		"UserID":     args.UserID,
		"DialogType": args.DialogType,
		"Created":    time.Now().Format("2006-01-02"),
	}

	// Сохраняем диалог как страницу
	pageID, err := s.notionClient.createPage(ctx, args.Title, dialogContent, args.ParentPageID, properties)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to save dialog: %v", err)},
			},
		}, nil
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("✅ Dialog '%s' saved to Notion", args.Title)},
		},
		Meta: map[string]interface{}{
			"page_id":     pageID,
			"title":       args.Title,
			"user":        args.Username,
			"dialog_type": args.DialogType,
			"success":     true,
		},
	}, nil
}

// getProperty извлекает свойство из карты с fallback значением
func getProperty(props map[string]interface{}, key, defaultValue string) string {
	if props == nil {
		return defaultValue
	}
	if val, ok := props[key].(string); ok {
		return val
	}
	return defaultValue
}

func main() {
	if err := godotenv.Load(".env" /*, "../.env", "cmd/bot/.env"*/); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}
	// Получаем токен Notion из переменной окружения
	notionToken := os.Getenv("NOTION_TOKEN")
	if notionToken == "" {
		log.Fatal("❌ NOTION_TOKEN environment variable is required")
	}

	log.Printf("🚀 Starting Custom Notion MCP Server")
	log.Printf("🔑 Using Notion token: %s...%s", notionToken[:10], notionToken[len(notionToken)-5:])

	// Создаем MCP сервер
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-chatter-notion-mcp",
		Version: "1.0.0",
	}, nil)

	// Создаем наш Notion сервер
	notionServer := NewNotionMCPServer(notionToken)

	// Регистрируем инструменты
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_page",
		Description: "Creates a new page in Notion with the specified title and content",
	}, notionServer.CreatePage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_pages",
		Description: "Searches for pages in Notion workspace",
	}, notionServer.SearchPages)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_dialog_to_notion",
		Description: "Saves a dialog conversation to Notion as a summary page",
	}, notionServer.SaveDialog)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_pages_with_id",
		Description: "Searches for pages in Notion and returns their ID, title, and URL",
	}, notionServer.SearchPagesWithID)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_available_pages",
		Description: "Lists available pages in Notion workspace that can be used as parent pages",
	}, notionServer.ListAvailablePages)

	log.Printf("📋 Registered %d tools: create_page, search_pages, save_dialog_to_notion, search_pages_with_id, list_available_pages", 5)
	log.Printf("🔗 Starting server on stdin/stdout...")

	// Запускаем сервер через stdin/stdout
	transport := mcp.NewStdioTransport()
	if err := server.Run(context.Background(), transport); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}
