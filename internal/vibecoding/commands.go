package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-chatter/internal/codevalidation"
	"ai-chatter/internal/llm"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramSender интерфейс для отправки сообщений
type TelegramSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	GetFile(config tgbotapi.FileConfig) (tgbotapi.File, error)
}

// MessageFormatter интерфейс для форматирования сообщений
type MessageFormatter interface {
	EscapeText(text string) string
	ParseModeValue() string
}

// VibeCodingHandler обрабатывает команды и сообщения в режиме vibecoding
type VibeCodingHandler struct {
	sessionManager   *SessionManager
	sender           TelegramSender
	formatter        MessageFormatter
	llmClient        llm.Client
	protocolClient   *VibeCodingLLMClient
	awaitingAutoTask map[int64]bool // Пользователи, ожидающие ввода задачи для автономной работы
}

// NewVibeCodingHandler создает новый обработчик vibecoding
func NewVibeCodingHandler(sender TelegramSender, formatter MessageFormatter, llmClient llm.Client) *VibeCodingHandler {
	sessionManager := NewSessionManager()
	protocolClient := NewVibeCodingLLMClient(llmClient)

	// Создаем MCP клиент и подключаем его к LLM клиенту
	mcpClient := NewVibeCodingMCPClient()
	protocolClient.SetMCPClient(mcpClient)

	return &VibeCodingHandler{
		sessionManager:   sessionManager,
		sender:           sender,
		formatter:        formatter,
		llmClient:        llmClient,
		protocolClient:   protocolClient,
		awaitingAutoTask: make(map[int64]bool),
	}
}

// HandleArchiveUpload обрабатывает загрузку архива для создания vibecoding сессии
func (h *VibeCodingHandler) HandleArchiveUpload(ctx context.Context, userID, chatID int64, archiveData []byte, archiveName, caption string) error {
	log.Printf("🔥 HandleArchiveUpload called for user %d", userID)

	// Проверяем, есть ли у пользователя активная сессия
	if h.sessionManager.HasActiveSession(userID) {
		text := "[vibecoding] ❌ У вас уже есть активная сессия вайбкодинга. Завершите её командой /vibecoding_end перед созданием новой."
		return h.sendMessage(chatID, text)
	}

	// Проверяем, что нет вопросов в описании (условие для vibecoding)
	if strings.TrimSpace(caption) != "" {
		return fmt.Errorf("для запуска vibecoding режима архив должен быть загружен без вопросов в описании")
	}

	// Извлекаем файлы из архива
	files, projectName, err := ExtractFilesFromArchive(archiveData, archiveName)
	if err != nil {
		text := fmt.Sprintf("[vibecoding] ❌ Ошибка обработки архива: %s", err.Error())
		h.sendMessage(chatID, text)
		return err
	}

	// Проверяем, что архив содержит подходящие файлы
	if !IsValidProjectArchive(files) {
		text := "[vibecoding] ❌ Архив не содержит подходящих файлов для анализа кода."
		h.sendMessage(chatID, text)
		return fmt.Errorf("invalid project archive")
	}

	// Отправляем сообщение о начале настройки
	stats := GetProjectStats(files)
	startMsg := fmt.Sprintf(`[vibecoding] 🔥 Запуск сессии вайбкодинга

Проект: %s
Файлов: %d
Размер: %d bytes

🔧 Настройка окружения... (до 3 попыток)`,
		projectName,
		stats["total_files"].(int),
		stats["total_size"].(int))

	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(startMsg))
	msg.ParseMode = h.formatter.ParseModeValue()
	setupMsg, _ := h.sender.Send(msg)

	// Создаем сессию
	session, err := h.sessionManager.CreateSession(userID, chatID, projectName, files, h.llmClient)
	if err != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка создания сессии: %s", err.Error())
		h.updateMessage(chatID, setupMsg.MessageID, errorMsg)
		return err
	}

	// Подключаем MCP клиент через HTTP для прямых вызовов от LLM
	if h.protocolClient != nil && h.protocolClient.mcpClient != nil {
		log.Printf("🔗 Connecting MCP client for VibeCoding session user %d", userID)
		if err := h.protocolClient.mcpClient.ConnectHTTP(ctx, h.sessionManager); err != nil {
			log.Printf("⚠️ Failed to connect MCP client via HTTP: %v. LLM tools may not work properly.", err)
			// Не прерываем создание сессии, т.к. MCP не критичен для базовой функциональности
		} else {
			log.Printf("✅ MCP client connected via HTTP for user %d", userID)
		}
	}

	// Настраиваем окружение
	if err := session.SetupEnvironment(ctx); err != nil {
		// Очищаем сессию при ошибке
		h.sessionManager.EndSession(userID)

		errorMsg := fmt.Sprintf(`[vibecoding] ❌ Не удалось настроить окружение

Ошибка: %s

Сессия завершена. Проверьте содержимое архива и попробуйте снова.`,
			err.Error())

		h.updateMessage(chatID, setupMsg.MessageID, errorMsg)
		return err
	}

	// Окружение настроено успешно
	successMsg := fmt.Sprintf(`[vibecoding] 🔥 Сессия вайбкодинга готова!

Проект: %s
Язык: %s
Команда тестов: %s

🌐 Веб-интерфейс: http://localhost:3000?user=%d

Доступные команды:
/vibecoding_info - информация о сессии
/vibecoding_context - обновить контекст проекта
/vibecoding_test - запустить тесты
/vibecoding_generate_tests - сгенерировать тесты
/vibecoding_auto - автономная работа с проектом
/vibecoding_end - завершить сессию

Теперь вы можете задавать вопросы по коду и запрашивать изменения!`,
		session.ProjectName,
		session.Analysis.Language,
		session.TestCommand,
		userID)

	h.updateMessage(chatID, setupMsg.MessageID, successMsg)
	return nil
}

// HandleVibeCodingCommand обрабатывает команды vibecoding режима
func (h *VibeCodingHandler) HandleVibeCodingCommand(ctx context.Context, userID, chatID int64, command string) error {
	session := h.sessionManager.GetSession(userID)
	if session == nil {
		text := "[vibecoding] ❌ У вас нет активной сессии вайбкодинга. Загрузите архив с кодом для начала."
		return h.sendMessage(chatID, text)
	}

	switch command {
	case "/vibecoding_info":
		return h.handleInfoCommand(chatID, session)
	case "/vibecoding_context":
		return h.handleContextCommand(ctx, chatID, session)
	case "/vibecoding_test":
		return h.handleTestCommand(ctx, chatID, session)
	case "/vibecoding_generate_tests":
		return h.handleGenerateTestsCommand(ctx, chatID, session)
	case "/vibecoding_auto":
		return h.handleAutoCommand(ctx, chatID, userID, session)
	case "/vibecoding_end":
		return h.handleEndCommand(ctx, chatID, userID, session)
	default:
		text := "[vibecoding] ❓ Неизвестная команда. Используйте /vibecoding_info для списка доступных команд."
		return h.sendMessage(chatID, text)
	}
}

// HandleVibeCodingMessage обрабатывает текстовые сообщения в vibecoding режиме
func (h *VibeCodingHandler) HandleVibeCodingMessage(ctx context.Context, userID, chatID int64, messageText string) error {
	session := h.sessionManager.GetSession(userID)
	if session == nil {
		return nil // Не наша задача если нет сессии
	}

	// Проверяем, ожидается ли задача для автономной работы
	if h.awaitingAutoTask[userID] {
		delete(h.awaitingAutoTask, userID) // Сбрасываем состояние ожидания
		return h.HandleAutoWorkRequest(ctx, userID, chatID, messageText)
	}

	log.Printf("🔥 Processing vibecoding message from user %d: %s", userID, messageText)

	// Генерируем ответ через LLM
	response, err := h.generateCodeResponse(ctx, session, messageText)
	if err != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка генерации ответа: %s", err.Error())
		return h.sendMessage(chatID, errorMsg)
	}

	// Отправляем ответ пользователю
	return h.sendLongMessage(chatID, fmt.Sprintf("[vibecoding] %s", response))
}

// handleInfoCommand обрабатывает команду получения информации о сессии
func (h *VibeCodingHandler) handleInfoCommand(chatID int64, session *VibeCodingSession) error {
	info := session.GetSessionInfo()
	duration := time.Since(session.StartTime).Round(time.Second)

	infoMsg := fmt.Sprintf(`[vibecoding] 📊 Информация о сессии

Проект: %s
Язык: %s
Начало: %s
Длительность: %s
Файлов: %d исходных + %d сгенерированных
Команда тестов: %s
Контейнер: %s`,
		info["project_name"].(string),
		info["language"].(string),
		info["start_time"].(time.Time).Format("15:04:05"),
		duration,
		info["files_count"].(int),
		info["generated_count"].(int),
		info["test_command"].(string),
		info["container_id"].(string))

	// Добавляем информацию о контексте
	if contextAvailable, exists := info["context_available"].(bool); exists && contextAvailable {
		infoMsg += fmt.Sprintf(`

📋 Контекст проекта: доступен
Сгенерирован: %s
Файлов в контексте: %d
Полный контекст: PROJECT_CONTEXT.md`,
			info["context_generated_at"].(time.Time).Format("15:04:05"),
			info["context_files_count"].(int))
	} else {
		infoMsg += "\n\n📋 Контекст проекта: не доступен"
	}

	return h.sendMessage(chatID, infoMsg)
}

