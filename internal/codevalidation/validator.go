package codevalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-chatter/internal/llm"
)

// CodeValidationWorkflow координирует валидацию кода
type CodeValidationWorkflow struct {
	llmClient    llm.Client
	dockerClient DockerManager
}

// NewCodeValidationWorkflow создает новый workflow валидации кода
func NewCodeValidationWorkflow(llmClient llm.Client, dockerClient DockerManager) *CodeValidationWorkflow {
	return &CodeValidationWorkflow{
		llmClient:    llmClient,
		dockerClient: dockerClient,
	}
}

// ProgressCallback интерфейс для уведомлений о прогрессе
type ProgressCallback interface {
	UpdateProgress(step string, status string) // step - название шага, status - статус (in_progress, completed, error)
}

// CodeAnalysisResult результат анализа кода
type CodeAnalysisResult struct {
	Language        string   `json:"language"`
	Framework       string   `json:"framework,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	InstallCommands []string `json:"install_commands"`
	Commands        []string `json:"commands"`
	DockerImage     string   `json:"docker_image"`
	ProjectType     string   `json:"project_type,omitempty"`
	WorkingDir      string   `json:"working_dir,omitempty"` // Относительный путь к рабочей директории внутри /workspace
	Reasoning       string   `json:"reasoning"`
}

// ValidationResult результат валидации кода
type ValidationResult struct {
	Success        bool     `json:"success"`
	Output         string   `json:"output"`
	Errors         []string `json:"errors,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	ExitCode       int      `json:"exit_code"`
	Duration       string   `json:"duration"`
	Suggestions    []string `json:"suggestions,omitempty"`
	UserQuestion   string   `json:"user_question,omitempty"`   // Вопрос пользователя
	QuestionAnswer string   `json:"question_answer,omitempty"` // Ответ на вопрос пользователя
	ErrorAnalysis  string   `json:"error_analysis,omitempty"`  // Анализ ошибок (код vs сборка)
	RetryAttempt   int      `json:"retry_attempt,omitempty"`   // Номер попытки (для retry логики)
	BuildProblems  []string `json:"build_problems,omitempty"`  // Проблемы со сборкой
	CodeProblems   []string `json:"code_problems,omitempty"`   // Проблемы в коде
}

// ProcessCodeValidation обрабатывает валидацию кода с progress tracking
func (w *CodeValidationWorkflow) ProcessCodeValidation(ctx context.Context, codeContent, fileName string, progressCallback ProgressCallback) (*ValidationResult, error) {
	return w.ProcessProjectValidation(ctx, map[string]string{fileName: codeContent}, progressCallback)
}

// ProcessProjectValidation обрабатывает валидацию проекта из множественных файлов
func (w *CodeValidationWorkflow) ProcessProjectValidation(ctx context.Context, files map[string]string, progressCallback ProgressCallback) (*ValidationResult, error) {
	return w.ProcessProjectValidationWithQuestion(ctx, files, "", progressCallback)
}

