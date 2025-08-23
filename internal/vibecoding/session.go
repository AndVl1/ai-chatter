package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ai-chatter/internal/codevalidation"
	"ai-chatter/internal/llm"
)

// Глобальные переменные для доступа к MCP клиенту
var (
	globalSessionManager atomic.Value
	globalMCPClient      atomic.Value
)

// SetGlobalSessionManager устанавливает глобальный менеджер сессий
func SetGlobalSessionManager(sm *SessionManager) {
	globalSessionManager.Store(sm)
}

// SetGlobalMCPClient устанавливает глобальный MCP клиент
func SetGlobalMCPClient(client *VibeCodingMCPClient) {
	globalMCPClient.Store(client)
}

// getGlobalMCPClient возвращает глобальный MCP клиент
func getGlobalMCPClient() *VibeCodingMCPClient {
	if client, ok := globalMCPClient.Load().(*VibeCodingMCPClient); ok {
		return client
	}
	return nil
}

// VibeCodingSession представляет активную сессию вайбкодинга для пользователя
type VibeCodingSession struct {
	UserID         int64                              // ID пользователя Telegram
	ChatID         int64                              // ID чата
	ProjectName    string                             // Название проекта
	StartTime      time.Time                          // Время начала сессии
	Files          map[string]string                  // Файлы проекта: имя -> содержимое
	GeneratedFiles map[string]string                  // Сгенерированные файлы
	ContainerID    string                             // ID Docker контейнера
	Analysis       *codevalidation.CodeAnalysisResult // Анализ проекта (unified from validator)
	TestCommand    string                             // Команда для запуска тестов
	Docker         *DockerAdapter                     // Docker адаптер
	LLMClient      llm.Client                         // LLM клиент для анализа ошибок
	Context        *ProjectContextLLM                 // Сжатый контекст проекта для LLM (LLM-generated)
	mutex          sync.RWMutex                       // Мьютекс для безопасности потоков
}

// SessionManager управляет активными сессиями вайбкодинга
type SessionManager struct {
	sessions  map[int64]*VibeCodingSession // Активные сессии по UserID
	mutex     sync.RWMutex                 // Мьютекс для безопасности потоков
	webServer *WebServer                   // Веб-сервер для отображения сессий
}

// NewSessionManager создает новый менеджер сессий
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[int64]*VibeCodingSession),
	}

	// Запускаем веб-сервер на порту 8080
	sm.webServer = NewWebServer(sm, 8080)
	go func() {
		if err := sm.webServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Failed to start VibeCoding web server: %v", err)
		}
	}()

	return sm
}

// NewSessionManagerWithoutWebServer создает менеджер сессий без веб-сервера
func NewSessionManagerWithoutWebServer() *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*VibeCodingSession),
	}
}

// CreateSession создает новую сессию вайбкодинга
func (sm *SessionManager) CreateSession(userID, chatID int64, projectName string, files map[string]string, llmClient llm.Client) (*VibeCodingSession, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Проверяем, есть ли уже активная сессия у пользователя
	if existingSession, exists := sm.sessions[userID]; exists {
		return nil, fmt.Errorf("у пользователя %d уже есть активная сессия: %s", userID, existingSession.ProjectName)
	}

	// Создаем Docker клиент и адаптер
	var dockerManager codevalidation.DockerManager
	realDockerClient, err := codevalidation.NewDockerClient()
	if err != nil {
		log.Printf("⚠️ Docker not available, using mock client for vibecoding session")
		dockerManager = codevalidation.NewMockDockerClient()
	} else {
		dockerManager = realDockerClient
	}

	dockerAdapter := NewDockerAdapter(dockerManager)

	session := &VibeCodingSession{
		UserID:         userID,
		ChatID:         chatID,
		ProjectName:    projectName,
		StartTime:      time.Now(),
		Files:          make(map[string]string),
		GeneratedFiles: make(map[string]string),
		Docker:         dockerAdapter,
		LLMClient:      llmClient,
	}

	// Копируем файлы
	for filename, content := range files {
		session.Files[filename] = content
	}

	sm.sessions[userID] = session
	log.Printf("🔥 Created vibecoding session for user %d: %s", userID, projectName)

	return session, nil
}

// CreatedAt возвращает время создания сессии для совместимости с MCP
func (s *VibeCodingSession) CreatedAt() time.Time {
	return s.StartTime
}

// GetSession получает активную сессию пользователя
func (sm *SessionManager) GetSession(userID int64) *VibeCodingSession {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[userID]
	if !exists {
		return nil
	}
	return session
}

// GetAllSessions возвращает все активные сессии (для админки)
func (sm *SessionManager) GetAllSessions() map[int64]*VibeCodingSession {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Создаем копию карты для безопасности
	sessionsCopy := make(map[int64]*VibeCodingSession)
	for userID, session := range sm.sessions {
		sessionsCopy[userID] = session
	}
	return sessionsCopy
}

// EndSession завершает сессию пользователя
func (sm *SessionManager) EndSession(userID int64) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[userID]
	if !exists {
		return fmt.Errorf("у пользователя %d нет активной сессии", userID)
	}

	// Очищаем ресурсы сессии
	if err := session.Cleanup(); err != nil {
		log.Printf("⚠️ Error cleaning up session for user %d: %v", userID, err)
	}

	delete(sm.sessions, userID)
	log.Printf("🔥 Ended vibecoding session for user %d: %s", userID, session.ProjectName)

	return nil
}

// HasActiveSession проверяет, есть ли у пользователя активная сессия
func (sm *SessionManager) HasActiveSession(userID int64) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	_, exists := sm.sessions[userID]
	return exists
}

// GetActiveSessions возвращает количество активных сессий
func (sm *SessionManager) GetActiveSessions() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return len(sm.sessions)
}

// SetupEnvironment настраивает окружение для проекта с единым LLM запросом для анализа и контекста
func (s *VibeCodingSession) SetupEnvironment(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Printf("🔥 Setting up environment for vibecoding session: %s", s.ProjectName)

	// Примечание: VibeCoding MCP сервер запускается отдельно, клиенты подключаются к нему

	maxAttempts := 3
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🔥 Environment setup attempt %d/%d", attempt, maxAttempts)

		// 1. Выполняем единый анализ проекта и генерацию контекста
		if err := s.analyzeProjectAndGenerateContext(ctx); err != nil {
			lastError = fmt.Errorf("project analysis and context generation failed: %w", err)
			log.Printf("❌ Attempt %d failed: %v", attempt, lastError)
			continue
		}

		// 2. Создаем контейнер
		containerID, err := s.Docker.CreateContainer(ctx, s.Analysis)
		if err != nil {
			lastError = fmt.Errorf("container creation failed: %w", err)
			log.Printf("❌ Attempt %d failed: %v", attempt, lastError)
			continue
		}
		s.ContainerID = containerID

		// 3. Копируем файлы
		if err := s.Docker.CopyFilesToContainer(ctx, s.ContainerID, s.Files); err != nil {
			lastError = fmt.Errorf("file copying failed: %w", err)
			log.Printf("❌ Attempt %d failed: %v", attempt, lastError)
			// Очищаем контейнер при ошибке
			s.Docker.RemoveContainer(ctx, s.ContainerID)
			s.ContainerID = ""
			continue
		}

		// 4. Устанавливаем зависимости
		if err := s.Docker.InstallDependencies(ctx, s.ContainerID, s.Analysis); err != nil {
			lastError = fmt.Errorf("dependency installation failed: %w", err)
			log.Printf("❌ Attempt %d failed: %v", attempt, lastError)

			// Анализируем ошибку и пытаемся исправить конфигурацию
			if attempt < maxAttempts {
				log.Printf("🔧 Analyzing error and trying to fix configuration...")
				if fixedAnalysis, fixErr := s.analyzeAndFixError(ctx, err, s.Analysis, attempt); fixErr == nil {
					s.Analysis = fixedAnalysis
					log.Printf("✅ Configuration updated, retrying with new settings")
				} else {
					log.Printf("⚠️ Could not fix configuration: %v", fixErr)
				}
			}

			// Очищаем контейнер при ошибке
			s.Docker.RemoveContainer(ctx, s.ContainerID)
			s.ContainerID = ""
			continue
		}

		// 5. Генерируем команду для тестов
		s.TestCommand = s.generateTestCommand()

		// 6. Сохраняем созданный контекст в файлы
		if s.Context != nil {
			if err := s.saveContextFiles(s.Context); err != nil {
				log.Printf("⚠️ Failed to save context files: %v", err)
				// Не прерываем настройку, файлы контекста не критичны
			} else {
				log.Printf("✅ Generated compressed project context with %d files, %d/%d tokens used",
					len(s.Context.Files), s.Context.TokensUsed, s.Context.TokensLimit)
			}
		}

		log.Printf("✅ Environment setup successful on attempt %d", attempt)
		return nil
	}

	return fmt.Errorf("environment setup failed after %d attempts: %w", maxAttempts, lastError)
}