// handleContextCommand обрабатывает команду обновления контекста проекта
func (h *VibeCodingHandler) handleContextCommand(ctx context.Context, chatID int64, session *VibeCodingSession) error {
	text := "[vibecoding] 📋 Обновление контекста проекта..."
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	sentMsg, _ := h.sender.Send(msg)

	// Обновляем контекст
	if err := session.RefreshProjectContext(); err != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка обновления контекста: %s", err.Error())
		h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
		return err
	}

	// Получаем обновленную информацию о LLM-контексте
	info := session.GetSessionInfo()
	successMsg := fmt.Sprintf(`[vibecoding] ✅ LLM-контекст проекта обновлен

📊 Статистика:
Всего файлов: %d
Ключевых элементов: %d
Токенов: %d / %d
Обновлен: %s

📋 Полный LLM-контекст доступен в файле PROJECT_CONTEXT.md
Используйте MCP tools для получения файлов по требованию.`,
		info["context_total_files"].(int),
		info["context_files_count"].(int),
		info["context_tokens_used"].(int),
		info["context_tokens_limit"].(int),
		info["context_generated_at"].(time.Time).Format("15:04:05"))

	h.updateMessage(chatID, sentMsg.MessageID, successMsg)
	return nil
}

// handleTestCommand обрабатывает команду запуска тестов с автоматическим исправлением при неудаче
func (h *VibeCodingHandler) handleTestCommand(ctx context.Context, chatID int64, session *VibeCodingSession) error {
	text := "[vibecoding] 🧪 Запуск тестов..."
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	sentMsg, _ := h.sender.Send(msg)

	maxAttempts := 3
	var lastResult *codevalidation.ValidationResult
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🧪 Test execution attempt %d/%d for user %d", attempt, maxAttempts, session.UserID)

		if attempt > 1 {
			// Обновляем сообщение с информацией о попытке исправления
			progressMsg := fmt.Sprintf("[vibecoding] 🧪 Запуск тестов... (попытка %d/%d)\n🔧 Исправление тестов через LLM...", attempt, maxAttempts)
			h.updateMessage(chatID, sentMsg.MessageID, progressMsg)
		}

		// Выполняем команду тестов
		result, err := session.ExecuteCommand(ctx, session.TestCommand)
		if err != nil {
			log.Printf("❌ Test execution failed on attempt %d for user %d: %v", attempt, session.UserID, err)
			lastError = err

			if attempt < maxAttempts {
				// Пытаемся исправить проблему выполнения через LLM
				if fixErr := h.fixTestExecutionIssues(ctx, session, err); fixErr != nil {
					log.Printf("⚠️ Could not fix test execution issues: %v", fixErr)
				}
				continue
			}

			errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка выполнения тестов после %d попыток: %s", maxAttempts, err.Error())
			h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
			return err
		}

		lastResult = result
		log.Printf("🧪 Test execution completed on attempt %d for user %d: success=%v, exit_code=%d", attempt, session.UserID, result.Success, result.ExitCode)

		// Если тесты прошли успешно - завершаем
		if result.Success {
			log.Printf("✅ Tests passed successfully on attempt %d", attempt)
			break
		}

		// Если тесты не прошли и есть еще попытки - пытаемся исправить
		if attempt < maxAttempts {
			log.Printf("🔧 Tests failed on attempt %d, attempting to fix with LLM", attempt)
			if fixErr := h.fixFailingTests(ctx, session, result); fixErr != nil {
				log.Printf("⚠️ Could not fix failing tests on attempt %d: %v", attempt, fixErr)
				lastError = fixErr
			} else {
				log.Printf("✅ Applied test fixes, retrying execution")
			}
		} else {
			log.Printf("❌ Tests failed after %d attempts, no more fixes to try", maxAttempts)
			lastError = fmt.Errorf("tests failed after %d attempts", maxAttempts)
		}
	}

	// Формируем финальный результат
	if lastResult != nil {
		status := "✅ успешно"
		attempts := ""
		if maxAttempts > 1 {
			attempts = fmt.Sprintf(" (за %d попыток)", maxAttempts)
		}

		if !lastResult.Success {
			status = "❌ с ошибками"
			attempts = fmt.Sprintf(" после %d попыток", maxAttempts)
			log.Printf("❌ Test validation failed for user %d: %s", session.UserID, lastResult.Output)
		}

		resultMsg := fmt.Sprintf(`[vibecoding] 🧪 Тесты выполнены %s%s

Код выхода: %d
Вывод:
%s`,
			status,
			attempts,
			lastResult.ExitCode,
			lastResult.Output)

		h.updateMessage(chatID, sentMsg.MessageID, resultMsg)

		// Возвращаем ошибку если тесты не прошли для правильного отображения статуса
		if !lastResult.Success {
			return fmt.Errorf("test validation failed with exit code %d", lastResult.ExitCode)
		}
	} else if lastError != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка выполнения тестов: %s", lastError.Error())
		h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
		return lastError
	}

	return nil
}

// handleGenerateTestsCommand обрабатывает команду генерации тестов
func (h *VibeCodingHandler) handleGenerateTestsCommand(ctx context.Context, chatID int64, session *VibeCodingSession) error {
	text := "[vibecoding] 🧠 Генерация тестов..."
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	sentMsg, _ := h.sender.Send(msg)

	// Генерируем тесты через LLM с детальным логированием
	tests, err := h.generateTestsWithProgress(ctx, session, chatID, sentMsg.MessageID)
	if err != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка генерации тестов: %s", err.Error())
		h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
		return err
	}

	// Сохраняем сгенерированные тесты в сессии
	for filename, content := range tests {
		session.AddGeneratedFile(filename, content)
	}

	// Копируем тесты в контейнер
	if err := session.Docker.CopyFilesToContainer(ctx, session.ContainerID, tests); err != nil {
		log.Printf("⚠️ Failed to copy generated tests to container: %v", err)
	}

	// Отправляем результат
	h.updateMessage(chatID, sentMsg.MessageID, "[vibecoding] ✅ Тесты сгенерированы и сохранены в проект")

	// Отправляем содержимое тестов
	for filename, content := range tests {
		testMsg := fmt.Sprintf(`[vibecoding] 📝 Сгенерированный файл: %s

%s`,
			filename,
			content)

		h.sendLongMessage(chatID, testMsg)
	}

	return nil
}

// handleEndCommand обрабатывает команду завершения сессии
func (h *VibeCodingHandler) handleEndCommand(ctx context.Context, chatID int64, userID int64, session *VibeCodingSession) error {
	text := "[vibecoding] 📦 Создание итогового архива..."
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	sentMsg, _ := h.sender.Send(msg)

	// Создаем архив с результатами
	archiveData, err := CreateResultArchive(session)
	if err != nil {
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка создания архива: %s", err.Error())
		h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
		return err
	}

	// Отключаем MCP клиент перед завершением сессии
	if h.protocolClient != nil && h.protocolClient.mcpClient != nil {
		log.Printf("🔌 Disconnecting MCP client for user %d", userID)
		if err := h.protocolClient.mcpClient.Close(); err != nil {
			log.Printf("⚠️ Error disconnecting MCP client: %v", err)
		} else {
			log.Printf("✅ MCP client disconnected for user %d", userID)
		}
	}

	// Завершаем сессию и очищаем состояние ожидания
	duration := time.Since(session.StartTime).Round(time.Second)
	delete(h.awaitingAutoTask, userID) // Очищаем состояние ожидания задачи
	if err := h.sessionManager.EndSession(userID); err != nil {
		log.Printf("⚠️ Error ending session: %v", err)
	}

	// Отправляем архив пользователю
	archiveName := fmt.Sprintf("%s-vibecoding-result.zip", session.ProjectName)
	document := tgbotapi.FileBytes{
		Name:  archiveName,
		Bytes: archiveData,
	}

	documentMsg := tgbotapi.NewDocument(chatID, document)
	caption := fmt.Sprintf(`[vibecoding] 🔥 Сессия завершена

Проект: %s
Длительность: %s
Файлов в архиве: %d

Архив содержит все исходные и сгенерированные файлы.`,
		session.ProjectName,
		duration,
		len(session.GetAllFiles()))
	documentMsg.Caption = h.formatter.EscapeText(caption)
	documentMsg.ParseMode = h.formatter.ParseModeValue()

	_, err = h.sender.Send(documentMsg)
	return err
}

// handleAutoCommand обрабатывает команду автономной работы
func (h *VibeCodingHandler) handleAutoCommand(ctx context.Context, chatID int64, userID int64, session *VibeCodingSession) error {
	h.awaitingAutoTask[userID] = true
	text := "[vibecoding] 🤖 Запуск автономной работы...\n\nВведите задачу для автономного выполнения:"
	return h.sendMessage(chatID, text)
}