// ProcessProjectValidationWithQuestion обрабатывает валидацию проекта с пользовательским вопросом
func (w *CodeValidationWorkflow) ProcessProjectValidationWithQuestion(ctx context.Context, files map[string]string, userQuestion string, progressCallback ProgressCallback) (*ValidationResult, error) {
	log.Printf("🔍 Starting project validation workflow with %d files", len(files))
	if userQuestion != "" {
		log.Printf("❓ User question: %s", userQuestion)
	}

	const maxRetries = 3
	var lastResult *ValidationResult

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("🔄 Validation attempt %d/%d", attempt, maxRetries)

		// Шаг 1: Анализ проекта и определение технологий
		if progressCallback != nil {
			progressCallback.UpdateProgress("code_analysis", "in_progress")
		}

		var analysis *CodeAnalysisResult
		var err error

		if attempt == 1 {
			// Первая попытка - стандартный анализ
			analysis, err = w.analyzeProject(ctx, files)
		} else {
			// Повторные попытки - анализ с учетом предыдущих ошибок
			analysis, err = w.analyzeProjectWithRetry(ctx, files, lastResult, attempt)
		}

		if err != nil {
			if progressCallback != nil {
				progressCallback.UpdateProgress("code_analysis", "error")
			}
			return nil, fmt.Errorf("failed to analyze project: %w", err)
		}

		// Сохраняем анализ для следующих попыток
		if progressCallback != nil {
			progressCallback.UpdateProgress("code_analysis", "completed")
		}

		// Выполняем валидацию
		result, err := w.executeValidationWithRetry(ctx, files, analysis, progressCallback, attempt)
		if err != nil {
			return nil, err
		}

		lastResult = result
		result.RetryAttempt = attempt
		result.UserQuestion = userQuestion

		// Анализируем результат
		if result.Success {
			// Если есть вопрос пользователя - отвечаем на него
			if userQuestion != "" {
				answer, err := w.answerUserQuestion(ctx, files, userQuestion, result)
				if err != nil {
					log.Printf("⚠️ Failed to answer user question: %v", err)
				} else {
					result.QuestionAnswer = answer
				}
			}
			log.Printf("✅ Code validation completed successfully on attempt %d", attempt)
			return result, nil
		}

		// Анализируем ошибки
		errorAnalysis, buildProblems, codeProblems := w.analyzeErrors(ctx, result, analysis)
		result.ErrorAnalysis = errorAnalysis
		result.BuildProblems = buildProblems
		result.CodeProblems = codeProblems

		log.Printf("📊 Error analysis: %s", errorAnalysis)
		log.Printf("🔧 Build problems: %v", buildProblems)
		log.Printf("💻 Code problems: %v", codeProblems)

		// Если это проблемы с кодом, не пытаемся повторно
		if len(codeProblems) > len(buildProblems) {
			log.Printf("❌ Code has logical errors, not retrying")
			// Отвечаем на вопрос пользователя даже если код не работает
			if userQuestion != "" {
				answer, err := w.answerUserQuestion(ctx, files, userQuestion, result)
				if err != nil {
					log.Printf("⚠️ Failed to answer user question: %v", err)
				} else {
					result.QuestionAnswer = answer
				}
			}
			return result, nil
		}

		// Если это последняя попытка, возвращаем результат
		if attempt == maxRetries {
			log.Printf("❌ Max retries reached, returning final result")
			// Отвечаем на вопрос пользователя
			if userQuestion != "" {
				answer, err := w.answerUserQuestion(ctx, files, userQuestion, result)
				if err != nil {
					log.Printf("⚠️ Failed to answer user question: %v", err)
				} else {
					result.QuestionAnswer = answer
				}
			}
			return result, nil
		}

		log.Printf("🔄 Build system issues detected, trying different approach on attempt %d", attempt+1)
	}

	return lastResult, nil
}

// createContainerWithRetry создает контейнер с повторными попытками
func (w *CodeValidationWorkflow) createContainerWithRetry(ctx context.Context, analysis *CodeAnalysisResult) (string, error) {
	const maxRetries = 3
	var lastErr error

	for retryAttempt := 1; retryAttempt <= maxRetries; retryAttempt++ {
		log.Printf("🐳 Creating container attempt %d/%d", retryAttempt, maxRetries)

		containerID, err := w.dockerClient.CreateContainer(ctx, analysis)
		if err == nil {
			log.Printf("✅ Container created successfully on attempt %d", retryAttempt)
			return containerID, nil
		}

		lastErr = err
		log.Printf("❌ Container creation attempt %d failed: %v", retryAttempt, err)

		if retryAttempt < maxRetries {
			log.Printf("🔄 Waiting before retry...")
			time.Sleep(time.Duration(retryAttempt) * time.Second) // Exponential backoff
		}
	}

	return "", fmt.Errorf("failed to create container after %d attempts: %w", maxRetries, lastErr)
}

// installDependenciesWithRetry устанавливает зависимости с повторными попытками
func (w *CodeValidationWorkflow) installDependenciesWithRetry(ctx context.Context, containerID string, analysis *CodeAnalysisResult) error {
	if len(analysis.InstallCommands) == 0 {
		return nil
	}

	const maxRetries = 3
	var lastErr error

	for retryAttempt := 1; retryAttempt <= maxRetries; retryAttempt++ {
		log.Printf("📦 Installing dependencies attempt %d/%d", retryAttempt, maxRetries)

		err := w.dockerClient.InstallDependencies(ctx, containerID, analysis)
		if err == nil {
			log.Printf("✅ Dependencies installed successfully on attempt %d", retryAttempt)
			return nil
		}

		lastErr = err
		log.Printf("❌ Dependencies installation attempt %d failed: %v", retryAttempt, err)

		if retryAttempt < maxRetries {
			log.Printf("🔄 Waiting before retry...")
			time.Sleep(time.Duration(retryAttempt) * time.Second) // Exponential backoff
		}
	}

	return fmt.Errorf("failed to install dependencies after %d attempts: %w", maxRetries, lastErr)
}

