package release

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-chatter/internal/github"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/rustore"
)

// ReleaseAgent главный агент для управления релизами
type ReleaseAgent struct {
	githubClient  *github.GitHubMCPClient
	rustoreClient *rustore.RuStoreMCPClient
	llmClient     llm.Client
	githubAgent   *GitHubDataAgent

	// Активные сессии релизов
	sessions map[string]*ReleaseSession
}

// NewReleaseAgent создает новый Release Agent
func NewReleaseAgent(
	githubClient *github.GitHubMCPClient,
	rustoreClient *rustore.RuStoreMCPClient,
	llmClient llm.Client,
) *ReleaseAgent {
	githubAgent := NewGitHubDataAgent(githubClient, llmClient)

	return &ReleaseAgent{
		githubClient:  githubClient,
		rustoreClient: rustoreClient,
		llmClient:     llmClient,
		githubAgent:   githubAgent,
		sessions:      make(map[string]*ReleaseSession),
	}
}

// StartAIRelease запускает AI-powered процесс создания релиза
func (r *ReleaseAgent) StartAIRelease(ctx context.Context, userID, chatID int64, repoOwner, repoName string) (*ReleaseSession, error) {
	sessionID := fmt.Sprintf("ai_release_%d_%d", userID, time.Now().Unix())

	session := &ReleaseSession{
		ID:                 sessionID,
		UserID:             userID,
		ChatID:             chatID,
		AgentStatuses:      make(map[string]*AgentStatus),
		PendingRequests:    []*DataCollectionRequest{},
		CollectedResponses: make(map[string]string),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		Status:             "active",
	}

	// Инициализируем статус GitHub агента
	githubStatus := &AgentStatus{
		AgentName: "GitHub Data Collector",
		Status:    "running",
		Progress:  0,
		Message:   "Инициализация...",
		StartedAt: time.Now(),
		Results:   make(map[string]interface{}),
	}
	session.AgentStatuses["github"] = githubStatus

	r.sessions[sessionID] = session

	// Запускаем сбор данных из GitHub в горутине
	go r.collectGitHubData(ctx, session, repoOwner, repoName, githubStatus)

	return session, nil
}

// collectGitHubData собирает данные из GitHub
func (r *ReleaseAgent) collectGitHubData(ctx context.Context, session *ReleaseSession, repoOwner, repoName string, status *AgentStatus) {
	releaseData, err := r.githubAgent.CollectReleaseData(ctx, repoOwner, repoName, status)
	if err != nil {
		status.Status = "failed"
		status.ErrorMessage = err.Error()
		log.Printf("❌ GitHub data collection failed: %v", err)
		return
	}

	releaseData.UserSessionID = session.UserID
	session.ReleaseData = releaseData
	session.UpdatedAt = time.Now()

	// После сбора данных улучшаем автоматизацией
	r.enhanceDataCollectionWithAutomation(ctx, session)

	// Генерируем AI-управляемые запросы для пользователя
	r.generateDataCollectionRequests(ctx, session)
}

// generateDataCollectionRequests генерирует запросы через ИИ-анализ недостающих данных
func (r *ReleaseAgent) generateDataCollectionRequests(ctx context.Context, session *ReleaseSession) {
	log.Printf("🤖 Starting AI-powered data collection analysis...")

	// Вызываем ИИ-анализатор для определения недостающих полей
	requests, err := r.analyzeAndGenerateRequests(ctx, session)
	if err != nil {
		log.Printf("❌ AI analysis failed, falling back to basic requests: %v", err)
		// Fallback к минимальному набору критически важных полей
		requests = r.generateFallbackRequests(session)
	}

	session.PendingRequests = requests
	session.Status = "waiting_user"
	session.UpdatedAt = time.Now()
	log.Printf("✅ Generated %d AI-determined data collection requests for session %s", len(requests), session.ID)
}

// GetSession возвращает сессию по ID
func (r *ReleaseAgent) GetSession(sessionID string) (*ReleaseSession, bool) {
	session, exists := r.sessions[sessionID]
	return session, exists
}

