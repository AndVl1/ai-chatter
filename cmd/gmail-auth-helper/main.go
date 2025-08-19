package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

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

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: gmail-auth-helper <credentials.json>")
	}

	credentialsFile := os.Args[1]

	// Читаем credentials из файла
	credentialsData, err := os.ReadFile(credentialsFile)
	if err != nil {
		log.Fatalf("Failed to read credentials file: %v", err)
	}

	// Парсим credentials с поддержкой формата Google Cloud Console
	credentials, err := parseGoogleCredentials(credentialsData)
	if err != nil {
		log.Fatalf("Failed to parse credentials: %v", err)
	}

	// Создаем OAuth2 config
	config := &oauth2.Config{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{gmail.GmailReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	// Генерируем URL для авторизации
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("🔗 Gmail OAuth2 Authorization Helper\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("1. Open this URL in your browser:\n")
	fmt.Printf("   %s\n\n", authURL)
	fmt.Printf("2. Authorize the application\n")
	fmt.Printf("3. Copy the authorization code and enter it below\n\n")
	fmt.Printf("📝 Enter the authorization code: ")

	// Читаем код авторизации
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Failed to read authorization code: %v", err)
	}

	// Обмениваем код на токен
	token, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		log.Fatalf("Failed to exchange code for token: %v", err)
	}

	fmt.Printf("\n✅ Successfully obtained tokens!\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("Add these to your .env file:\n\n")
	fmt.Printf("GMAIL_CREDENTIALS_JSON='%s'\n", string(credentialsData))
	if token.RefreshToken != "" {
		fmt.Printf("GMAIL_REFRESH_TOKEN='%s'\n", token.RefreshToken)
	}
	fmt.Printf("\n📝 Token details:\n")
	fmt.Printf("Access Token: %s\n", token.AccessToken[:20]+"...")
	if token.RefreshToken != "" {
		fmt.Printf("Refresh Token: %s\n", token.RefreshToken[:20]+"...")
	}
	fmt.Printf("Expires: %v\n", token.Expiry)
}

// parseGoogleCredentials парсит credentials JSON с поддержкой различных форматов
func parseGoogleCredentials(credentialsData []byte) (*OAuth2Credentials, error) {
	// Сначала пытаемся парсить как прямую структуру OAuth2Credentials
	var directCredentials OAuth2Credentials
	if err := json.Unmarshal(credentialsData, &directCredentials); err == nil {
		if directCredentials.ClientID != "" && directCredentials.ClientSecret != "" {
			fmt.Printf("✅ Parsed direct OAuth2 credentials format\n")
			return &directCredentials, nil
		}
	}

	// Если не получилось, пытаемся парсить как формат Google Cloud Console
	var googleFile GoogleCredentialsFile
	if err := json.Unmarshal(credentialsData, &googleFile); err != nil {
		return nil, fmt.Errorf("failed to parse credentials as Google format: %w", err)
	}

	// Проверяем installed credentials (Desktop application)
	if googleFile.Installed != nil {
		fmt.Printf("✅ Parsed Google Cloud Console credentials (installed/desktop format)\n")
		return googleFile.Installed, nil
	}

	// Проверяем web credentials (Web application)
	if googleFile.Web != nil {
		fmt.Printf("✅ Parsed Google Cloud Console credentials (web format)\n")
		return googleFile.Web, nil
	}

	return nil, fmt.Errorf("no valid credentials found in JSON - expected 'installed' or 'web' section")
}