// executeValidationWithRetry выполняет валидацию с повторными попытками
func (w *CodeValidationWorkflow) executeValidationWithRetry(ctx context.Context, files map[string]string, analysis *CodeAnalysisResult, progressCallback ProgressCallback, attempt int) (*ValidationResult, error) {
	// Шаг 2: Подготовка Docker окружения с retry
	if progressCallback != nil {
		progressCallback.UpdateProgress("docker_setup", "in_progress")
	}

	containerID, err := w.createContainerWithRetry(ctx, analysis)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("docker_setup", "error")
		}
		return nil, fmt.Errorf("failed to create Docker container: %w", err)
	}

	// Обязательно удаляем контейнер в конце
	defer func() {
		if cleanupErr := w.dockerClient.RemoveContainer(ctx, containerID); cleanupErr != nil {
			log.Printf("⚠️ Failed to cleanup container %s: %v", containerID, cleanupErr)
		}
	}()

	if progressCallback != nil {
		progressCallback.UpdateProgress("docker_setup", "completed")
	}

	// Шаг 3: Копирование файлов проекта в контейнер (ПЕРЕД установкой зависимостей!)
	if progressCallback != nil {
		progressCallback.UpdateProgress("copy_code", "in_progress")
	}

	err = w.dockerClient.CopyFilesToContainer(ctx, containerID, files)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("copy_code", "error")
		}
		return nil, fmt.Errorf("failed to copy files to container: %w", err)
	}

	if progressCallback != nil {
		progressCallback.UpdateProgress("copy_code", "completed")
	}

	// Шаг 4: Установка зависимостей с retry (ПОСЛЕ копирования файлов!)
	if len(analysis.InstallCommands) > 0 {
		if progressCallback != nil {
			progressCallback.UpdateProgress("install_deps", "in_progress")
		}

		err = w.installDependenciesWithRetry(ctx, containerID, analysis)
		if err != nil {
			if progressCallback != nil {
				progressCallback.UpdateProgress("install_deps", "error")
			}
			return nil, fmt.Errorf("failed to install dependencies: %w", err)
		}

		if progressCallback != nil {
			progressCallback.UpdateProgress("install_deps", "completed")
		}
	}

	// Шаг 5: Выполнение валидации
	if progressCallback != nil {
		progressCallback.UpdateProgress("run_validation", "in_progress")
	}

	startTime := time.Now()
	result, err := w.dockerClient.ExecuteValidation(ctx, containerID, analysis)
	duration := time.Since(startTime)

	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("run_validation", "error")
		}
		return nil, fmt.Errorf("failed to execute validation: %w", err)
	}

	result.Duration = duration.String()

	if progressCallback != nil {
		if result.Success {
			progressCallback.UpdateProgress("run_validation", "completed")
		} else {
			progressCallback.UpdateProgress("run_validation", "error")
		}
	}

	return result, nil
}

// analyzeCode анализирует код и определяет технологии (legacy метод для одного файла)
func (w *CodeValidationWorkflow) analyzeCode(ctx context.Context, codeContent, fileName string) (*CodeAnalysisResult, error) {
	return w.analyzeProject(ctx, map[string]string{fileName: codeContent})
}

