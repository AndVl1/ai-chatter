package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GmailSearchParams параметры для поиска в Gmail
type GmailSearchParams struct {
	Query     string `json:"query" mcp:"Gmail search query (e.g., 'from:example@gmail.com subject:important')"`
	MaxEmails int    `json:"max_emails,omitempty" mcp:"maximum number of emails to return (default: 10, max: 50)"`
	TimeRange string `json:"time_range,omitempty" mcp:"time range filter: 'today', 'week', 'month' (default: 'today')"`
}

// GmailEmailResult результат поиска email
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

// OAuth2Credentials структура для OAuth2 credentials
type OAuth2Credentials struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
}

// GoogleCredentialsFile структура для файла credentials.json из Google Cloud Console
type GoogleCredentialsFile struct {
	Installed *OAuth2Credentials `json:"installed,omitempty"`
	Web       *OAuth2Credentials `json:"web,omitempty"`
}

// GmailMCPServer кастомный MCP сервер для Gmail
type GmailMCPServer struct {
	gmailService *gmail.Service
	config       *oauth2.Config
}

// NewGmailMCPServer создает новый MCP сервер для Gmail
func NewGmailMCPServer(credentialsJSON string) (*GmailMCPServer, error) {
	log.Printf("🔑 Initializing Gmail MCP Server with OAuth2 credentials")

	// Парсим OAuth2 credentials с поддержкой формата Google Cloud Console
	credentials, err := parseGoogleCredentials(credentialsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OAuth2 credentials: %w", err)
	}

	// Создаем OAuth2 config
	config := &oauth2.Config{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob", // для desktop приложений
		Scopes:       []string{gmail.GmailReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	// Получаем токен (с кэшированием)
	token, err := getToken(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth2 token: %w", err)
	}

	// Создаем HTTP клиент с токеном
	httpClient := config.Client(context.Background(), token)

	// Создаем Gmail service
	gmailService, err := gmail.NewService(
		context.Background(),
		option.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gmail service: %w", err)
	}

	return &GmailMCPServer{
		gmailService: gmailService,
		config:       config,
	}, nil
}

// getToken получает OAuth2 токен (с кэшированием)
func getToken(config *oauth2.Config) (*oauth2.Token, error) {
	// Путь к файлу с токеном
	tokenFile := getTokenFilePath()

	// Пытаемся загрузить существующий токен
	token, err := loadTokenFromFile(tokenFile)
	if err == nil {
		// Проверяем, что токен не истек
		if token.Valid() {
			log.Printf("✅ Using cached OAuth2 token")
			return token, nil
		}
		log.Printf("⚠️ Cached token expired, refreshing...")
	}

	// Если токена нет или он истек, запускаем OAuth flow
	log.Printf("🔄 Starting OAuth2 flow for Gmail authentication")

	// Генерируем URL для авторизации
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("🔗 Open this URL in your browser and authorize the application:")
	log.Printf("   %s", authURL)
	log.Printf("📝 Enter the authorization code: ")

	// В Docker контейнере мы не можем использовать интерактивный ввод
	// Проверяем, есть ли переменная окружения с refresh token
	if refreshToken := os.Getenv("GMAIL_REFRESH_TOKEN"); refreshToken != "" {
		log.Printf("📄 Using refresh token from environment variable")
		token = &oauth2.Token{
			RefreshToken: refreshToken,
		}

		// Обновляем токен
		tokenSource := config.TokenSource(context.Background(), token)
		newToken, err := tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}

		// Сохраняем обновленный токен
		if err := saveTokenToFile(tokenFile, newToken); err != nil {
			log.Printf("⚠️ Warning: failed to save refreshed token: %v", err)
		}

		return newToken, nil
	}

	// Если нет refresh token, пытаемся интерактивно получить код авторизации
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("failed to read authorization code (you can also set GMAIL_REFRESH_TOKEN env var): %w", err)
	}

	// Обмениваем код на токен
	token, err = config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Сохраняем токен для будущего использования
	if err := saveTokenToFile(tokenFile, token); err != nil {
		log.Printf("⚠️ Warning: failed to save token to cache: %v", err)
	}

	log.Printf("✅ Successfully obtained OAuth2 token")
	return token, nil
}