// GetUserActiveSession возвращает активную сессию пользователя
func (r *ReleaseAgent) GetUserActiveSession(userID int64) (*ReleaseSession, bool) {
	for _, session := range r.sessions {
		if session.UserID == userID && (session.Status == "active" || session.Status == "waiting_user") {
			return session, true
		}
	}
	return nil, false
}

// ProcessUserResponse обрабатывает ответ пользователя
func (r *ReleaseAgent) ProcessUserResponse(ctx context.Context, sessionID, field, value string) (*ValidationResult, error) {
	session, exists := r.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	// Находим соответствующий запрос
	var request *DataCollectionRequest
	for _, req := range session.PendingRequests {
		if req.Field == field {
			request = req
			break
		}
	}

	if request == nil {
		return nil, fmt.Errorf("field not found in pending requests")
	}

	// Валидируем ответ
	validation := r.validateResponse(value, request)
	if !validation.Valid {
		return validation, nil
	}

	// Сохраняем валидный ответ
	session.CollectedResponses[field] = value
	session.UpdatedAt = time.Now()

	// Удаляем обработанный запрос из pending
	newPending := []*DataCollectionRequest{}
	for _, req := range session.PendingRequests {
		if req.Field != field {
			newPending = append(newPending, req)
		}
	}
	session.PendingRequests = newPending

	log.Printf("✅ Collected response for field '%s' in session %s", field, sessionID)

	// Проверяем, все ли данные собраны
	if len(session.PendingRequests) == 0 {
		log.Printf("🎯 All data collected for session %s", sessionID)

		// Проверяем, это retry с теми же данными или новый сбор данных
		if session.Status == "retry_needed" && session.PreviousResponses != nil {
			// Сравниваем текущие и предыдущие ответы
			sameValues := r.compareResponseValues(session.CollectedResponses, session.PreviousResponses)
			if sameValues {
				log.Printf("🔄 User confirmed same values, retrying publication for session %s", sessionID)
				session.Status = "publishing"
				go func() {
					if err := r.autoPublishToRuStore(ctx, session); err != nil {
						log.Printf("❌ Retry publication still failed for session %s: %v", sessionID, err)
						session.Status = "failed"
						session.UpdatedAt = time.Now()
					}
				}()
				return validation, nil
			}
		}

		// Обычный процесс публикации (первый раз или с измененными данными)
		log.Printf("🎯 Starting auto-publication for session %s", sessionID)
		go func() {
			if err := r.processCompletedSession(ctx, session); err != nil {
				log.Printf("❌ Auto-publication failed for session %s: %v", sessionID, err)
				// Не устанавливаем статус "failed" здесь, так как handlePublicationError
				// может инициировать retry процесс
			}
		}()
	}

	return validation, nil
}