// analyzeProject анализирует проект и определяет технологии
func (w *CodeValidationWorkflow) analyzeProject(ctx context.Context, files map[string]string) (*CodeAnalysisResult, error) {
	log.Printf("📊 Analyzing project with %d files for language and framework detection", len(files))

	systemPrompt := `You are a code analysis agent. Analyze the provided project files and determine the SIMPLEST way to validate the code.

CRITICAL EXECUTION CONTEXT:
- All files will be copied to /workspace directory in the Docker container
- You need to determine the correct working_dir within /workspace where the project should run
- If files are in a subdirectory (e.g. extracted from archive), specify working_dir (e.g. "project-name")
- All commands (install_commands and validation commands) will be executed in /workspace/working_dir
- Use relative paths or assume files are in the current working directory
- DO NOT use absolute paths like /workspace/file.py - use just file.py

WORKING DIRECTORY ANALYSIS:
- Look at file paths to determine project structure
- ONLY set working_dir if ALL files are in the SAME subdirectory
- If files have different directory paths, keep working_dir empty and use relative paths
- Examples:
  * Files: "project/src/main.py", "project/build.gradle" → working_dir: "project"
  * Files: "src/main.py", "build.gradle" → working_dir: "" (files are at different levels)
  * Files: "main.py", "requirements.txt" → working_dir: "" (files are at root level)
- BE CONSERVATIVE: when in doubt, use working_dir: ""

CRITICAL PRINCIPLE: Choose the SIMPLEST build/validation approach possible:
- Single Kotlin file → Use kotlinc (NOT Gradle)
- Single Java file → Use javac (NOT Maven/Gradle) 
- Single Python script → Direct python execution (NOT setuptools)
- Single C++ file → Use g++ directly (NOT CMake)
- No package manager files → Use language compiler directly
- Only use build systems if they are REQUIRED (config files present)

1. Programming language
2. Framework/library used (if any)  
3. Project type (web app, library, CLI tool, etc.)
4. Required dependencies
5. Commands for dependency installation (SIMPLEST approach)
6. Commands for validation (SIMPLEST approach)
7. Appropriate Docker base image

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "language": "programming language name",
  "framework": "framework or library name (optional)",
  "project_type": "type of project (web app, library, CLI, etc.)",
  "dependencies": ["dependency1", "dependency2"],
  "install_commands": ["install command1", "install command2"],
  "commands": ["validation command1", "validation command2"],
  "docker_image": "appropriate docker base image",
  "working_dir": "relative path within /workspace (empty for root, e.g. 'project-name' for subdirectory)",
  "reasoning": "explanation of choices made and why this is the simplest approach"
}

SIMPLE BUILD EXAMPLES:

Single Kotlin file (NO Gradle needed):
{
  "language": "Kotlin",
  "project_type": "script",
  "dependencies": [],
  "install_commands": [],
  "commands": ["kotlinc hello.kt -include-runtime -d hello.jar", "java -jar hello.jar"],
  "docker_image": "openjdk:11-slim",
  "working_dir": "",
  "reasoning": "Single Kotlin file - using kotlinc directly instead of Gradle for simplicity"
}

Single Java file (NO Maven needed):
{
  "language": "Java",
  "project_type": "script",
  "dependencies": [],
  "install_commands": [],
  "commands": ["javac *.java", "java Main"],
  "docker_image": "openjdk:11-slim",
  "working_dir": "",
  "reasoning": "Single Java file - using javac directly instead of build system"
}

Python script (NO pip install needed):
{
  "language": "Python",
  "project_type": "script",
  "dependencies": [],
  "install_commands": [],
  "commands": ["python -m py_compile *.py", "python main.py"],
  "docker_image": "python:3.11-slim",
  "working_dir": "",
  "reasoning": "Simple Python script with no external dependencies"
}

C++ single file:
{
  "language": "C++",
  "project_type": "script",
  "dependencies": [],
  "install_commands": ["apt-get update && apt-get install -y g++"],
  "commands": ["g++ -o program *.cpp", "./program"],
  "docker_image": "debian:bullseye-slim",
  "working_dir": "",
  "reasoning": "Single C++ file - direct compilation with g++"
}

ONLY use complex build systems when they are ACTUALLY needed:
- Use npm/yarn only if package.json exists
- Use Maven only if pom.xml exists  
- Use Gradle only if build.gradle exists
- Use pip requirements only if requirements.txt exists
- Use Cargo only if Cargo.toml exists

ADVANCED BUILD SYSTEMS (use only when config files present):

Python with requirements.txt:
{
  "language": "Python",
  "framework": "Flask",
  "project_type": "web application", 
  "dependencies": [],
  "install_commands": ["pip install -r requirements.txt"],
  "commands": ["python -m flake8 *.py", "python -m pytest", "python app.py"],
  "docker_image": "python:3.11-slim",
  "working_dir": "",
  "reasoning": "Flask web app with requirements.txt - dependencies required"
}

Node.js with package.json:
{
  "language": "JavaScript",
  "framework": "Express",
  "project_type": "web application",
  "dependencies": [],
  "install_commands": ["npm install"],
  "commands": ["npm run lint", "npm test", "npm start"],
  "docker_image": "node:18-alpine",
  "working_dir": "",
  "reasoning": "Express.js app with package.json - npm needed for dependencies"
}`

	// Формируем описание проекта
	var projectDescription strings.Builder
	projectDescription.WriteString(fmt.Sprintf("Project with %d files:\n\n", len(files)))

	// Анализ структуры файлов для определения рабочей директории
	projectDescription.WriteString("FILE STRUCTURE ANALYSIS:\n")
	for filename := range files {
		projectDescription.WriteString(fmt.Sprintf("- %s\n", filename))
	}
	projectDescription.WriteString("\nBased on file paths above, determine the correct working_dir.\n")
	projectDescription.WriteString("Remember: working_dir should be the common parent directory of all files, or empty if files are at different levels.\n\n")

	for filename, content := range files {
		projectDescription.WriteString(fmt.Sprintf("=== File: %s ===\n", filename))
		// Ограничиваем размер контента для большых файлов
		if len(content) > 2000 {
			projectDescription.WriteString(content[:2000])
			projectDescription.WriteString("\n... [truncated]\n\n")
		} else {
			projectDescription.WriteString(content)
			projectDescription.WriteString("\n\n")
		}
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: projectDescription.String()},
	}

	response, err := w.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project: %w", err)
	}

	var analysis CodeAnalysisResult
	if err := parseJSONResponse(response.Content, &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse project analysis response: %w", err)
	}

	log.Printf("🔍 Detected language: %s", analysis.Language)
	if analysis.Framework != "" {
		log.Printf("📚 Framework: %s", analysis.Framework)
	}
	if analysis.ProjectType != "" {
		log.Printf("🏗️ Project type: %s", analysis.ProjectType)
	}
	log.Printf("🐳 Docker image: %s", analysis.DockerImage)
	log.Printf("📦 Dependencies: %v", analysis.Dependencies)
	log.Printf("⚡ Install commands: %v", analysis.InstallCommands)
	log.Printf("⚡ Validation commands: %v", analysis.Commands)

	return &analysis, nil
}

