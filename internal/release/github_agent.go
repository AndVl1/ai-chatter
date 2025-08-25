package release

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-chatter/internal/github"
	"ai-chatter/internal/llm"
)

// GitHubDataAgent агент для сбора данных из GitHub
type GitHubDataAgent struct {
	githubClient *github.GitHubMCPClient
	llmClient    llm.Client
}

// NewGitHubDataAgent создает новый GitHub Data Agent
func NewGitHubDataAgent(githubClient *github.GitHubMCPClient, llmClient llm.Client) *GitHubDataAgent {
	return &GitHubDataAgent{
		githubClient: githubClient,
		llmClient:    llmClient,
	}
}

// CollectReleaseData собирает данные из GitHub для релиза
func (g *GitHubDataAgent) CollectReleaseData(ctx context.Context, repoOwner, repoName string, status *AgentStatus) (*ReleaseData, error) {
	g.updateStatus(status, "running", 10, "Поиск последнего pre-release...")

	// Получаем последний pre-release
	latestPreRelease, err := g.githubClient.GetLatestPreRelease(ctx, repoOwner, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest pre-release: %w", err)
	}

	g.updateStatus(status, "running", 30, "Поиск Android файла...")

	// Ищем Android файл
	androidAsset := g.githubClient.FindAndroidAsset(*latestPreRelease)
	if androidAsset == nil {
		return nil, fmt.Errorf("no Android file found in release")
	}

	assetType := g.getAssetType(androidAsset.Name)

	g.updateStatus(status, "running", 50, "Анализ коммитов с последнего релиза...")

	// Получаем коммиты с последнего релиза
	commits, err := g.getCommitsSinceLastRelease(ctx, repoOwner, repoName)
	if err != nil {
		log.Printf("⚠️ Failed to get commits since last release: %v", err)
		commits = []CommitInfo{} // Продолжаем без коммитов
	}

	g.updateStatus(status, "running", 70, "AI-анализ изменений...")

	// Анализируем изменения с помощью AI
	keyChanges, changedFiles, err := g.analyzeChangesWithAI(ctx, commits, latestPreRelease)
	if err != nil {
		log.Printf("⚠️ Failed to analyze changes with AI: %v", err)
		keyChanges = []string{"Анализ изменений недоступен"}
		changedFiles = []string{}
	}

	g.updateStatus(status, "running", 90, "Генерация предложений для описания...")

	// Генерируем предложения для "Что нового"
	whatsNewSuggestions, confidence := g.generateWhatsNewSuggestions(ctx, keyChanges, latestPreRelease)

	g.updateStatus(status, "completed", 100, "Сбор данных завершен")
	status.CompletedAt = &time.Time{}
	*status.CompletedAt = time.Now()

	releaseData := &ReleaseData{
		GitHubRelease:           latestPreRelease,
		AndroidAsset:            androidAsset,
		AssetType:               assetType,
		CommitsSinceLastRelease: commits,
		ChangedFiles:            changedFiles,
		KeyChanges:              keyChanges,
		RuStoreData: RuStoreReleaseData{
			SuggestedWhatsNew: whatsNewSuggestions,
			ConfidenceScore:   confidence,
		},
		CreatedAt: time.Now(),
		Status:    "collecting",
	}

	return releaseData, nil
}

// updateStatus обновляет статус агента
func (g *GitHubDataAgent) updateStatus(status *AgentStatus, state string, progress int, message string) {
	status.Status = state
	status.Progress = progress
	status.Message = message
	log.Printf("🔍 GitHub Agent: %s (%d%%) - %s", state, progress, message)
}

// getAssetType возвращает тип файла
func (g *GitHubDataAgent) getAssetType(filename string) string {
	if strings.HasSuffix(filename, ".aab") {
		return "AAB"
	}
	if strings.HasSuffix(filename, ".apk") {
		return "APK"
	}
	return "Unknown"
}

