package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai-chatter/internal/codevalidation"
	"ai-chatter/internal/llm"
)

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

// GetSession получает активную сессию пользователя
func (sm *SessionManager) GetSession(userID int64) (*VibeCodingSession, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[userID]
	return session, exists
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

// SetupEnvironment настраивает окружение для проекта (до 3 попыток)
func (s *VibeCodingSession) SetupEnvironment(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Printf("🔥 Setting up environment for vibecoding session: %s", s.ProjectName)

	maxAttempts := 3
	var lastError error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🔥 Environment setup attempt %d/%d", attempt, maxAttempts)

		// 1. Анализируем проект
		if err := s.analyzeProject(ctx); err != nil {
			lastError = fmt.Errorf("project analysis failed: %w", err)
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

		log.Printf("✅ Environment setup successful on attempt %d", attempt)
		return nil
	}

	return fmt.Errorf("environment setup failed after %d attempts: %w", maxAttempts, lastError)
}

// analyzeProject анализирует проект используя LLM (unified approach from validator.go)
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

// GetSessionInfo возвращает информацию о сессии
func (s *VibeCodingSession) GetSessionInfo() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return map[string]interface{}{
		"project_name":    s.ProjectName,
		"language":        s.Analysis.Language,
		"start_time":      s.StartTime,
		"files_count":     len(s.Files),
		"generated_count": len(s.GeneratedFiles),
		"test_command":    s.TestCommand,
		"container_id":    s.ContainerID,
	}
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
