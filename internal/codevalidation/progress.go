package codevalidation

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CodeValidationProgressTracker отслеживает прогресс валидации кода
type CodeValidationProgressTracker struct {
	bot       BotInterface
	chatID    int64
	messageID int
	steps     map[string]*ProgressStep
	mu        sync.RWMutex
	filename  string
	language  string
}

// BotInterface интерфейс для отправки сообщений (для избежания циклических зависимостей)
type BotInterface interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	ParseModeValue() string
}

// ProgressStep представляет шаг валидации кода
type ProgressStep struct {
	Name        string
	Description string
	Status      string // pending, in_progress, completed, error
	StartTime   time.Time
	EndTime     time.Time
}

// NewCodeValidationProgressTracker создает новый трекер прогресса
func NewCodeValidationProgressTracker(bot BotInterface, chatID int64, messageID int, filename, language string) *CodeValidationProgressTracker {
	steps := map[string]*ProgressStep{
		"code_analysis":  {Name: "🔍 Анализ кода", Description: "Определение языка, фреймворка и зависимостей", Status: "pending"},
		"docker_setup":   {Name: "🔧 Настройка окружения", Description: "Подготовка среды выполнения", Status: "pending"},
		"copy_code":      {Name: "📋 Копирование файлов", Description: "Загрузка кода в контейнер", Status: "pending"},
		"install_deps":   {Name: "📦 Установка зависимостей", Description: "Установка необходимых библиотек", Status: "pending"},
		"run_validation": {Name: "⚡ Валидация кода", Description: "Проверка структуры и качества кода", Status: "pending"},
	}

	return &CodeValidationProgressTracker{
		bot:       bot,
		chatID:    chatID,
		messageID: messageID,
		steps:     steps,
		filename:  filename,
		language:  language,
	}
}

// UpdateProgress реализует интерфейс ProgressCallback
func (pt *CodeValidationProgressTracker) UpdateProgress(stepKey string, status string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if step, exists := pt.steps[stepKey]; exists {
		step.Status = status
		if status == "in_progress" {
			step.StartTime = time.Now()
		} else if status == "completed" || status == "error" {
			step.EndTime = time.Now()
		}
	}

	// Обновляем сообщение
	pt.updateMessage()
}

// SetFinalResult устанавливает финальный результат валидации
func (pt *CodeValidationProgressTracker) SetFinalResult(result *ValidationResult) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Генерируем финальное сообщение с результатами
	message := pt.buildFinalMessage(result)

	editMsg := tgbotapi.NewEditMessageText(pt.chatID, pt.messageID, message)
	editMsg.ParseMode = pt.bot.ParseModeValue()

	if _, err := pt.bot.Send(editMsg); err != nil {
		// В случае ошибки логируем, но не прерываем выполнение
		fmt.Printf("⚠️ Failed to update final result message: %v\n", err)
	}
}

// updateMessage обновляет сообщение с текущим прогрессом
func (pt *CodeValidationProgressTracker) updateMessage() {
	message := pt.buildProgressMessage()

	editMsg := tgbotapi.NewEditMessageText(pt.chatID, pt.messageID, message)
	editMsg.ParseMode = pt.bot.ParseModeValue()

	if _, err := pt.bot.Send(editMsg); err != nil {
		// В случае ошибки логируем, но не прерываем выполнение
		fmt.Printf("⚠️ Failed to update progress message: %v\n", err)
	}
}

// buildProgressMessage формирует текст сообщения с прогрессом
func (pt *CodeValidationProgressTracker) buildProgressMessage() string {
	var message strings.Builder

	message.WriteString("🔄 **Валидация кода в процессе...**\n\n")
	message.WriteString(fmt.Sprintf("📄 **Файл:** %s\n", html.EscapeString(pt.filename)))
	if pt.language != "" {
		message.WriteString(fmt.Sprintf("💻 **Язык:** %s\n\n", html.EscapeString(pt.language)))
	}

	// Добавляем информацию о шагах (в правильном порядке)
	stepOrder := []string{"code_analysis", "docker_setup", "copy_code", "install_deps", "run_validation"}

	for _, stepKey := range stepOrder {
		if step, exists := pt.steps[stepKey]; exists {
			var statusIcon string
			switch step.Status {
			case "pending":
				statusIcon = "⏳"
			case "in_progress":
				statusIcon = "🔄"
			case "completed":
				statusIcon = "✅"
			case "error":
				statusIcon = "❌"
			default:
				statusIcon = "❓"
			}

			message.WriteString(fmt.Sprintf("%s %s\n", statusIcon, step.Name))

			// Показываем время выполнения для завершенных шагов
			if step.Status == "completed" && !step.EndTime.IsZero() && !step.StartTime.IsZero() {
				duration := step.EndTime.Sub(step.StartTime)
				if duration > 0 && duration < 5*time.Minute { // Разумные пределы
					if duration < time.Second {
						message.WriteString(fmt.Sprintf("   ⏱️ %.0fms\n", float64(duration.Nanoseconds())/1e6))
					} else {
						message.WriteString(fmt.Sprintf("   ⏱️ %.1fs\n", duration.Seconds()))
					}
				}
			}
		}
	}

	message.WriteString("\n💭 *Процесс может занять 1-3 минуты в зависимости от размера кода и количества зависимостей...*")

	return message.String()
}