// analyzeProjectAndGenerateContext выполняет анализ проекта и генерацию контекста в одном запросе
func (s *VibeCodingSession) analyzeProjectAndGenerateContext(ctx context.Context) error {
	log.Printf("📊🧠 Analyzing VibeCoding project and generating context with %d files using LLM", len(s.Files))

	if s.LLMClient == nil {
		return fmt.Errorf("LLM client not available")
	}

	// Создаем системный промпт для объединенного анализа
	systemPrompt := `You are an expert DevOps engineer and code analyst. Your task is to:

1. ANALYZE the project for environment setup (Docker image, dependencies, commands)
2. GENERATE a compressed project context for AI understanding

Provide a JSON response with this exact structure:
{
  "analysis": {
    "language": "primary programming language",
    "framework": "detected framework (if any)",
    "docker_image": "appropriate docker image:tag",
    "install_commands": ["list", "of", "install", "commands"],
    "validation_commands": ["list", "of", "validation", "commands"],
    "test_commands": ["list", "of", "test", "commands"],
    "working_dir": "working directory (usually /workspace)",
    "project_type": "type description",
    "dependencies": ["key", "dependencies"],
    "reasoning": "brief explanation of choices"
  },
  "context": {
    "description": "Brief project description (max 100 chars)",
    "language": "same as analysis.language",
    "structure": {
      "directories": [
        {"path": "dirname", "purpose": "directory purpose", "file_count": 0}
      ],
      "file_types": [
        {"extension": ".ext", "language": "Language", "count": 0}
      ]
    },
    "dependencies": ["extracted", "dependencies"],
    "files": {
      "path/to/file.ext": {
        "summary": "Brief file description",
        "key_elements": ["main", "functions", "classes"],
        "purpose": "File's role in project",
        "dependencies": ["other", "files"],
        "type": "file_type"
      }
    }
  }
}

IMPORTANT:
- Be specific about Docker images (use exact tags like golang:1.22, python:3.11-slim)
- Include complete install commands for the detected language/framework
- Generate meaningful file summaries focusing on architecture and key components
- Keep file summaries concise but informative
- Focus on the most important files first
- Extract real dependencies from package files (go.mod, package.json, requirements.txt)
`

	// Подготавливаем информацию о файлах проекта
	fileList := make([]string, 0, len(s.Files))
	fileContents := make(map[string]string)

	for filename, content := range s.Files {
		fileList = append(fileList, filename)
		// Ограничиваем размер содержимого для включения в промпт
		if len(content) > 1000 {
			fileContents[filename] = content[:1000] + "... (truncated)"
		} else {
			fileContents[filename] = content
		}
	}

	// Создаем пользовательский промпт
	userPrompt := fmt.Sprintf(`PROJECT: %s
TOTAL FILES: %d

FILE LIST:
%s

KEY FILE CONTENTS:
%s

Please analyze this project and generate both environment setup configuration and compressed context.
Focus on:
1. Detecting the correct language/framework and appropriate Docker setup
2. Extracting key architectural components and file purposes
3. Understanding dependencies and project structure
4. Creating concise but informative file summaries`,
		s.ProjectName,
		len(s.Files),
		strings.Join(fileList, "\n"),
		s.formatFileContentsForPrompt(fileContents))

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	log.Printf("🧠 Requesting combined analysis and context generation from LLM...")
	response, err := s.LLMClient.Generate(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to get combined analysis: %w", err)
	}

	// Парсим объединенный ответ
	var combinedResult struct {
		Analysis struct {
			Language           string   `json:"language"`
			Framework          string   `json:"framework"`
			DockerImage        string   `json:"docker_image"`
			InstallCommands    []string `json:"install_commands"`
			ValidationCommands []string `json:"validation_commands"`
			TestCommands       []string `json:"test_commands"`
			WorkingDir         string   `json:"working_dir"`
			ProjectType        string   `json:"project_type"`
			Dependencies       []string `json:"dependencies"`
			Reasoning          string   `json:"reasoning"`
		} `json:"analysis"`
		Context struct {
			Description string `json:"description"`
			Language    string `json:"language"`
			Structure   struct {
				Directories []struct {
					Path      string `json:"path"`
					Purpose   string `json:"purpose"`
					FileCount int    `json:"file_count"`
				} `json:"directories"`
				FileTypes []struct {
					Extension string `json:"extension"`
					Language  string `json:"language"`
					Count     int    `json:"count"`
				} `json:"file_types"`
			} `json:"structure"`
			Dependencies []string `json:"dependencies"`
			Files        map[string]struct {
				Summary      string   `json:"summary"`
				KeyElements  []string `json:"key_elements"`
				Purpose      string   `json:"purpose"`
				Dependencies []string `json:"dependencies"`
				Type         string   `json:"type"`
			} `json:"files"`
		} `json:"context"`
	}

	if err := json.Unmarshal([]byte(response.Content), &combinedResult); err != nil {
		log.Printf("⚠️ Failed to parse combined analysis response: %v", err)
		log.Printf("Raw response: %s", response.Content[:min(500, len(response.Content))])
		return fmt.Errorf("failed to parse combined analysis response: %w", err)
	}

	// Устанавливаем результаты анализа проекта
	s.Analysis = &codevalidation.CodeAnalysisResult{
		Language:        combinedResult.Analysis.Language,
		Framework:       combinedResult.Analysis.Framework,
		DockerImage:     combinedResult.Analysis.DockerImage,
		InstallCommands: combinedResult.Analysis.InstallCommands,
		Commands:        combinedResult.Analysis.ValidationCommands,
		TestCommands:    combinedResult.Analysis.TestCommands,
		WorkingDir:      combinedResult.Analysis.WorkingDir,
		ProjectType:     combinedResult.Analysis.ProjectType,
		Dependencies:    combinedResult.Analysis.Dependencies,
		Reasoning:       combinedResult.Analysis.Reasoning,
	}

	// Создаем контекст проекта из ответа LLM
	s.Context = &ProjectContextLLM{
		ProjectName:  s.ProjectName,
		Language:     combinedResult.Context.Language,
		GeneratedAt:  time.Now(),
		TotalFiles:   len(s.Files),
		Description:  combinedResult.Context.Description,
		Dependencies: combinedResult.Context.Dependencies,
		Files:        make(map[string]LLMFileContext),
		TokensLimit:  5000,
		Structure: ProjectStructure{
			Directories: make([]Directory, 0),
			FileTypes:   make([]FileType, 0),
		},
	}

	// Конвертируем структуру каталогов
	for _, dir := range combinedResult.Context.Structure.Directories {
		s.Context.Structure.Directories = append(s.Context.Structure.Directories, Directory{
			Path:      dir.Path,
			Purpose:   dir.Purpose,
			FileCount: dir.FileCount,
		})
	}

	// Конвертируем типы файлов
	for _, ft := range combinedResult.Context.Structure.FileTypes {
		s.Context.Structure.FileTypes = append(s.Context.Structure.FileTypes, FileType{
			Extension: ft.Extension,
			Language:  ft.Language,
			Count:     ft.Count,
		})
	}

	// Конвертируем информацию о файлах
	tokenEstimator := &TokenEstimator{}
	totalTokens := 0
	for filePath, fileInfo := range combinedResult.Context.Files {
		fileContext := LLMFileContext{
			Path:         filePath,
			Type:         fileInfo.Type,
			Size:         len(s.Files[filePath]),
			LastModified: time.Now(),
			Summary:      fileInfo.Summary,
			KeyElements:  fileInfo.KeyElements,
			Purpose:      fileInfo.Purpose,
			Dependencies: fileInfo.Dependencies,
			NeedsUpdate:  false,
		}

		// Оцениваем токены
		fileContext.TokensUsed = tokenEstimator.EstimateTokens(
			fileContext.Summary +
				strings.Join(fileContext.KeyElements, " ") +
				fileContext.Purpose)
		totalTokens += fileContext.TokensUsed

		s.Context.Files[filePath] = fileContext
	}
	s.Context.TokensUsed = totalTokens

	log.Printf("🔥 Combined analysis complete: %s (%s)", s.Analysis.Language, s.Analysis.DockerImage)
	log.Printf("📦 Install commands: %v", s.Analysis.InstallCommands)
	log.Printf("⚡ Validation commands: %v", s.Analysis.Commands)
	log.Printf("🧪 Test commands: %v", s.Analysis.TestCommands)
	log.Printf("🧠 Generated context: %d files, %d tokens, '%s'", len(s.Context.Files), s.Context.TokensUsed, s.Context.Description)

	return nil
}

