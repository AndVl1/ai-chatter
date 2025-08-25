package rustore

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RuStoreMCPClient клиент для работы с RuStore MCP сервером
type RuStoreMCPClient struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewRuStoreMCPClient создает новый RuStore MCP клиент
func NewRuStoreMCPClient() *RuStoreMCPClient {
	return &RuStoreMCPClient{}
}

// Connect подключается к RuStore MCP серверу через stdio
func (r *RuStoreMCPClient) Connect(ctx context.Context) error {
	log.Printf("🔗 Connecting to RuStore MCP server via stdio")

	// Создаем MCP клиент
	r.client = mcp.NewClient(&mcp.Implementation{
		Name:    "ai-chatter-bot-rustore",
		Version: "1.0.0",
	}, nil)

	// Запускаем RuStore MCP сервер как подпроцесс
	serverPath := "./bin/rustore-mcp-server"
	if customPath := os.Getenv("RUSTORE_MCP_SERVER_PATH"); customPath != "" {
		serverPath = customPath
	}

	log.Printf("🔍 RuStore MCP: Trying to start server at path: %s", serverPath)

	// Логируем текущий рабочий каталог
	if pwd, err := os.Getwd(); err == nil {
		log.Printf("🔍 RuStore MCP: Current working directory: %s", pwd)
	}

	// Проверяем существование файла
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		log.Printf("❌ RuStore MCP: Server binary not found at %s", serverPath)
		// Попробуем найти файл в альтернативных местах
		if _, err := os.Stat("./rustore-mcp-server"); err == nil {
			log.Printf("💡 RuStore MCP: Found server at ./rustore-mcp-server")
		} else if _, err := os.Stat("/app/rustore-mcp-server"); err == nil {
			log.Printf("💡 RuStore MCP: Found server at /app/rustore-mcp-server")
		}
		return fmt.Errorf("rustore MCP server binary not found at %s", serverPath)
	}

	// Проверяем права на выполнение
	if info, err := os.Stat(serverPath); err == nil {
		log.Printf("🔍 RuStore MCP: Server file exists, mode: %v, size: %d bytes", info.Mode(), info.Size())
	}

	cmd := exec.CommandContext(ctx, serverPath)
	cmd.Env = os.Environ()

	transport := mcp.NewCommandTransport(cmd)

	session, err := r.client.Connect(ctx, transport)
	if err != nil {
		return fmt.Errorf("failed to connect to RuStore MCP server: %w", err)
	}

	r.session = session
	log.Printf("✅ Connected to RuStore MCP server")
	return nil
}

// Close закрывает соединение с RuStore MCP сервером
func (r *RuStoreMCPClient) Close() error {
	if r.session != nil {
		return r.session.Close()
	}
	return nil
}

// Authenticate выполняет авторизацию в RuStore API
func (r *RuStoreMCPClient) Authenticate(ctx context.Context, companyID, keyID, keySecret string) RuStoreMCPResult {
	if r.session == nil {
		return RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}
	}

	log.Printf("🔐 Authenticating with RuStore via MCP: company=%s", companyID)

	// Вызываем инструмент rustore_auth
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "rustore_auth",
		Arguments: map[string]any{
			"company_id": companyID,
			"key_id":     keyID,
			"key_secret": keySecret,
		},
	})

	if err != nil {
		log.Printf("❌ RuStore MCP auth error: %v", err)
		return RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP auth error: %v", err)}
	}

	if result.IsError {
		return RuStoreMCPResult{Success: false, Message: "RuStore authentication tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return RuStoreMCPResult{
		Success: true,
		Message: responseText,
	}
}

