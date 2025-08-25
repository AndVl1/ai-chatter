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
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RuStoreAuthParams параметры для авторизации в RuStore (DEPRECATED - используется env RUSTORE_KEY)
type RuStoreAuthParams struct {
	CompanyID string `json:"company_id" mcp:"RuStore company ID (deprecated)"`
	KeyID     string `json:"key_id" mcp:"RuStore API key ID (deprecated)"`
	KeySecret string `json:"key_secret" mcp:"RuStore API key secret (deprecated)"`
}

// RuStoreCreateDraftParams параметры для создания черновика версии
type RuStoreCreateDraftParams struct {
	PackageName      string   `json:"package_name" mcp:"Package name of the application (e.g., 'com.myapp.example')"`
	AppName          string   `json:"app_name,omitempty" mcp:"Application name (max 5 characters)"`
	AppType          string   `json:"app_type,omitempty" mcp:"Application type: GAMES or MAIN"`
	Categories       []string `json:"categories,omitempty" mcp:"Application categories (max 2)"`
	AgeLegal         string   `json:"age_legal,omitempty" mcp:"Age category: 0+, 6+, 12+, 16+, 18+"`
	ShortDescription string   `json:"short_description,omitempty" mcp:"Short description (max 80 characters)"`
	FullDescription  string   `json:"full_description,omitempty" mcp:"Full description (max 4000 characters)"`
	WhatsNew         string   `json:"whats_new,omitempty" mcp:"What's new description (max 5000 characters)"`
	ModerInfo        string   `json:"moder_info,omitempty" mcp:"Comment for moderator (max 180 characters)"`
	PriceValue       int      `json:"price_value,omitempty" mcp:"Price in kopecks (e.g., 8799 for 87.99 rubles)"`
	SeoTagIds        []int    `json:"seo_tag_ids,omitempty" mcp:"SEO tag IDs (max 5)"`
	PublishType      string   `json:"publish_type,omitempty" mcp:"Publish type: MANUAL, INSTANTLY, DELAYED"`
	PublishDateTime  string   `json:"publish_date_time,omitempty" mcp:"Publish date for DELAYED type (yyyy-MM-ddTHH:mm:ssXXX)"`
	PartialValue     int      `json:"partial_value,omitempty" mcp:"Partial publish percentage: 5, 10, 25, 50, 75, 100"`
}

// RuStoreUploadAABParams параметры для загрузки AAB файла
type RuStoreUploadAABParams struct {
	AppID     string `json:"app_id" mcp:"RuStore application ID"`
	VersionID string `json:"version_id" mcp:"version ID from draft creation"`
	AABData   string `json:"aab_data" mcp:"base64-encoded AAB file content"`
	AABName   string `json:"aab_name" mcp:"AAB file name"`
}

// RuStoreUploadAPKParams параметры для загрузки APK файла
type RuStoreUploadAPKParams struct {
	AppID     string `json:"app_id" mcp:"RuStore application ID"`
	VersionID string `json:"version_id" mcp:"version ID from draft creation"`
	APKData   string `json:"apk_data" mcp:"base64-encoded APK file content"`
	APKName   string `json:"apk_name" mcp:"APK file name"`
}

// RuStoreSubmitParams параметры для отправки на модерацию
type RuStoreSubmitParams struct {
	AppID     string `json:"app_id" mcp:"RuStore application ID"`
	VersionID string `json:"version_id" mcp:"version ID"`
}

// RuStoreGetAppsParams параметры для получения списка приложений
type RuStoreGetAppsParams struct {
	AppName    string `json:"app_name,omitempty" mcp:"Поиск по названию приложения"`
	AppPackage string `json:"app_package,omitempty" mcp:"Поиск по package name"`
	PageSize   int    `json:"page_size,omitempty" mcp:"Количество приложений на странице (1-1000)"`
}

// RuStoreTokenResponse ответ на запрос токена
type RuStoreTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// RuStoreDraftResponse ответ на создание черновика согласно API v1
type RuStoreDraftResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Body      int    `json:"body"` // Version ID как число
	Timestamp string `json:"timestamp"`
}

