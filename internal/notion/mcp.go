package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// MCPClient клиент для работы с Notion (улучшенная версия)
type MCPClient struct {
	token      string
	httpClient *http.Client
}

// NewMCPClient создает новый MCP клиент для Notion
func NewMCPClient(token string) *MCPClient {
	return &MCPClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect - заглушка для совместимости (в реальной реализации соединение не требуется)
func (m *MCPClient) Connect(ctx context.Context, notionToken string) error {
	if notionToken == "" {
		return fmt.Errorf("notion token is empty")
	}
	m.token = notionToken
	log.Printf("✅ Notion client initialized with REST API")
	return nil
}

// Close - заглушка для совместимости
func (m *MCPClient) Close() error {
	return nil
}

// CreateDialogSummary создает страницу с сохранением диалога
func (m *MCPClient) CreateDialogSummary(ctx context.Context, title, content, userID, username, dialogType string) MCPResult {
	log.Printf("📝 Creating Notion page: %s", title)

	// Ищем подходящую родительскую страницу
	parentPageID := m.findParentPageID(ctx, "AI Chatter")

	var parent map[string]interface{}
	if parentPageID != "" {
		parent = map[string]interface{}{"page_id": parentPageID}
		log.Printf("🔗 Creating page under parent: %s", parentPageID)
	} else {
		// Создаем на уровне workspace, если родитель не найден
		parent = map[string]interface{}{"workspace": true}
		log.Printf("📄 Creating workspace-level page")
	}

	// Формируем содержимое с метаданными
	formattedContent := fmt.Sprintf(`# %s

**Пользователь:** %s (%s)  
**Тип:** %s  
**Создано:** %s

---

%s`, title, username, userID, dialogType, time.Now().Format("2006-01-02 15:04:05"), content)

	// Формируем блоки для Notion API
	reqBody := map[string]interface{}{
		"parent": parent,
		"properties": map[string]interface{}{
			"title": []map[string]interface{}{
				{"type": "text", "text": map[string]interface{}{"content": title}},
			},
		},
		"children": m.createNotionBlocks(formattedContent),
	}

	payload, _ := json.Marshal(reqBody)
	respBody, status, err := m.doNotionRequest(ctx, http.MethodPost, "/pages", payload)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("create page error: %v", err)}
	}
	if status < 200 || status >= 300 {
		return MCPResult{Success: false, Message: fmt.Sprintf("create page bad status: %d, body: %s", status, string(respBody))}
	}

	var created struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("decode response error: %v", err)}
	}

	data := map[string]interface{}{"page_id": created.ID, "title": title, "type": dialogType, "url": created.URL}
	dataJSON, _ := json.Marshal(data)
	return MCPResult{Success: true, Message: fmt.Sprintf("Successfully created page: %s", title), PageID: created.ID, Data: string(dataJSON)}
}

// SearchDialogSummaries ищет сохраненные диалоги
func (m *MCPClient) SearchDialogSummaries(ctx context.Context, query, userID, dialogType string) MCPResult {
	log.Printf("🔍 Searching Notion: query='%s', user='%s', type='%s'", query, userID, dialogType)

	payload := map[string]interface{}{
		"query": query,
		"filter": map[string]interface{}{
			"value":    "page",
			"property": "object",
		},
		"page_size": 20,
	}
	body, _ := json.Marshal(payload)
	respBody, status, err := m.doNotionRequest(ctx, http.MethodPost, "/search", body)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("search error: %v", err)}
	}
	if status < 200 || status >= 300 {
		return MCPResult{Success: false, Message: fmt.Sprintf("search bad status: %d, body: %s", status, string(respBody))}
	}

	// Парсим результаты поиска
	var searchResult struct {
		Results []struct {
			ID         string                 `json:"id"`
			URL        string                 `json:"url"`
			Properties map[string]interface{} `json:"properties"`
		} `json:"results"`
	}

	if err := json.Unmarshal(respBody, &searchResult); err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("failed to parse search results: %v", err)}
	}

	// Форматируем результаты для пользователя
	var resultText strings.Builder
	resultText.WriteString(fmt.Sprintf("Найдено %d результатов для запроса '%s':\n\n", len(searchResult.Results), query))

	for i, result := range searchResult.Results {
		title := extractTitleFromProperties(result.Properties)
		if title == "" {
			title = "Без названия"
		}
		resultText.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n\n", i+1, title, result.URL))
	}

	return MCPResult{Success: true, Message: resultText.String(), Data: string(respBody)}
}