// HandleAutoWorkRequest обрабатывает запрос на автономную работу с конкретной задачей
func (h *VibeCodingHandler) HandleAutoWorkRequest(ctx context.Context, userID, chatID int64, task string) error {
	session := h.sessionManager.GetSession(userID)
	if session == nil {
		text := "[vibecoding] ❌ У вас нет активной сессии вайбкодинга."
		return h.sendMessage(chatID, text)
	}

	text := fmt.Sprintf("[vibecoding] 🤖 Запуск автономной работы...\n\nЗадача: %s", task)
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	sentMsg, _ := h.sender.Send(msg)

	// Создаем запрос для автономной работы
	options := map[string]interface{}{
		"user_id": userID,
	}

	// Добавляем сжатый контекст проекта если доступен
	if session.Context != nil {
		options["project_context"] = session.Context
	}

	request := VibeCodingRequest{
		Action: "autonomous_work",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query:   task,
		Options: options,
	}

	log.Printf("🤖 Starting autonomous work for user %d: %s", userID, task)

	// Запускаем автономную работу
	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		log.Printf("❌ Autonomous work failed: %v", err)
		errorMsg := fmt.Sprintf("[vibecoding] ❌ Ошибка автономной работы: %s", err.Error())
		h.updateMessage(chatID, sentMsg.MessageID, errorMsg)
		return err
	}

	// Формируем результат
	var resultMsg strings.Builder
	resultMsg.WriteString("[vibecoding] 🤖 Автономная работа завершена\n\n")
	resultMsg.WriteString(fmt.Sprintf("Статус: %s\n", response.Status))

	if response.Status == "success" {
		resultMsg.WriteString(fmt.Sprintf("Результат: %s\n", response.Response))

		// Добавляем информацию о сгенерированных файлах
		if len(response.Code) > 0 {
			resultMsg.WriteString(fmt.Sprintf("\n📝 Создано файлов: %d\n", len(response.Code)))
			for filename := range response.Code {
				resultMsg.WriteString(fmt.Sprintf("- %s\n", filename))
			}
		}

		// Добавляем лог выполнения если доступен
		if response.Metadata != nil {
			if executionLog, exists := response.Metadata["execution_log"].([]string); exists && len(executionLog) > 0 {
				resultMsg.WriteString("\n🔍 Журнал выполнения:\n")
				for i, logEntry := range executionLog {
					if i < 10 { // Показываем только первые 10 записей
						resultMsg.WriteString(fmt.Sprintf("%s\n", logEntry))
					}
				}
				if len(executionLog) > 10 {
					resultMsg.WriteString(fmt.Sprintf("... и еще %d записей\n", len(executionLog)-10))
				}
			}
		}

		// Добавляем предложения
		if len(response.Suggestions) > 0 {
			resultMsg.WriteString("\n💡 Рекомендации:\n")
			for _, suggestion := range response.Suggestions {
				resultMsg.WriteString(fmt.Sprintf("- %s\n", suggestion))
			}
		}
	} else {
		resultMsg.WriteString(fmt.Sprintf("Ошибка: %s\n", response.Error))
	}

	h.updateMessage(chatID, sentMsg.MessageID, resultMsg.String())
	return nil
}

// generateCodeResponse генерирует ответ на вопрос пользователя о коде через JSON протокол
func (h *VibeCodingHandler) generateCodeResponse(ctx context.Context, session *VibeCodingSession, question string) (string, error) {
	// Создаем запрос через JSON протокол
	request := VibeCodingRequest{
		Action: "answer_question",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query: question,
	}

	// Обрабатываем запрос через протокол клиент
	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		log.Printf("❌ JSON protocol request failed: %v", err)
		// Fallback на старый метод
		return h.generateCodeResponseLegacy(ctx, session, question)
	}

	// Обрабатываем ответ
	if response.Status == "error" {
		return "", fmt.Errorf("LLM returned error: %s", response.Error)
	}

	var result strings.Builder
	result.WriteString(response.Response)

	// Добавляем сгенерированный код если есть
	if len(response.Code) > 0 {
		result.WriteString("\n\n📝 Сгенерированный код:\n")
		for filename, content := range response.Code {
			result.WriteString(fmt.Sprintf("\n**%s:**\n```\n%s\n```", filename, content))

			// Сохраняем сгенерированный код в сессии
			session.AddGeneratedFile(filename, content)
		}
	}

	// Добавляем предложения если есть
	if len(response.Suggestions) > 0 {
		result.WriteString("\n\n💡 Предложения:\n")
		for _, suggestion := range response.Suggestions {
			result.WriteString(fmt.Sprintf("• %s\n", suggestion))
		}
	}

	return result.String(), nil
}

// generateCodeResponseLegacy - запасной метод без JSON протокола
func (h *VibeCodingHandler) generateCodeResponseLegacy(ctx context.Context, session *VibeCodingSession, question string) (string, error) {
	log.Printf("⚠️ Using legacy code response generation")

	// Формируем контекст для LLM
	projectContext := h.buildProjectContext(session)

	prompt := fmt.Sprintf(`Ты работаешь в режиме VibeCoding - интерактивной сессии разработки.

КОНТЕКСТ ПРОЕКТА:
Проект: %s
Язык: %s
Файлов: %d

СТРУКТУРА ПРОЕКТА:
%s

ВОПРОС ПОЛЬЗОВАТЕЛЯ:
%s

ИНСТРУКЦИИ:
1. Отвечай конкретно и практично
2. If you need to show code, format it properly for Telegram
3. Если генерируешь новый код, укажи в какой файл его нужно поместить
4. Будь краток но информативен`,
		session.ProjectName,
		session.Analysis.Language,
		len(session.Files),
		projectContext,
		question)

	resp, err := h.llmClient.Generate(ctx, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	return resp.Content, nil
}

// generateTestsWithProgress генерирует тесты с детальным логированием в Telegram
func (h *VibeCodingHandler) generateTestsWithProgress(ctx context.Context, session *VibeCodingSession, chatID int64, messageID int) (map[string]string, error) {
	maxAttempts := 5
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Обновляем сообщение с прогрессом
		progressMsg := fmt.Sprintf("[vibecoding] 🧠 Генерация тестов... (попытка %d/%d)", attempt, maxAttempts)
		h.updateMessage(chatID, messageID, progressMsg)

		log.Printf("🧪 Test generation attempt %d/%d", attempt, maxAttempts)

		// Генерируем тесты
		tests, err := h.generateTestsOnce(ctx, session, attempt)
		if err != nil {
			lastError = fmt.Errorf("test generation failed: %w", err)
			log.Printf("❌ Test generation attempt %d failed: %v", attempt, err)

			// Обновляем сообщение об ошибке
			errorMsg := fmt.Sprintf("[vibecoding] ⚠️ Попытка %d/%d не удалась: %s", attempt, maxAttempts, err.Error())
			h.updateMessage(chatID, messageID, errorMsg)
			time.Sleep(2 * time.Second) // Дать пользователю прочитать ошибку
			continue
		}

		if len(tests) == 0 {
			lastError = fmt.Errorf("no tests generated")
			log.Printf("⚠️ No tests generated on attempt %d", attempt)

			// Обновляем сообщение
			h.updateMessage(chatID, messageID, fmt.Sprintf("[vibecoding] ⚠️ Попытка %d/%d: тесты не сгенерированы", attempt, maxAttempts))
			time.Sleep(2 * time.Second)
			continue
		}

		// Валидируем сгенерированные тесты
		validationMsg := fmt.Sprintf("[vibecoding] 🔍 Валидация %d тестовых файлов... (попытка %d/%d)", len(tests), attempt, maxAttempts)
		h.updateMessage(chatID, messageID, validationMsg)
		log.Printf("🔍 Validating %d generated test files", len(tests))

		validationResult, err := h.validateGeneratedTests(ctx, session, tests)
		if err != nil {
			log.Printf("❌ Test validation failed on attempt %d: %v", attempt, err)
			lastError = err

			// Обновляем сообщение об ошибке валидации
			h.updateMessage(chatID, messageID, fmt.Sprintf("[vibecoding] ❌ Валидация не прошла (попытка %d/%d): %s", attempt, maxAttempts, err.Error()))
			time.Sleep(2 * time.Second)
			continue
		}

		if validationResult.Success {
			log.Printf("✅ All tests passed validation on attempt %d", attempt)

			// Обновляем сообщение об успехе
			successMsg := fmt.Sprintf("[vibecoding] ✅ Тесты успешно сгенерированы и проверены (попытка %d/%d)", attempt, maxAttempts)
			h.updateMessage(chatID, messageID, successMsg)

			return validationResult.ValidTests, nil
		}

		// Если валидация не прошла, пытаемся исправить тесты
		if attempt < maxAttempts {
			log.Printf("🔧 Attempting to fix test issues on attempt %d", attempt)

			// Обновляем сообщение об исправлении
			fixMsg := fmt.Sprintf("[vibecoding] 🔧 Исправление ошибок тестов... (попытка %d/%d)", attempt, maxAttempts)
			h.updateMessage(chatID, messageID, fixMsg)

			fixedTests, err := h.fixTestIssues(ctx, session, tests, validationResult)
			if err != nil {
				log.Printf("⚠️ Could not fix test issues: %v", err)
				lastError = fmt.Errorf("test fixing failed: %w", err)

				h.updateMessage(chatID, messageID, fmt.Sprintf("[vibecoding] ⚠️ Не удалось исправить ошибки (попытка %d/%d)", attempt, maxAttempts))
				time.Sleep(2 * time.Second)
				continue
			}
			// Используем исправленные тесты для следующей итерации
			tests = fixedTests

			// Обновляем сообщение об успешном исправлении
			h.updateMessage(chatID, messageID, fmt.Sprintf("[vibecoding] ✅ Ошибки исправлены, повтор валидации... (попытка %d/%d)", attempt, maxAttempts))
		} else {
			lastError = fmt.Errorf("test validation failed after %d attempts", maxAttempts)
		}
	}

	// Если все попытки неудачны, возвращаем ошибку
	log.Printf("❌ Test generation and validation failed after %d attempts", maxAttempts)
	finalErrorMsg := fmt.Sprintf("[vibecoding] ❌ Не удалось сгенерировать тесты после %d попыток: %s", maxAttempts, lastError.Error())
	h.updateMessage(chatID, messageID, finalErrorMsg)

	return nil, fmt.Errorf("test generation failed after %d attempts: %w", maxAttempts, lastError)
}