// RuStoreUploadResponse ответ на загрузку файла
type RuStoreUploadResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// RuStoreApplication информация о приложении из RuStore
type RuStoreApplication struct {
	AppId       string   `json:"appId"`
	PackageName string   `json:"packageName"`
	AppName     string   `json:"appName"`
	AppStatus   string   `json:"appStatus"`
	CompanyName string   `json:"companyName"`
	VersionName string   `json:"versionName"`
	Categories  []string `json:"categories,omitempty"`
	AgeLegal    string   `json:"ageLegal,omitempty"`
	AppType     string   `json:"appType,omitempty"`
}

// RuStoreAppListResponse ответ на получение списка приложений
type RuStoreAppListResponse struct {
	Content           []RuStoreApplication `json:"content"`
	ContinuationToken string               `json:"continuationToken,omitempty"`
	TotalElements     int                  `json:"totalElements,omitempty"`
}

// RuStoreMCPServer кастомный MCP сервер для RuStore
type RuStoreMCPServer struct {
	client      *http.Client
	accessToken string
	tokenExpiry time.Time
	baseURL     string
}

// NewRuStoreMCPServer создает новый MCP сервер для RuStore с готовым токеном
func NewRuStoreMCPServer(token string) (*RuStoreMCPServer, error) {
	log.Printf("🔑 Initializing RuStore MCP Server with token-based auth")

	if token == "" {
		log.Printf("⚠️ RUSTORE_KEY not set, using test mode")
		token = "test-token-placeholder"
	}

	// Токен готов к использованию, устанавливаем время истечения в будущем
	tokenExpiry := time.Now().Add(24 * time.Hour) // Токен действителен 24 часа

	return &RuStoreMCPServer{
		client:      &http.Client{Timeout: 60 * time.Second},
		baseURL:     "https://public-api.rustore.ru/public/v1",
		accessToken: token,
		tokenExpiry: tokenExpiry,
	}, nil
}

// authenticate проверяет действительность токена (теперь токен приходит готовым)
func (r *RuStoreMCPServer) authenticate(ctx context.Context) error {
	// Проверяем, есть ли действующий токен
	if r.accessToken != "" && time.Now().Before(r.tokenExpiry) {
		log.Printf("✅ Using existing valid RUSTORE_KEY token")
		return nil
	}

	if r.accessToken == "" {
		return fmt.Errorf("RUSTORE_KEY token is empty")
	}

	// Токен из RUSTORE_KEY готов к использованию
	log.Printf("✅ RUSTORE_KEY token ready for API calls")
	return nil
}

// makeAuthorizedRequest выполняет авторизованный запрос к RuStore API
func (r *RuStoreMCPServer) makeAuthorizedRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Public-Token", r.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ai-chatter-rustore-mcp/1.0.0")

	return r.client.Do(req)
}

