package release

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-chatter/internal/llm"
)

// AIFieldAnalysis результат анализа ИИ для определения недостающих полей
type AIFieldAnalysis struct {
	Analysis       string            `json:"analysis"`
	RequiredFields []AIRequiredField `json:"required_fields"`
}

// AIRequiredField поле которое нужно запросить у пользователя по мнению ИИ
type AIRequiredField struct {
	Field       string   `json:"field"`
	Reason      string   `json:"reason"`
	Priority    string   `json:"priority"` // "high", "medium", "low"
	Suggestions []string `json:"suggestions"`
}

// analyzeAndGenerateRequests использует LLM для анализа собранных данных и определения недостающих полей
func (r *ReleaseAgent) analyzeAndGenerateRequests(ctx context.Context, session *ReleaseSession) ([]*DataCollectionRequest, error) {
	// Подготавливаем контекст с собранными данными
	analysisContext := r.buildAnalysisContext(session)

	// Составляем промпт для LLM анализа
	systemPrompt := `Ты эксперт по публикации Android приложений в RuStore. 
Проанализируй собранные данные о релизе и определи, какие поля ДЕЙСТВИТЕЛЬНО нужно запросить у пользователя.

КРИТЕРИИ АНАЛИЗА:
1. Технические поля (package_name, app_name, app_type, categories, age_legal) - пытайся определить автоматически
2. Пользовательские поля (описания, changelog) - всегда нужно спрашивать, если нет качественных данных
3. Опциональные поля - запрашивай только если есть специфическая потребность

ВОЗМОЖНЫЕ ПОЛЯ RUSTORE API v1:
- package_name (обязательно, формат: com.company.app)
- app_name (обязательно, макс 5 символов)
- app_type (обязательно: GAMES или MAIN)  
- categories (обязательно, макс 2)
- age_legal (обязательно: 0+, 6+, 12+, 16+, 18+)
- short_description (опционально, макс 80)
- full_description (опционально, макс 4000)
- whats_new (опционально, макс 5000) 
- moder_info (опционально, макс 180)
- price_value (опционально, копейки)
- publish_type (опционально: MANUAL, INSTANTLY, DELAYED)

ОТВЕЧАЙ В JSON ФОРМАТЕ:
{
  "analysis": "краткий анализ ситуации",
  "required_fields": [
    {
      "field": "название_поля",
      "reason": "почему нужно спросить",
      "priority": "high/medium/low",
      "suggestions": ["предложение1", "предложение2"]
    }
  ]
}`

	userPrompt := fmt.Sprintf("Проанализируй данные релиза и определи недостающие поля:\n\n%s", analysisContext)

	// Отправляем запрос к LLM
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := r.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	log.Printf("🤖 LLM Analysis Response: %s", response.Content)

	// Парсим ответ LLM
	analysisResult, err := r.parseLLMAnalysis(response.Content)
	if err != nil {
		log.Printf("⚠️ Failed to parse LLM analysis: %v", err)
		log.Printf("🔍 Raw LLM response: %s", response.Content)
		return nil, fmt.Errorf("failed to parse LLM analysis: %w", err)
	}

	log.Printf("📊 AI Analysis: %s", analysisResult.Analysis)
	log.Printf("📝 AI found %d fields to request", len(analysisResult.RequiredFields))

	// Преобразуем результат анализа в запросы
	return r.convertAnalysisToRequests(session, analysisResult)
}