// validateResponse валидирует ответ пользователя (обновлено для API v1)
func (r *ReleaseAgent) validateResponse(value string, request *DataCollectionRequest) *ValidationResult {
	value = strings.TrimSpace(value)

	// Проверка на обязательность
	if request.Required && value == "" {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: fmt.Sprintf("Поле '%s' обязательно для заполнения", request.DisplayName),
		}
	}

	// Если поле не обязательное и пустое - это валидно
	if !request.Required && value == "" {
		return &ValidationResult{Valid: true}
	}

	// Проверка максимальной длины
	if request.MaxLength > 0 && len(value) > request.MaxLength {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: fmt.Sprintf("Поле '%s' превышает максимальную длину %d символов (текущая: %d)", request.DisplayName, request.MaxLength, len(value)),
			Suggestions:  []string{fmt.Sprintf("Сократите текст до %d символов", request.MaxLength)},
		}
	}

	// Валидация по типу
	switch request.ValidationType {
	case "numeric":
		if _, err := strconv.Atoi(value); err != nil {
			return &ValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("'%s' должно быть числом", request.DisplayName),
				Suggestions:  []string{"Введите число, например: 0"},
			}
		}

	case "url":
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return &ValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("'%s' должно быть корректным URL", request.DisplayName),
				Suggestions:  []string{"Начните с http:// или https://", "Пример: https://example.com/privacy"},
			}
		}

	case "enum":
		// Проверка что значение есть в списке допустимых
		validValue := false
		valueUpper := strings.ToUpper(value)
		for _, validVal := range request.ValidValues {
			if strings.ToUpper(validVal) == valueUpper {
				validValue = true
				break
			}
		}
		if !validValue {
			return &ValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("Некорректное значение для '%s'. Допустимые значения: %s", request.DisplayName, strings.Join(request.ValidValues, ", ")),
				Suggestions:  request.ValidValues,
			}
		}

	case "categories":
		// Парсим категории
		categories := strings.Split(value, ",")
		for i, cat := range categories {
			categories[i] = strings.TrimSpace(cat)
		}

		// Проверка количества
		if request.MaxCategories > 0 && len(categories) > request.MaxCategories {
			return &ValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("Максимум %d категорий, указано: %d", request.MaxCategories, len(categories)),
				Suggestions:  []string{"Выберите не более 2 категорий через запятую"},
			}
		}

		// Проверка на пустые категории
		for _, cat := range categories {
			if cat == "" {
				return &ValidationResult{
					Valid:        false,
					ErrorMessage: "Категории не могут быть пустыми",
					Suggestions:  []string{"Пример: games,entertainment или utilities,productivity"},
				}
			}
		}
	}

	// Валидация по паттерну
	if request.Pattern != "" {
		matched, err := regexp.MatchString(request.Pattern, value)
		if err != nil || !matched {
			return &ValidationResult{
				Valid:        false,
				ErrorMessage: fmt.Sprintf("'%s' не соответствует требуемому формату", request.DisplayName),
				Suggestions:  request.Suggestions,
			}
		}
	}

	return &ValidationResult{Valid: true}
}

// IsReadyForPublishing проверяет готовность к публикации
func (r *ReleaseAgent) IsReadyForPublishing(sessionID string) bool {
	session, exists := r.sessions[sessionID]
	if !exists {
		return false
	}

	// Проверяем что все обязательные поля собраны (обновлено: без RuStore креденшалов)
	requiredFields := []string{"package_name", "app_name", "app_type", "categories", "age_legal"}

	for _, field := range requiredFields {
		if _, exists := session.CollectedResponses[field]; !exists {
			return false
		}
	}

	return true
}

// BuildFinalReleaseData строит финальную структуру данных для релиза
func (r *ReleaseAgent) BuildFinalReleaseData(sessionID string) (*ReleaseData, error) {
	session, exists := r.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	if !r.IsReadyForPublishing(sessionID) {
		return nil, fmt.Errorf("not all required data collected")
	}

	releaseData := session.ReleaseData
	if releaseData == nil {
		return nil, fmt.Errorf("no release data available")
	}

	// Заполняем RuStore данные из собранных ответов (без креденшалов - используем RUSTORE_KEY)
	// Креденшалы берутся автоматически из .env
	releaseData.RuStoreData.PackageName = session.CollectedResponses["package_name"]
	releaseData.RuStoreData.AppName = session.CollectedResponses["app_name"]
	releaseData.RuStoreData.AppType = session.CollectedResponses["app_type"]
	releaseData.RuStoreData.AgeLegal = session.CollectedResponses["age_legal"]

	// Парсим категории
	if categoriesStr, exists := session.CollectedResponses["categories"]; exists && categoriesStr != "" {
		categories := strings.Split(categoriesStr, ",")
		for i, cat := range categories {
			categories[i] = strings.TrimSpace(cat)
		}
		releaseData.RuStoreData.Categories = categories
	}

	// Опциональные поля
	if shortDesc, exists := session.CollectedResponses["short_description"]; exists && shortDesc != "" {
		releaseData.RuStoreData.ShortDescription = shortDesc
	}
	if fullDesc, exists := session.CollectedResponses["full_description"]; exists && fullDesc != "" {
		releaseData.RuStoreData.FullDescription = fullDesc
	}
	if whatsNew, exists := session.CollectedResponses["whats_new"]; exists && whatsNew != "" {
		releaseData.RuStoreData.WhatsNew = whatsNew
	}
	if moderInfo, exists := session.CollectedResponses["moder_info"]; exists && moderInfo != "" {
		releaseData.RuStoreData.ModerInfo = moderInfo
	}
	if priceStr, exists := session.CollectedResponses["price_value"]; exists && priceStr != "" {
		if price, err := strconv.Atoi(priceStr); err == nil {
			releaseData.RuStoreData.PriceValue = price
		}
	}
	if publishType, exists := session.CollectedResponses["publish_type"]; exists && publishType != "" {
		releaseData.RuStoreData.PublishType = publishType
	}

	releaseData.Status = "ready_for_publishing"

	return releaseData, nil
}