// CreateDraft создает черновик версии приложения
func (r *RuStoreMCPServer) CreateDraft(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreCreateDraftParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📝 MCP Server: Creating RuStore draft for package %s", args.PackageName)

	// Проверяем токен из RUSTORE_KEY
	if err := r.authenticate(ctx); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	// Формируем URL для создания черновика согласно API v1
	draftURL := fmt.Sprintf("%s/application/%s/version", r.baseURL, args.PackageName)

	// Подготавливаем данные для создания черновика согласно актуальной документации
	draftData := make(map[string]interface{})

	// Добавляем только непустые поля
	if args.AppName != "" {
		draftData["appName"] = args.AppName
	}
	if args.AppType != "" {
		draftData["appType"] = args.AppType
	}
	if len(args.Categories) > 0 {
		draftData["categories"] = args.Categories
	}
	if args.AgeLegal != "" {
		draftData["ageLegal"] = args.AgeLegal
	}
	if args.ShortDescription != "" {
		draftData["shortDescription"] = args.ShortDescription
	}
	if args.FullDescription != "" {
		draftData["fullDescription"] = args.FullDescription
	}
	if args.WhatsNew != "" {
		draftData["whatsNew"] = args.WhatsNew
	}
	if args.ModerInfo != "" {
		draftData["moderInfo"] = args.ModerInfo
	}
	if args.PriceValue > 0 {
		draftData["priceValue"] = args.PriceValue
	}
	if len(args.SeoTagIds) > 0 {
		draftData["seoTagIds"] = args.SeoTagIds
	}
	if args.PublishType != "" {
		draftData["publishType"] = args.PublishType
	}
	if args.PublishDateTime != "" {
		draftData["publishDateTime"] = args.PublishDateTime
	}
	if args.PartialValue > 0 {
		draftData["partialValue"] = args.PartialValue
	}

	jsonData, err := json.Marshal(draftData)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to marshal draft data: %v", err)},
			},
		}, nil
	}

	resp, err := r.makeAuthorizedRequest(ctx, "POST", draftURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Draft creation request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Draft creation failed with status %d: %s", resp.StatusCode, string(respBody))},
			},
		}, nil
	}

	var draftResp RuStoreDraftResponse
	if err := json.Unmarshal(respBody, &draftResp); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to parse draft response: %v", err)},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Successfully created draft version for app %s\n", args.PackageName)
	resultMessage += fmt.Sprintf("**Version ID:** %d\n", draftResp.Body)
	resultMessage += fmt.Sprintf("**Response Code:** %s\n", draftResp.Code)
	resultMessage += fmt.Sprintf("**Timestamp:** %s\n", draftResp.Timestamp)
	if draftResp.Message != "" {
		resultMessage += fmt.Sprintf("**Message:** %s\n", draftResp.Message)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":      true,
			"package_name": args.PackageName,
			"version_id":   draftResp.Body,
			"code":         draftResp.Code,
			"timestamp":    draftResp.Timestamp,
		},
	}, nil
}

// UploadAAB загружает AAB файл для версии
func (r *RuStoreMCPServer) UploadAAB(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreUploadAABParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("⬆️ MCP Server: Uploading AAB file for app %s, version %s", args.AppID, args.VersionID)

	// Проверяем токен из RUSTORE_KEY
	if err := r.authenticate(ctx); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	// Формируем URL для загрузки AAB
	uploadURL := fmt.Sprintf("%s/application/%s/version/%s/apk", r.baseURL, args.AppID, args.VersionID)

	// Подготавливаем данные для загрузки
	uploadData := map[string]string{
		"file": args.AABData, // base64-encoded content
		"name": args.AABName,
	}

	jsonData, err := json.Marshal(uploadData)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to marshal upload data: %v", err)},
			},
		}, nil
	}

	resp, err := r.makeAuthorizedRequest(ctx, "POST", uploadURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ AAB upload request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ AAB upload failed with status %d: %s", resp.StatusCode, string(respBody))},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Successfully uploaded AAB file %s\n", args.AABName)
	resultMessage += fmt.Sprintf("**App ID:** %s\n", args.AppID)
	resultMessage += fmt.Sprintf("**Version ID:** %s\n", args.VersionID)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":    true,
			"app_id":     args.AppID,
			"version_id": args.VersionID,
			"aab_name":   args.AABName,
		},
	}, nil
}

// UploadAPK загружает APK файл для версии
func (r *RuStoreMCPServer) UploadAPK(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreUploadAPKParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("⬆️ MCP Server: Uploading APK file for app %s, version %s", args.AppID, args.VersionID)

	// Проверяем токен из RUSTORE_KEY
	if err := r.authenticate(ctx); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	// Формируем URL для загрузки APK (используем тот же endpoint что и для AAB)
	uploadURL := fmt.Sprintf("%s/application/%s/version/%s/apk", r.baseURL, args.AppID, args.VersionID)

	// Подготавливаем данные для загрузки
	uploadData := map[string]string{
		"file": args.APKData, // base64-encoded content
		"name": args.APKName,
	}

	jsonData, err := json.Marshal(uploadData)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to marshal upload data: %v", err)},
			},
		}, nil
	}

	resp, err := r.makeAuthorizedRequest(ctx, "POST", uploadURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ APK upload request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ APK upload failed with status %d: %s", resp.StatusCode, string(respBody))},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Successfully uploaded APK file %s\n", args.APKName)
	resultMessage += fmt.Sprintf("**App ID:** %s\n", args.AppID)
	resultMessage += fmt.Sprintf("**Version ID:** %s\n", args.VersionID)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":    true,
			"app_id":     args.AppID,
			"version_id": args.VersionID,
			"apk_name":   args.APKName,
		},
	}, nil
}