// DetectCodeInMessage определяет наличие кода в сообщении и извлекает пользовательские вопросы
func DetectCodeInMessage(ctx context.Context, llmClient llm.Client, messageContent string) (bool, string, string, string, error) {
	log.Printf("🔍 Detecting code and user questions in message")

	systemPrompt := `You are a code detection agent. Analyze the message to determine if it contains code that should be validated AND extract any user questions.

Look for:
- Code blocks (` + "```" + `language code` + "```" + `)
- Inline code snippets
- File contents
- Programming-related content
- Mentions of debugging, testing, errors
- User questions about the code (why, how, what does this do, etc.)

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "has_code": true/false,
  "extracted_code": "the actual code found (empty if no code)",
  "filename": "suggested filename with extension (empty if no code)",
  "user_question": "user's question about the code if any (empty if no question)",
  "reasoning": "explanation of decision"
}

IMPORTANT:
- Only return has_code: true if there's actual executable code
- Don't trigger on configuration files unless they contain logic
- Extract the cleanest version of the code (remove markdown formatting)
- Suggest appropriate filename based on language and content
- Extract user questions like "Почему это не работает?", "Как исправить эту ошибку?", "What does this code do?"
- Questions should be in the same language as the user's message

Examples:
"Вот мой код на Python: [code]. Почему он не работает?" → user_question: "Почему он не работает?"
"Here's my Java code: [code]. How can I optimize it?" → user_question: "How can I optimize it?"
"[code] without any questions" → user_question: ""
"Can you explain this algorithm: [code]" → user_question: "Can you explain this algorithm"`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Analyze this message for code and user questions:\n\n%s", messageContent)},
	}

	response, err := llmClient.Generate(ctx, messages)
	if err != nil {
		return false, "", "", "", fmt.Errorf("failed to detect code: %w", err)
	}

	var detection struct {
		HasCode       bool   `json:"has_code"`
		ExtractedCode string `json:"extracted_code"`
		Filename      string `json:"filename"`
		UserQuestion  string `json:"user_question"`
		Reasoning     string `json:"reasoning"`
	}

	if err := parseJSONResponse(response.Content, &detection); err != nil {
		return false, "", "", "", fmt.Errorf("failed to parse code detection response: %w", err)
	}

	if detection.HasCode {
		log.Printf("✅ Code detected: %s (%s)", detection.Filename, strings.Split(detection.Reasoning, ".")[0])
		if detection.UserQuestion != "" {
			log.Printf("❓ User question detected: %s", detection.UserQuestion)
		}
	} else {
		log.Printf("❌ No code detected: %s", detection.Reasoning)
	}

	return detection.HasCode, detection.ExtractedCode, detection.Filename, detection.UserQuestion, nil
}