// analyzeProject анализирует проект используя LLM (unified approach from validator.go) - DEPRECATED
func (s *VibeCodingSession) analyzeProject(ctx context.Context) error {
	log.Printf("📊 Analyzing VibeCoding project with %d files using LLM", len(s.Files))

	// Используем унифицированный подход из CodeValidationWorkflow
	workflow := codevalidation.NewCodeValidationWorkflow(s.LLMClient, nil)

	analysis, err := workflow.AnalyzeProjectForVibeCoding(ctx, s.Files)
	if err != nil {
		return fmt.Errorf("failed to analyze project: %w", err)
	}

	s.Analysis = analysis
	log.Printf("🔥 VibeCoding project analysis complete: %s (%s)", s.Analysis.Language, s.Analysis.DockerImage)
	log.Printf("📦 Install commands: %v", s.Analysis.InstallCommands)
	log.Printf("⚡ Validation commands: %v", s.Analysis.Commands)

	return nil
}

// formatFileContentsForPrompt форматирует содержимое файлов для включения в промпт
func (s *VibeCodingSession) formatFileContentsForPrompt(fileContents map[string]string) string {
	var result strings.Builder
	count := 0
	maxFiles := 10 // Ограничиваем количество файлов в промпте

	for filename, content := range fileContents {
		if count >= maxFiles {
			result.WriteString("... (and more files)\n")
			break
		}

		result.WriteString(fmt.Sprintf("\n=== %s ===\n", filename))
		result.WriteString(content)
		result.WriteString("\n")
		count++
	}

	return result.String()
}

// Note: Hardcoded language detection methods removed - now using unified LLM-based approach from validator.go

// generateTestCommand генерирует команду для запуска тестов на основе анализа LLM
func (s *VibeCodingSession) generateTestCommand() string {
	// Используем специальные команды тестирования из LLM анализа (приоритет)
	if len(s.Analysis.TestCommands) > 0 {
		log.Printf("🧪 Using test command from LLM analysis: %s", s.Analysis.TestCommands[0])
		return s.Analysis.TestCommands[0]
	}

	// Fallback на обычные команды если TestCommands пустые
	for _, cmd := range s.Analysis.Commands {
		// Базовая проверка на наличие "test" в команде
		if strings.Contains(strings.ToLower(cmd), "test") {
			log.Printf("🧪 Found test-like command from validation commands: %s", cmd)
			return cmd
		}
	}

	// Последний fallback - используем первую команду валидации как тест
	if len(s.Analysis.Commands) > 0 {
		log.Printf("⚠️ No test command found, using first validation command as test: %s", s.Analysis.Commands[0])
		return s.Analysis.Commands[0]
	}

	log.Printf("⚠️ No commands available from LLM analysis, using fallback")
	return "echo 'No test command available from LLM analysis'"
}

// AddGeneratedFile добавляет сгенерированный файл в сессию
func (s *VibeCodingSession) AddGeneratedFile(filename, content string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.GeneratedFiles[filename] = content
	log.Printf("🔥 Added generated file to session: %s (%d bytes)", filename, len(content))
}

// GetAllFiles возвращает все файлы (исходные + сгенерированные)
func (s *VibeCodingSession) GetAllFiles() map[string]string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	allFiles := make(map[string]string)

	// Копируем исходные файлы
	for filename, content := range s.Files {
		allFiles[filename] = content
	}

	// Копируем сгенерированные файлы
	for filename, content := range s.GeneratedFiles {
		allFiles[filename] = content
	}

	return allFiles
}

// Cleanup очищает ресурсы сессии
func (s *VibeCodingSession) Cleanup() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.ContainerID != "" {
		ctx := context.Background()
		if err := s.Docker.RemoveContainer(ctx, s.ContainerID); err != nil {
			return fmt.Errorf("failed to remove container %s: %w", s.ContainerID, err)
		}
		s.ContainerID = ""
	}

	log.Printf("🔥 Session cleanup completed")
	return nil
}

// ExecuteCommand выполняет команду в контейнере сессии
func (s *VibeCodingSession) ExecuteCommand(ctx context.Context, command string) (*codevalidation.ValidationResult, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.ContainerID == "" {
		return nil, fmt.Errorf("session environment not set up")
	}

	// Создаем временный анализ для выполнения команды
	tempAnalysis := &codevalidation.CodeAnalysisResult{
		Language:    s.Analysis.Language,
		DockerImage: s.Analysis.DockerImage,
		Commands:    []string{command},
		WorkingDir:  s.Analysis.WorkingDir,
	}

	return s.Docker.ExecuteValidation(ctx, s.ContainerID, tempAnalysis)
}

// ListFiles возвращает список всех файлов в сессии
func (s *VibeCodingSession) ListFiles(ctx context.Context) ([]string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var files []string
	for filename := range s.Files {
		files = append(files, filename)
	}
	for filename := range s.GeneratedFiles {
		files = append(files, filename+" (generated)")
	}

	return files, nil
}