// SubmitForReview отправляет версию на модерацию
func (r *RuStoreMCPServer) SubmitForReview(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreSubmitParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("🔍 MCP Server: Submitting app %s version %s for review", args.AppID, args.VersionID)

	// Проверяем токен из RUSTORE_KEY
	if err := r.authenticate(ctx); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	// Формируем URL для отправки на модерацию
	submitURL := fmt.Sprintf("%s/application/%s/version/%s/commit", r.baseURL, args.AppID, args.VersionID)

	resp, err := r.makeAuthorizedRequest(ctx, "POST", submitURL, nil)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Submit request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Submit failed with status %d: %s", resp.StatusCode, string(respBody))},
			},
		}, nil
	}

	resultMessage := fmt.Sprintf("✅ Successfully submitted version for review\n")
	resultMessage += fmt.Sprintf("**App ID:** %s\n", args.AppID)
	resultMessage += fmt.Sprintf("**Version ID:** %s\n", args.VersionID)
	resultMessage += fmt.Sprintf("**Status:** Submitted for moderation\n")

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":    true,
			"app_id":     args.AppID,
			"version_id": args.VersionID,
			"status":     "submitted",
		},
	}, nil
}

// GetAppList получает список приложений из RuStore
func (r *RuStoreMCPServer) GetAppList(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreGetAppsParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📱 MCP Server: Getting RuStore app list")

	// Проверяем токен из RUSTORE_KEY
	if err := r.authenticate(ctx); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	// Формируем URL для получения списка приложений
	appsURL := fmt.Sprintf("%s/application", r.baseURL)

	// Добавляем query параметры
	urlWithParams := appsURL + "?"
	if args.AppName != "" {
		urlWithParams += fmt.Sprintf("appName=%s&", args.AppName)
	}
	if args.AppPackage != "" {
		urlWithParams += fmt.Sprintf("appPackage=%s&", args.AppPackage)
	}
	if args.PageSize > 0 {
		urlWithParams += fmt.Sprintf("pageSize=%d&", args.PageSize)
	} else {
		urlWithParams += "pageSize=100&" // По умолчанию 100 приложений
	}

	// Убираем лишний '&' в конце
	if urlWithParams[len(urlWithParams)-1] == '&' {
		urlWithParams = urlWithParams[:len(urlWithParams)-1]
	}

	resp, err := r.makeAuthorizedRequest(ctx, "GET", urlWithParams, nil)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ App list request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ App list request failed with status %d: %s", resp.StatusCode, string(respBody))},
			},
		}, nil
	}

	var appListResp RuStoreAppListResponse
	if err := json.Unmarshal(respBody, &appListResp); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to parse app list response: %v", err)},
			},
		}, nil
	}

	var resultMessage strings.Builder
	resultMessage.WriteString(fmt.Sprintf("✅ Found %d applications in RuStore\n\n", len(appListResp.Content)))

	for i, app := range appListResp.Content {
		resultMessage.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, app.AppName))
		resultMessage.WriteString(fmt.Sprintf("   📦 Package: `%s`\n", app.PackageName))
		resultMessage.WriteString(fmt.Sprintf("   🆔 App ID: `%s`\n", app.AppId))
		resultMessage.WriteString(fmt.Sprintf("   📊 Status: %s\n", app.AppStatus))
		if app.AppType != "" {
			resultMessage.WriteString(fmt.Sprintf("   🎮 Type: %s\n", app.AppType))
		}
		if len(app.Categories) > 0 {
			resultMessage.WriteString(fmt.Sprintf("   🏷️ Categories: %s\n", strings.Join(app.Categories, ", ")))
		}
		if app.AgeLegal != "" {
			resultMessage.WriteString(fmt.Sprintf("   🔞 Age: %s\n", app.AgeLegal))
		}
		if app.VersionName != "" {
			resultMessage.WriteString(fmt.Sprintf("   🏗️ Version: %s\n", app.VersionName))
		}
		resultMessage.WriteString("\n")
	}

	if appListResp.TotalElements > 0 {
		resultMessage.WriteString(fmt.Sprintf("**Total apps:** %d\n", appListResp.TotalElements))
	}

	// Подготавливаем метаданные с приложениями для автоматизации
	appsMeta := make([]map[string]interface{}, 0, len(appListResp.Content))
	for _, app := range appListResp.Content {
		appMeta := map[string]interface{}{
			"appId":       app.AppId,
			"packageName": app.PackageName,
			"appName":     app.AppName,
			"appStatus":   app.AppStatus,
			"appType":     app.AppType,
			"categories":  app.Categories,
			"ageLegal":    app.AgeLegal,
		}
		appsMeta = append(appsMeta, appMeta)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage.String()},
		},
		Meta: map[string]interface{}{
			"success":      true,
			"apps_count":   len(appListResp.Content),
			"total_apps":   appListResp.TotalElements,
			"applications": appsMeta,
			"continuation": appListResp.ContinuationToken,
		},
	}, nil
}