// generateTests генерирует тесты для проекта через JSON протокол с валидацией и исправлением (без Telegram updates)
func (h *VibeCodingHandler) generateTests(ctx context.Context, session *VibeCodingSession) (map[string]string, error) {
	maxAttempts := 5
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🧪 Test generation attempt %d/%d", attempt, maxAttempts)

		// Генерируем тесты
		tests, err := h.generateTestsOnce(ctx, session, attempt)
		if err != nil {
			lastError = fmt.Errorf("test generation failed: %w", err)
			log.Printf("❌ Test generation attempt %d failed: %v", attempt, err)
			continue
		}

		if len(tests) == 0 {
			lastError = fmt.Errorf("no tests generated")
			log.Printf("⚠️ No tests generated on attempt %d", attempt)
			continue
		}

		// Валидируем сгенерированные тесты
		log.Printf("🔍 Validating %d generated test files", len(tests))
		validationResult, err := h.validateGeneratedTests(ctx, session, tests)
		if err != nil {
			log.Printf("❌ Test validation failed on attempt %d: %v", attempt, err)
			lastError = err
			continue
		}

		if validationResult.Success {
			log.Printf("✅ All tests passed validation on attempt %d", attempt)
			return validationResult.ValidTests, nil
		}

		// Если валидация не прошла, пытаемся исправить тесты
		if attempt < maxAttempts {
			log.Printf("🔧 Attempting to fix test issues on attempt %d", attempt)
			fixedTests, err := h.fixTestIssues(ctx, session, tests, validationResult)
			if err != nil {
				log.Printf("⚠️ Could not fix test issues: %v", err)
				lastError = fmt.Errorf("test fixing failed: %w", err)
				continue
			}

			// Используем исправленные тесты для следующей итерации
			tests = fixedTests
		} else {
			lastError = fmt.Errorf("test validation failed after %d attempts", maxAttempts)
		}
	}

	// Если все попытки неудачны, возвращаем ошибку без fallback
	log.Printf("❌ Test generation and validation failed after %d attempts", maxAttempts)
	return nil, fmt.Errorf("test generation failed after %d attempts: %w", maxAttempts, lastError)
}

// generateTestsOnce выполняет однократную генерацию тестов
func (h *VibeCodingHandler) generateTestsOnce(ctx context.Context, session *VibeCodingSession, attempt int) (map[string]string, error) {
	// Сначала генерируем специализированный промпт для написания тестов
	testPrompt, err := h.generateTestWritingPrompt(ctx, session)
	if err != nil {
		log.Printf("⚠️ Failed to generate test writing prompt: %v, using default", err)
		testPrompt = fmt.Sprintf("Generate comprehensive tests for this %s project. Include unit tests and integration tests where appropriate. Follow best practices and testing conventions for %s.", session.Analysis.Language, session.Analysis.Language)
	}

	log.Printf("🎯 Generated specialized test writing prompt for %s", session.Analysis.Language)

	// Создаем запрос через JSON протокол с специализированным промптом
	query := testPrompt

	if attempt > 1 {
		query += fmt.Sprintf(" This is attempt %d - ensure tests are syntactically correct and runnable.", attempt)
	}

	request := VibeCodingRequest{
		Action: "generate_code",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query: query,
		Options: map[string]interface{}{
			"task_type": "test_generation",
			"language":  session.Analysis.Language,
			"attempt":   attempt,
		},
	}

	// Обрабатываем запрос через протокол клиент
	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("JSON protocol request failed: %w", err)
	}

	// Обрабатываем ответ
	if response.Status == "error" {
		return nil, fmt.Errorf("LLM returned error: %s", response.Error)
	}

	if len(response.Code) == 0 {
		return nil, fmt.Errorf("no code generated")
	}

	log.Printf("Generated tests code:\n%s", response.Code)

	return response.Code, nil
}

// buildProjectContext строит контекст проекта для LLM
func (h *VibeCodingHandler) buildProjectContext(session *VibeCodingSession) string {
	// Используем сжатый контекст если доступен
	if session.Context != nil {
		return h.buildCompressedContext(session)
	}

	// Fallback к старому методу если контекст не сгенерирован
	var context strings.Builder
	context.WriteString("⚠️ Project context not available, using file excerpts:\n\n")

	for filename, content := range session.Files {
		context.WriteString(fmt.Sprintf("\n=== %s ===\n", filename))

		// Ограничиваем размер файла в контексте
		if len(content) > 2000 {
			context.WriteString(content[:2000])
			context.WriteString("\n... (файл обрезан)")
		} else {
			context.WriteString(content)
		}
		context.WriteString("\n")
	}

	return context.String()
}

// buildCompressedContext строит сжатый LLM-генерируемый контекст для LLM
func (h *VibeCodingHandler) buildCompressedContext(session *VibeCodingSession) string {
	var context strings.Builder

	ctx := session.Context

	context.WriteString("# LLM-Generated Compressed Project Context\n\n")
	context.WriteString(fmt.Sprintf("**Project:** %s | **Language:** %s | **Files:** %d | **Tokens:** %d/%d\n",
		ctx.ProjectName, ctx.Language, ctx.TotalFiles, ctx.TokensUsed, ctx.TokensLimit))

	if ctx.Description != "" {
		context.WriteString(fmt.Sprintf("**Description:** %s\n", ctx.Description))
	}
	context.WriteString("\n")

	// Краткая структура проекта
	context.WriteString("## Project Structure:\n")
	for i, dir := range ctx.Structure.Directories {
		if i >= 5 { // Показываем только топ-5 директорий
			context.WriteString(fmt.Sprintf("- ... и еще %d директорий\n", len(ctx.Structure.Directories)-5))
			break
		}
		context.WriteString(fmt.Sprintf("- **%s** (%d files) - %s\n", dir.Path, dir.FileCount, dir.Purpose))
	}
	context.WriteString("\n")

	// LLM-сгенерированные описания файлов
	context.WriteString("## Key Files & LLM-Generated Descriptions:\n")
	fileCount := 0
	for filePath, fileCtx := range ctx.Files {
		if fileCount >= 10 { // Показываем только первые 10 файлов
			context.WriteString(fmt.Sprintf("- ... и еще %d файлов (используйте MCP tools для доступа)\n", len(ctx.Files)-10))
			break
		}
		fileCount++

		context.WriteString(fmt.Sprintf("\n### %s (%s, %d bytes, %d tokens)\n", filePath, fileCtx.Type, fileCtx.Size, fileCtx.TokensUsed))

		if fileCtx.Summary != "" {
			context.WriteString(fmt.Sprintf("**Summary:** %s\n", fileCtx.Summary))
		}

		if fileCtx.Purpose != "" {
			context.WriteString(fmt.Sprintf("**Purpose:** %s\n", fileCtx.Purpose))
		}

		// Показываем ключевые элементы (LLM-определенные)
		if len(fileCtx.KeyElements) > 0 {
			context.WriteString(fmt.Sprintf("**Key Elements (%d):** ", len(fileCtx.KeyElements)))
			elementNames := make([]string, 0, len(fileCtx.KeyElements))
			for j, element := range fileCtx.KeyElements {
				if j >= 5 { // Показываем только первые 5 элементов
					elementNames = append(elementNames, "...")
					break
				}
				elementNames = append(elementNames, element)
			}
			context.WriteString(strings.Join(elementNames, ", ") + "\n")
		}

		// Зависимости файла
		if len(fileCtx.Dependencies) > 0 {
			context.WriteString(fmt.Sprintf("**Dependencies:** %s\n", strings.Join(fileCtx.Dependencies, ", ")))
		}
	}

	// Инструкции для работы с MCP
	context.WriteString("\n## 🔧 MCP Tools Available:\n")
	context.WriteString("- `vibe_list_files` - получить актуальный список файлов\n")
	context.WriteString("- `vibe_read_file` - прочитать полное содержимое файла\n")
	context.WriteString("- `vibe_write_file` - создать или изменить файл\n")
	context.WriteString("- `vibe_execute_command` - выполнить команду в окружении\n")
	context.WriteString("- `vibe_run_tests` - запустить тесты проекта\n")
	context.WriteString("- `vibe_validate_code` - проверить синтаксис кода\n")
	context.WriteString("- `vibe_get_session_info` - получить информацию о сессии\n\n")

	context.WriteString("**ВАЖНО:** Этот контекст генерируется LLM с ограничением токенов. ")
	context.WriteString("Используйте MCP tools для получения полного содержимого файлов и выполнения операций. ")
	context.WriteString("Для больших файлов LLM может запросить содержимое через MCP по мере необходимости.\n")

	// Добавляем PROJECT_CONTEXT.md в сгенерированные файлы для доступа через MCP
	if _, exists := session.GeneratedFiles["PROJECT_CONTEXT.md"]; exists {
		context.WriteString("\n📋 Полный LLM-контекст доступен в файле PROJECT_CONTEXT.md (используйте vibe_read_file)\n")
	}

	return context.String()
}

// sendLongMessage отправляет длинное сообщение, разбивая его при необходимости
func (h *VibeCodingHandler) sendLongMessage(chatID int64, text string) error {
	maxLength := 4000 // Оставляем немного места для заголовка

	if len(text) <= maxLength {
		msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
		msg.ParseMode = h.formatter.ParseModeValue()
		_, err := h.sender.Send(msg)
		return err
	}

	// Разбиваем сообщение на части
	parts := h.splitMessage(text, maxLength)
	for i, part := range parts {
		partText := part
		if len(parts) > 1 {
			partText = fmt.Sprintf("%s\n\n<i>Часть %d из %d</i>", part, i+1, len(parts))
		}

		msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(partText))
		msg.ParseMode = h.formatter.ParseModeValue()
		_, err := h.sender.Send(msg)
		if err != nil {
			return err
		}

		time.Sleep(100 * time.Millisecond) // Небольшая задержка между сообщениями
	}

	return nil
}

// splitMessage разбивает сообщение на логические части
func (h *VibeCodingHandler) splitMessage(text string, maxLength int) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var parts []string
	remaining := text

	for len(remaining) > maxLength {
		// Ищем подходящее место для разрыва (конец строки, блока кода и т.д.)
		breakPoint := maxLength

		// Ищем последний перенос строки в пределах лимита
		for i := maxLength - 1; i > maxLength/2; i-- {
			if remaining[i] == '\n' {
				breakPoint = i + 1
				break
			}
		}

		parts = append(parts, remaining[:breakPoint])
		remaining = remaining[breakPoint:]
	}

	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}

	return parts
}