// CreateFreeFormPage создает произвольную страницу
func (m *MCPClient) CreateFreeFormPage(ctx context.Context, title, content, parentPageName string, tags []string) MCPResult {
	log.Printf("📄 Creating free-form Notion page: %s", title)

	var parent map[string]interface{}
	if parentPageName != "" {
		if parentID := m.findParentPageID(ctx, parentPageName); parentID != "" {
			parent = map[string]interface{}{"page_id": parentID}
			log.Printf("🔗 Will create under parent: %s (%s)", parentPageName, parentID)
		} else {
			parent = map[string]interface{}{"workspace": true}
			log.Printf("⚠️ Parent '%s' not found, creating in workspace", parentPageName)
		}
	} else {
		parent = map[string]interface{}{"workspace": true}
		log.Printf("📄 Creating workspace-level page")
	}

	reqBody := map[string]interface{}{
		"parent": parent,
		"properties": map[string]interface{}{
			"title": []map[string]interface{}{
				{"type": "text", "text": map[string]interface{}{"content": title}},
			},
		},
		"children": m.createNotionBlocks(content),
	}

	payload, _ := json.Marshal(reqBody)
	respBody, status, err := m.doNotionRequest(ctx, http.MethodPost, "/pages", payload)
	if err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("create page error: %v", err)}
	}
	if status < 200 || status >= 300 {
		return MCPResult{Success: false, Message: fmt.Sprintf("create page bad status: %d, body: %s", status, string(respBody))}
	}

	var created struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return MCPResult{Success: false, Message: fmt.Sprintf("decode response error: %v", err)}
	}

	data := map[string]interface{}{"page_id": created.ID, "title": title, "url": created.URL, "type": "free-form", "tags": tags}
	dataJSON, _ := json.Marshal(data)
	return MCPResult{Success: true, Message: fmt.Sprintf("Successfully created free-form page: %s", title), PageID: created.ID, Data: string(dataJSON)}
}

// SearchWorkspace выполняет поиск по workspace
func (m *MCPClient) SearchWorkspace(ctx context.Context, query, pageType string, tags []string) MCPResult {
	return m.SearchDialogSummaries(ctx, query, "", "")
}

// createNotionBlocks создает блоки для Notion API из markdown содержимого
func (m *MCPClient) createNotionBlocks(content string) []map[string]interface{} {
	// Разбиваем содержимое на абзацы
	paragraphs := strings.Split(content, "\n\n")
	var blocks []map[string]interface{}

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		// Обработка заголовков
		if strings.HasPrefix(paragraph, "# ") {
			blocks = append(blocks, map[string]interface{}{
				"object": "block",
				"type":   "heading_1",
				"heading_1": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{"type": "text", "text": map[string]interface{}{"content": strings.TrimPrefix(paragraph, "# ")}},
					},
				},
			})
		} else if strings.HasPrefix(paragraph, "## ") {
			blocks = append(blocks, map[string]interface{}{
				"object": "block",
				"type":   "heading_2",
				"heading_2": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{"type": "text", "text": map[string]interface{}{"content": strings.TrimPrefix(paragraph, "## ")}},
					},
				},
			})
		} else if strings.HasPrefix(paragraph, "### ") {
			blocks = append(blocks, map[string]interface{}{
				"object": "block",
				"type":   "heading_3",
				"heading_3": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{"type": "text", "text": map[string]interface{}{"content": strings.TrimPrefix(paragraph, "### ")}},
					},
				},
			})
		} else if strings.Contains(paragraph, "---") {
			// Разделитель
			blocks = append(blocks, map[string]interface{}{
				"object":  "block",
				"type":    "divider",
				"divider": map[string]interface{}{},
			})
		} else {
			// Обычный параграф
			blocks = append(blocks, map[string]interface{}{
				"object": "block",
				"type":   "paragraph",
				"paragraph": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{"type": "text", "text": map[string]interface{}{"content": paragraph}},
					},
				},
			})
		}
	}

	return blocks
}

// findParentPageID ищет ID родительской страницы по названию
func (m *MCPClient) findParentPageID(ctx context.Context, pageName string) string {
	log.Printf("🔍 Searching for parent page: %s", pageName)

	payload := map[string]interface{}{
		"query": pageName,
		"filter": map[string]interface{}{
			"value":    "page",
			"property": "object",
		},
		"page_size": 25,
	}
	body, _ := json.Marshal(payload)
	respBody, status, err := m.doNotionRequest(ctx, http.MethodPost, "/search", body)
	if err != nil || status < 200 || status >= 300 {
		log.Printf("❌ Parent page search failed: status=%d err=%v", status, err)
		return ""
	}

	var search struct {
		Results []struct {
			ID         string                 `json:"id"`
			URL        string                 `json:"url"`
			Properties map[string]interface{} `json:"properties"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &search); err != nil {
		log.Printf("❌ decode search response error: %v", err)
		return ""
	}

	lower := strings.ToLower(pageName)
	for _, result := range search.Results {
		if result.URL != "" && strings.Contains(strings.ToLower(result.URL), strings.ReplaceAll(lower, " ", "-")) {
			return result.ID
		}
		if title := extractTitleFromProperties(result.Properties); strings.EqualFold(title, pageName) {
			return result.ID
		}
	}
	return ""
}

// doNotionRequest выполняет HTTP запрос к Notion API
func (m *MCPClient) doNotionRequest(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	if m.token == "" {
		return nil, 0, fmt.Errorf("NOTION_TOKEN is empty")
	}

	const notionAPIBase = "https://api.notion.com/v1"
	const notionAPIVersion = "2022-06-28"

	req, err := http.NewRequestWithContext(ctx, method, notionAPIBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	return respBytes, resp.StatusCode, nil
}

// extractTitleFromProperties извлекает заголовок страницы из properties
func extractTitleFromProperties(props map[string]interface{}) string {
	if props == nil {
		return ""
	}
	if v, ok := props["title"]; ok {
		if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]interface{}); ok {
				if text, ok := first["text"].(map[string]interface{}); ok {
					if content, ok := text["content"].(string); ok {
						return content
					}
				}
			}
		}
	}
	return ""
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