// analyzeProjectWithRetry повторный анализ проекта с учетом предыдущих ошибок
func (w *CodeValidationWorkflow) analyzeProjectWithRetry(ctx context.Context, files map[string]string, lastResult *ValidationResult, attempt int) (*CodeAnalysisResult, error) {
	log.Printf("🔄 Analyzing project with retry logic (attempt %d)", attempt)

	// Извлекаем информацию о предыдущих ошибках
	var previousErrors []string
	if lastResult != nil {
		previousErrors = append(previousErrors, lastResult.Errors...)
		previousErrors = append(previousErrors, lastResult.BuildProblems...)
	}

	systemPrompt := `You are a code analysis agent with retry capability. Based on the previous validation errors, choose a DIFFERENT and SIMPLER approach.

CRITICAL EXECUTION CONTEXT:
- All files will be copied to /workspace directory in the Docker container
- You need to determine the correct working_dir within /workspace where the project should run
- If files are in a subdirectory (e.g. extracted from archive), specify working_dir (e.g. "project-name")
- All commands (install_commands and validation commands) will be executed in /workspace/working_dir
- Use relative paths or assume files are in the current working directory
- DO NOT use absolute paths like /workspace/file.py - use just file.py

WORKING DIRECTORY ANALYSIS:
- Look at file paths to determine project structure
- ONLY set working_dir if ALL files are in the SAME subdirectory
- If files have different directory paths, keep working_dir empty and use relative paths
- Examples:
  * Files: "project/src/main.py", "project/build.gradle" → working_dir: "project"
  * Files: "src/main.py", "build.gradle" → working_dir: "" (files are at different levels)
  * Files: "main.py", "requirements.txt" → working_dir: "" (files are at root level)
- BE CONSERVATIVE: when in doubt, use working_dir: ""

RETRY STRATEGY:
1. If Gradle failed → try kotlinc directly
2. If Maven failed → try javac directly  
3. If npm failed → try node directly
4. If complex build failed → use simplest compiler
5. If dependencies failed → try without dependencies
6. If linting failed → try compilation only

CRITICAL PRINCIPLE: Choose the SIMPLEST build/validation approach possible:
- Single file → Direct compiler (kotlinc, javac, python, node)
- Avoid complex build systems on retry
- Focus on basic compilation/execution over testing
- Skip optional dependencies if they cause problems

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "language": "programming language name",
  "framework": "framework or library name (optional)",
  "project_type": "type of project (web app, library, CLI, etc.)",
  "dependencies": ["dependency1", "dependency2"],
  "install_commands": ["install command1", "install command2"],
  "commands": ["validation command1", "validation command2"],
  "docker_image": "appropriate docker base image",
  "working_dir": "relative path within /workspace (empty for root, e.g. 'project-name' for subdirectory)",
  "reasoning": "explanation of why this simpler approach was chosen based on previous errors"
}`

	// Формируем описание проекта с информацией о предыдущих ошибках
	var projectDescription strings.Builder
	projectDescription.WriteString(fmt.Sprintf("Project with %d files (RETRY ATTEMPT %d):\n\n", len(files), attempt))

	if len(previousErrors) > 0 {
		projectDescription.WriteString("PREVIOUS VALIDATION ERRORS TO AVOID:\n")
		for _, err := range previousErrors {
			projectDescription.WriteString(fmt.Sprintf("- %s\n", err))
		}
		projectDescription.WriteString("\nCHOOSE A SIMPLER APPROACH TO AVOID THESE ISSUES.\n\n")
	}

	// Анализ структуры файлов для определения рабочей директории
	projectDescription.WriteString("FILE STRUCTURE ANALYSIS:\n")
	for filename := range files {
		projectDescription.WriteString(fmt.Sprintf("- %s\n", filename))
	}
	projectDescription.WriteString("\nBased on file paths above, determine the correct working_dir.\n")
	projectDescription.WriteString("Remember: working_dir should be the common parent directory of all files, or empty if files are at different levels.\n")
	projectDescription.WriteString("For retry attempts, prefer working_dir: \"\" for maximum simplicity.\n\n")

	for filename, content := range files {
		projectDescription.WriteString(fmt.Sprintf("=== File: %s ===\n", filename))
		if len(content) > 1500 {
			projectDescription.WriteString(content[:1500])
			projectDescription.WriteString("\n... [truncated]\n\n")
		} else {
			projectDescription.WriteString(content)
			projectDescription.WriteString("\n\n")
		}
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: projectDescription.String()},
	}

	response, err := w.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project with retry: %w", err)
	}

	var analysis CodeAnalysisResult
	if err := parseJSONResponse(response.Content, &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse project analysis response: %w", err)
	}

	log.Printf("🔄 Retry analysis - Language: %s, Approach: %s", analysis.Language, analysis.Reasoning)
	return &analysis, nil
}