// buildFinalMessage формирует финальное сообщение с результатами
func (pt *CodeValidationProgressTracker) buildFinalMessage(result *ValidationResult) string {
	var message strings.Builder

	// Формируем заголовок с информацией о токенах
	var statusEmoji, statusText string
	if result.Success {
		statusEmoji = "✅"
		statusText = "успешно завершена"
	} else {
		statusEmoji = "❌"
		statusText = "завершена с ошибками"
	}

	if result.TotalTokens > 0 {
		message.WriteString(fmt.Sprintf("%s **Валидация кода %s** | 🧠 %d токенов\n\n", statusEmoji, statusText, result.TotalTokens))
	} else {
		message.WriteString(fmt.Sprintf("%s **Валидация кода %s**\n\n", statusEmoji, statusText))
	}

	message.WriteString(fmt.Sprintf("📄 **Файл:** %s\n", html.EscapeString(pt.filename)))
	if pt.language != "" {
		message.WriteString(fmt.Sprintf("💻 **Язык:** %s\n", html.EscapeString(pt.language)))
	}
	message.WriteString(fmt.Sprintf("⏱️ **Время выполнения:** %s\n", result.Duration))
	message.WriteString(fmt.Sprintf("🔢 **Exit Code:** %d", result.ExitCode))

	// Показываем номер попытки если было несколько
	if result.RetryAttempt > 1 {
		message.WriteString(fmt.Sprintf(" (попытка %d)", result.RetryAttempt))
	}
	message.WriteString("\n\n")

	// Показываем ответ на вопрос пользователя если есть
	if result.UserQuestion != "" && result.QuestionAnswer != "" {
		message.WriteString("❓ **Ваш вопрос:** ")
		message.WriteString(html.EscapeString(result.UserQuestion))
		message.WriteString("\n\n💬 **Ответ:**\n")
		message.WriteString(html.EscapeString(result.QuestionAnswer))
		message.WriteString("\n\n")
	}

	// Показываем выполненные этапы
	message.WriteString("📊 **Выполненные этапы:**\n")
	stepOrder := []string{"code_analysis", "docker_setup", "copy_code", "install_deps", "run_validation"}

	for _, stepKey := range stepOrder {
		if step, exists := pt.steps[stepKey]; exists {
			var statusIcon string
			switch step.Status {
			case "completed":
				statusIcon = "✅"
			case "error":
				statusIcon = "❌"
			case "in_progress":
				statusIcon = "🔄" // Прерван
			default:
				statusIcon = "⏳" // Не начат
			}

			message.WriteString(fmt.Sprintf("%s %s\n", statusIcon, step.Name))
		}
	}

	// Показываем анализ ошибок если есть
	if result.ErrorAnalysis != "" {
		message.WriteString(fmt.Sprintf("\n🔍 **Анализ ошибок:** %s\n", html.EscapeString(result.ErrorAnalysis)))
	}

	// Показываем результаты
	if result.Success {
		message.WriteString("\n🎉 **Все проверки пройдены успешно!**\n")
	} else {
		// Разделяем проблемы сборки и проблемы кода
		if len(result.BuildProblems) > 0 {
			message.WriteString("\n🔧 **Проблемы сборки:**\n")
			for _, problem := range result.BuildProblems {
				message.WriteString(fmt.Sprintf("• %s\n", html.EscapeString(problem)))
			}
		}

		if len(result.CodeProblems) > 0 {
			message.WriteString("\n💻 **Проблемы в коде:**\n")
			for _, problem := range result.CodeProblems {
				message.WriteString(fmt.Sprintf("• %s\n", html.EscapeString(problem)))
			}
		}

		// Если анализ не разделил ошибки, показываем все как обычно
		if len(result.BuildProblems) == 0 && len(result.CodeProblems) == 0 && len(result.Errors) > 0 {
			message.WriteString("\n❌ **Обнаружены проблемы:**\n")
			for _, err := range result.Errors {
				message.WriteString(fmt.Sprintf("• %s\n", html.EscapeString(err)))
			}
		}
	}

	// Показываем предупреждения если есть
	if len(result.Warnings) > 0 {
		message.WriteString("\n⚠️ **Предупреждения:**\n")
		for _, warning := range result.Warnings {
			message.WriteString(fmt.Sprintf("• %s\n", html.EscapeString(warning)))
		}
	}

	// Показываем рекомендации
	if len(result.Suggestions) > 0 {
		message.WriteString("\n💡 **Рекомендации:**\n")
		for _, suggestion := range result.Suggestions {
			message.WriteString(fmt.Sprintf("• %s\n", html.EscapeString(suggestion)))
		}
	}

	// Показываем output если он не слишком длинный (в блоке кода HTML экранирование не нужно)
	if len(result.Output) > 0 && len(result.Output) < 1000 {
		message.WriteString(fmt.Sprintf("\n📋 **Детали выполнения:**\n```\n%s\n```", result.Output))
	}

	return message.String()
}