// getTokenFilePath возвращает путь к файлу с токеном
func getTokenFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "gmail-token.json"
	}
	return filepath.Join(homeDir, ".ai-chatter", "gmail-token.json")
}

// loadTokenFromFile загружает токен из файла
func loadTokenFromFile(filename string) (*oauth2.Token, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(file).Decode(token)
	return token, err
}

// saveTokenToFile сохраняет токен в файл
func saveTokenToFile(filename string, token *oauth2.Token) error {
	// Создаем директорию если не существует
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(token)
}

// parseGoogleCredentials парсит credentials JSON с поддержкой различных форматов
func parseGoogleCredentials(credentialsJSON string) (*OAuth2Credentials, error) {
	// Сначала пытаемся парсить как прямую структуру OAuth2Credentials
	var directCredentials OAuth2Credentials
	if err := json.Unmarshal([]byte(credentialsJSON), &directCredentials); err == nil {
		if directCredentials.ClientID != "" && directCredentials.ClientSecret != "" {
			log.Printf("✅ Parsed direct OAuth2 credentials format")
			return &directCredentials, nil
		}
	}

	// Если не получилось, пытаемся парсить как формат Google Cloud Console
	var googleFile GoogleCredentialsFile
	if err := json.Unmarshal([]byte(credentialsJSON), &googleFile); err != nil {
		return nil, fmt.Errorf("failed to parse credentials as Google format: %w", err)
	}

	// Проверяем installed credentials (Desktop application)
	if googleFile.Installed != nil {
		log.Printf("✅ Parsed Google Cloud Console credentials (installed/desktop format)")
		return googleFile.Installed, nil
	}

	// Проверяем web credentials (Web application)
	if googleFile.Web != nil {
		log.Printf("✅ Parsed Google Cloud Console credentials (web format)")
		return googleFile.Web, nil
	}

	return nil, fmt.Errorf("no valid credentials found in JSON - expected 'installed' or 'web' section")
}

// SearchEmails ищет email в Gmail по запросу
func (s *GmailMCPServer) SearchEmails(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GmailSearchParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📧 MCP Server: Searching Gmail for query '%s'", args.Query)

	// Устанавливаем лимит по умолчанию
	maxResults := int64(args.MaxEmails)
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 50 {
		maxResults = 50
	}

	// Добавляем временной фильтр к запросу
	query := args.Query
	if args.TimeRange != "" {
		switch args.TimeRange {
		case "today":
			query += " newer_than:1d"
		case "week":
			query += " newer_than:7d"
		case "month":
			query += " newer_than:30d"
		}
	} else {
		// По умолчанию показываем письма за последний день
		query += " newer_than:1d"
	}

	// Поиск сообщений
	listCall := s.gmailService.Users.Messages.List("me").Q(query).MaxResults(maxResults)
	messages, err := listCall.Do()
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Gmail search failed: %v", err)},
			},
		}, nil
	}

	var results []GmailEmailResult

	// Получаем детали для каждого сообщения
	for _, msg := range messages.Messages {
		if int64(len(results)) >= maxResults {
			break
		}

		// Получаем полную информацию о сообщении
		fullMsg, err := s.gmailService.Users.Messages.Get("me", msg.Id).Do()
		if err != nil {
			log.Printf("⚠️ Failed to get message details for %s: %v", msg.Id, err)
			continue
		}

		// Извлекаем данные из сообщения
		email := s.parseGmailMessage(fullMsg)
		results = append(results, email)
	}

	// Формируем ответ
	var resultMessage string
	if len(results) == 0 {
		resultMessage = fmt.Sprintf("📧 No emails found for query '%s' in the specified time range", args.Query)
	} else {
		resultMessage = fmt.Sprintf("📧 Found %d emails for query '%s':\n\n", len(results), args.Query)
		for i, email := range results {
			importance := ""
			if email.IsImportant {
				importance = " ⭐"
			}
			unread := ""
			if email.IsUnread {
				unread = " 🔵"
			}

			resultMessage += fmt.Sprintf("%d. **From:** %s%s%s\n", i+1, email.From, importance, unread)
			resultMessage += fmt.Sprintf("   **Subject:** %s\n", email.Subject)
			resultMessage += fmt.Sprintf("   **Date:** %s\n", email.Date.Format("2006-01-02 15:04"))
			resultMessage += fmt.Sprintf("   **Snippet:** %s\n\n", email.Snippet)
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"query":       args.Query,
			"time_range":  args.TimeRange,
			"emails":      results,
			"total_found": len(results),
			"success":     true,
		},
	}, nil
}