// ReadFile читает содержимое файла
func (s *VibeCodingSession) ReadFile(ctx context.Context, filename string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Сначала ищем в обычных файлах
	if content, exists := s.Files[filename]; exists {
		return content, nil
	}

	// Потом в сгенерированных файлах
	if content, exists := s.GeneratedFiles[filename]; exists {
		return content, nil
	}

	return "", fmt.Errorf("file not found: %s", filename)
}

// WriteFile записывает файл в сессию
func (s *VibeCodingSession) WriteFile(ctx context.Context, filename, content string, generated bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if generated {
		s.GeneratedFiles[filename] = content
	} else {
		s.Files[filename] = content
	}

	// Также копируем в контейнер если он существует
	if s.ContainerID != "" {
		files := map[string]string{filename: content}
		if err := s.Docker.CopyFilesToContainer(ctx, s.ContainerID, files); err != nil {
			log.Printf("⚠️ Failed to copy file to container: %v", err)
			// Не возвращаем ошибку, файл все равно сохранен в сессии
		}
	}

	// Обновляем контекст проекта инкриментально (для LLM контекста)
	if filename != "PROJECT_CONTEXT.md" && s.Context != nil && s.LLMClient != nil {
		go func() {
			// Используем инкриментальное обновление для LLM контекста
			generator := NewLLMContextGenerator(s.LLMClient, 5000)
			ctx := context.Background()
			if err := generator.UpdateFileContext(ctx, s.Context, filename, content); err != nil {
				log.Printf("⚠️ Failed to update LLM context for file %s: %v", filename, err)
				// Fallback: полное обновление контекста
				if err := s.RefreshProjectContext(); err != nil {
					log.Printf("⚠️ Failed to refresh project context after file write: %v", err)
				}
			} else {
				log.Printf("✅ Updated LLM context for file: %s", filename)
				// Обновляем PROJECT_CONTEXT.md с новым контекстом
				contextMarkdown := s.generateContextMarkdown()
				s.GeneratedFiles["PROJECT_CONTEXT.md"] = contextMarkdown
			}
		}()
	}

	return nil
}

// RemoveFile удаляет файл из сессии и обновляет контекст
func (s *VibeCodingSession) RemoveFile(ctx context.Context, filename string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Удаляем из обычных и сгенерированных файлов
	deleted := false
	if _, exists := s.Files[filename]; exists {
		delete(s.Files, filename)
		deleted = true
	}
	if _, exists := s.GeneratedFiles[filename]; exists {
		delete(s.GeneratedFiles, filename)
		deleted = true
	}

	if !deleted {
		return fmt.Errorf("file not found: %s", filename)
	}

	// Обновляем контекст (удаляем из LLM контекста)
	if filename != "PROJECT_CONTEXT.md" && s.Context != nil && s.LLMClient != nil {
		go func() {
			generator := NewLLMContextGenerator(s.LLMClient, 5000)
			generator.RemoveFileContext(s.Context, filename)
			log.Printf("✅ Removed file from LLM context: %s", filename)

			// Обновляем PROJECT_CONTEXT.md
			contextMarkdown := s.generateContextMarkdown()
			s.GeneratedFiles["PROJECT_CONTEXT.md"] = contextMarkdown
		}()
	}

	log.Printf("🔥 Removed file from session: %s", filename)
	return nil
}

// ValidateCode валидирует код файла
func (s *VibeCodingSession) ValidateCode(ctx context.Context, code, filename string) (*codevalidation.ValidationResult, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.ContainerID == "" {
		return nil, fmt.Errorf("session environment not set up")
	}

	// Используем команды валидации из анализа
	if len(s.Analysis.Commands) == 0 {
		return &codevalidation.ValidationResult{
			Success:  true,
			Output:   "No validation commands available",
			ExitCode: 0,
		}, nil
	}

	// Создаем временный анализ для валидации
	tempAnalysis := &codevalidation.CodeAnalysisResult{
		Language:    s.Analysis.Language,
		DockerImage: s.Analysis.DockerImage,
		Commands:    s.Analysis.Commands,
		WorkingDir:  s.Analysis.WorkingDir,
	}

	return s.Docker.ExecuteValidation(ctx, s.ContainerID, tempAnalysis)
}

// GetSessionInfo возвращает информацию о сессии
func (s *VibeCodingSession) GetSessionInfo() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	info := map[string]interface{}{
		"project_name":    s.ProjectName,
		"language":        s.Analysis.Language,
		"start_time":      s.StartTime,
		"files_count":     len(s.Files),
		"generated_count": len(s.GeneratedFiles),
		"test_command":    s.TestCommand,
		"container_id":    s.ContainerID,
	}

	// Добавляем информацию о контексте если доступен
	if s.Context != nil {
		info["context_available"] = true
		info["context_generated_at"] = s.Context.GeneratedAt
		info["context_total_files"] = s.Context.TotalFiles
		info["context_tokens_used"] = s.Context.TokensUsed
		info["context_tokens_limit"] = s.Context.TokensLimit
		info["context_files_count"] = len(s.Context.Files)
	} else {
		info["context_available"] = false
	}

	return info
}