// buildAnalysisContext собирает контекст для анализа ИИ
func (r *ReleaseAgent) buildAnalysisContext(session *ReleaseSession) string {
	var context strings.Builder

	context.WriteString("=== СОБРАННЫЕ ДАННЫЕ ===\n\n")

	// GitHub данные
	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		github := session.ReleaseData.GitHubData
		context.WriteString(fmt.Sprintf("GitHub Repository: %s\n", github.RepoName))
		context.WriteString(fmt.Sprintf("Description: %s\n", github.Description))
		context.WriteString(fmt.Sprintf("Primary Language: %s\n", github.PrimaryLanguage))
		context.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(github.Topics, ", ")))
		if github.ReadmeContent != "" {
			context.WriteString(fmt.Sprintf("README (fragment): %s\n", truncateString(github.ReadmeContent, 300)))
		}
		context.WriteString("\n")
	}

	// Релиз данные
	if session.ReleaseData != nil && session.ReleaseData.GitHubRelease != nil {
		release := session.ReleaseData.GitHubRelease
		context.WriteString(fmt.Sprintf("Release Tag: %s\n", release.TagName))
		context.WriteString(fmt.Sprintf("Release Name: %s\n", release.Name))
		if release.Body != "" {
			context.WriteString(fmt.Sprintf("Release Notes: %s\n", truncateString(release.Body, 200)))
		}
		context.WriteString("\n")
	}

	// Уже собранные ответы пользователя
	if len(session.CollectedResponses) > 0 {
		context.WriteString("УЖЕ ЗАПОЛНЕННЫЕ ПОЛЯ:\n")
		for field, value := range session.CollectedResponses {
			context.WriteString(fmt.Sprintf("- %s: %s\n", field, value))
		}
		context.WriteString("\n")
	}

	// AI предложения по изменениям
	if session.ReleaseData != nil && len(session.ReleaseData.RuStoreData.SuggestedWhatsNew) > 0 {
		context.WriteString("AI GENERATED CHANGELOG:\n")
		for _, suggestion := range session.ReleaseData.RuStoreData.SuggestedWhatsNew {
			context.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
		context.WriteString("\n")
	}

	// Коммиты
	if session.ReleaseData != nil && len(session.ReleaseData.CommitsSinceLastRelease) > 0 {
		context.WriteString("RECENT COMMITS:\n")
		for i, commit := range session.ReleaseData.CommitsSinceLastRelease {
			if i >= 3 { // Показываем только первые 3 коммита
				break
			}
			context.WriteString(fmt.Sprintf("- %s: %s\n", commit.ShortSHA(), commit.Message))
		}
		context.WriteString("\n")
	}

	context.WriteString("=== ЗАДАЧА ===\n")
	context.WriteString("Определи минимальный набор полей для запроса у пользователя для публикации в RuStore.\n")
	context.WriteString("Приоритет: автоматизация > качество > полнота.")

	return context.String()
}