// TestValidationResult представляет результат валидации тестов
type TestValidationResult struct {
	Success    bool              `json:"success"`
	ValidTests map[string]string `json:"valid_tests"`
	Issues     []TestIssue       `json:"issues"`
	Output     string            `json:"output"`
}

// TestIssue представляет проблему в тесте
type TestIssue struct {
	Filename    string `json:"filename"`
	Type        string `json:"type"` // "syntax_error", "runtime_error", "missing_dependency", "invalid_test"
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

// TestLLMValidationRequest запрос на валидацию тестов через LLM
type TestLLMValidationRequest struct {
	Language     string            `json:"language"`
	ProjectFiles map[string]string `json:"project_files"`
	TestFiles    map[string]string `json:"test_files"`
	Context      string            `json:"context"`
}

// TestLLMValidationResponse ответ валидации тестов через LLM
type TestLLMValidationResponse struct {
	Status      string                   `json:"status"` // "ok", "needs_fix", "error"
	Issues      []TestLLMValidationIssue `json:"issues,omitempty"`
	FixedTests  map[string]string        `json:"fixed_tests,omitempty"`
	Reasoning   string                   `json:"reasoning"`
	Suggestions []string                 `json:"suggestions,omitempty"`
}

// TestLLMValidationIssue проблема найденная LLM в тестах
type TestLLMValidationIssue struct {
	Filename   string `json:"filename"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"` // "critical", "warning", "info"
	Fix        string `json:"fix"`
	LineNumber int    `json:"line_number,omitempty"`
}

// validateGeneratedTests валидирует сгенерированные тесты с обязательным выполнением
func (h *VibeCodingHandler) validateGeneratedTests(ctx context.Context, session *VibeCodingSession, tests map[string]string) (*TestValidationResult, error) {
	log.Printf("🔍 Starting strict validation of %d test files", len(tests))

	// Сначала валидируем тесты через LLM
	llmValidatedTests, err := h.validateTestsWithLLM(ctx, session, tests)
	if err != nil {
		log.Printf("⚠️ LLM validation failed, proceeding with original tests: %v", err)
		// Продолжаем с оригинальными тестами если LLM валидация не удалась
		llmValidatedTests = tests
	}

	log.Printf("✅ LLM validation complete, proceeding with execution validation of %d test files", len(llmValidatedTests))

	result := &TestValidationResult{
		Success:    true,
		ValidTests: make(map[string]string),
		Issues:     make([]TestIssue, 0),
	}

	// Создаем временную копию файлов сессии с валидированными тестами
	tempFiles := make(map[string]string)
	for k, v := range session.Files {
		tempFiles[k] = v
	}
	for filename, content := range llmValidatedTests {
		tempFiles[filename] = content
	}

	// Копируем файлы в контейнер для валидации
	if err := session.Docker.CopyFilesToContainer(ctx, session.ContainerID, tempFiles); err != nil {
		return nil, fmt.Errorf("failed to copy test files to container: %w", err)
	}

	// КРИТИЧНО: выполняем реальную валидацию каждого тестового файла
	for filename, content := range llmValidatedTests {
		log.Printf("🔍 Executing real validation for test file: %s", filename)

		// Выполняем ФАКТИЧЕСКОЕ выполнение тестов для валидации
		executionOK, executionIssue := h.executeTestForValidation(ctx, session, filename)
		if !executionOK {
			result.Success = false
			result.Issues = append(result.Issues, *executionIssue)
			log.Printf("❌ Test execution validation FAILED for %s: %s", filename, executionIssue.Description)

			// НЕ добавляем файл в valid_tests если он не выполняется
			continue
		}

		// Добавляем файл в валидные тесты ТОЛЬКО если он действительно выполняется
		result.ValidTests[filename] = content
		log.Printf("✅ Test file %s PASSED execution validation", filename)
	}

	log.Printf("🔍 Strict validation complete: %d valid files, %d issues found", len(result.ValidTests), len(result.Issues))

	// Если НИ ОДИН тест не прошел валидацию - это ошибка
	if len(result.ValidTests) == 0 && len(llmValidatedTests) > 0 {
		result.Success = false
		log.Printf("❌ CRITICAL: No tests passed execution validation")
	}

	return result, nil
}

// executeTestForValidation выполняет фактическое тестирование для строгой валидации
func (h *VibeCodingHandler) executeTestForValidation(ctx context.Context, session *VibeCodingSession, filename string) (bool, *TestIssue) {
	log.Printf("🧪 Executing test file for validation: %s", filename)

	// Используем команды тестирования из LLM анализа
	if len(session.Analysis.TestCommands) == 0 {
		log.Printf("❌ No test commands provided by LLM analysis for validation of %s", filename)
		return false, &TestIssue{
			Filename:    filename,
			Type:        "configuration_error",
			Description: "No test commands available for validation",
		}
	}

	// Определяем подходящую команду тестирования через LLM
	var command string
	for _, testCmd := range session.Analysis.TestCommands {
		// Проверяем через LLM, подходит ли команда для данного файла
		if h.isTestCommandSuitableForFile(ctx, testCmd, filename, session.Analysis.Language) {
			command = h.adaptTestCommandForFile(ctx, testCmd, filename, session.Analysis.Language)
			break
		}
	}

	// Если не нашли подходящую команду, используем первую и адаптируем через LLM
	if command == "" && len(session.Analysis.TestCommands) > 0 {
		command = h.adaptTestCommandForFile(ctx, session.Analysis.TestCommands[0], filename, session.Analysis.Language)
	}

	if command == "" {
		log.Printf("❌ No suitable test command found for validation of %s", filename)
		return false, &TestIssue{
			Filename:    filename,
			Type:        "configuration_error",
			Description: "No suitable test command found",
		}
	}

	log.Printf("🧪 Executing validation command for %s: %s", filename, command)

	// КРИТИЧНО: выполняем команду тестирования и строго проверяем результат
	result, err := session.ExecuteCommand(ctx, command)
	if err != nil {
		log.Printf("❌ Test execution FAILED for %s: %v", filename, err)
		return false, &TestIssue{
			Filename:    filename,
			Type:        "execution_error",
			Description: fmt.Sprintf("Test execution failed: %v", err),
		}
	}

	// СТРОГАЯ проверка: тесты должны выполняться БЕЗ ошибок
	if result.ExitCode != 0 {
		log.Printf("❌ Test validation FAILED for %s with exit code %d", filename, result.ExitCode)

		// Анализируем тип ошибки для более точной диагностики
		errorType := "test_failure"
		if strings.Contains(result.Output, "SyntaxError") ||
			strings.Contains(result.Output, "IndentationError") ||
			strings.Contains(result.Output, "compilation error") ||
			strings.Contains(result.Output, "syntax error") {
			errorType = "syntax_error"
		} else if strings.Contains(result.Output, "ModuleNotFoundError") ||
			strings.Contains(result.Output, "cannot find module") ||
			strings.Contains(result.Output, "ImportError") ||
			strings.Contains(result.Output, "package") && strings.Contains(result.Output, "not found") {
			errorType = "missing_dependency"
		} else if strings.Contains(result.Output, "NameError") ||
			strings.Contains(result.Output, "AttributeError") ||
			strings.Contains(result.Output, "is not defined") {
			errorType = "invalid_reference"
		}

		return false, &TestIssue{
			Filename:    filename,
			Type:        errorType,
			Description: fmt.Sprintf("Test failed with exit code %d: %s", result.ExitCode, result.Output),
		}
	}

	// Дополнительная проверка: убеждаемся что тесты фактически выполнились
	if !result.Success {
		log.Printf("❌ Test execution marked as unsuccessful for %s", filename)
		return false, &TestIssue{
			Filename:    filename,
			Type:        "execution_error",
			Description: fmt.Sprintf("Test execution unsuccessful: %s", result.Output),
		}
	}

	log.Printf("✅ Test file %s PASSED validation execution", filename)
	return true, nil
}

// Note: validateTestSyntax метод удален - синтаксические ошибки обнаруживаются при выполнении тестов

// validateTestExecution проверяет выполнимость тестов используя команды из LLM анализа (устаревший, заменен на executeTestForValidation)
func (h *VibeCodingHandler) validateTestExecution(ctx context.Context, session *VibeCodingSession, filename string) (bool, *TestIssue) {
	// Используем команды тестирования из LLM анализа
	if len(session.Analysis.TestCommands) == 0 {
		log.Printf("ℹ️ No test commands provided by LLM analysis, skipping test execution validation for %s", filename)
		return true, nil
	}

	// Выбираем подходящую команду тестирования через LLM
	var command string
	for _, testCmd := range session.Analysis.TestCommands {
		// Проверяем через LLM, подходит ли команда для данного файла
		if h.isTestCommandSuitableForFile(ctx, testCmd, filename, session.Analysis.Language) {
			command = h.adaptTestCommandForFile(ctx, testCmd, filename, session.Analysis.Language)
			break
		}
	}

	// Если не нашли подходящую команду, используем первую и адаптируем через LLM
	if command == "" && len(session.Analysis.TestCommands) > 0 {
		command = h.adaptTestCommandForFile(ctx, session.Analysis.TestCommands[0], filename, session.Analysis.Language)
	}

	if command == "" {
		log.Printf("ℹ️ No suitable test command found for %s", filename)
		return true, nil
	}

	log.Printf("🧪 Executing test command for %s: %s", filename, command)

	result, err := session.ExecuteCommand(ctx, command)
	if err != nil {
		return false, &TestIssue{
			Filename:    filename,
			Type:        "runtime_error",
			Description: fmt.Sprintf("Test execution failed: %v", err),
		}
	}

	// Некоторые тесты могут завершиться с кодом ошибки, но это не значит что они невалидны
	if !result.Success && result.ExitCode != 0 {
		// Проверяем, не является ли это ошибкой missing dependency или подобным
		if strings.Contains(result.Output, "ModuleNotFoundError") ||
			strings.Contains(result.Output, "cannot find module") ||
			strings.Contains(result.Output, "package") && strings.Contains(result.Output, "not found") {
			return false, &TestIssue{
				Filename:    filename,
				Type:        "missing_dependency",
				Description: fmt.Sprintf("Missing dependency: %s", result.Output),
			}
		}

		// В остальных случаях это может быть нормально (тесты могут падать)
		log.Printf("ℹ️ Test %s completed with non-zero exit code, but may be valid", filename)
	}

	return true, nil
}

// isTestCommandSuitableForFile проверяет через LLM, подходит ли команда тестирования для данного файла
func (h *VibeCodingHandler) isTestCommandSuitableForFile(ctx context.Context, command, filename, language string) bool {
	systemPrompt := `You are a testing expert. Determine if a given test command is suitable for running a specific test file.

Respond with a JSON object matching this exact schema:
{
  "is_suitable": true/false,
  "confidence": "high|medium|low",
  "reasoning": "brief explanation"
}

Consider:
- Command compatibility with file type
- Language-specific testing frameworks  
- File extension matching
- Command syntax and parameters`

	userPrompt := fmt.Sprintf(`Is this test command suitable for this file?

Language: %s
Test Command: %s
Test File: %s

Determine if the command can properly execute tests in this file.`, language, command, filename)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := h.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("⚠️ LLM command suitability check failed for %s: %v, assuming suitable", filename, err)
		return true // Fallback: assume suitable
	}

	var suitabilityResponse struct {
		IsSuitable bool   `json:"is_suitable"`
		Confidence string `json:"confidence"`
		Reasoning  string `json:"reasoning"`
	}

	content := response.Content
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = strings.TrimSpace(content[start : start+end])
		}
	}

	if err := json.Unmarshal([]byte(content), &suitabilityResponse); err != nil {
		log.Printf("⚠️ Failed to parse LLM suitability response for %s: %v, assuming suitable", filename, err)
		return true
	}

	log.Printf("🤖 LLM command suitability for %s: suitable=%v (confidence: %s) - %s",
		filename, suitabilityResponse.IsSuitable, suitabilityResponse.Confidence, suitabilityResponse.Reasoning)

	return suitabilityResponse.IsSuitable
}

