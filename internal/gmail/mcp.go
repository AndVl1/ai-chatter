package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GmailMCPClient клиент для работы с Gmail MCP сервером
type GmailMCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewGmailMCPClient создает новый Gmail MCP клиент
func NewGmailMCPClient() *GmailMCPClient {
	return &GmailMCPClient{}
}

// Connect подключается к Gmail MCP серверу через stdio
func (m *GmailMCPClient) Connect(ctx context.Context, gmailCredentialsJSON string) error {
	log.Printf("🔗 Connecting to Gmail MCP server via stdio")

	// Создаем MCP клиент
	m.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot-gmail",
		Version: "1.0.0",
	}, nil)

	// Запускаем Gmail MCP сервер как подпроцесс
	serverPath := "./gmail-mcp-server"
	if customPath := os.Getenv("GMAIL_MCP_SERVER_PATH"); customPath != "" {
		serverPath = customPath
	}

	cmd := exec.CommandContext(ctx, serverPath)
	// Передаем credentials напрямую в JSON формате
	cmd.Env = append(os.Environ(), fmt.Sprintf("GMAIL_CREDENTIALS_JSON=%s", gmailCredentialsJSON))

	transport := mcp.NewCommandTransport(cmd)

	session, err := m.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to Gmail MCP server: %w", err)
	}

	m.session = session
	log.Printf("✅ Connected to Gmail MCP server")
	return nil
}

// Close закрывает соединение с Gmail MCP сервером
func (m *GmailMCPClient) Close() error {
	if m.session != nil {
		return m.session.Close()
	}
	return nil
}

// SearchEmails ищет email в Gmail через MCP
func (m *GmailMCPClient) SearchEmails(ctx context.Context, query string, maxEmails int, timeRange string) GmailMCPResult {
	if m.session == nil {
		return GmailMCPResult{Success: false, Message: "Gmail MCP session not connected"}
	}

	log.Printf("📧 Searching Gmail via MCP: query='%s', max=%d, timeRange='%s'", query, maxEmails, timeRange)

	// Вызываем инструмент search_gmail
	result, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_gmail",
		Arguments: map[string]any{
			"query":      query,
			"max_emails": maxEmails,
			"time_range": timeRange,
		},
	})

	if err != nil {
		log.Printf("❌ Gmail MCP search error: %v", err)
		return GmailMCPResult{Success: false, Message: fmt.Sprintf("Gmail MCP search error: %v", err)}
	}

	if result.IsError {
		return GmailMCPResult{Success: false, Message: "Gmail search tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные с результатами
	var emails []GmailEmailResult
	var totalFound int

	if result.Meta != nil {
		// Извлекаем total_found
		if count, ok := result.Meta["total_found"].(float64); ok {
			totalFound = int(count)
		}

		// Извлекаем результаты email
		if emailsData, ok := result.Meta["emails"].([]any); ok {
			for _, item := range emailsData {
				if emailData, ok := item.(map[string]any); ok {
					email := GmailEmailResult{}
					if id, ok := emailData["id"].(string); ok {
						email.ID = id
					}
					if subject, ok := emailData["subject"].(string); ok {
						email.Subject = subject
					}
					if from, ok := emailData["from"].(string); ok {
						email.From = from
					}
					if snippet, ok := emailData["snippet"].(string); ok {
						email.Snippet = snippet
					}
					if body, ok := emailData["body"].(string); ok {
						email.Body = body
					}
					if dateStr, ok := emailData["date"].(string); ok {
						if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
							email.Date = parsedDate
						}
					}
					if isImportant, ok := emailData["is_important"].(bool); ok {
						email.IsImportant = isImportant
					}
					if isUnread, ok := emailData["is_unread"].(bool); ok {
						email.IsUnread = isUnread
					}
					emails = append(emails, email)
				}
			}
		}
	}

	return GmailMCPResult{
		Success:    true,
		Message:    responseText,
		Emails:     emails,
		TotalFound: totalFound,
	}
}

// GmailMCPResult результат Gmail MCP операции
type GmailMCPResult struct {
	Success    bool               `json:"success"`
	Message    string             `json:"message"`
	Emails     []GmailEmailResult `json:"emails"`
	TotalFound int                `json:"total_found"`
}

// GmailEmailResult информация о найденном email
type GmailEmailResult struct {
	ID          string    `json:"id"`
	Subject     string    `json:"subject"`
	From        string    `json:"from"`
	Date        time.Time `json:"date"`
	Snippet     string    `json:"snippet"`
	IsImportant bool      `json:"is_important"`
	IsUnread    bool      `json:"is_unread"`
	Body        string    `json:"body,omitempty"`
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