// CreateDraft создает черновик версии приложения
func (r *RuStoreMCPClient) CreateDraft(ctx context.Context, params CreateDraftParams) RuStoreDraftResult {
	if r.session == nil {
		return RuStoreDraftResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}}
	}

	log.Printf("📝 Creating RuStore draft via MCP: package=%s", params.PackageName)

	// Подготавливаем аргументы согласно новому API
	arguments := map[string]any{
		"package_name": params.PackageName,
	}

	// Добавляем только непустые поля
	if params.AppName != "" {
		arguments["app_name"] = params.AppName
	}
	if params.AppType != "" {
		arguments["app_type"] = params.AppType
	}
	if len(params.Categories) > 0 {
		arguments["categories"] = params.Categories
	}
	if params.AgeLegal != "" {
		arguments["age_legal"] = params.AgeLegal
	}
	if params.ShortDescription != "" {
		arguments["short_description"] = params.ShortDescription
	}
	if params.FullDescription != "" {
		arguments["full_description"] = params.FullDescription
	}
	if params.WhatsNew != "" {
		arguments["whats_new"] = params.WhatsNew
	}
	if params.ModerInfo != "" {
		arguments["moder_info"] = params.ModerInfo
	}
	if params.PriceValue > 0 {
		arguments["price_value"] = params.PriceValue
	}
	if len(params.SeoTagIds) > 0 {
		arguments["seo_tag_ids"] = params.SeoTagIds
	}
	if params.PublishType != "" {
		arguments["publish_type"] = params.PublishType
	}
	if params.PublishDateTime != "" {
		arguments["publish_date_time"] = params.PublishDateTime
	}
	if params.PartialValue > 0 {
		arguments["partial_value"] = params.PartialValue
	}

	// Вызываем инструмент rustore_create_draft
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "rustore_create_draft",
		Arguments: arguments,
	})

	if err != nil {
		log.Printf("❌ RuStore MCP create draft error: %v", err)
		return RuStoreDraftResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP create draft error: %v", err)}}
	}

	if result.IsError {
		return RuStoreDraftResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: "RuStore create draft tool returned error"}}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем метаданные согласно новому API
	draftResult := RuStoreDraftResult{
		RuStoreMCPResult: RuStoreMCPResult{
			Success: true,
			Message: responseText,
		},
	}

	if result.Meta != nil {
		if packageName, ok := result.Meta["package_name"].(string); ok {
			draftResult.AppID = packageName // Используем package_name как AppID для обратной совместимости
		}
		if versionID, ok := result.Meta["version_id"].(float64); ok {
			draftResult.VersionID = fmt.Sprintf("%.0f", versionID) // Конвертируем число в строку
		}
		if code, ok := result.Meta["code"].(string); ok {
			draftResult.Status = code
		}
		// Для новых полей используем значения по умолчанию
		draftResult.VersionName = "Draft"
		draftResult.VersionCode = 0
	}

	return draftResult
}

// UploadAAB загружает AAB файл для версии
func (r *RuStoreMCPClient) UploadAAB(ctx context.Context, appID, versionID, aabData, aabName string) RuStoreMCPResult {
	if r.session == nil {
		return RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}
	}

	log.Printf("⬆️ Uploading AAB to RuStore via MCP: app=%s, version=%s, file=%s", appID, versionID, aabName)

	// Вызываем инструмент rustore_upload_aab
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "rustore_upload_aab",
		Arguments: map[string]any{
			"app_id":     appID,
			"version_id": versionID,
			"aab_data":   aabData,
			"aab_name":   aabName,
		},
	})

	if err != nil {
		log.Printf("❌ RuStore MCP upload AAB error: %v", err)
		return RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP upload AAB error: %v", err)}
	}

	if result.IsError {
		return RuStoreMCPResult{Success: false, Message: "RuStore upload AAB tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return RuStoreMCPResult{
		Success: true,
		Message: responseText,
	}
}

// UploadAPK загружает APK файл для версии
func (r *RuStoreMCPClient) UploadAPK(ctx context.Context, appID, versionID, apkData, apkName string) RuStoreMCPResult {
	if r.session == nil {
		return RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}
	}

	log.Printf("⬆️ Uploading APK to RuStore via MCP: app=%s, version=%s, file=%s", appID, versionID, apkName)

	// Вызываем инструмент rustore_upload_apk
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "rustore_upload_apk",
		Arguments: map[string]any{
			"app_id":     appID,
			"version_id": versionID,
			"apk_data":   apkData,
			"apk_name":   apkName,
		},
	})

	if err != nil {
		log.Printf("❌ RuStore MCP upload APK error: %v", err)
		return RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP upload APK error: %v", err)}
	}

	if result.IsError {
		return RuStoreMCPResult{Success: false, Message: "RuStore upload APK tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return RuStoreMCPResult{
		Success: true,
		Message: responseText,
	}
}