// adaptTestCommandForFile адаптирует команду тестирования для конкретного файла через LLM
func (h *VibeCodingHandler) adaptTestCommandForFile(ctx context.Context, command, filename, language string) string {
	systemPrompt := `You are a testing command expert. Adapt a generic test command to run a specific test file.

Respond with a JSON object matching this exact schema:
{
  "adapted_command": "modified command string",
  "changes_made": "description of changes",
  "reasoning": "brief explanation"
}

Consider:
- File-specific targeting in test commands
- Language-specific test runners
- Proper command syntax
- Framework-specific patterns`

	userPrompt := fmt.Sprintf(`Adapt this test command to run the specific test file:

Language: %s
Original Command: %s
Test File: %s

Modify the command to specifically target this test file while maintaining proper syntax.`, language, command, filename)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := h.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("⚠️ LLM command adaptation failed for %s: %v, using original command", filename, err)
		return command // Fallback: use original command
	}

	var adaptationResponse struct {
		AdaptedCommand string `json:"adapted_command"`
		ChangesMade    string `json:"changes_made"`
		Reasoning      string `json:"reasoning"`
	}

	content := response.Content
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = strings.TrimSpace(content[start : start+end])
		}
	}

	if err := json.Unmarshal([]byte(content), &adaptationResponse); err != nil {
		log.Printf("⚠️ Failed to parse LLM adaptation response for %s: %v, using original command", filename, err)
		return command
	}

	log.Printf("🤖 LLM command adaptation for %s: %s -> %s (%s)",
		filename, command, adaptationResponse.AdaptedCommand, adaptationResponse.Reasoning)

	if adaptationResponse.AdaptedCommand == "" {
		return command
	}

	return adaptationResponse.AdaptedCommand
}

// fixTestIssues исправляет проблемы в тестах через LLM
func (h *VibeCodingHandler) fixTestIssues(ctx context.Context, session *VibeCodingSession, tests map[string]string, validationResult *TestValidationResult) (map[string]string, error) {
	if len(validationResult.Issues) == 0 {
		return tests, nil
	}

	log.Printf("🔧 Attempting to fix %d test issues", len(validationResult.Issues))

	// Подготавливаем описание проблем для LLM
	var issuesDescription strings.Builder
	issuesDescription.WriteString("Issues found in generated tests:\n")
	for i, issue := range validationResult.Issues {
		issuesDescription.WriteString(fmt.Sprintf("%d. File: %s, Type: %s, Description: %s\n",
			i+1, issue.Filename, issue.Type, issue.Description))
	}

	request := VibeCodingRequest{
		Action: "generate_code",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query: fmt.Sprintf(`Fix the following test issues for this %s project:

%s

Current test files that need fixing:
%s

Please fix the issues and return corrected test files. Remove any test files that cannot be fixed or are unnecessary.`,
			session.Analysis.Language,
			issuesDescription.String(),
			h.formatTestFiles(tests)),
		Options: map[string]interface{}{
			"task_type": "test_fixing",
			"language":  session.Analysis.Language,
			"issues":    validationResult.Issues,
		},
	}

	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get test fixes from LLM: %w", err)
	}

	if response.Status == "error" {
		return nil, fmt.Errorf("LLM could not fix tests: %s", response.Error)
	}

	if len(response.Code) == 0 {
		return nil, fmt.Errorf("no fixed tests returned from LLM")
	}

	log.Printf("✅ LLM provided %d fixed test files", len(response.Code))
	return response.Code, nil
}

// formatTestFiles форматирует тестовые файлы для отправки в LLM
func (h *VibeCodingHandler) formatTestFiles(tests map[string]string) string {
	var result strings.Builder

	for filename, content := range tests {
		result.WriteString(fmt.Sprintf("\n=== %s ===\n", filename))
		if len(content) > 1000 {
			result.WriteString(content[:1000])
			result.WriteString("\n... (truncated)")
		} else {
			result.WriteString(content)
		}
		result.WriteString("\n")
	}

	return result.String()
}

// validateTestsWithLLM валидирует тесты через LLM перед выполнением
func (h *VibeCodingHandler) validateTestsWithLLM(ctx context.Context, session *VibeCodingSession, tests map[string]string) (map[string]string, error) {
	log.Printf("🧠 Validating tests with LLM before execution")

	// Подготавливаем системный промпт для валидации тестов
	systemPrompt := `You are an expert test reviewer and validator. Your task is to analyze generated test files and ensure they are:
1. Syntactically correct
2. Follow best practices for the given programming language
3. Actually test the code they're supposed to test (CRITICAL: only test functions/classes that actually exist)
4. Are runnable and don't have obvious errors
5. Don't test non-existent functions, classes, or methods

CRITICAL VALIDATION RULES:
- Only test functions and classes that are explicitly listed in the "AVAILABLE FUNCTIONS AND CLASSES" section
- Remove or fix any tests that try to test non-existent functions or classes
- Ensure imports correspond to actual project files
- Verify that method calls exist on the classes being tested

Respond with a JSON object matching this exact schema:

{
  "status": "ok|needs_fix|error",
  "issues": [
    {
      "filename": "test_file.py",
      "issue": "specific description of the issue",
      "severity": "critical|warning|info", 
      "fix": "specific fix to apply",
      "line_number": 42
    }
  ],
  "fixed_tests": {
    "test_file.py": "corrected test content"
  },
  "reasoning": "brief explanation of what was checked and why",
  "suggestions": ["improvement suggestions"]
}

Guidelines:
- Use "ok" if tests are good as-is
- Use "needs_fix" if tests have issues but can be fixed
- Use "error" if tests are completely broken or test non-existent code
- Only include "fixed_tests" if status is "needs_fix" and you can fix them
- Be specific about issues and fixes
- Remove any test files that are unnecessary or cannot be fixed
- Flag tests that reference functions/classes not in the available list as "critical" issues`

	// Подготавливаем детальный контекст для валидации
	userPrompt := fmt.Sprintf(`Please validate these test files for a %s project:

PROJECT CONTEXT:
Language: %s
Project: %s

%s

TEST FILES TO VALIDATE:
%s

CRITICAL VALIDATION REQUIREMENTS:
1. Every function/method call in tests MUST exist in the project files
2. Every class instantiation MUST reference existing classes
3. Every import MUST correspond to actual project files
4. No tests should reference non-existent code elements
5. All syntax must be correct and runnable
6. Test structure must follow %s testing conventions

Carefully cross-reference all test code against the available functions and classes listed above.`,
		session.Analysis.Language,
		session.Analysis.Language,
		session.ProjectName,
		h.formatProjectFilesForValidation(session.Files),
		h.formatTestFilesForValidation(tests),
		session.Analysis.Language)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	log.Printf("🔍 Requesting test validation from LLM")

	maxAttempts := 3
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, err := h.llmClient.Generate(ctx, messages)
		if err != nil {
			lastError = fmt.Errorf("LLM validation request failed: %w", err)
			log.Printf("❌ LLM validation attempt %d failed: %v", attempt, err)
			continue
		}

		// Парсим JSON ответ
		var validationResponse TestLLMValidationResponse
		if err := json.Unmarshal([]byte(response.Content), &validationResponse); err != nil {
			// Пытаемся извлечь JSON из markdown блока
			content := response.Content
			if strings.Contains(content, "```json") {
				start := strings.Index(content, "```json") + 7
				end := strings.Index(content[start:], "```")
				if end > 0 {
					content = strings.TrimSpace(content[start : start+end])
				}
			}

			if err := json.Unmarshal([]byte(content), &validationResponse); err != nil {
				lastError = fmt.Errorf("failed to parse LLM validation response: %w", err)
				log.Printf("⚠️ Failed to parse LLM response attempt %d: %v", attempt, err)
				log.Printf("Raw response: %s", response.Content)
				continue
			}
		}

		log.Printf("🔍 LLM validation result: status=%s, issues=%d", validationResponse.Status, len(validationResponse.Issues))
		if validationResponse.Reasoning != "" {
			log.Printf("🧠 LLM reasoning: %s", validationResponse.Reasoning)
		}

		switch validationResponse.Status {
		case "ok":
			log.Printf("✅ LLM approved all tests as-is")
			return tests, nil

		case "needs_fix":
			if len(validationResponse.FixedTests) > 0 {
				log.Printf("🔧 LLM provided %d fixed test files", len(validationResponse.FixedTests))
				// Логируем найденные проблемы
				for _, issue := range validationResponse.Issues {
					log.Printf("  🐛 Issue in %s: %s (severity: %s)", issue.Filename, issue.Issue, issue.Severity)
				}
				return validationResponse.FixedTests, nil
			} else {
				lastError = fmt.Errorf("LLM says tests need fixing but provided no fixed tests")
				log.Printf("⚠️ LLM indicated fixes needed but provided no fixed tests")
				continue
			}

		case "error":
			lastError = fmt.Errorf("LLM validation failed: tests have critical issues")
			log.Printf("❌ LLM validation failed: tests have critical issues")
			continue

		default:
			lastError = fmt.Errorf("unknown LLM validation status: %s", validationResponse.Status)
			log.Printf("⚠️ Unknown validation status: %s", validationResponse.Status)
			continue
		}
	}

	return nil, fmt.Errorf("LLM test validation failed after %d attempts: %w", maxAttempts, lastError)
}