// CompleteSession завершает сессию
func (r *ReleaseAgent) CompleteSession(sessionID string, status string) {
	if session, exists := r.sessions[sessionID]; exists {
		session.Status = status
		session.UpdatedAt = time.Now()

		// Через некоторое время можно удалить завершенную сессию
		// Для демонстрации оставляем в памяти
	}
}

// GetSessionSummary возвращает краткую сводку по сессии
func (r *ReleaseAgent) GetSessionSummary(sessionID string) string {
	session, exists := r.sessions[sessionID]
	if !exists {
		return "Сессия не найдена"
	}

	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("📋 **Сессия AI Release:** %s\n", sessionID))
	summary.WriteString(fmt.Sprintf("⏰ **Создана:** %s\n", session.CreatedAt.Format("2006-01-02 15:04")))
	summary.WriteString(fmt.Sprintf("📊 **Статус:** %s\n\n", session.Status))

	// Статус агентов
	for name, status := range session.AgentStatuses {
		icon := "🔄"
		if status.Status == "completed" {
			icon = "✅"
		} else if status.Status == "failed" {
			icon = "❌"
		}
		summary.WriteString(fmt.Sprintf("%s **%s:** %s (%d%%)\n", icon, name, status.Message, status.Progress))
	}

	// Прогресс сбора данных
	totalRequests := len(session.PendingRequests) + len(session.CollectedResponses)
	collectedCount := len(session.CollectedResponses)

	if totalRequests > 0 {
		summary.WriteString(fmt.Sprintf("\n📝 **Сбор данных:** %d/%d полей заполнено\n", collectedCount, totalRequests))
	}

	return summary.String()
}

// 🤖 Умные методы генерации предложений для полей RuStore API v1

// generateAppNameSuggestions генерирует предложения для названия приложения (max 5 символов)
func (r *ReleaseAgent) generateAppNameSuggestions(session *ReleaseSession) []string {
	suggestions := []string{}

	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		repoName := session.ReleaseData.GitHubData.RepoName

		// Умное сокращение названия репозитория до 5 символов
		if len(repoName) <= 5 {
			suggestions = append(suggestions, repoName)
		} else {
			// Убираем общие слова и сокращаем
			cleaned := strings.ToLower(repoName)
			cleaned = strings.ReplaceAll(cleaned, "snake", "Snk")
			cleaned = strings.ReplaceAll(cleaned, "game", "G")
			cleaned = strings.ReplaceAll(cleaned, "app", "")
			cleaned = strings.ReplaceAll(cleaned, "-", "")

			if len(cleaned) <= 5 {
				suggestions = append(suggestions, strings.Title(cleaned))
			} else {
				// Берем первые 5 символов
				suggestions = append(suggestions, strings.Title(cleaned[:5]))
			}
		}
	}

	// Дополнительные универсальные предложения
	suggestions = append(suggestions, "Game", "App", "MyApp")

	return suggestions
}