// analyzeAndFixError анализирует ошибку и предлагает исправления конфигурации
func (s *VibeCodingSession) analyzeAndFixError(ctx context.Context, setupError error, currentAnalysis *codevalidation.CodeAnalysisResult, attempt int) (*codevalidation.CodeAnalysisResult, error) {
	if s.LLMClient == nil {
		return nil, fmt.Errorf("LLM client not available for error analysis")
	}

	log.Printf("🔍 Analyzing setup error on attempt %d: %v", attempt, setupError)

	// Создаем системный промпт для анализа ошибок
	systemPrompt := `You are an expert DevOps engineer specializing in fixing environment setup issues. 
Analyze the error and current project configuration, then suggest concrete fixes.

Your task:
1. Identify the root cause of the error
2. Suggest specific changes to docker image, install commands, or project configuration
3. For Go projects: Pay special attention to Go version requirements in go.mod files
4. For version conflicts: Suggest appropriate Docker images with correct tool versions
5. Always provide SPECIFIC alternative docker images and commands

Common issues and solutions:
- Go version conflicts: Use golang:1.21 or golang:1.22 images
- Python version issues: Use python:3.9-slim or python:3.11-slim
- Node.js version issues: Use node:18-alpine or node:20-alpine
- Missing system packages: Add apt-get install commands

Return your response as a JSON object with this exact schema:
{
  "analysis": "brief explanation of what went wrong and why",
  "root_cause": "specific root cause (e.g., 'go_version_mismatch', 'missing_dependency', 'wrong_docker_image')",
  "suggested_fixes": {
    "docker_image": "alternative docker image if needed (provide specific image:tag, not null)",
    "install_commands": ["updated install commands array with specific commands"],
    "working_dir": "updated working directory (or null to keep current)",
    "additional_setup": ["any additional setup commands if needed"],
    "pre_install_commands": ["commands to run before main install commands"]
  },
  "confidence": "high|medium|low",
  "retry_recommended": true
}`

	// Подготавливаем контекст для анализа
	errorContext := fmt.Sprintf(`ERROR DETAILS:
Error: %s

CURRENT CONFIGURATION:
Language: %s
Docker Image: %s
Install Commands: %s
Working Directory: %s
Project Type: %s

PROJECT FILES:
%s

PROJECT SPECIFIC DETAILS:
%s

ATTEMPT NUMBER: %d (max 3 attempts)

Please analyze this error and suggest fixes to make the environment setup succeed. 
Be very specific about Docker image versions and install commands.`,
		setupError.Error(),
		currentAnalysis.Language,
		currentAnalysis.DockerImage,
		strings.Join(currentAnalysis.InstallCommands, ", "),
		currentAnalysis.WorkingDir,
		currentAnalysis.ProjectType,
		s.getProjectFilesSummary(),
		s.getProjectSpecificDetails(currentAnalysis.Language),
		attempt)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: errorContext},
	}

	log.Printf("🧠 Requesting error analysis from LLM")

	response, err := s.LLMClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to get error analysis: %w", err)
	}

	// Парсим JSON ответ
	var analysisResult struct {
		Analysis       string `json:"analysis"`
		RootCause      string `json:"root_cause"`
		SuggestedFixes struct {
			DockerImage        string   `json:"docker_image"`
			InstallCommands    []string `json:"install_commands"`
			WorkingDir         *string  `json:"working_dir"`
			AdditionalSetup    []string `json:"additional_setup"`
			PreInstallCommands []string `json:"pre_install_commands"`
		} `json:"suggested_fixes"`
		Confidence       string `json:"confidence"`
		RetryRecommended bool   `json:"retry_recommended"`
	}

	if err := json.Unmarshal([]byte(response.Content), &analysisResult); err != nil {
		log.Printf("⚠️ Failed to parse LLM error analysis response: %v", err)
		log.Printf("Raw response: %s", response.Content)
		return nil, fmt.Errorf("failed to parse error analysis response: %w", err)
	}

	log.Printf("🔧 Error analysis: %s", analysisResult.Analysis)
	log.Printf("🎯 Root cause: %s (confidence: %s, retry: %v)", analysisResult.RootCause, analysisResult.Confidence, analysisResult.RetryRecommended)

	// Проверяем, рекомендуется ли повтор
	if !analysisResult.RetryRecommended {
		log.Printf("❌ LLM does not recommend retry for this error type")
		return nil, fmt.Errorf("error analysis suggests this issue cannot be fixed automatically: %s", analysisResult.Analysis)
	}

	// Применяем предлагаемые исправления
	fixedAnalysis := &codevalidation.CodeAnalysisResult{
		Language:        currentAnalysis.Language,
		Framework:       currentAnalysis.Framework,
		Dependencies:    currentAnalysis.Dependencies,
		InstallCommands: make([]string, 0), // Начинаем с чистого списка
		Commands:        currentAnalysis.Commands,
		DockerImage:     currentAnalysis.DockerImage,
		ProjectType:     currentAnalysis.ProjectType,
		WorkingDir:      currentAnalysis.WorkingDir,
		Reasoning:       currentAnalysis.Reasoning + fmt.Sprintf(" | Fix attempt %d [%s]: %s", attempt, analysisResult.RootCause, analysisResult.Analysis),
	}

	// Применяем новый Docker образ
	if analysisResult.SuggestedFixes.DockerImage != "" {
		fixedAnalysis.DockerImage = analysisResult.SuggestedFixes.DockerImage
		log.Printf("📦 Updated Docker image: %s (was: %s)", fixedAnalysis.DockerImage, currentAnalysis.DockerImage)
	}

	// Добавляем pre-install команды сначала
	if len(analysisResult.SuggestedFixes.PreInstallCommands) > 0 {
		fixedAnalysis.InstallCommands = append(fixedAnalysis.InstallCommands, analysisResult.SuggestedFixes.PreInstallCommands...)
		log.Printf("🔧 Added pre-install commands: %v", analysisResult.SuggestedFixes.PreInstallCommands)
	}

	// Добавляем основные команды установки
	if len(analysisResult.SuggestedFixes.InstallCommands) > 0 {
		fixedAnalysis.InstallCommands = append(fixedAnalysis.InstallCommands, analysisResult.SuggestedFixes.InstallCommands...)
		log.Printf("⚙️ Updated install commands: %v", analysisResult.SuggestedFixes.InstallCommands)
	} else {
		// Если не предложены новые команды, используем старые
		fixedAnalysis.InstallCommands = append(fixedAnalysis.InstallCommands, currentAnalysis.InstallCommands...)
		log.Printf("♻️ Keeping original install commands: %v", currentAnalysis.InstallCommands)
	}

	// Добавляем дополнительные команды установки
	if len(analysisResult.SuggestedFixes.AdditionalSetup) > 0 {
		fixedAnalysis.InstallCommands = append(fixedAnalysis.InstallCommands, analysisResult.SuggestedFixes.AdditionalSetup...)
		log.Printf("➕ Added additional setup commands: %v", analysisResult.SuggestedFixes.AdditionalSetup)
	}

	// Обновляем рабочую директорию если нужно
	if analysisResult.SuggestedFixes.WorkingDir != nil {
		fixedAnalysis.WorkingDir = *analysisResult.SuggestedFixes.WorkingDir
		log.Printf("📁 Updated working directory: %s", fixedAnalysis.WorkingDir)
	}

	return fixedAnalysis, nil
}

// getProjectFilesSummary возвращает краткое описание файлов проекта для анализа
func (s *VibeCodingSession) getProjectFilesSummary() string {
	var summary strings.Builder
	fileCount := 0
	maxFiles := 10 // Ограничиваем количество файлов для анализа

	for filename := range s.Files {
		if fileCount >= maxFiles {
			summary.WriteString("... and more files")
			break
		}
		summary.WriteString(fmt.Sprintf("- %s\n", filename))
		fileCount++
	}

	return summary.String()
}

// getProjectSpecificDetails возвращает специфические детали проекта для анализа ошибок
func (s *VibeCodingSession) getProjectSpecificDetails(language string) string {
	var details strings.Builder

	switch language {
	case "Go":
		// Ищем go.mod файл и извлекаем информацию о версии Go
		if goMod, exists := s.Files["go.mod"]; exists {
			details.WriteString("go.mod content (first 500 chars):\n")
			if len(goMod) > 500 {
				details.WriteString(goMod[:500])
				details.WriteString("\n... (truncated)")
			} else {
				details.WriteString(goMod)
			}
			details.WriteString("\n\n")

			// Попытка извлечь версию Go из go.mod
			if strings.Contains(goMod, "go 1.") {
				lines := strings.Split(goMod, "\n")
				for _, line := range lines {
					if strings.Contains(line, "go 1.") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
						details.WriteString(fmt.Sprintf("DETECTED GO VERSION REQUIREMENT: %s\n", strings.TrimSpace(line)))
						break
					}
				}
			}
		} else {
			details.WriteString("No go.mod file found in project\n")
		}

	case "Python":
		// Ищем requirements.txt или pyproject.toml
		if req, exists := s.Files["requirements.txt"]; exists {
			details.WriteString("requirements.txt content (first 300 chars):\n")
			if len(req) > 300 {
				details.WriteString(req[:300])
				details.WriteString("\n... (truncated)")
			} else {
				details.WriteString(req)
			}
			details.WriteString("\n\n")
		}

		if pyproject, exists := s.Files["pyproject.toml"]; exists {
			details.WriteString("pyproject.toml content (first 300 chars):\n")
			if len(pyproject) > 300 {
				details.WriteString(pyproject[:300])
				details.WriteString("\n... (truncated)")
			} else {
				details.WriteString(pyproject)
			}
			details.WriteString("\n\n")
		}

	case "JavaScript":
		// Ищем package.json
		if pkg, exists := s.Files["package.json"]; exists {
			details.WriteString("package.json content (first 500 chars):\n")
			if len(pkg) > 500 {
				details.WriteString(pkg[:500])
				details.WriteString("\n... (truncated)")
			} else {
				details.WriteString(pkg)
			}
			details.WriteString("\n\n")
		}
	}

	if details.Len() == 0 {
		details.WriteString(fmt.Sprintf("No specific configuration files found for %s project\n", language))
	}

	return details.String()
}