// formatProjectFilesForValidation форматирует файлы проекта для контекста валидации
func (h *VibeCodingHandler) formatProjectFilesForValidation(files map[string]string) string {
	var result strings.Builder

	// Сначала создаем обзор всех функций и классов
	result.WriteString("AVAILABLE FUNCTIONS AND CLASSES:\n")
	result.WriteString(h.extractFunctionsAndClasses(files))
	result.WriteString("\n\nPROJECT FILES (sample content):\n")

	fileCount := 0
	maxFiles := 8 // Увеличиваем лимит файлов

	for filename, content := range files {
		if fileCount >= maxFiles {
			result.WriteString("... (additional files not shown for brevity)\n")
			break
		}

		result.WriteString(fmt.Sprintf("=== %s ===\n", filename))
		if len(content) > 1500 { // Увеличиваем лимит символов
			result.WriteString(content[:1500])
			result.WriteString("\n... (truncated)\n")
		} else {
			result.WriteString(content)
		}
		result.WriteString("\n\n")
		fileCount++
	}

	return result.String()
}

// extractFunctionsAndClasses извлекает список функций и классов из файлов проекта
func (h *VibeCodingHandler) extractFunctionsAndClasses(files map[string]string) string {
	var result strings.Builder

	for filename, content := range files {
		lines := strings.Split(content, "\n")
		var functions, classes []string

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Python/Go function detection
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "func ") {
				funcName := h.extractNameFromDefinition(trimmed)
				if funcName != "" {
					functions = append(functions, funcName)
				}
			}

			// Python/Go/Java class detection
			if strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, "struct") {
				className := h.extractNameFromDefinition(trimmed)
				if className != "" {
					classes = append(classes, className)
				}
			}
		}

		if len(functions) > 0 || len(classes) > 0 {
			result.WriteString(fmt.Sprintf("File: %s\n", filename))
			if len(classes) > 0 {
				result.WriteString(fmt.Sprintf("  Classes: %s\n", strings.Join(classes, ", ")))
			}
			if len(functions) > 0 {
				result.WriteString(fmt.Sprintf("  Functions: %s\n", strings.Join(functions, ", ")))
			}
		}
	}

	return result.String()
}

// extractNameFromDefinition извлекает имя функции или класса из строки определения
func (h *VibeCodingHandler) extractNameFromDefinition(line string) string {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return ""
	}

	// Для "def function_name(" или "func functionName("
	if parts[0] == "def" {
		name := parts[1]
		if idx := strings.Index(name, "("); idx != -1 {
			return name[:idx]
		}
		return name
	}

	// Для Go функций: "func functionName(" или "func (receiver) MethodName("
	if parts[0] == "func" {
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "(") {
			// Go метод: func (c *Config) GetName() string {
			// Ищем имя метода после закрывающей скобки
			for i := 2; i < len(parts); i++ {
				name := parts[i]
				if idx := strings.Index(name, "("); idx != -1 {
					return name[:idx]
				}
				if name != "" && !strings.Contains(name, ")") {
					return name
				}
			}
		} else {
			// Go функция: func functionName() {
			name := parts[1]
			if idx := strings.Index(name, "("); idx != -1 {
				return name[:idx]
			}
			return name
		}
	}

	// Для "class ClassName:" или "type StructName struct"
	if parts[0] == "class" || parts[0] == "type" {
		name := parts[1]
		if idx := strings.Index(name, "("); idx != -1 {
			return name[:idx]
		}
		if idx := strings.Index(name, ":"); idx != -1 {
			return name[:idx]
		}
		if idx := strings.Index(name, " "); idx != -1 {
			return name[:idx]
		}
		return name
	}

	return ""
}

// formatTestFilesForValidation форматирует тестовые файлы для валидации
func (h *VibeCodingHandler) formatTestFilesForValidation(tests map[string]string) string {
	var result strings.Builder

	for filename, content := range tests {
		result.WriteString(fmt.Sprintf("=== %s ===\n", filename))
		result.WriteString(content)
		result.WriteString("\n\n")
	}

	return result.String()
}

// updateMessage обновляет существующее сообщение
func (h *VibeCodingHandler) updateMessage(chatID int64, messageID int, newText string) error {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, h.formatter.EscapeText(newText))
	editMsg.ParseMode = h.formatter.ParseModeValue()
	_, err := h.sender.Send(editMsg)
	return err
}

// sendMessage отправляет сообщение с правильным форматированием
func (h *VibeCodingHandler) sendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, h.formatter.EscapeText(text))
	msg.ParseMode = h.formatter.ParseModeValue()
	_, err := h.sender.Send(msg)
	return err
}

// fixFailingTests исправляет проваливающиеся тесты через анализ вывода LLM
func (h *VibeCodingHandler) fixFailingTests(ctx context.Context, session *VibeCodingSession, testResult *codevalidation.ValidationResult) error {
	log.Printf("🔧 Analyzing failing tests for user %d", session.UserID)

	// Получаем все тестовые файлы из сессии через LLM анализ
	testFiles := make(map[string]string)
	for filename, content := range session.GeneratedFiles {
		// Определяем тестовые файлы через LLM анализ
		if h.isTestFile(ctx, filename, session.Analysis.Language) {
			testFiles[filename] = content
		}
	}

	if len(testFiles) == 0 {
		return fmt.Errorf("no test files found to fix")
	}

	// Создаем запрос на исправление тестов
	query := fmt.Sprintf(`The tests are failing with the following output:

EXIT CODE: %d
OUTPUT:
%s

Please analyze the test failures and fix the test files to make them pass. Focus on:
1. Fixing syntax errors
2. Correcting import statements
3. Fixing assertion logic
4. Ensuring test setup is correct
5. Making tests compatible with the actual code structure

Provide updated test files that will pass.`, testResult.ExitCode, testResult.Output)

	request := VibeCodingRequest{
		Action: "generate_code",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query: query,
		Options: map[string]interface{}{
			"task_type":     "test_fixing",
			"language":      session.Analysis.Language,
			"failing_tests": testFiles,
			"test_output":   testResult.Output,
			"exit_code":     testResult.ExitCode,
		},
	}

	log.Printf("🧠 Requesting test fixes from LLM")
	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to get test fixes from LLM: %w", err)
	}

	if response.Status == "error" {
		return fmt.Errorf("LLM could not fix tests: %s", response.Error)
	}

	if len(response.Code) == 0 {
		return fmt.Errorf("no fixed tests returned from LLM")
	}

	// Обновляем тестовые файлы в сессии
	for filename, content := range response.Code {
		session.AddGeneratedFile(filename, content)
		log.Printf("🔧 Updated test file: %s (%d bytes)", filename, len(content))
	}

	// Копируем обновленные файлы в контейнер
	if err := session.Docker.CopyFilesToContainer(ctx, session.ContainerID, response.Code); err != nil {
		log.Printf("⚠️ Failed to copy fixed tests to container: %v", err)
		return fmt.Errorf("failed to update tests in container: %w", err)
	}

	log.Printf("✅ Successfully applied %d test fixes", len(response.Code))
	return nil
}