// parseGmailMessage извлекает данные из Gmail сообщения
func (s *GmailMCPServer) parseGmailMessage(msg *gmail.Message) GmailEmailResult {
	result := GmailEmailResult{
		ID:      msg.Id,
		Snippet: msg.Snippet,
	}

	// Извлекаем дату
	if msg.InternalDate > 0 {
		result.Date = time.Unix(msg.InternalDate/1000, 0)
	}

	// Проверяем метки для определения важности и статуса прочтения
	for _, labelID := range msg.LabelIds {
		switch labelID {
		case "IMPORTANT":
			result.IsImportant = true
		case "UNREAD":
			result.IsUnread = true
		}
	}

	// Извлекаем заголовки
	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "Subject":
			result.Subject = header.Value
		case "From":
			result.From = header.Value
		}
	}

	// Извлекаем тело письма
	result.Body = s.extractMessageBody(msg.Payload)

	return result
}

// extractMessageBody извлекает текстовое содержимое письма
func (s *GmailMCPServer) extractMessageBody(payload *gmail.MessagePart) string {
	if payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}

	// Ищем в частях сообщения
	for _, part := range payload.Parts {
		if part.MimeType == "text/plain" && part.Body.Data != "" {
			decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
			if err == nil {
				return string(decoded)
			}
		}
	}

	return ""
}

func main() {
	if err := godotenv.Load(".env" /*, "../.env", "cmd/bot/.env"*/); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Получаем учетные данные Gmail из переменной окружения
	gmailCredentials := os.Getenv("GMAIL_CREDENTIALS_JSON")

	// Если не задано прямо в переменной, пытаемся прочитать из файла
	if gmailCredentials == "" {
		if credentialsPath := os.Getenv("GMAIL_CREDENTIALS_JSON_PATH"); credentialsPath != "" {
			if credentialsData, err := os.ReadFile(credentialsPath); err == nil {
				gmailCredentials = string(credentialsData)
			}
		}
	}

	if gmailCredentials == "" {
		log.Fatal("❌ Either GMAIL_CREDENTIALS_JSON or GMAIL_CREDENTIALS_JSON_PATH environment variable is required")
	}

	log.Printf("🚀 Starting Gmail MCP Server")
	log.Printf("🔑 Using Gmail credentials (length: %d chars)", len(gmailCredentials))

	// Создаем Gmail сервер
	gmailServer, err := NewGmailMCPServer(gmailCredentials)
	if err != nil {
		log.Fatalf("❌ Failed to create Gmail server: %v", err)
	}

	// Создаем MCP сервер
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-chatter-gmail-mcp",
		Version: "1.0.0",
	}, nil)

	// Регистрируем инструменты
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_gmail",
		Description: "Searches for emails in Gmail using specified query and filters",
	}, gmailServer.SearchEmails)

	log.Printf("📋 Registered Gmail MCP tools: search_gmail")
	log.Printf("🔗 Starting Gmail MCP server on stdin/stdout...")

	// Запускаем сервер через stdin/stdout
	transport := mcp.NewStdioTransport()
	if err := server.Run(context.Background(), transport); err != nil {
		log.Fatalf("❌ Gmail MCP Server failed: %v", err)
	}
}