// parseLLMAnalysis парсит ответ LLM в структурированный формат
func (r *ReleaseAgent) parseLLMAnalysis(content string) (*AIFieldAnalysis, error) {
	// Ищем JSON в ответе (может быть обёрнут в markdown блок)
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	jsonContent := content[jsonStart : jsonEnd+1]

	var analysis AIFieldAnalysis
	if err := json.Unmarshal([]byte(jsonContent), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &analysis, nil
}

// convertAnalysisToRequests преобразует результат AI анализа в запросы
func (r *ReleaseAgent) convertAnalysisToRequests(session *ReleaseSession, analysis *AIFieldAnalysis) ([]*DataCollectionRequest, error) {
	requests := []*DataCollectionRequest{}

	for _, aiField := range analysis.RequiredFields {
		// Пропускаем поля которые уже заполнены
		if _, exists := session.CollectedResponses[aiField.Field]; exists {
			log.Printf("⏭️ Skipping already filled field: %s", aiField.Field)
			continue
		}

		request := &DataCollectionRequest{
			Field:       aiField.Field,
			DisplayName: r.getFieldDisplayName(aiField.Field),
			Description: r.getFieldDescription(aiField.Field) + " (" + aiField.Reason + ")",
			Required:    aiField.Priority == "high",
			Suggestions: aiField.Suggestions,
		}

		// Устанавливаем тип валидации и ограничения
		r.setFieldValidation(request, aiField.Field)

		// Дополняем предложения если они не были предоставлены AI
		if len(request.Suggestions) == 0 {
			request.Suggestions = r.generateFieldSuggestions(session, aiField.Field)
		}

		requests = append(requests, request)
		log.Printf("📝 Added AI-determined field request: %s (priority: %s)", aiField.Field, aiField.Priority)
	}

	return requests, nil
}

// generateFallbackRequests создает минимальный набор запросов в случае сбоя AI анализа
func (r *ReleaseAgent) generateFallbackRequests(session *ReleaseSession) []*DataCollectionRequest {
	requests := []*DataCollectionRequest{}

	// Критически важные поля для RuStore API v1
	criticalFields := []string{"package_name", "app_name", "app_type", "categories", "age_legal"}

	for _, field := range criticalFields {
		if _, exists := session.CollectedResponses[field]; !exists {
			request := &DataCollectionRequest{
				Field:       field,
				DisplayName: r.getFieldDisplayName(field),
				Description: r.getFieldDescription(field),
				Required:    true,
				Suggestions: r.generateFieldSuggestions(session, field),
			}
			r.setFieldValidation(request, field)
			requests = append(requests, request)
		}
	}

	// Обязательные пользовательские поля
	userFields := []string{"whats_new"}
	for _, field := range userFields {
		request := &DataCollectionRequest{
			Field:       field,
			DisplayName: r.getFieldDisplayName(field),
			Description: r.getFieldDescription(field),
			Required:    false,
			Suggestions: r.generateFieldSuggestions(session, field),
		}
		r.setFieldValidation(request, field)
		requests = append(requests, request)
	}

	log.Printf("🔄 Generated %d fallback requests", len(requests))
	return requests
}

// getFieldDisplayName возвращает человекочитаемое название поля
func (r *ReleaseAgent) getFieldDisplayName(field string) string {
	displayNames := map[string]string{
		"package_name":      "Package Name",
		"app_name":          "Название приложения",
		"app_type":          "Тип приложения",
		"categories":        "Категории",
		"age_legal":         "Возрастная категория",
		"short_description": "Краткое описание",
		"full_description":  "Полное описание",
		"whats_new":         "Что нового",
		"moder_info":        "Комментарий для модератора",
		"price_value":       "Цена",
		"publish_type":      "Тип публикации",
	}

	if name, exists := displayNames[field]; exists {
		return name
	}
	return field
}

// getFieldDescription возвращает описание поля
func (r *ReleaseAgent) getFieldDescription(field string) string {
	descriptions := map[string]string{
		"package_name":      "Имя пакета приложения (например: com.company.app)",
		"app_name":          "Краткое название (макс 5 символов)",
		"app_type":          "Тип приложения: GAMES или MAIN",
		"categories":        "Категории приложения (макс 2, через запятую)",
		"age_legal":         "Возрастные ограничения: 0+, 6+, 12+, 16+, 18+",
		"short_description": "Краткое описание приложения (макс 80 символов)",
		"full_description":  "Подробное описание приложения (макс 4000 символов)",
		"whats_new":         "Описание изменений в этой версии (макс 5000 символов)",
		"moder_info":        "Дополнительная информация для модераторов (макс 180 символов)",
		"price_value":       "Цена в копейках (0 для бесплатного приложения)",
		"publish_type":      "Тип публикации: MANUAL, INSTANTLY, DELAYED",
	}

	if desc, exists := descriptions[field]; exists {
		return desc
	}
	return field
}

// setFieldValidation устанавливает параметры валидации для поля
func (r *ReleaseAgent) setFieldValidation(request *DataCollectionRequest, field string) {
	switch field {
	case "package_name":
		request.ValidationType = "text"
		request.Pattern = `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`
	case "app_name":
		request.ValidationType = "text"
		request.MaxLength = 5
	case "app_type":
		request.ValidationType = "enum"
		request.ValidValues = []string{"GAMES", "MAIN"}
	case "categories":
		request.ValidationType = "categories"
		request.MaxCategories = 2
	case "age_legal":
		request.ValidationType = "enum"
		request.ValidValues = []string{"0+", "6+", "12+", "16+", "18+"}
	case "short_description":
		request.ValidationType = "text"
		request.MaxLength = 80
	case "full_description":
		request.ValidationType = "text"
		request.MaxLength = 4000
	case "whats_new":
		request.ValidationType = "text"
		request.MaxLength = 5000
	case "moder_info":
		request.ValidationType = "text"
		request.MaxLength = 180
	case "price_value":
		request.ValidationType = "numeric"
		request.Pattern = `^\d+$`
	case "publish_type":
		request.ValidationType = "enum"
		request.ValidValues = []string{"MANUAL", "INSTANTLY", "DELAYED"}
	default:
		request.ValidationType = "text"
	}
}

// validateCompleteness использует LLM для валидации полноты собранных данных
func (r *ReleaseAgent) validateCompleteness(ctx context.Context, session *ReleaseSession) (*ValidationResult, error) {
	log.Printf("🔍 LLM validating data completeness for RuStore publication...")

	// Подготавливаем контекст с всеми собранными данными
	validationContext := r.buildValidationContext(session)

	systemPrompt := `Ты эксперт по публикации Android приложений в RuStore API v1.
Проанализируй собранные данные и определи, готовы ли они для публикации в RuStore.

ОБЯЗАТЕЛЬНЫЕ ПОЛЯ RuStore API v1:
- package_name (формат: com.company.app) 
- app_name (макс 5 символов)
- app_type (GAMES или MAIN)
- categories (макс 2 категории)
- age_legal (0+, 6+, 12+, 16+, 18+)

КРИТЕРИИ ГОТОВНОСТИ:
1. Все обязательные поля заполнены
2. Данные соответствуют требованиям API
3. Качество описаний достаточное для публикации
4. Нет противоречий в данных

ОТВЕЧАЙ В JSON ФОРМАТЕ:
{
  "ready_for_publication": true/false,
  "analysis": "детальный анализ готовности",
  "missing_critical": ["список_критически_важных_полей"],
  "quality_issues": ["проблемы_качества_данных"],
  "recommendations": ["рекомендации_по_улучшению"]
}`

	userPrompt := fmt.Sprintf("Проанализируй готовность данных для публикации в RuStore:\n\n%s", validationContext)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := r.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM validation failed: %w", err)
	}

	log.Printf("🤖 LLM Validation Response: %s", response.Content)

	// Парсим результат валидации
	validation, err := r.parseValidationResult(response.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse validation result: %w", err)
	}

	return validation, nil
}

