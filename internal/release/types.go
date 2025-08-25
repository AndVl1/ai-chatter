package release

import (
	"time"

	"ai-chatter/internal/github"
)

// ReleaseData содержит все данные для создания релиза
type ReleaseData struct {
	// GitHub данные
	GitHubRelease *github.GitHubRelease      `json:"github_release"`
	AndroidAsset  *github.GitHubReleaseAsset `json:"android_asset"`
	AssetType     string                     `json:"asset_type"` // "AAB" или "APK"

	// GitHub анализ
	CommitsSinceLastRelease []CommitInfo `json:"commits_since_last_release"`
	ChangedFiles            []string     `json:"changed_files"`
	KeyChanges              []string     `json:"key_changes"` // AI-анализ основных изменений

	// GitHub данные для умных предложений
	GitHubData *GitHubProjectData `json:"github_data"`

	// RuStore данные (заполняются пользователем или AI)
	RuStoreData RuStoreReleaseData `json:"rustore_data"`

	// Метаданные
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"` // "collecting", "reviewing", "publishing", "completed", "failed"
	UserSessionID int64     `json:"user_session_id"`
}

// GitHubProjectData данные о проекте из GitHub для умных предложений
type GitHubProjectData struct {
	RepoName        string   `json:"repo_name"`        // Название репозитория
	Description     string   `json:"description"`      // Описание репозитория
	ReadmeContent   string   `json:"readme_content"`   // Содержимое README
	PrimaryLanguage string   `json:"primary_language"` // Основной язык программирования
	Topics          []string `json:"topics"`           // Теги репозитория
}

// RuStoreReleaseData данные для публикации в RuStore API v1
type RuStoreReleaseData struct {
	// 🔐 Поля аутентификации (автоматически из RUSTORE_KEY в .env)
	CompanyID string `json:"company_id,omitempty"` // DEPRECATED: берется из RUSTORE_KEY
	KeyID     string `json:"key_id,omitempty"`     // DEPRECATED: берется из RUSTORE_KEY
	KeySecret string `json:"key_secret,omitempty"` // DEPRECATED: берется из RUSTORE_KEY

	// 📦 Обязательные поля приложения (API v1)
	PackageName string   `json:"package_name"`         // Имя пакета приложения (обязательно)
	AppName     string   `json:"app_name,omitempty"`   // Название приложения (макс 5 символов)
	AppType     string   `json:"app_type,omitempty"`   // Тип: GAMES или MAIN
	Categories  []string `json:"categories,omitempty"` // Категории (макс 2)
	AgeLegal    string   `json:"age_legal,omitempty"`  // Возрастная категория: 0+, 6+, 12+, 16+, 18+

	// 📝 Описательные поля (опциональные)
	ShortDescription string `json:"short_description,omitempty"` // Краткое описание (макс 80)
	FullDescription  string `json:"full_description,omitempty"`  // Полное описание (макс 4000)
	WhatsNew         string `json:"whats_new,omitempty"`         // Что нового (макс 5000)
	ModerInfo        string `json:"moder_info,omitempty"`        // Комментарий для модератора (макс 180)

	// 💰 Коммерческие поля (опциональные)
	PriceValue int `json:"price_value,omitempty"` // Цена в копейках

	// 🏷️ SEO и публикация (опциональные)
	SeoTagIds       []int  `json:"seo_tag_ids,omitempty"`       // SEO теги (макс 5)
	PublishType     string `json:"publish_type,omitempty"`      // MANUAL, INSTANTLY, DELAYED
	PublishDateTime string `json:"publish_date_time,omitempty"` // Дата публикации для DELAYED
	PartialValue    int    `json:"partial_value,omitempty"`     // Процент частичной публикации

	// 🤖 AI-генерированные предложения
	SuggestedWhatsNew []string `json:"suggested_whats_new"` // Варианты описания изменений
	ConfidenceScore   float64  `json:"confidence_score"`    // Уверенность AI в анализе (0-1)

	// 🔄 Обратная совместимость (deprecated, используется для миграции)
	AppID            string `json:"app_id,omitempty"`             // Deprecated: используйте PackageName
	VersionCode      int    `json:"version_code,omitempty"`       // Deprecated: версия определяется автоматически
	PrivacyPolicyURL string `json:"privacy_policy_url,omitempty"` // Deprecated: не используется в API v1
}

// CommitInfo информация о коммите
type CommitInfo struct {
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	Author       string    `json:"author"`
	Date         time.Time `json:"date"`
	ChangedFiles []string  `json:"changed_files"`
}

// ShortSHA возвращает короткую версию SHA (до 7 символов)
func (c *CommitInfo) ShortSHA() string {
	if c.SHA == "" {
		return ""
	}
	if len(c.SHA) > 7 {
		return c.SHA[:7]
	}
	return c.SHA
}

// DataCollectionRequest запрос на сбор данных от пользователя (расширен для API v1)
type DataCollectionRequest struct {
	Field          string   `json:"field"`                    // Поле для заполнения
	DisplayName    string   `json:"display_name"`             // Человекочитаемое название
	Description    string   `json:"description"`              // Описание поля
	Required       bool     `json:"required"`                 // Обязательно ли поле
	Suggestions    []string `json:"suggestions"`              // AI-предложения
	ValidationType string   `json:"validation_type"`          // "numeric", "text", "url", "email", "enum", "categories"
	Pattern        string   `json:"pattern,omitempty"`        // Regex паттерн для валидации
	MaxLength      int      `json:"max_length,omitempty"`     // Максимальная длина текста
	ValidValues    []string `json:"valid_values,omitempty"`   // Допустимые значения для enum
	MaxCategories  int      `json:"max_categories,omitempty"` // Максимальное количество категорий
}

// ValidationResult результат валидации ответа пользователя
type ValidationResult struct {
	Valid        bool     `json:"valid"`
	ErrorMessage string   `json:"error_message,omitempty"`
	Suggestions  []string `json:"suggestions,omitempty"` // Предложения по исправлению
}

// ReleaseAgent статусы и результаты работы агентов
type AgentStatus struct {
	AgentName    string                 `json:"agent_name"`
	Status       string                 `json:"status"`   // "running", "completed", "failed"
	Progress     int                    `json:"progress"` // 0-100
	Message      string                 `json:"message"`  // Текущее действие
	Results      map[string]interface{} `json:"results"`  // Результаты работы
	ErrorMessage string                 `json:"error_message,omitempty"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}

// ReleaseSession сессия создания релиза
type ReleaseSession struct {
	ID                 string                   `json:"id"`
	UserID             int64                    `json:"user_id"`
	ChatID             int64                    `json:"chat_id"`
	ReleaseData        *ReleaseData             `json:"release_data"`
	AgentStatuses      map[string]*AgentStatus  `json:"agent_statuses"`
	PendingRequests    []*DataCollectionRequest `json:"pending_requests"`
	CollectedResponses map[string]string        `json:"collected_responses"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
	Status             string                   `json:"status"` // "active", "waiting_user", "publishing", "failed", "retry_needed", "completed", "cancelled"

	// Retry and Error Recovery
	LastError         string            `json:"last_error,omitempty"`         // Последняя ошибка публикации
	RetryCount        int               `json:"retry_count"`                  // Количество попыток публикации
	FailedAtStep      string            `json:"failed_at_step,omitempty"`     // На каком этапе произошла ошибка
	PreviousResponses map[string]string `json:"previous_responses,omitempty"` // Предыдущие ответы для сравнения
}