// UploadAndroidFile загружает Android файл (AAB или APK) для версии
func (r *RuStoreMCPClient) UploadAndroidFile(ctx context.Context, appID, versionID, fileData, fileName string) RuStoreMCPResult {
	// Определяем тип файла и вызываем соответствующий метод
	if len(fileName) > 4 && fileName[len(fileName)-4:] == ".aab" {
		return r.UploadAAB(ctx, appID, versionID, fileData, fileName)
	} else if len(fileName) > 4 && fileName[len(fileName)-4:] == ".apk" {
		return r.UploadAPK(ctx, appID, versionID, fileData, fileName)
	} else {
		return RuStoreMCPResult{Success: false, Message: fmt.Sprintf("Unsupported file type: %s. Only .aab and .apk files are supported", fileName)}
	}
}

// SubmitForReview отправляет версию на модерацию
func (r *RuStoreMCPClient) SubmitForReview(ctx context.Context, appID, versionID string) RuStoreMCPResult {
	if r.session == nil {
		return RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}
	}

	log.Printf("🔍 Submitting RuStore version for review via MCP: app=%s, version=%s", appID, versionID)

	// Вызываем инструмент rustore_submit_review
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name: "rustore_submit_review",
		Arguments: map[string]any{
			"app_id":     appID,
			"version_id": versionID,
		},
	})

	if err != nil {
		log.Printf("❌ RuStore MCP submit error: %v", err)
		return RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP submit error: %v", err)}
	}

	if result.IsError {
		return RuStoreMCPResult{Success: false, Message: "RuStore submit tool returned error"}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	return RuStoreMCPResult{
		Success: true,
		Message: responseText,
	}
}

// Структуры данных

// RuStoreMCPResult результат RuStore MCP операции
type RuStoreMCPResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// RuStoreDraftResult результат создания черновика
type RuStoreDraftResult struct {
	RuStoreMCPResult
	AppID       string `json:"app_id"`
	VersionID   string `json:"version_id"`
	VersionName string `json:"version_name"`
	VersionCode int    `json:"version_code"`
	Status      string `json:"status"`
}

// CreateDraftParams параметры для создания черновика (обновлено согласно RuStore API v1)
type CreateDraftParams struct {
	PackageName      string   `json:"package_name"`                // Имя пакета приложения (обязательно)
	AppName          string   `json:"app_name,omitempty"`          // Название приложения (макс 5 символов!)
	AppType          string   `json:"app_type,omitempty"`          // Тип приложения: GAMES или MAIN
	Categories       []string `json:"categories,omitempty"`        // Категории приложения (макс 2)
	AgeLegal         string   `json:"age_legal,omitempty"`         // Возрастная категория: 0+, 6+, 12+, 16+, 18+
	ShortDescription string   `json:"short_description,omitempty"` // Краткое описание (макс 80 символов)
	FullDescription  string   `json:"full_description,omitempty"`  // Полное описание (макс 4000 символов)
	WhatsNew         string   `json:"whats_new,omitempty"`         // Что нового (макс 5000 символов)
	ModerInfo        string   `json:"moder_info,omitempty"`        // Комментарий для модератора (макс 180 символов)
	PriceValue       int      `json:"price_value,omitempty"`       // Цена в копейках
	SeoTagIds        []int    `json:"seo_tag_ids,omitempty"`       // ID SEO тегов (макс 5)
	PublishType      string   `json:"publish_type,omitempty"`      // Тип публикации: MANUAL, INSTANTLY, DELAYED
	PublishDateTime  string   `json:"publish_date_time,omitempty"` // Дата публикации для DELAYED
	PartialValue     int      `json:"partial_value,omitempty"`     // Процент частичной публикации
}

// RuStoreCredentials учетные данные RuStore
type RuStoreCredentials struct {
	CompanyID string `json:"company_id"`
	KeyID     string `json:"key_id"`
	KeySecret string `json:"key_secret"`
}