// Authenticate выполняет проверку токена RUSTORE_KEY (DEPRECATED - токен настраивается через env)
func (r *RuStoreMCPServer) Authenticate(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[RuStoreAuthParams]) (*mcp.CallToolResultFor[any], error) {
	log.Printf("⚠️ MCP Server: rustore_auth tool is DEPRECATED. Using RUSTORE_KEY from environment.")

	err := r.authenticate(ctx)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ RUSTORE_KEY authentication failed: %v", err)},
			},
		}, nil
	}

	resultMessage := "✅ Using RUSTORE_KEY token from environment\n"
	resultMessage += "**Note:** rustore_auth tool is deprecated. Set RUSTORE_KEY in .env file.\n"
	resultMessage += fmt.Sprintf("**Token valid until:** %s\n", r.tokenExpiry.Format("2006-01-02 15:04:05"))

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":      true,
			"method":       "rustore_key_env",
			"token_expiry": r.tokenExpiry,
		},
	}, nil
}

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	log.Printf("🚀 Starting RuStore MCP Server")

	// Создаем RuStore сервер
	rustoreServer, err := NewRuStoreMCPServer(os.Getenv("RUSTORE_KEY"))
	if err != nil {
		log.Fatalf("❌ Failed to create RuStore server: %v", err)
	}

	// Создаем MCP сервер
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-chatter-rustore-mcp",
		Version: "1.0.0",
	}, nil)

	// Регистрируем инструменты
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_auth",
		Description: "Authenticates with RuStore API using company credentials",
	}, rustoreServer.Authenticate)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_create_draft",
		Description: "Creates a draft version of an application in RuStore",
	}, rustoreServer.CreateDraft)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_upload_aab",
		Description: "Uploads AAB file for a draft version in RuStore",
	}, rustoreServer.UploadAAB)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_upload_apk",
		Description: "Uploads APK file for a draft version in RuStore",
	}, rustoreServer.UploadAPK)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_submit_review",
		Description: "Submits application version for moderation in RuStore",
	}, rustoreServer.SubmitForReview)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rustore_get_apps",
		Description: "Gets list of applications from RuStore for automation",
	}, rustoreServer.GetAppList)

	log.Printf("📋 Registered RuStore MCP tools: rustore_auth, rustore_create_draft, rustore_upload_aab, rustore_upload_apk, rustore_submit_review, rustore_get_apps")
	log.Printf("🔗 Starting RuStore MCP server on stdin/stdout...")

	// Запускаем сервер через stdin/stdout
	transport := mcp.NewStdioTransport()
	if err := server.Run(context.Background(), transport); err != nil {
		log.Fatalf("❌ RuStore MCP Server failed: %v", err)
	}
}