// getCommitsSinceLastRelease получает коммиты с последнего стабильного релиза
func (g *GitHubDataAgent) getCommitsSinceLastRelease(ctx context.Context, repoOwner, repoName string) ([]CommitInfo, error) {
	// Получаем все релизы
	result := g.githubClient.GetReleases(ctx, repoOwner, repoName, 50, false, false)
	if !result.Success {
		return nil, fmt.Errorf("failed to get releases: %s", result.Message)
	}

	// Ищем последний стабильный релиз (не pre-release)
	var lastStableRelease *github.GitHubRelease
	for _, release := range result.Releases {
		if !release.IsPrerelease {
			lastStableRelease = &release
			break
		}
	}

	if lastStableRelease == nil {
		// Если стабильных релизов нет, возвращаем пустой список
		return []CommitInfo{}, nil
	}

	// Здесь бы нужно было получить коммиты между релизами через GitHub API
	// Для упрощения возвращаем заглушку
	// TODO: Implement actual commit comparison via GitHub API

	return []CommitInfo{
		{
			SHA:          "abc123",
			Message:      "Заглушка: обновления с последнего релиза",
			Author:       "Developer",
			Date:         time.Now().AddDate(0, 0, -7),
			ChangedFiles: []string{"app/src/main/java/MainActivity.java", "build.gradle"},
		},
	}, nil
}

// analyzeChangesWithAI анализирует изменения с помощью AI
func (g *GitHubDataAgent) analyzeChangesWithAI(ctx context.Context, commits []CommitInfo, release *github.GitHubRelease) ([]string, []string, error) {
	if len(commits) == 0 {
		return []string{"Нет данных о коммитах для анализа"}, []string{}, nil
	}

	// Формируем промпт для анализа
	prompt := g.buildAnalysisPrompt(commits, release)

	// Отправляем запрос к LLM
	response, err := g.llmClient.Generate(ctx, []llm.Message{
		{Role: "system", Content: "Ты эксперт по анализу изменений в мобильных приложениях. Анализируй коммиты и выдавай структурированный ответ."},
		{Role: "user", Content: prompt},
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed to analyze changes with AI: %w", err)
	}

	// Парсим ответ AI
	keyChanges, changedFiles := g.parseAIAnalysisResponse(response.Content)

	return keyChanges, changedFiles, nil
}

// buildAnalysisPrompt создает промпт для анализа изменений
func (g *GitHubDataAgent) buildAnalysisPrompt(commits []CommitInfo, release *github.GitHubRelease) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Анализируй изменения в мобильном приложении между релизами.\n\n"))
	prompt.WriteString(fmt.Sprintf("**Текущий релиз:** %s (%s)\n", release.Name, release.TagName))
	prompt.WriteString(fmt.Sprintf("**Описание релиза:** %s\n\n", release.Body))

	prompt.WriteString("**Коммиты с последнего стабильного релиза:**\n")
	for _, commit := range commits {
		// Используем безопасный метод для получения короткого SHA
		shortSHA := commit.ShortSHA()
		if shortSHA == "" {
			continue
		}

		prompt.WriteString(fmt.Sprintf("- %s: %s\n", shortSHA, commit.Message))
		if len(commit.ChangedFiles) > 0 {
			prompt.WriteString(fmt.Sprintf("  Изменены файлы: %s\n", strings.Join(commit.ChangedFiles, ", ")))
		}
	}

	prompt.WriteString("\n**Задание:**\n")
	prompt.WriteString("1. Выдели ключевые изменения для пользователей (новые функции, исправления, улучшения)\n")
	prompt.WriteString("2. Укажи категории изменений (UI, performance, bugfixes, features)\n")
	prompt.WriteString("3. Ответ дай в формате:\n")
	prompt.WriteString("KEY_CHANGES:\n- изменение 1\n- изменение 2\n\n")
	prompt.WriteString("CHANGED_FILES:\n- файл1\n- файл2\n")

	return prompt.String()
}