// GetAppList получает список приложений из RuStore для автоматизации
func (r *RuStoreMCPClient) GetAppList(ctx context.Context, params GetAppListParams) RuStoreAppListResult {
	if r.session == nil {
		return RuStoreAppListResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: "RuStore MCP session not connected"}}
	}

	log.Printf("📱 Getting RuStore app list via MCP")

	// Подготавливаем аргументы
	arguments := map[string]any{}

	if params.AppName != "" {
		arguments["app_name"] = params.AppName
	}
	if params.AppPackage != "" {
		arguments["app_package"] = params.AppPackage
	}
	if params.PageSize > 0 {
		arguments["page_size"] = params.PageSize
	}

	// Вызываем инструмент rustore_get_apps
	result, err := r.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "rustore_get_apps",
		Arguments: arguments,
	})

	if err != nil {
		log.Printf("❌ RuStore MCP get apps error: %v", err)
		return RuStoreAppListResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: fmt.Sprintf("RuStore MCP get apps error: %v", err)}}
	}

	if result.IsError {
		return RuStoreAppListResult{RuStoreMCPResult: RuStoreMCPResult{Success: false, Message: "RuStore get apps tool returned error"}}
	}

	// Извлекаем текст из результата
	var responseText string
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			responseText += textContent.Text
		}
	}

	// Извлекаем приложения из метаданных
	appListResult := RuStoreAppListResult{
		RuStoreMCPResult: RuStoreMCPResult{
			Success: true,
			Message: responseText,
		},
	}

	if result.Meta != nil {
		if applications, ok := result.Meta["applications"].([]interface{}); ok {
			for _, app := range applications {
				if appMap, ok := app.(map[string]interface{}); ok {
					appInfo := RuStoreAppInfo{}

					if appId, ok := appMap["appId"].(string); ok {
						appInfo.AppID = appId
					}
					if packageName, ok := appMap["packageName"].(string); ok {
						appInfo.PackageName = packageName
					}
					if appName, ok := appMap["appName"].(string); ok {
						appInfo.Name = appName
					}
					if appStatus, ok := appMap["appStatus"].(string); ok {
						appInfo.Status = appStatus
					}
					if appType, ok := appMap["appType"].(string); ok {
						appInfo.AppType = appType
					}
					if categories, ok := appMap["categories"].([]interface{}); ok {
						for _, cat := range categories {
							if catStr, ok := cat.(string); ok {
								appInfo.Categories = append(appInfo.Categories, catStr)
							}
						}
					}
					if ageLegal, ok := appMap["ageLegal"].(string); ok {
						appInfo.AgeLegal = ageLegal
					}

					appListResult.Applications = append(appListResult.Applications, appInfo)
				}
			}
		}

		if appsCount, ok := result.Meta["apps_count"].(float64); ok {
			appListResult.Count = int(appsCount)
		}
		if totalApps, ok := result.Meta["total_apps"].(float64); ok {
			appListResult.TotalElements = int(totalApps)
		}
		if continuation, ok := result.Meta["continuation"].(string); ok {
			appListResult.ContinuationToken = continuation
		}
	}

	return appListResult
}

// GetAppListParams параметры для получения списка приложений
type GetAppListParams struct {
	AppName    string `json:"app_name,omitempty"`    // Поиск по названию приложения
	AppPackage string `json:"app_package,omitempty"` // Поиск по package name
	PageSize   int    `json:"page_size,omitempty"`   // Количество приложений на странице (1-1000)
}

// RuStoreAppListResult результат получения списка приложений
type RuStoreAppListResult struct {
	RuStoreMCPResult
	Applications      []RuStoreAppInfo `json:"applications"`
	Count             int              `json:"count"`
	TotalElements     int              `json:"total_elements"`
	ContinuationToken string           `json:"continuation_token,omitempty"`
}

// RuStoreAppInfo информация о приложении RuStore
type RuStoreAppInfo struct {
	AppID            string   `json:"app_id"`
	Name             string   `json:"name"`
	PackageName      string   `json:"package_name"`
	Status           string   `json:"status,omitempty"`
	AppType          string   `json:"app_type,omitempty"`
	Categories       []string `json:"categories,omitempty"`
	AgeLegal         string   `json:"age_legal,omitempty"`
	PrivacyPolicyURL string   `json:"privacy_policy_url,omitempty"`
}