// startMCPServerInContainer запускает VibeCoding MCP сервер внутри контейнера
func (s *VibeCodingSession) startMCPServerInContainer(ctx context.Context) {
	if s.ContainerID == "" {
		log.Printf("⚠️ Cannot start MCP server: no container ID")
		return
	}

	log.Printf("🚀 Starting VibeCoding MCP server in container %s", s.ContainerID)

	// Копируем исполняемый файл MCP сервера в контейнер
	mcpServerPath := "./cmd/vibecoding-mcp-server/vibecoding-mcp-server"
	copyCmd := fmt.Sprintf("docker cp %s %s:/workspace/vibecoding-mcp-server", mcpServerPath, s.ContainerID)

	if _, err := s.Docker.ExecuteValidation(ctx, s.ContainerID, &codevalidation.CodeAnalysisResult{
		Commands: []string{copyCmd},
	}); err != nil {
		log.Printf("❌ Failed to copy MCP server to container: %v", err)
		return
	}

	// Делаем файл исполняемым
	chmodCmd := "chmod +x /workspace/vibecoding-mcp-server"
	if _, err := s.Docker.ExecuteValidation(ctx, s.ContainerID, &codevalidation.CodeAnalysisResult{
		Commands: []string{chmodCmd},
	}); err != nil {
		log.Printf("❌ Failed to make MCP server executable: %v", err)
		return
	}

	// Запускаем MCP сервер в фоне в контейнере
	startCmd := "nohup /workspace/vibecoding-mcp-server > /tmp/mcp-server.log 2>&1 &"
	if _, err := s.Docker.ExecuteValidation(ctx, s.ContainerID, &codevalidation.CodeAnalysisResult{
		Commands: []string{startCmd},
	}); err != nil {
		log.Printf("❌ Failed to start MCP server in container: %v", err)
		return
	}

	log.Printf("✅ VibeCoding MCP server started in container %s", s.ContainerID)
}

// generateProjectContext генерирует сжатый контекст проекта с помощью LLM (синхронно)
func (s *VibeCodingSession) generateProjectContext() error {
	log.Printf("📋 Generating LLM-based compressed project context...")

	// Используем LLM-генератор контекста с лимитом токенов (5000 по умолчанию)
	generator := NewLLMContextGenerator(s.LLMClient, 5000)

	// Получаем все файлы (исходные + сгенерированные)
	allFiles := s.GetAllFiles()

	ctx := context.Background()
	context, err := generator.GenerateContext(ctx, s.ProjectName, allFiles)
	if err != nil {
		return fmt.Errorf("failed to generate LLM context: %w", err)
	}

	s.Context = context

	// Создаем JSON и Markdown файлы контекста
	if err := s.saveContextFiles(context); err != nil {
		log.Printf("⚠️ Failed to save context files: %v", err)
	}

	log.Printf("✅ Generated LLM project context: %d files, %d/%d tokens used",
		len(context.Files), context.TokensUsed, context.TokensLimit)

	return nil
}

// generateContextMarkdown генерирует Markdown представление LLM-генерируемого контекста
func (s *VibeCodingSession) generateContextMarkdown() string {
	if s.Context == nil {
		return "# Project Context\n\nContext not available."
	}

	var md strings.Builder

	md.WriteString("# LLM-Generated Project Context\n\n")
	md.WriteString(fmt.Sprintf("**Generated:** %s\n", s.Context.GeneratedAt.Format("2006-01-02 15:04:05")))
	md.WriteString(fmt.Sprintf("**Project:** %s\n", s.Context.ProjectName))
	md.WriteString(fmt.Sprintf("**Language:** %s\n", s.Context.Language))
	md.WriteString(fmt.Sprintf("**Total Files:** %d\n", s.Context.TotalFiles))
	md.WriteString(fmt.Sprintf("**Tokens Used:** %d / %d\n\n", s.Context.TokensUsed, s.Context.TokensLimit))

	if s.Context.Description != "" {
		md.WriteString(fmt.Sprintf("**Description:** %s\n\n", s.Context.Description))
	}

	// Dependencies
	if len(s.Context.Dependencies) > 0 {
		md.WriteString("## Dependencies\n\n")
		for _, dep := range s.Context.Dependencies {
			md.WriteString(fmt.Sprintf("- %s\n", dep))
		}
		md.WriteString("\n")
	}

	// Project structure
	md.WriteString("## Project Structure\n\n")
	for _, dir := range s.Context.Structure.Directories {
		md.WriteString(fmt.Sprintf("- **%s** (%d files) - %s\n", dir.Path, dir.FileCount, dir.Purpose))
	}
	md.WriteString("\n")

	// File types
	if len(s.Context.Structure.FileTypes) > 0 {
		md.WriteString("### File Types\n\n")
		for _, ft := range s.Context.Structure.FileTypes {
			md.WriteString(fmt.Sprintf("- %s: %d files (%s)\n", ft.Extension, ft.Count, ft.Language))
		}
		md.WriteString("\n")
	}

	// LLM-generated file descriptions
	md.WriteString("## File Descriptions (LLM-Generated)\n\n")
	for filePath, fileContext := range s.Context.Files {
		md.WriteString(fmt.Sprintf("### %s\n", filePath))
		md.WriteString(fmt.Sprintf("**Type:** %s | **Size:** %d bytes | **Last Modified:** %s\n",
			fileContext.Type, fileContext.Size, fileContext.LastModified.Format("2006-01-02 15:04:05")))
		md.WriteString(fmt.Sprintf("**Tokens Used:** %d\n\n", fileContext.TokensUsed))

		if fileContext.Summary != "" {
			md.WriteString(fmt.Sprintf("**Summary:** %s\n\n", fileContext.Summary))
		}

		if fileContext.Purpose != "" {
			md.WriteString(fmt.Sprintf("**Purpose:** %s\n\n", fileContext.Purpose))
		}

		// Key elements
		if len(fileContext.KeyElements) > 0 {
			md.WriteString("**Key Elements:**\n")
			for _, element := range fileContext.KeyElements {
				md.WriteString(fmt.Sprintf("- %s\n", element))
			}
			md.WriteString("\n")
		}

		// Dependencies
		if len(fileContext.Dependencies) > 0 {
			md.WriteString("**File Dependencies:**\n")
			for _, dep := range fileContext.Dependencies {
				md.WriteString(fmt.Sprintf("- %s\n", dep))
			}
			md.WriteString("\n")
		}

		md.WriteString("---\n\n")
	}

	// Usage instructions
	md.WriteString("## Usage Instructions for LLM\n\n")
	md.WriteString(s.generateUsageInstructionsMarkdown())

	return md.String()
}