// detectAppType определяет тип приложения на основе GitHub данных
func (r *ReleaseAgent) detectAppType(session *ReleaseSession) []string {
	if session.ReleaseData == nil || session.ReleaseData.GitHubData == nil {
		return []string{"MAIN"}
	}

	repoName := strings.ToLower(session.ReleaseData.GitHubData.RepoName)
	description := strings.ToLower(session.ReleaseData.GitHubData.Description)

	// Ключевые слова для игр
	gameKeywords := []string{"game", "snake", "puzzle", "arcade", "racing", "adventure", "rpg", "strategy"}

	for _, keyword := range gameKeywords {
		if strings.Contains(repoName, keyword) || strings.Contains(description, keyword) {
			return []string{"GAMES"}
		}
	}

	return []string{"MAIN"}
}

// generateCategorySuggestions генерирует предложения категорий
func (r *ReleaseAgent) generateCategorySuggestions(session *ReleaseSession) []string {
	suggestions := []string{}

	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		repoName := strings.ToLower(session.ReleaseData.GitHubData.RepoName)
		description := strings.ToLower(session.ReleaseData.GitHubData.Description)

		// Детекция игровых категорий
		if strings.Contains(repoName, "snake") {
			suggestions = append(suggestions, "arcade,puzzle")
		} else if strings.Contains(repoName, "game") {
			suggestions = append(suggestions, "games,entertainment")
		}

		// Детекция по описанию
		if strings.Contains(description, "productivity") {
			suggestions = append(suggestions, "productivity,utilities")
		} else if strings.Contains(description, "social") {
			suggestions = append(suggestions, "social,communication")
		}
	}

	// Универсальные предложения
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "utilities,productivity", "entertainment,lifestyle")
	}

	return suggestions
}

// detectAgeRating определяет возрастной рейтинг на основе контента
func (r *ReleaseAgent) detectAgeRating(session *ReleaseSession) []string {
	if session.ReleaseData == nil || session.ReleaseData.GitHubData == nil {
		return []string{"12+"}
	}

	repoName := strings.ToLower(session.ReleaseData.GitHubData.RepoName)

	// Простые игры обычно 0+ или 6+
	if strings.Contains(repoName, "snake") || strings.Contains(repoName, "puzzle") {
		return []string{"6+"}
	}

	// Игры обычно 12+
	if strings.Contains(repoName, "game") {
		return []string{"12+"}
	}

	// По умолчанию для приложений
	return []string{"12+"}
}

// generateShortDescriptionSuggestions генерирует краткие описания (max 80)
func (r *ReleaseAgent) generateShortDescriptionSuggestions(session *ReleaseSession) []string {
	suggestions := []string{}

	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		description := session.ReleaseData.GitHubData.Description

		if description != "" && len(description) <= 80 {
			suggestions = append(suggestions, description)
		} else if description != "" {
			// Обрезаем до 80 символов
			shortDesc := description
			if len(shortDesc) > 77 {
				shortDesc = shortDesc[:77] + "..."
			}
			suggestions = append(suggestions, shortDesc)
		}

		repoName := session.ReleaseData.GitHubData.RepoName
		if strings.Contains(strings.ToLower(repoName), "snake") {
			suggestions = append(suggestions, "Классическая игра Змейка для мобильных устройств")
		}
	}

	// Универсальные предложения
	suggestions = append(suggestions, "Мобильное приложение для повседневного использования")

	return suggestions
}

// generateFullDescriptionSuggestions генерирует полные описания из README (max 4000)
func (r *ReleaseAgent) generateFullDescriptionSuggestions(session *ReleaseSession) []string {
	suggestions := []string{}

	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		// Используем README как основу для полного описания
		readme := session.ReleaseData.GitHubData.ReadmeContent
		if readme != "" {
			// Очищаем markdown разметку и обрезаем до 4000 символов
			cleanedReadme := strings.ReplaceAll(readme, "#", "")
			cleanedReadme = strings.ReplaceAll(cleanedReadme, "*", "")
			cleanedReadme = strings.ReplaceAll(cleanedReadme, "`", "")
			cleanedReadme = strings.TrimSpace(cleanedReadme)

			if len(cleanedReadme) <= 4000 {
				suggestions = append(suggestions, cleanedReadme)
			} else {
				suggestions = append(suggestions, cleanedReadme[:3997]+"...")
			}
		}
	}

	// Универсальное предложение
	suggestions = append(suggestions, "Удобное мобильное приложение с простым интерфейсом и полезными функциями.")

	return suggestions
}