// fixTestExecutionIssues исправляет проблемы с выполнением тестов (например, отсутствующие зависимости)
func (h *VibeCodingHandler) fixTestExecutionIssues(ctx context.Context, session *VibeCodingSession, execError error) error {
	log.Printf("🔧 Analyzing test execution issues for user %d", session.UserID)

	// Анализируем ошибку выполнения
	query := fmt.Sprintf(`Test execution failed with the following error:

ERROR: %s

This appears to be an execution issue (not test failure). Please analyze and suggest fixes for:
1. Missing dependencies that need to be installed
2. Environment setup issues
3. Path or import problems
4. Configuration issues

Provide specific commands or configuration changes needed to fix the execution environment.`, execError.Error())

	request := VibeCodingRequest{
		Action: "analyze_error",
		Context: VibeCodingContext{
			ProjectName:     session.ProjectName,
			Language:        session.Analysis.Language,
			Files:           session.Files,
			GeneratedFiles:  session.GeneratedFiles,
			SessionDuration: time.Since(session.StartTime).Round(time.Second).String(),
		},
		Query: query,
		Options: map[string]interface{}{
			"task_type":        "execution_fix",
			"language":         session.Analysis.Language,
			"execution_error":  execError.Error(),
			"current_analysis": session.Analysis,
		},
	}

	log.Printf("🧠 Requesting execution issue fixes from LLM")
	response, err := h.protocolClient.ProcessRequest(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to get execution fixes from LLM: %w", err)
	}

	if response.Status == "error" {
		return fmt.Errorf("LLM could not fix execution issues: %s", response.Error)
	}

	// Если LLM предложил дополнительные команды установки, выполняем их
	if response.Metadata != nil {
		if additionalCommands, exists := response.Metadata["install_commands"].([]string); exists && len(additionalCommands) > 0 {
			log.Printf("🔧 Executing additional installation commands: %v", additionalCommands)

			// Создаем временный анализ с дополнительными командами
			tempAnalysis := &codevalidation.CodeAnalysisResult{
				Language:        session.Analysis.Language,
				DockerImage:     session.Analysis.DockerImage,
				InstallCommands: additionalCommands,
				WorkingDir:      session.Analysis.WorkingDir,
			}

			// Выполняем дополнительные команды установки
			if err := session.Docker.InstallDependencies(ctx, session.ContainerID, tempAnalysis); err != nil {
				log.Printf("⚠️ Failed to execute additional install commands: %v", err)
				return fmt.Errorf("failed to execute additional install commands: %w", err)
			}

			log.Printf("✅ Successfully executed additional install commands")
		}
	}

	return nil
}

// isTestFile определяет, является ли файл тестовым через LLM анализ
func (h *VibeCodingHandler) isTestFile(ctx context.Context, filename string, projectLanguage string) bool {
	// Создаем запрос к LLM для определения тестового файла
	systemPrompt := `You are a programming language expert. Determine if a given filename represents a test file.

Respond with a JSON object matching this exact schema:
{
  "is_test_file": true/false,
  "confidence": "high|medium|low",
  "reasoning": "brief explanation"
}

Consider:
- Common test file naming conventions for the specified language
- Directory structures
- File extensions
- Language-specific testing frameworks`

	userPrompt := fmt.Sprintf(`Is this file a test file?

Language: %s
Filename: %s

Please determine if this filename follows test file naming conventions for %s.`, projectLanguage, filename, projectLanguage)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := h.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("⚠️ LLM test file detection failed for %s: %v, falling back to basic detection", filename, err)
		// Fallback: очень базовое определение
		return strings.Contains(strings.ToLower(filename), "test")
	}

	// Парсим JSON ответ
	var testFileResponse struct {
		IsTestFile bool   `json:"is_test_file"`
		Confidence string `json:"confidence"`
		Reasoning  string `json:"reasoning"`
	}

	content := response.Content
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = strings.TrimSpace(content[start : start+end])
		}
	}

	if err := json.Unmarshal([]byte(content), &testFileResponse); err != nil {
		log.Printf("⚠️ Failed to parse LLM test file response for %s: %v, falling back to basic detection", filename, err)
		return strings.Contains(strings.ToLower(filename), "test")
	}

	log.Printf("🤖 LLM test file analysis for %s: is_test=%v (confidence: %s) - %s",
		filename, testFileResponse.IsTestFile, testFileResponse.Confidence, testFileResponse.Reasoning)

	return testFileResponse.IsTestFile
}

// generateTestWritingPrompt генерирует специализированный промпт для написания тестов через LLM
func (h *VibeCodingHandler) generateTestWritingPrompt(ctx context.Context, session *VibeCodingSession) (string, error) {
	log.Printf("🎯 Generating specialized test writing prompt for %s project", session.Analysis.Language)

	systemPrompt := `You are an expert test writing advisor. Your task is to create a detailed, language-specific prompt for writing high-quality tests that will definitely pass execution.

Respond with a JSON object matching this exact schema:
{
  "test_prompt": "detailed test writing instructions",
  "key_rules": ["rule1", "rule2", "rule3"],
  "testing_framework": "recommended framework",
  "file_naming": "naming convention",
  "best_practices": ["practice1", "practice2"],
  "common_pitfalls": ["pitfall1", "pitfall2"]
}

Create a comprehensive prompt that includes:
- Language-specific testing conventions and frameworks
- Specific rules to avoid test failures
- Import and dependency management
- Assertion patterns that work reliably
- File structure and naming conventions
- Common mistakes to avoid for this language`

	userPrompt := fmt.Sprintf(`Create a specialized test writing prompt for this project:

LANGUAGE: %s
PROJECT: %s
TEST COMMANDS: %s

PROJECT ANALYSIS:
- Language: %s
- Docker Image: %s
- Working Directory: %s
- Install Commands: %d configured
- Test Commands: %d configured

PROJECT FILES STRUCTURE:
%s

REQUIREMENTS:
1. Tests must be syntactically correct and executable
2. Tests should use proper testing frameworks for %s
3. Tests must import only existing functions and classes
4. Tests should follow %s conventions
5. Tests must pass execution without errors
6. Include specific rules to prevent common test failures
7. Consider the project's actual file structure and dependencies

Generate a detailed prompt that will help an AI write tests that actually work and pass execution.`,
		session.Analysis.Language,
		session.ProjectName,
		strings.Join(session.Analysis.TestCommands, "; "),
		session.Analysis.Language,
		session.Analysis.DockerImage,
		session.Analysis.WorkingDir,
		len(session.Analysis.InstallCommands),
		len(session.Analysis.TestCommands),
		h.extractProjectStructureForPrompt(session.Files),
		session.Analysis.Language,
		session.Analysis.Language)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	log.Printf("🔍 Requesting specialized test prompt from LLM")

	response, err := h.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM test prompt generation failed: %w", err)
	}

	// Парсим JSON ответ
	var promptResponse struct {
		TestPrompt       string   `json:"test_prompt"`
		KeyRules         []string `json:"key_rules"`
		TestingFramework string   `json:"testing_framework"`
		FileNaming       string   `json:"file_naming"`
		BestPractices    []string `json:"best_practices"`
		CommonPitfalls   []string `json:"common_pitfalls"`
	}

	content := response.Content
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = strings.TrimSpace(content[start : start+end])
		}
	}

	if err := json.Unmarshal([]byte(content), &promptResponse); err != nil {
		log.Printf("⚠️ Failed to parse LLM prompt response: %v", err)
		return "", fmt.Errorf("failed to parse test prompt response: %w", err)
	}

	// Формируем финальный промпт
	var finalPrompt strings.Builder

	finalPrompt.WriteString(promptResponse.TestPrompt)
	finalPrompt.WriteString("\n\n")

	if promptResponse.TestingFramework != "" {
		finalPrompt.WriteString(fmt.Sprintf("RECOMMENDED TESTING FRAMEWORK: %s\n", promptResponse.TestingFramework))
	}

	if promptResponse.FileNaming != "" {
		finalPrompt.WriteString(fmt.Sprintf("FILE NAMING CONVENTION: %s\n", promptResponse.FileNaming))
	}

	if len(promptResponse.KeyRules) > 0 {
		finalPrompt.WriteString("\nCRITICAL RULES (tests MUST follow these to pass):\n")
		for i, rule := range promptResponse.KeyRules {
			finalPrompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule))
		}
	}

	if len(promptResponse.BestPractices) > 0 {
		finalPrompt.WriteString("\nBEST PRACTICES:\n")
		for _, practice := range promptResponse.BestPractices {
			finalPrompt.WriteString(fmt.Sprintf("- %s\n", practice))
		}
	}

	if len(promptResponse.CommonPitfalls) > 0 {
		finalPrompt.WriteString("\nAVOID THESE PITFALLS:\n")
		for _, pitfall := range promptResponse.CommonPitfalls {
			finalPrompt.WriteString(fmt.Sprintf("⚠️ %s\n", pitfall))
		}
	}

	// Добавляем контекст проекта
	finalPrompt.WriteString(fmt.Sprintf("\nPROJECT CONTEXT:\n%s", h.formatProjectFilesForValidation(session.Files)))

	log.Printf("✅ Generated specialized test prompt (%d chars) with framework: %s",
		len(finalPrompt.String()), promptResponse.TestingFramework)

	return finalPrompt.String(), nil
}

// extractProjectStructureForPrompt извлекает структуру проекта для анализа промпта
func (h *VibeCodingHandler) extractProjectStructureForPrompt(files map[string]string) string {
	var result strings.Builder

	// Анализируем структуру файлов
	result.WriteString("FILE STRUCTURE:\n")
	for filename := range files {
		result.WriteString(fmt.Sprintf("- %s\n", filename))
	}

	// Анализируем зависимости
	result.WriteString("\nDETECTED DEPENDENCIES:\n")
	dependencies := make(map[string]bool)

	for _, content := range files {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Python imports
			if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
				dependencies[trimmed] = true
			}

			// Go imports
			if strings.Contains(trimmed, "\"") && (strings.Contains(line, "import") || strings.Contains(line, "package")) {
				dependencies[trimmed] = true
			}
		}
	}

	count := 0
	for dep := range dependencies {
		if count >= 10 { // Показываем только первые 10
			result.WriteString("... (and more)\n")
			break
		}
		result.WriteString(fmt.Sprintf("- %s\n", dep))
		count++
	}

	return result.String()
}