// analyzeErrors анализирует ошибки валидации и разделяет их на проблемы сборки и проблемы кода
func (w *CodeValidationWorkflow) analyzeErrors(ctx context.Context, result *ValidationResult, analysis *CodeAnalysisResult) (string, []string, []string) {
	if result.Success || len(result.Errors) == 0 {
		return "", []string{}, []string{}
	}

	systemPrompt := `You are an error analysis agent. Analyze validation errors to determine if they are:

1. BUILD/SETUP PROBLEMS (can be fixed by changing build approach):
   - Missing dependencies
   - Wrong build commands
   - Package manager issues
   - Build system configuration problems
   - Missing tools (gradle, maven, npm, etc.)

2. CODE PROBLEMS (actual bugs/issues in the code):
   - Syntax errors
   - Runtime errors  
   - Logic errors
   - Type errors
   - Missing imports/libraries that are part of the code logic

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "analysis_summary": "brief explanation of error types found",
  "build_problems": ["list of build/setup issues"],
  "code_problems": ["list of actual code issues"]
}

IMPORTANT:
- Be precise in categorization
- Build problems can potentially be fixed by changing approach
- Code problems require actual code changes by user
- If unsure, lean towards build problems for retry logic`

	// Собираем информацию об ошибках
	errorInfo := fmt.Sprintf(`VALIDATION RESULTS:
Language: %s
Docker Image: %s
Install Commands: %v
Validation Commands: %v
Exit Code: %d

ERRORS:
%s

OUTPUT:
%s`, analysis.Language, analysis.DockerImage, analysis.InstallCommands, analysis.Commands, result.ExitCode, strings.Join(result.Errors, "\n"), result.Output)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: errorInfo},
	}

	response, err := w.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("⚠️ Failed to analyze errors: %v", err)
		// Fallback: assume all are build problems for retry
		return "Error analysis failed - assuming build issues", result.Errors, []string{}
	}

	var errorAnalysis struct {
		AnalysisSummary string   `json:"analysis_summary"`
		BuildProblems   []string `json:"build_problems"`
		CodeProblems    []string `json:"code_problems"`
	}

	if err := parseJSONResponse(response.Content, &errorAnalysis); err != nil {
		log.Printf("⚠️ Failed to parse error analysis: %v", err)
		// Fallback
		return "Error parsing failed - assuming build issues", result.Errors, []string{}
	}

	return errorAnalysis.AnalysisSummary, errorAnalysis.BuildProblems, errorAnalysis.CodeProblems
}