// generateModeratorInfoSuggestions генерирует комментарии для модераторов (max 180)
func (r *ReleaseAgent) generateModeratorInfoSuggestions(session *ReleaseSession) []string {
	suggestions := []string{}

	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		repoName := session.ReleaseData.GitHubData.RepoName

		if strings.Contains(strings.ToLower(repoName), "game") {
			suggestions = append(suggestions, "Игровое приложение без рекламы и встроенных покупок. Подходит для всех возрастов.")
		}
	}

	// Универсальные предложения
	suggestions = append(suggestions,
		"Приложение не содержит рекламы и встроенных покупок.",
		"Стабильная версия, готовая к публикации.",
		"Приложение протестировано и соответствует требованиям платформы.",
	)

	return suggestions
}

// detectPackageName автоматически определяет package name из GitHub репозитория
func (r *ReleaseAgent) detectPackageName(ctx context.Context, session *ReleaseSession) string {
	if session.ReleaseData == nil || session.ReleaseData.GitHubData == nil {
		return ""
	}

	log.Printf("🔍 Analyzing GitHub repository for package name detection...")

	// Используем LLM для анализа структуры проекта и определения package name
	context := fmt.Sprintf(`
Проект: %s
Описание: %s
Основной язык: %s
Теги: %s

README фрагмент: %s

Задача: Определи Android package name для этого приложения.
Ответ должен быть в формате: com.company.appname
Если не можешь точно определить, верни пустую строку.
`,
		session.ReleaseData.GitHubData.RepoName,
		session.ReleaseData.GitHubData.Description,
		session.ReleaseData.GitHubData.PrimaryLanguage,
		strings.Join(session.ReleaseData.GitHubData.Topics, ", "),
		truncateString(session.ReleaseData.GitHubData.ReadmeContent, 500),
	)

	messages := []llm.Message{
		{Role: "system", Content: context},
		{Role: "user", Content: "Определи package name или верни пустую строку"},
	}

	response, err := r.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ LLM package name detection failed: %v", err)
		return ""
	}

	// Проверяем формат package name
	packageName := strings.TrimSpace(response.Content)
	matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`, packageName)
	if matched {
		log.Printf("✅ Package name detected via LLM: %s", packageName)
		return packageName
	}

	// Fallback: генерируем на основе названия репозитория
	repoName := strings.ToLower(session.ReleaseData.GitHubData.RepoName)
	repoName = strings.ReplaceAll(repoName, "-", "")
	repoName = strings.ReplaceAll(repoName, "_", "")

	if repoName != "" {
		packageName = fmt.Sprintf("com.example.%s", repoName)
		log.Printf("💡 Generated fallback package name: %s", packageName)
		return packageName
	}

	return ""
}

// getExistingRuStoreAppData получает существующие данные приложения из RuStore API
func (r *ReleaseAgent) getExistingRuStoreAppData(ctx context.Context, session *ReleaseSession, packageName string) *rustore.RuStoreAppInfo {
	if r.rustoreClient == nil {
		log.Printf("⚠️ RuStore client not available")
		return nil
	}

	log.Printf("🔍 Searching for existing RuStore app with package: %s", packageName)

	// Ищем приложение по package name
	params := rustore.GetAppListParams{
		AppPackage: packageName,
		PageSize:   10,
	}

	result := r.rustoreClient.GetAppList(ctx, params)
	if !result.Success {
		log.Printf("❌ Failed to get RuStore app list: %s", result.Message)
		return nil
	}

	// Ищем точное совпадение по package name
	for _, app := range result.Applications {
		if app.PackageName == packageName {
			log.Printf("✅ Found existing RuStore app: %s (%s)", app.Name, app.AppID)
			return &app
		}
	}

	log.Printf("ℹ️ No existing RuStore app found for package: %s", packageName)
	return nil
}

// autoFillFromExistingApp автоматически заполняет поля из существующего приложения RuStore
func (r *ReleaseAgent) autoFillFromExistingApp(session *ReleaseSession, appInfo *rustore.RuStoreAppInfo) {
	if appInfo == nil {
		return
	}

	log.Printf("📝 Auto-filling data from existing RuStore app: %s", appInfo.Name)

	// Заполняем только если поля еще не заполнены
	if _, exists := session.CollectedResponses["app_name"]; !exists && appInfo.Name != "" {
		// Обрезаем до 5 символов для нового API
		appName := appInfo.Name
		if len(appName) > 5 {
			appName = appName[:5]
		}
		session.CollectedResponses["app_name"] = appName
		log.Printf("✅ Auto-filled app_name: %s", appName)
	}

	if _, exists := session.CollectedResponses["app_type"]; !exists && appInfo.AppType != "" {
		session.CollectedResponses["app_type"] = appInfo.AppType
		log.Printf("✅ Auto-filled app_type: %s", appInfo.AppType)
	}

	if _, exists := session.CollectedResponses["categories"]; !exists && len(appInfo.Categories) > 0 {
		// Ограничиваем до 2 категорий
		categories := appInfo.Categories
		if len(categories) > 2 {
			categories = categories[:2]
		}
		session.CollectedResponses["categories"] = strings.Join(categories, ",")
		log.Printf("✅ Auto-filled categories: %s", strings.Join(categories, ","))
	}

	if _, exists := session.CollectedResponses["age_legal"]; !exists && appInfo.AgeLegal != "" {
		session.CollectedResponses["age_legal"] = appInfo.AgeLegal
		log.Printf("✅ Auto-filled age_legal: %s", appInfo.AgeLegal)
	}
}

// enhanceDataCollectionWithAutomation улучшает сбор данных автоматизацией
func (r *ReleaseAgent) enhanceDataCollectionWithAutomation(ctx context.Context, session *ReleaseSession) {
	// Шаг 1: Автоматически определяем package name
	packageName := r.detectPackageName(ctx, session)
	if packageName != "" {
		session.CollectedResponses["package_name"] = packageName
		log.Printf("✅ Package name auto-detected: %s", packageName)

		// Удаляем запрос package_name из pending requests, если он там есть
		filteredRequests := []*DataCollectionRequest{}
		for _, req := range session.PendingRequests {
			if req.Field != "package_name" {
				filteredRequests = append(filteredRequests, req)
			}
		}
		session.PendingRequests = filteredRequests
	} else {
		// Используем package_name из уже собранных ответов (если есть)
		packageName = session.CollectedResponses["package_name"]
		if packageName == "" {
			return
		}
	}

	// Шаг 2: Ищем существующее приложение в RuStore
	existingApp := r.getExistingRuStoreAppData(ctx, session, packageName)

	// Шаг 3: Автоматически заполняем поля из существующего приложения
	if existingApp != nil {
		r.autoFillFromExistingApp(session, existingApp)
		log.Printf("🎯 Enhanced session with existing RuStore app data")
	} else {
		log.Printf("💡 No existing app found, using AI-generated suggestions")
	}

	session.UpdatedAt = time.Now()
}

// compareResponseValues сравнивает два набора ответов пользователя
func (r *ReleaseAgent) compareResponseValues(current, previous map[string]string) bool {
	if len(current) != len(previous) {
		return false
	}

	for key, currentValue := range current {
		previousValue, exists := previous[key]
		if !exists || currentValue != previousValue {
			return false
		}
	}

	return true
}

// truncateString обрезает строку до указанной длины
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