// buildValidationContext собирает контекст для валидации LLM
func (r *ReleaseAgent) buildValidationContext(session *ReleaseSession) string {
	var context strings.Builder

	context.WriteString("=== СОБРАННЫЕ ДАННЫЕ ДЛЯ ПУБЛИКАЦИИ ===\n\n")

	// Все собранные ответы пользователя
	context.WriteString("ПОЛЬЗОВАТЕЛЬСКИЕ ДАННЫЕ:\n")
	for field, value := range session.CollectedResponses {
		context.WriteString(fmt.Sprintf("- %s: %s\n", field, value))
	}
	context.WriteString("\n")

	// GitHub контекст
	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		github := session.ReleaseData.GitHubData
		context.WriteString("GITHUB КОНТЕКСТ:\n")
		context.WriteString(fmt.Sprintf("- Repository: %s\n", github.RepoName))
		context.WriteString(fmt.Sprintf("- Description: %s\n", github.Description))
		context.WriteString(fmt.Sprintf("- Language: %s\n", github.PrimaryLanguage))
		context.WriteString("\n")
	}

	// Релиз информация
	if session.ReleaseData != nil && session.ReleaseData.GitHubRelease != nil {
		release := session.ReleaseData.GitHubRelease
		context.WriteString("РЕЛИЗ ИНФОРМАЦИЯ:\n")
		context.WriteString(fmt.Sprintf("- Tag: %s\n", release.TagName))
		context.WriteString(fmt.Sprintf("- Asset Type: %s\n", session.ReleaseData.AssetType))
		context.WriteString("\n")
	}

	context.WriteString("ЗАДАЧА: Определи готовность для автоматической публикации в RuStore")

	return context.String()
}