// saveContextFiles сохраняет контекст в различных форматах (JSON, Markdown)
func (s *VibeCodingSession) saveContextFiles(projectContext *ProjectContextLLM) error {
	log.Printf("💾 Saving context files to project root...")

	// 1. Сохраняем JSON контекст (универсальный формат)
	jsonContent, err := s.generateContextJSON(projectContext)
	if err != nil {
		return fmt.Errorf("failed to generate JSON context: %w", err)
	}

	workingDir := "."
	if s.Analysis != nil && s.Analysis.WorkingDir != "" {
		workingDir = s.Analysis.WorkingDir
	}

	jsonPath := filepath.Join(workingDir, "vibecoding-context.json")
	if err := s.writeFile(jsonPath, jsonContent); err != nil {
		return fmt.Errorf("failed to write JSON context: %w", err)
	}
	log.Printf("💾 ✅ JSON context saved: %s", jsonPath)

	// 2. Сохраняем Markdown контекст (человеко-читаемый)
	mdContent := s.generateContextMarkdown()
	mdPath := filepath.Join(workingDir, "vibecoding-context.md")
	if err := s.writeFile(mdPath, mdContent); err != nil {
		return fmt.Errorf("failed to write Markdown context: %w", err)
	}
	log.Printf("💾 ✅ Markdown context saved: %s", mdPath)

	return nil
}

// generateContextJSON генерирует универсальную JSON структуру контекста
func (s *VibeCodingSession) generateContextJSON(projectContext *ProjectContextLLM) (string, error) {
	log.Printf("🔄 Generating universal JSON context...")

	// Создаем универсальную структуру для JSON
	universalContext := map[string]interface{}{
		"metadata": map[string]interface{}{
			"project_name": projectContext.ProjectName,
			"language":     projectContext.Language,
			"generator":    "LLM",
			"version":      "1.0",
			"generated_at": projectContext.GeneratedAt.Format(time.RFC3339),
			"total_files":  projectContext.TotalFiles,
			"tokens_used":  projectContext.TokensUsed,
			"tokens_limit": projectContext.TokensLimit,
		},
		"description":  projectContext.Description,
		"dependencies": projectContext.Dependencies,
		"structure": map[string]interface{}{
			"directories": projectContext.Structure.Directories,
			"file_types":  projectContext.Structure.FileTypes,
		},
		"files":              make(map[string]interface{}),
		"usage_instructions": s.generateUsageInstructions(),
	}

	// Добавляем информацию о файлах
	filesData := make(map[string]interface{})
	for filePath, fileContext := range projectContext.Files {
		filesData[filePath] = map[string]interface{}{
			"path":          fileContext.Path,
			"type":          fileContext.Type,
			"size":          fileContext.Size,
			"last_modified": fileContext.LastModified.Format(time.RFC3339),
			"summary":       fileContext.Summary,
			"key_elements":  fileContext.KeyElements,
			"purpose":       fileContext.Purpose,
			"dependencies":  fileContext.Dependencies,
			"tokens_used":   fileContext.TokensUsed,
			"needs_update":  fileContext.NeedsUpdate,
		}
	}
	universalContext["files"] = filesData

	// Сериализуем в JSON с красивым форматированием
	jsonBytes, err := json.MarshalIndent(universalContext, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	log.Printf("🔄 ✅ JSON context generated: %d bytes", len(jsonBytes))
	return string(jsonBytes), nil
}

// writeFile записывает содержимое в файл
func (s *VibeCodingSession) writeFile(filePath, content string) error {
	// Создаем директорию если не существует
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Записываем файл
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// GetProjectContext возвращает контекст проекта
func (s *VibeCodingSession) GetProjectContext() *ProjectContextLLM {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.Context
}

// RefreshProjectContext обновляет контекст проекта (например, после изменений)
func (s *VibeCodingSession) RefreshProjectContext() error {
	log.Printf("🔄 Refreshing LLM project context...")

	// Используем новую унифицированную архитектуру для пересоздания контекста
	// Не используем mutex здесь, так как analyzeProjectAndGenerateContext может содержать собственные блокировки
	ctx := context.Background()
	if err := s.analyzeProjectAndGenerateContext(ctx); err != nil {
		return fmt.Errorf("failed to refresh LLM context using unified analysis: %w", err)
	}

	log.Printf("✅ LLM project context refreshed successfully")
	return nil
}

// countTotalKeyElements подсчитывает общее количество ключевых элементов в проекте (LLM контекст)
func (s *VibeCodingSession) countTotalKeyElements() int {
	if s.Context == nil {
		return 0
	}

	total := 0
	for _, file := range s.Context.Files {
		total += len(file.KeyElements)
	}
	return total
}

// countTotalFiles подсчитывает общее количество файлов в LLM контексте
func (s *VibeCodingSession) countTotalFiles() int {
	if s.Context == nil {
		return 0
	}

	return len(s.Context.Files)
}

// ValidateAndFixTests проверяет сгенерированные тесты и запрашивает исправления если они не проходят
func (s *VibeCodingSession) ValidateAndFixTests(ctx context.Context, testFiles []string) error {
	const maxAttempts = 3

	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Printf("🧪 Starting test validation for %d test files", len(testFiles))

	for _, testFile := range testFiles {
		log.Printf("🔍 Validating test file: %s", testFile)

		// Проверяем, является ли файл сгенерированным тестом
		if !s.isTestFile(testFile) {
			log.Printf("⏭️ Skipping non-test file: %s", testFile)
			continue
		}

		// Запускаем тесты для этого файла с несколькими попытками исправления
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			log.Printf("🧪 Running test validation attempt %d/%d for %s", attempt, maxAttempts, testFile)

			// Запуск тестов
			result, err := s.ExecuteCommand(ctx, s.buildTestCommand(testFile))
			if err != nil {
				log.Printf("❌ Failed to execute test command: %v", err)
				return fmt.Errorf("failed to execute test for %s: %w", testFile, err)
			}

			// Если тесты прошли, переходим к следующему файлу
			if result.Success && result.ExitCode == 0 {
				log.Printf("✅ Test %s passed on attempt %d", testFile, attempt)
				break
			}

			// Если тесты провалились и это не последняя попытка, запрашиваем исправления
			if attempt < maxAttempts {
				log.Printf("❌ Test %s failed on attempt %d (exit code: %d), requesting fixes...", testFile, attempt, result.ExitCode)

				if err := s.requestTestFix(ctx, testFile, result.Output); err != nil {
					log.Printf("⚠️ Failed to request test fix: %v", err)
					continue // Попробуем еще раз без исправления
				}
			} else {
				// Последняя попытка - логируем провал
				log.Printf("❌ Test %s failed after %d attempts (final exit code: %d)", testFile, maxAttempts, result.ExitCode)
				return fmt.Errorf("test %s failed after %d attempts: %s", testFile, maxAttempts, result.Output)
			}
		}
	}

	log.Printf("✅ All test files validated successfully")
	return nil
}

// isTestFile проверяет, является ли файл тестом
func (s *VibeCodingSession) isTestFile(filename string) bool {
	// Определяем по расширению и паттернам имен
	lowerName := strings.ToLower(filename)

	// Общие паттерны тестовых файлов
	testPatterns := []string{
		"test_", "_test.", "test.", ".test.",
		"spec_", "_spec.", ".spec.",
		"__test__", "__tests__",
	}

	for _, pattern := range testPatterns {
		if strings.Contains(lowerName, pattern) {
			return true
		}
	}

	// Языко-специфичные паттерны
	if s.Analysis != nil {
		switch strings.ToLower(s.Analysis.Language) {
		case "go":
			return strings.HasSuffix(lowerName, "_test.go")
		case "python":
			return strings.HasPrefix(lowerName, "test_") || strings.HasSuffix(lowerName, "_test.py")
		case "javascript", "typescript", "node.js":
			return strings.Contains(lowerName, ".test.") || strings.Contains(lowerName, ".spec.") || strings.Contains(lowerName, "__tests__")
		case "java":
			return strings.HasSuffix(lowerName, "test.java") || strings.HasSuffix(lowerName, "tests.java")
		}
	}

	return false
}

// buildTestCommand создает команду для запуска конкретного тестового файла
func (s *VibeCodingSession) buildTestCommand(testFile string) string {
	if s.TestCommand == "" {
		return fmt.Sprintf("echo 'No test command configured for %s'", testFile)
	}

	// Если команда содержит плейсхолдер, заменяем его
	if strings.Contains(s.TestCommand, "%s") || strings.Contains(s.TestCommand, "{file}") {
		command := strings.ReplaceAll(s.TestCommand, "{file}", testFile)
		return fmt.Sprintf(command, testFile)
	}

	// Для некоторых языков добавляем файл к команде
	if s.Analysis != nil {
		switch strings.ToLower(s.Analysis.Language) {
		case "go":
			return fmt.Sprintf("go test -v %s", testFile)
		case "python":
			return fmt.Sprintf("python -m pytest %s -v", testFile)
		case "javascript", "node.js":
			return fmt.Sprintf("npm test -- %s", testFile)
		}
	}

	// По умолчанию просто запускаем общую команду тестирования
	return s.TestCommand
}

// getMCPToolsInfo получает информацию о доступности MCP и список тулов
func (s *VibeCodingSession) getMCPToolsInfo() (available bool, tools []string) {
	// Попробуем получить реальный список тулов через менеджер сессий
	if sessionManager, exists := globalSessionManager.Load().(*SessionManager); exists && sessionManager != nil {
		if mcpClient := getGlobalMCPClient(); mcpClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if toolList, err := mcpClient.GetAvailableTools(ctx); err == nil && len(toolList) > 0 {
				log.Printf("✅ MCP server available with %d tools: %v", len(toolList), toolList)
				return true, toolList
			} else {
				log.Printf("⚠️ MCP server unavailable: %v", err)
			}
		} else {
			log.Printf("⚠️ MCP client not initialized")
		}
	} else {
		log.Printf("⚠️ Session manager not available")
	}

	log.Printf("🔧 MCP server is not available - context will not include MCP instructions")
	return false, nil
}