// answerUserQuestion отвечает на вопрос пользователя о коде
func (w *CodeValidationWorkflow) answerUserQuestion(ctx context.Context, files map[string]string, userQuestion string, result *ValidationResult) (string, error) {
	log.Printf("❓ Answering user question: %s", userQuestion)

	systemPrompt := `You are a code analysis and explanation assistant. Answer the user's question about their code/project based on the validation results and file contents.

IMPORTANT:
- Answer in the SAME LANGUAGE as the user's question (Russian for Russian questions, English for English questions)
- Be helpful, educational, and comprehensive
- Reference specific files, functions, and code structures when relevant
- If validation failed, explain potential issues and how to fix them
- If validation succeeded, explain how the code/project works
- For project description requests, provide comprehensive overview including:
  * Project purpose and main functionality
  * Programming languages and frameworks used
  * Architecture and file structure
  * Key components and their responsibilities
  * Dependencies and build system
  * Potential improvements or observations
- Use clear structure with headers and bullet points for project descriptions
- Be detailed but readable
- Use no more then 2000 symbols per answer

Focus on:
1. Direct answer to the user's question
2. Project/code analysis and explanation
3. Technical stack identification
4. Architecture overview and file structure analysis
5. Problem diagnosis if issues exist
6. Suggestions for improvement or development`

	// Формируем контекст для ответа
	var codeContext strings.Builder
	codeContext.WriteString(fmt.Sprintf("PROJECT ANALYSIS REQUEST:\nUser wants to know: %s\n\n", userQuestion))

	codeContext.WriteString(fmt.Sprintf("PROJECT STRUCTURE (%d files):\n", len(files)))

	// Сначала показываем структуру файлов
	for filename := range files {
		codeContext.WriteString(fmt.Sprintf("- %s\n", filename))
	}
	codeContext.WriteString("\n")

	codeContext.WriteString("FILE CONTENTS:\n\n")

	for filename, content := range files {
		codeContext.WriteString(fmt.Sprintf("=== %s ===\n", filename))
		if len(content) > 1500 {
			codeContext.WriteString(content[:1500])
			codeContext.WriteString("\n... [truncated for brevity]\n\n")
		} else {
			codeContext.WriteString(content)
			codeContext.WriteString("\n\n")
		}
	}

	codeContext.WriteString("VALIDATION RESULTS:\n")
	codeContext.WriteString(fmt.Sprintf("- Overall Success: %t\n", result.Success))

	if result.RetryAttempt > 1 {
		codeContext.WriteString(fmt.Sprintf("- Completed after %d attempts\n", result.RetryAttempt))
	}

	if len(result.BuildProblems) > 0 {
		codeContext.WriteString("- Build Problems:\n")
		for _, problem := range result.BuildProblems {
			codeContext.WriteString(fmt.Sprintf("  * %s\n", problem))
		}
	}

	if len(result.CodeProblems) > 0 {
		codeContext.WriteString("- Code Problems:\n")
		for _, problem := range result.CodeProblems {
			codeContext.WriteString(fmt.Sprintf("  * %s\n", problem))
		}
	}

	if len(result.Errors) > 0 && len(result.BuildProblems) == 0 && len(result.CodeProblems) == 0 {
		codeContext.WriteString("- General Errors:\n")
		for _, err := range result.Errors {
			codeContext.WriteString(fmt.Sprintf("  * %s\n", err))
		}
	}

	if len(result.Warnings) > 0 {
		codeContext.WriteString("- Warnings:\n")
		for _, warning := range result.Warnings {
			codeContext.WriteString(fmt.Sprintf("  * %s\n", warning))
		}
	}

	if result.Output != "" && len(result.Output) < 800 {
		codeContext.WriteString(fmt.Sprintf("- Execution Output:\n%s\n", result.Output))
	}

	codeContext.WriteString(fmt.Sprintf("\nPlease provide a comprehensive answer to: %s", userQuestion))

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: codeContext.String()},
	}

	response, err := w.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to answer user question: %w", err)
	}

	return response.Content, nil
}

// parseJSONResponse парсит JSON ответ от LLM с обработкой ошибок
func parseJSONResponse(content string, target interface{}) error {
	// Удаляем возможные markdown блоки
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Добавляем недостающий import
	return json.Unmarshal([]byte(content), target)
}