// parseValidationResult парсит результат LLM валидации
func (r *ReleaseAgent) parseValidationResult(content string) (*ValidationResult, error) {
	// Ищем JSON в ответе
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON found in validation response")
	}

	jsonContent := content[jsonStart : jsonEnd+1]

	var result struct {
		ReadyForPublication bool     `json:"ready_for_publication"`
		Analysis            string   `json:"analysis"`
		MissingCritical     []string `json:"missing_critical"`
		QualityIssues       []string `json:"quality_issues"`
		Recommendations     []string `json:"recommendations"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse validation JSON: %w", err)
	}

	return &ValidationResult{
		Valid:        result.ReadyForPublication,
		ErrorMessage: result.Analysis,
		Suggestions:  result.Recommendations,
	}, nil
}

// autoPublishToRuStore выполняет автоматическую публикацию в RuStore
func (r *ReleaseAgent) autoPublishToRuStore(ctx context.Context, session *ReleaseSession) error {
	log.Printf("🚀 Starting automatic RuStore publication for session %s", session.ID)

	// Строим финальные данные релиза
	releaseData, err := r.BuildFinalReleaseData(session.ID)
	if err != nil {
		return fmt.Errorf("failed to build final release data: %w", err)
	}

	// Обновляем статус сессии
	session.Status = "publishing"
	session.UpdatedAt = time.Now()

	// Симуляция RuStore публикации (временно для тестирования AI Release агента)
	log.Printf("🧪 SIMULATION: Creating RuStore draft for package: %s", releaseData.RuStoreData.PackageName)
	log.Printf("📝 Draft data:")
	log.Printf("   - App Name: %s", releaseData.RuStoreData.AppName)
	log.Printf("   - App Type: %s", releaseData.RuStoreData.AppType)
	log.Printf("   - Categories: %v", releaseData.RuStoreData.Categories)
	log.Printf("   - Age Legal: %s", releaseData.RuStoreData.AgeLegal)
	log.Printf("   - Short Description: %s", releaseData.RuStoreData.ShortDescription)
	log.Printf("   - Full Description: %s", releaseData.RuStoreData.FullDescription)
	log.Printf("   - What's New: %s", releaseData.RuStoreData.WhatsNew)
	log.Printf("   - Moder Info: %s", releaseData.RuStoreData.ModerInfo)
	log.Printf("   - Price Value: %d", releaseData.RuStoreData.PriceValue)
	log.Printf("   - Publish Type: %s", releaseData.RuStoreData.PublishType)

	log.Printf("✅ SIMULATION: Draft created successfully with ID: draft-123456")
	log.Printf("⚠️ SIMULATION: File upload skipped - would upload %s: %s", releaseData.AssetType, releaseData.AndroidAsset.Name)
	log.Printf("✅ SIMULATION: Successfully submitted to RuStore moderation")

	// Обновляем статус на завершенный
	session.Status = "completed"
	session.UpdatedAt = time.Now()

	return nil
}

// processCompletedSession автоматически обрабатывает сессию после сбора всех данных
func (r *ReleaseAgent) processCompletedSession(ctx context.Context, session *ReleaseSession) error {
	log.Printf("🔄 Processing completed data collection for session %s", session.ID)

	// Базовая проверка готовности (без LLM валидации)
	if !r.IsReadyForPublishing(session.ID) {
		return fmt.Errorf("data not ready for publishing")
	}

	log.Printf("✅ Basic validation passed: ready for publication")

	// Автоматическая публикация в RuStore
	if err := r.autoPublishToRuStore(ctx, session); err != nil {
		log.Printf("❌ Auto-publication failed: %v", err)
		return r.handlePublicationError(ctx, session, err)
	}

	log.Printf("🎉 Session %s completed successfully - published to RuStore!", session.ID)
	return nil
}

// handlePublicationError обрабатывает ошибки публикации и запускает recovery процесс
func (r *ReleaseAgent) handlePublicationError(ctx context.Context, session *ReleaseSession, publishError error) error {
	log.Printf("🔧 Handling publication error for session %s: %v", session.ID, publishError)

	// Сохраняем информацию об ошибке
	session.Status = "retry_needed"
	session.LastError = publishError.Error()
	session.RetryCount++
	session.UpdatedAt = time.Now()

	// Сохраняем текущие ответы для сравнения при retry
	if session.PreviousResponses == nil {
		session.PreviousResponses = make(map[string]string)
	}
	for k, v := range session.CollectedResponses {
		session.PreviousResponses[k] = v
	}

	// Простая retry логика без LLM анализа
	retryRequests := r.generateFallbackRetryRequests(session, publishError)

	if len(retryRequests) == 0 {
		log.Printf("🔄 No additional fields needed, retrying publication with same data...")
		// Пытаемся еще раз с теми же данными (возможно временная ошибка)
		session.Status = "publishing"
		return r.autoPublishToRuStore(ctx, session)
	}

	// Устанавливаем новые запросы и переводим в режим ожидания пользователя
	session.PendingRequests = retryRequests
	session.Status = "waiting_user"

	log.Printf("🔄 Generated %d retry requests for session %s", len(retryRequests), session.ID)
	return fmt.Errorf("publication failed, requesting additional fields: %s", publishError.Error())
}

// analyzeErrorAndGenerateRetryRequests анализирует ошибку публикации и генерирует запросы на исправление
func (r *ReleaseAgent) analyzeErrorAndGenerateRetryRequests(ctx context.Context, session *ReleaseSession, publishError error) ([]*DataCollectionRequest, error) {
	log.Printf("🤖 Analyzing publication error with LLM...")

	errorContext := r.buildErrorAnalysisContext(session, publishError)

	systemPrompt := `Ты эксперт по публикации Android приложений в RuStore API v1.
Проанализируй ошибку публикации и определи, какие поля нужно исправить или добавить.

ВОЗМОЖНЫЕ ПРИЧИНЫ ОШИБОК:
1. Неправильный формат данных (package_name, app_name)
2. Нарушение ограничений (длина полей, количество категорий)
3. Недостающие обязательные поля
4. Некорректные значения enum полей
5. Проблемы с описаниями (слишком короткие/длинные)

ПОЛЯ RuStore API v1:
- package_name (формат: com.company.app)
- app_name (макс 5 символов)
- app_type (GAMES или MAIN)  
- categories (макс 2 категории)
- age_legal (0+, 6+, 12+, 16+, 18+)
- short_description (макс 80 символов)
- full_description (макс 4000 символов)
- whats_new (макс 5000 символов)
- moder_info (макс 180 символов)

ОТВЕЧАЙ В JSON ФОРМАТЕ:
{
  "error_analysis": "анализ причины ошибки",
  "retry_strategy": "стратегия исправления",
  "required_fields": [
    {
      "field": "название_поля",
      "reason": "почему нужно исправить это поле",
      "current_issue": "проблема с текущим значением", 
      "suggestions": ["исправленные_варианты"]
    }
  ]
}`

	userPrompt := fmt.Sprintf("Проанализируй ошибку публикации и определи поля для исправления:\n\n%s", errorContext)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := r.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM error analysis failed: %w", err)
	}

	log.Printf("🤖 LLM Error Analysis: %s", response.Content)

	// Парсим анализ ошибки
	errorAnalysis, err := r.parseErrorAnalysis(response.Content)
	if err != nil {
		log.Printf("⚠️ Failed to parse error analysis, using fallback: %v", err)
		return r.generateFallbackRetryRequests(session, publishError), nil
	}

	// Преобразуем анализ в запросы
	return r.convertErrorAnalysisToRequests(session, errorAnalysis), nil
}

// buildErrorAnalysisContext строит контекст для анализа ошибки
func (r *ReleaseAgent) buildErrorAnalysisContext(session *ReleaseSession, publishError error) string {
	var context strings.Builder

	context.WriteString("=== ОШИБКА ПУБЛИКАЦИИ ===\n\n")
	context.WriteString(fmt.Sprintf("Error: %s\n", publishError.Error()))
	context.WriteString(fmt.Sprintf("Failed at step: %s\n", session.FailedAtStep))
	context.WriteString(fmt.Sprintf("Retry attempt: %d\n\n", session.RetryCount))

	context.WriteString("ТЕКУЩИЕ ДАННЫЕ:\n")
	for field, value := range session.CollectedResponses {
		context.WriteString(fmt.Sprintf("- %s: '%s'\n", field, value))
	}
	context.WriteString("\n")

	// GitHub контекст
	if session.ReleaseData != nil && session.ReleaseData.GitHubData != nil {
		github := session.ReleaseData.GitHubData
		context.WriteString("КОНТЕКСТ ПРОЕКТА:\n")
		context.WriteString(fmt.Sprintf("- Repository: %s\n", github.RepoName))
		context.WriteString(fmt.Sprintf("- Description: %s\n", github.Description))
		context.WriteString(fmt.Sprintf("- Language: %s\n", github.PrimaryLanguage))
		context.WriteString("\n")
	}

	context.WriteString("ЗАДАЧА: Определи, какие поля нужно исправить для успешной публикации")

	return context.String()
}

// parseErrorAnalysis парсит результат анализа ошибки от LLM
func (r *ReleaseAgent) parseErrorAnalysis(content string) (*ErrorAnalysisResult, error) {
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON found in error analysis")
	}

	jsonContent := content[jsonStart : jsonEnd+1]

	var result ErrorAnalysisResult
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse error analysis JSON: %w", err)
	}

	return &result, nil
}

// convertErrorAnalysisToRequests преобразует анализ ошибки в запросы
func (r *ReleaseAgent) convertErrorAnalysisToRequests(session *ReleaseSession, analysis *ErrorAnalysisResult) []*DataCollectionRequest {
	requests := []*DataCollectionRequest{}

	for _, errorField := range analysis.RequiredFields {
		request := &DataCollectionRequest{
			Field:       errorField.Field,
			DisplayName: r.getFieldDisplayName(errorField.Field),
			Description: fmt.Sprintf("%s. %s", r.getFieldDescription(errorField.Field), errorField.Reason),
			Required:    true,
			Suggestions: errorField.Suggestions,
		}

		// Добавляем информацию о проблеме с текущим значением
		if currentValue, exists := session.CollectedResponses[errorField.Field]; exists {
			request.Description += fmt.Sprintf(" (Текущее значение '%s': %s)", currentValue, errorField.CurrentIssue)
		}

		r.setFieldValidation(request, errorField.Field)
		requests = append(requests, request)

		log.Printf("📝 Added error recovery field: %s (reason: %s)", errorField.Field, errorField.Reason)
	}

	return requests
}

// generateFallbackRetryRequests создает базовые retry запросы при сбое анализа LLM
func (r *ReleaseAgent) generateFallbackRetryRequests(session *ReleaseSession, publishError error) []*DataCollectionRequest {
	requests := []*DataCollectionRequest{}

	errorMsg := strings.ToLower(publishError.Error())

	// Анализируем ошибку простыми правилами
	if strings.Contains(errorMsg, "package") || strings.Contains(errorMsg, "packagename") {
		requests = append(requests, &DataCollectionRequest{
			Field:          "package_name",
			DisplayName:    "Package Name (Исправление)",
			Description:    "Исправьте package name (формат: com.company.app). Ошибка: " + publishError.Error(),
			Required:       true,
			ValidationType: "text",
			Pattern:        `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`,
			Suggestions:    []string{"com.example.app", "com.mycompany.game"},
		})
	}

	if strings.Contains(errorMsg, "appname") || strings.Contains(errorMsg, "name") {
		requests = append(requests, &DataCollectionRequest{
			Field:          "app_name",
			DisplayName:    "App Name (Исправление)",
			Description:    "Исправьте название приложения (макс 5 символов). Ошибка: " + publishError.Error(),
			Required:       true,
			ValidationType: "text",
			MaxLength:      5,
			Suggestions:    []string{"Game", "App", "Tool"},
		})
	}

	return requests
}

// ErrorAnalysisResult результат анализа ошибки публикации
type ErrorAnalysisResult struct {
	ErrorAnalysis  string       `json:"error_analysis"`
	RetryStrategy  string       `json:"retry_strategy"`
	RequiredFields []ErrorField `json:"required_fields"`
}

// ErrorField поле, которое нужно исправить после ошибки
type ErrorField struct {
	Field        string   `json:"field"`
	Reason       string   `json:"reason"`
	CurrentIssue string   `json:"current_issue"`
	Suggestions  []string `json:"suggestions"`
}

// generateFieldSuggestions генерирует предложения для конкретного поля
func (r *ReleaseAgent) generateFieldSuggestions(session *ReleaseSession, field string) []string {
	switch field {
	case "package_name":
		return []string{"com.example.app", "com.mycompany.game"}
	case "app_name":
		return r.generateAppNameSuggestions(session)
	case "app_type":
		return r.detectAppType(session)
	case "categories":
		return r.generateCategorySuggestions(session)
	case "age_legal":
		return r.detectAgeRating(session)
	case "short_description":
		return r.generateShortDescriptionSuggestions(session)
	case "full_description":
		return r.generateFullDescriptionSuggestions(session)
	case "whats_new":
		if session.ReleaseData != nil {
			return session.ReleaseData.RuStoreData.SuggestedWhatsNew
		}
		return []string{"Исправления ошибок и улучшения производительности"}
	case "moder_info":
		return r.generateModeratorInfoSuggestions(session)
	case "price_value":
		return []string{"0", "9900", "19900"}
	case "publish_type":
		return []string{"MANUAL"}
	default:
		return []string{}
	}
}