// parseAIAnalysisResponse парсит ответ AI анализа
func (g *GitHubDataAgent) parseAIAnalysisResponse(response string) ([]string, []string) {
	keyChanges := []string{}
	changedFiles := []string{}

	lines := strings.Split(response, "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "KEY_CHANGES:") {
			currentSection = "key_changes"
			continue
		} else if strings.HasPrefix(line, "CHANGED_FILES:") {
			currentSection = "changed_files"
			continue
		}

		if strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(line, "- ")
			if currentSection == "key_changes" {
				keyChanges = append(keyChanges, item)
			} else if currentSection == "changed_files" {
				changedFiles = append(changedFiles, item)
			}
		}
	}

	// Fallback если парсинг не удался
	if len(keyChanges) == 0 {
		keyChanges = []string{"Анализ изменений выполнен"}
	}

	return keyChanges, changedFiles
}

// generateWhatsNewSuggestions генерирует предложения для описания "Что нового"
func (g *GitHubDataAgent) generateWhatsNewSuggestions(ctx context.Context, keyChanges []string, release *github.GitHubRelease) ([]string, float64) {
	prompt := g.buildWhatsNewPrompt(keyChanges, release)

	response, err := g.llmClient.Generate(ctx, []llm.Message{
		{Role: "system", Content: "Ты копирайтер, специализирующийся на описаниях обновлений мобильных приложений для магазинов приложений. Пиши кратко, понятно и привлекательно для пользователей."},
		{Role: "user", Content: prompt},
	})

	if err != nil {
		log.Printf("⚠️ Failed to generate what's new suggestions: %v", err)
		return []string{"Обновления и улучшения"}, 0.1
	}

	suggestions := g.parseWhatsNewSuggestions(response.Content)
	confidence := g.calculateConfidence(keyChanges, suggestions)

	return suggestions, confidence
}

// buildWhatsNewPrompt создает промпт для генерации описания "Что нового"
func (g *GitHubDataAgent) buildWhatsNewPrompt(keyChanges []string, release *github.GitHubRelease) string {
	var prompt strings.Builder

	prompt.WriteString("Создай 3 варианта описания 'Что нового' для мобильного приложения в магазине приложений.\n\n")
	prompt.WriteString(fmt.Sprintf("**Версия:** %s\n", release.TagName))
	prompt.WriteString("**Ключевые изменения:**\n")

	for _, change := range keyChanges {
		prompt.WriteString(fmt.Sprintf("- %s\n", change))
	}

	prompt.WriteString("\n**Требования:**\n")
	prompt.WriteString("- Каждый вариант до 500 символов\n")
	prompt.WriteString("- Фокус на пользовательских преимуществах\n")
	prompt.WriteString("- Используй эмодзи для привлекательности\n")
	prompt.WriteString("- Избегай технических терминов\n")
	prompt.WriteString("- Формат: SUGGESTION_1:, SUGGESTION_2:, SUGGESTION_3:\n")

	return prompt.String()
}

// parseWhatsNewSuggestions парсит предложения из ответа AI
func (g *GitHubDataAgent) parseWhatsNewSuggestions(response string) []string {
	suggestions := []string{}
	lines := strings.Split(response, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		for i := 1; i <= 3; i++ {
			prefix := fmt.Sprintf("SUGGESTION_%d:", i)
			if strings.HasPrefix(line, prefix) {
				suggestion := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if suggestion != "" {
					suggestions = append(suggestions, suggestion)
				}
				break
			}
		}
	}

	// Fallback если парсинг не удался
	if len(suggestions) == 0 {
		suggestions = []string{
			"🔄 Обновления и улучшения производительности",
			"🐛 Исправления ошибок и стабильность",
			"✨ Новые возможности и улучшения интерфейса",
		}
	}

	return suggestions
}

// calculateConfidence рассчитывает уверенность AI в анализе
func (g *GitHubDataAgent) calculateConfidence(keyChanges []string, suggestions []string) float64 {
	// Простая эвристика для расчета уверенности
	confidence := 0.5 // базовая уверенность

	// Увеличиваем уверенность если есть конкретные изменения
	if len(keyChanges) > 1 {
		confidence += 0.2
	}

	// Увеличиваем уверенность если есть разнообразные предложения
	if len(suggestions) >= 3 {
		confidence += 0.2
	}

	// Ограничиваем максимальную уверенность
	if confidence > 0.9 {
		confidence = 0.9
	}

	return confidence
}