// getMCPToolsList получает список доступных MCP тулов для обратной совместимости
func (s *VibeCodingSession) getMCPToolsList() []string {
	available, tools := s.getMCPToolsInfo()
	if available {
		return tools
	}

	// Возвращаем пустой список если MCP недоступен
	return []string{}
}

// generateUsageInstructions создает инструкции по использованию для JSON контекста
func (s *VibeCodingSession) generateUsageInstructions() map[string]interface{} {
	mcpAvailable, mcpTools := s.getMCPToolsInfo()

	if mcpAvailable {
		return map[string]interface{}{
			"description":   "LLM-generated compressed project context with token budgeting and MCP tool access",
			"mcp_available": true,
			"mcp_tools":     mcpTools,
			"notes": []string{
				"Context is generated by LLM and provides high-level descriptions",
				"Use MCP tools to access actual file contents for implementation",
				"Context is token-limited and may not include all files due to budget constraints",
				"For large files, LLM can request content through MCP tools on-demand",
			},
		}
	} else {
		return map[string]interface{}{
			"description":   "LLM-generated compressed project context with token budgeting (MCP not available)",
			"mcp_available": false,
			"notes": []string{
				"Context is generated by LLM and provides high-level descriptions",
				"MCP server is not available - work only with provided context information",
				"Context is token-limited and may not include all files due to budget constraints",
				"File access is limited to information provided in this context",
			},
		}
	}
}

// generateUsageInstructionsMarkdown создает инструкции по использованию для Markdown контекста
func (s *VibeCodingSession) generateUsageInstructionsMarkdown() string {
	mcpAvailable, mcpTools := s.getMCPToolsInfo()

	var md strings.Builder

	if mcpAvailable {
		md.WriteString("This is an LLM-generated compressed project context with token budgeting and MCP tool access.\n\n")
		md.WriteString("**Available MCP Tools:**\n")
		for i, tool := range mcpTools {
			md.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, tool))
		}
		md.WriteString("\n**Important Notes:**\n")
		md.WriteString("- This context is generated by an LLM and provides high-level descriptions\n")
		md.WriteString("- Use MCP tools to access actual file contents for implementation\n")
		md.WriteString("- Context is token-limited and may not include all files due to budget constraints\n")
		md.WriteString("- For large files, LLM can request content through MCP tools on-demand\n")
	} else {
		md.WriteString("This is an LLM-generated compressed project context with token budgeting.\n\n")
		md.WriteString("**⚠️ MCP Server Not Available**\n")
		md.WriteString("MCP tools are not accessible in this session. Work only with the provided context information.\n\n")
		md.WriteString("**Important Notes:**\n")
		md.WriteString("- This context is generated by an LLM and provides high-level descriptions\n")
		md.WriteString("- MCP server is not available - work only with provided context information\n")
		md.WriteString("- Context is token-limited and may not include all files due to budget constraints\n")
		md.WriteString("- File access is limited to information provided in this context\n")
	}

	return md.String()
}

// requestTestFix запрашивает исправление провалившегося теста у LLM
func (s *VibeCodingSession) requestTestFix(ctx context.Context, testFile string, errorOutput string) error {
	log.Printf("🔧 Requesting LLM to fix failing test: %s", testFile)

	// Получаем содержимое теста
	testContent, exists := s.GeneratedFiles[testFile]
	if !exists {
		// Проверяем в обычных файлах
		testContent, exists = s.Files[testFile]
		if !exists {
			return fmt.Errorf("test file %s not found", testFile)
		}
	}

	// Формируем запрос к LLM
	prompt := fmt.Sprintf(`Тест не прошел проверку. Нужно исправить ошибки.

**Файл теста:** %s

**Содержимое теста:**
%s

**Ошибки при выполнении:**
%s

**Задача:** Исправить тест так, чтобы он корректно работал. Верни только исправленный код теста без дополнительных объяснений.

**Требования:**
1. Сохрани исходную логику тестирования
2. Исправь синтаксические ошибки
3. Исправь проблемы с импортами/зависимостями  
4. Убедись что тест покрывает нужную функциональность
5. Возвращай только код без markdown форматирования`, testFile, testContent, errorOutput)

	// Отправляем запрос к LLM
	messages := []llm.Message{
		{Role: "system", Content: "Ты - опытный программист, специализирующийся на исправлении тестов. Отвечай только исправленным кодом."},
		{Role: "user", Content: prompt},
	}

	response, err := s.LLMClient.Generate(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to get LLM response for test fix: %w", err)
	}

	// Извлекаем исправленный код
	fixedCode := strings.TrimSpace(response.Content)

	// Убираем markdown форматирование если есть
	if strings.HasPrefix(fixedCode, "```") {
		lines := strings.Split(fixedCode, "\n")
		if len(lines) > 2 {
			// Убираем первую и последнюю строки с ```
			fixedCode = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	// Сохраняем исправленный тест
	s.GeneratedFiles[testFile] = fixedCode
	log.Printf("✅ Test %s has been fixed by LLM", testFile)

	return nil
}
