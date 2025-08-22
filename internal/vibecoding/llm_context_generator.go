package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-chatter/internal/llm"
)

// ProjectContext представляет сжатый контекст проекта для LLM
type ProjectContext struct {
	ProjectName  string           `json:"project_name"`
	Language     string           `json:"language"`
	GeneratedAt  time.Time        `json:"generated_at"`
	TotalFiles   int              `json:"total_files"`
	Description  string           `json:"description"`
	Dependencies []string         `json:"dependencies,omitempty"`
	Files        []FileSignature  `json:"files"`
	Structure    ProjectStructure `json:"structure"`
}

// FileSignature содержит сигнатуру файла (функции, структуры, интерфейсы)
type FileSignature struct {
	Path       string      `json:"path"`
	Type       string      `json:"type"` // "go", "js", "py", "md", "json", etc.
	Size       int         `json:"size"`
	Functions  []Function  `json:"functions,omitempty"`
	Structs    []Struct    `json:"structs,omitempty"`
	Interfaces []Interface `json:"interfaces,omitempty"`
	Constants  []Variable  `json:"constants,omitempty"`
	Variables  []Variable  `json:"variables,omitempty"`
	Imports    []string    `json:"imports,omitempty"`
	Summary    string      `json:"summary,omitempty"`
}

// Function представляет сигнатуру функции
type Function struct {
	Name       string   `json:"name"`
	Signature  string   `json:"signature"`
	Receiver   string   `json:"receiver,omitempty"`
	Parameters []string `json:"parameters,omitempty"`
	Returns    []string `json:"returns,omitempty"`
	IsExported bool     `json:"is_exported"`
	Comment    string   `json:"comment,omitempty"`
	LineNumber int      `json:"line_number"`
}

// Struct представляет сигнатуру структуры
type Struct struct {
	Name       string  `json:"name"`
	Fields     []Field `json:"fields,omitempty"`
	IsExported bool    `json:"is_exported"`
	Comment    string  `json:"comment,omitempty"`
	LineNumber int     `json:"line_number"`
}

// Interface представляет сигнатуру интерфейса
type Interface struct {
	Name       string   `json:"name"`
	Methods    []Method `json:"methods,omitempty"`
	IsExported bool     `json:"is_exported"`
	Comment    string   `json:"comment,omitempty"`
	LineNumber int      `json:"line_number"`
}

// Field представляет поле структуры
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

// Method представляет метод интерфейса
type Method struct {
	Name       string   `json:"name"`
	Signature  string   `json:"signature"`
	Parameters []string `json:"parameters,omitempty"`
	Returns    []string `json:"returns,omitempty"`
}

// Variable представляет константу или переменную
type Variable struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value,omitempty"`
	IsExported bool   `json:"is_exported"`
	Comment    string `json:"comment,omitempty"`
}

// ProjectStructure представляет структуру проекта
type ProjectStructure struct {
	Directories []Directory `json:"directories"`
	FileTypes   []FileType  `json:"file_types"`
}

// Directory представляет директорию проекта
type Directory struct {
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
	Purpose   string `json:"purpose,omitempty"`
}

// FileType представляет информацию о типе файлов
type FileType struct {
	Extension string `json:"extension"`
	Count     int    `json:"count"`
	Language  string `json:"language"`
}

// LLMContextGenerator генерирует сжатый контекст проекта с помощью LLM
type LLMContextGenerator struct {
	llmClient      llm.Client
	maxTokens      int // Максимальный размер контекста в токенах
	tokenEstimator *TokenEstimator
}

// TokenEstimator примерно оценивает количество токенов
type TokenEstimator struct{}

// ProjectContextLLM представляет LLM-генерируемый контекст проекта
type ProjectContextLLM struct {
	ProjectName  string                    `json:"project_name"`
	Language     string                    `json:"language"`
	GeneratedAt  time.Time                 `json:"generated_at"`
	TotalFiles   int                       `json:"total_files"`
	Description  string                    `json:"description"`
	Dependencies []string                  `json:"dependencies,omitempty"`
	Files        map[string]LLMFileContext `json:"files"` // path -> context
	Structure    ProjectStructure          `json:"structure"`
	TokensUsed   int                       `json:"tokens_used"`
	TokensLimit  int                       `json:"tokens_limit"`
}

// LLMFileContext содержит LLM-генерируемое описание файла
type LLMFileContext struct {
	Path         string    `json:"path"`
	Type         string    `json:"type"`
	Size         int       `json:"size"`
	LastModified time.Time `json:"last_modified"`
	Summary      string    `json:"summary"`      // LLM-генерируемое описание
	KeyElements  []string  `json:"key_elements"` // Основные функции/структуры/классы
	Purpose      string    `json:"purpose"`      // Назначение файла
	Dependencies []string  `json:"dependencies"` // Файлы, от которых зависит
	TokensUsed   int       `json:"tokens_used"`  // Количество токенов в описании
	NeedsUpdate  bool      `json:"needs_update"` // Флаг необходимости обновления
}

// ContextGenerationRequest запрос на генерацию описания файла
type ContextGenerationRequest struct {
	ProjectName     string            `json:"project_name"`
	Language        string            `json:"language"`
	FilePath        string            `json:"file_path"`
	FileContent     string            `json:"file_content,omitempty"`
	FileList        []string          `json:"file_list"`        // Список всех файлов проекта
	ExistingContext map[string]string `json:"existing_context"` // Существующие описания других файлов
	TokenBudget     int               `json:"token_budget"`     // Лимит токенов для описания
}

// ContextGenerationResponse ответ LLM с описанием файла
type ContextGenerationResponse struct {
	Summary      string   `json:"summary"`
	KeyElements  []string `json:"key_elements"`
	Purpose      string   `json:"purpose"`
	Dependencies []string `json:"dependencies"`
	Language     string   `json:"language,omitempty"`
}

// NewLLMContextGenerator создает новый LLM-генератор контекста
func NewLLMContextGenerator(llmClient llm.Client, maxTokens int) *LLMContextGenerator {
	if maxTokens <= 0 {
		maxTokens = 5000 // По умолчанию 5000 токенов
	}

	return &LLMContextGenerator{
		llmClient:      llmClient,
		maxTokens:      maxTokens,
		tokenEstimator: &TokenEstimator{},
	}
}

// EstimateTokens примерно оценивает количество токенов в тексте
func (te *TokenEstimator) EstimateTokens(text string) int {
	// Простая эвристика: ~4 символа на токен для английского, ~6 для русского
	// Учитываем пространство, JSON структуру
	return len(text) / 4
}

// GenerateContext генерирует сжатый контекст проекта с помощью LLM
func (g *LLMContextGenerator) GenerateContext(ctx context.Context, projectName string, files map[string]string) (*ProjectContextLLM, error) {
	log.Printf("🧠 [STEP 0] Starting LLM-based project context generation for '%s' with %d files (limit: %d tokens)", projectName, len(files), g.maxTokens)
	start := time.Now()

	context := &ProjectContextLLM{
		ProjectName: projectName,
		GeneratedAt: time.Now(),
		TotalFiles:  len(files),
		Files:       make(map[string]LLMFileContext),
		TokensLimit: g.maxTokens,
		Structure: ProjectStructure{
			Directories: make([]Directory, 0),
			FileTypes:   make([]FileType, 0),
		},
	}
	log.Printf("🧠 [STEP 1] Context structure initialized (%.2fs)", time.Since(start).Seconds())

	// 1. Определяем язык и базовую структуру проекта
	step1Start := time.Now()
	context.Language = g.detectMainLanguage(files)
	log.Printf("🧠 [STEP 2] Main language detected: %s (%.2fs)", context.Language, time.Since(step1Start).Seconds())

	step2Start := time.Now()
	g.analyzeProjectStructure(files, &context.Structure)
	log.Printf("🧠 [STEP 3] Project structure analyzed: %d dirs, %d file types (%.2fs)", len(context.Structure.Directories), len(context.Structure.FileTypes), time.Since(step2Start).Seconds())

	// 2. Генерируем описание проекта с помощью LLM
	step3Start := time.Now()
	log.Printf("🧠 [STEP 4] Starting LLM project description generation...")
	if err := g.generateProjectDescription(ctx, context, files); err != nil {
		log.Printf("⚠️ [STEP 4] Failed to generate project description: %v (%.2fs)", err, time.Since(step3Start).Seconds())
		context.Description = fmt.Sprintf("%s project", projectName)
	} else {
		log.Printf("🧠 [STEP 4] Project description generated: '%s' (%.2fs)", context.Description, time.Since(step3Start).Seconds())
	}

	// 3. Извлекаем зависимости
	step4Start := time.Now()
	context.Dependencies = g.extractDependencies(files, context.Language)
	log.Printf("🧠 [STEP 5] Dependencies extracted: %d deps (%.2fs)", len(context.Dependencies), time.Since(step4Start).Seconds())

	// 4. Сортируем файлы по важности
	step5Start := time.Now()
	fileList := g.sortFilesByImportance(files)
	log.Printf("🧠 [STEP 6] Files sorted by importance: %d files (%.2fs)", len(fileList), time.Since(step5Start).Seconds())
	if len(fileList) > 0 {
		log.Printf("🧠 [STEP 6] Top 5 files: %v", fileList[:min(5, len(fileList))])
	}

	// 5. Генерируем контекст для каждого файла с учетом лимита токенов
	tokenBudget := g.maxTokens - g.tokenEstimator.EstimateTokens(context.Description) - 500 // Резерв для метаданных
	log.Printf("🧠 [STEP 7] Starting individual file context generation. Token budget: %d tokens", tokenBudget)
	processedFiles := 0

	for i, filePath := range fileList {
		if tokenBudget <= 0 {
			log.Printf("⚠️ [STEP 7] Token budget exhausted after %d files, skipping remaining %d files", processedFiles, len(fileList)-i)
			break
		}

		fileStart := time.Now()
		log.Printf("🧠 [STEP 7.%d] Processing file %d/%d: %s (budget: %d tokens)", i+1, i+1, len(fileList), filePath, tokenBudget)

		fileContent := files[filePath]
		fileBudget := tokenBudget / 4 // 1/4 бюджета на файл
		if fileBudget < 50 {
			fileBudget = 50 // Минимальный бюджет
		}

		fileContext, err := g.generateFileContext(ctx, context, filePath, fileContent, fileBudget)
		if err != nil {
			log.Printf("⚠️ [STEP 7.%d] Failed to generate context for %s: %v (%.2fs)", i+1, filePath, err, time.Since(fileStart).Seconds())
			continue
		}

		context.Files[filePath] = *fileContext
		tokenBudget -= fileContext.TokensUsed
		context.TokensUsed += fileContext.TokensUsed
		processedFiles++
		log.Printf("🧠 [STEP 7.%d] ✅ File processed: %s (%d tokens used, %d remaining, %.2fs)", i+1, filePath, fileContext.TokensUsed, tokenBudget, time.Since(fileStart).Seconds())
	}

	log.Printf("✅ [FINAL] LLM context generation completed: %d/%d files processed, %d/%d tokens used (%.2fs total)",
		len(context.Files), len(files), context.TokensUsed, context.TokensLimit, time.Since(start).Seconds())
	return context, nil
}

// generateProjectDescription генерирует описание проекта с помощью LLM
func (g *LLMContextGenerator) generateProjectDescription(ctx context.Context, context *ProjectContextLLM, files map[string]string) error {
	start := time.Now()
	log.Printf("🧠 [DESC] Starting project description generation...")

	// Формируем список файлов для анализа
	var fileList []string
	for path := range files {
		fileList = append(fileList, path)
	}
	sort.Strings(fileList)
	log.Printf("🧠 [DESC] File list prepared: %d files (%.2fs)", len(fileList), time.Since(start).Seconds())

	systemPrompt := `You are a code analysis assistant. Analyze the project structure and generate a concise description.

Your task is to:
1. Understand the project's main purpose based on file structure and names
2. Identify the primary technology/framework being used
3. Generate a concise description (max 100 characters)

Respond with JSON:
{
  "description": "Brief project description",
  "language": "primary programming language"
}`

	userPrompt := fmt.Sprintf(`Project: %s
Language: %s
Total files: %d

File structure:
%s

Please analyze this project structure and provide a concise description.`,
		context.ProjectName,
		context.Language,
		len(fileList),
		strings.Join(fileList[:min(20, len(fileList))], "\n")) // Первые 20 файлов

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	log.Printf("🧠 [DESC] Prompt prepared, sending to LLM... (%.2fs)", time.Since(start).Seconds())

	llmStart := time.Now()
	response, err := g.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ [DESC] LLM request failed after %.2fs: %v", time.Since(llmStart).Seconds(), err)
		return fmt.Errorf("LLM request failed: %w", err)
	}
	log.Printf("🧠 [DESC] LLM response received (%.2fs), parsing...", time.Since(llmStart).Seconds())

	var result struct {
		Description string `json:"description"`
		Language    string `json:"language"`
	}

	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		log.Printf("⚠️ [DESC] JSON parsing failed, using raw response: %s", response.Content[:min(100, len(response.Content))])
		// Если JSON parsing не удался, используем raw ответ
		context.Description = strings.TrimSpace(response.Content)
		if len(context.Description) > 100 {
			context.Description = context.Description[:100] + "..."
		}
		log.Printf("🧠 [DESC] Description set from raw response: '%s' (%.2fs total)", context.Description, time.Since(start).Seconds())
		return nil
	}

	context.Description = result.Description
	if result.Language != "" && result.Language != "unknown" {
		context.Language = result.Language
	}
	log.Printf("🧠 [DESC] ✅ Description parsed successfully: '%s' (%.2fs total)", context.Description, time.Since(start).Seconds())

	return nil
}

// generateFileContext генерирует контекст для отдельного файла
func (g *LLMContextGenerator) generateFileContext(ctx context.Context, projectContext *ProjectContextLLM, filePath, fileContent string, tokenBudget int) (*LLMFileContext, error) {
	start := time.Now()
	log.Printf("🧠 [FILE] Starting context generation for %s (%d chars, budget: %d tokens)", filePath, len(fileContent), tokenBudget)

	fileContext := &LLMFileContext{
		Path:         filePath,
		Type:         g.getFileType(filepath.Ext(filePath)),
		Size:         len(fileContent),
		LastModified: time.Now(),
		NeedsUpdate:  false,
	}

	// Для небольших файлов (< 200 символов) создаем простое описание
	if len(fileContent) < 200 {
		fileContext.Summary = fmt.Sprintf("Small %s file (%d chars)", fileContext.Type, len(fileContent))
		fileContext.TokensUsed = g.tokenEstimator.EstimateTokens(fileContext.Summary)
		log.Printf("🧠 [FILE] ✅ Small file processed: %s (%.2fs)", filePath, time.Since(start).Seconds())
		return fileContext, nil
	}

	// Генерируем описание с помощью LLM только для кода
	if !g.isCodeFile(fileContext.Type) {
		fileContext.Summary = g.generateGenericSummary(fileContent, fileContext.Type)
		fileContext.TokensUsed = g.tokenEstimator.EstimateTokens(fileContext.Summary)
		log.Printf("🧠 [FILE] ✅ Non-code file processed: %s (%.2fs)", filePath, time.Since(start).Seconds())
		return fileContext, nil
	}

	log.Printf("🧠 [FILE] Preparing LLM request for code file: %s (%.2fs)", filePath, time.Since(start).Seconds())

	// Подготавливаем контекст для LLM
	request := ContextGenerationRequest{
		ProjectName: projectContext.ProjectName,
		Language:    projectContext.Language,
		FilePath:    filePath,
		TokenBudget: tokenBudget,
	}

	// Для больших файлов отправляем только имя файла, LLM запросит содержимое через MCP
	if len(fileContent) > 2000 {
		request.FileContent = "" // LLM должен запросить через MCP
		log.Printf("🧠 [FILE] Large file (%d chars), content will be requested via MCP", len(fileContent))
	} else {
		request.FileContent = fileContent
		log.Printf("🧠 [FILE] Sending file content directly (%d chars)", len(fileContent))
	}

	// Добавляем список файлов проекта для контекста
	for path := range projectContext.Files {
		request.FileList = append(request.FileList, path)
	}

	systemPrompt := g.buildFileAnalysisSystemPrompt()
	userPrompt := g.buildFileAnalysisUserPrompt(request)
	log.Printf("🧠 [FILE] Prompts prepared, sending to LLM... (%.2fs)", time.Since(start).Seconds())

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	llmStart := time.Now()
	response, err := g.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ [FILE] LLM analysis failed for %s after %.2fs: %v", filePath, time.Since(llmStart).Seconds(), err)
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}
	log.Printf("🧠 [FILE] LLM response received for %s (%.2fs), parsing...", filePath, time.Since(llmStart).Seconds())

	// Парсим ответ LLM
	var llmResponse ContextGenerationResponse
	if err := json.Unmarshal([]byte(response.Content), &llmResponse); err != nil {
		log.Printf("⚠️ [FILE] JSON parsing failed for %s, using raw response: %s", filePath, response.Content[:min(100, len(response.Content))])
		// Fallback: используем raw ответ как summary
		fileContext.Summary = strings.TrimSpace(response.Content)
		if len(fileContext.Summary) > 300 {
			fileContext.Summary = fileContext.Summary[:300] + "..."
		}
	} else {
		log.Printf("🧠 [FILE] JSON parsed successfully for %s: %d key elements, %d deps", filePath, len(llmResponse.KeyElements), len(llmResponse.Dependencies))
		fileContext.Summary = llmResponse.Summary
		fileContext.KeyElements = llmResponse.KeyElements
		fileContext.Purpose = llmResponse.Purpose
		fileContext.Dependencies = llmResponse.Dependencies
	}

	// Оцениваем использованные токены
	fileContext.TokensUsed = g.tokenEstimator.EstimateTokens(fileContext.Summary + strings.Join(fileContext.KeyElements, " ") + fileContext.Purpose)
	log.Printf("🧠 [FILE] ✅ Context generated for %s: %d tokens used (%.2fs total)", filePath, fileContext.TokensUsed, time.Since(start).Seconds())

	return fileContext, nil
}

// buildFileAnalysisSystemPrompt создает системный промпт для анализа файла
func (g *LLMContextGenerator) buildFileAnalysisSystemPrompt() string {
	return `You are a code analysis assistant. Your task is to analyze source code files and generate concise, structured descriptions.

For each file, you should:
1. Understand the file's purpose within the project
2. Identify key elements (functions, classes, interfaces, etc.)
3. Determine dependencies on other files
4. Generate a concise summary

IMPORTANT GUIDELINES:
- Keep descriptions concise but informative
- Focus on the file's role in the project architecture
- Identify public/exported APIs and main functionality
- If file content is not provided, you can use MCP tools to request it
- Stay within token budget provided in the request

Respond with JSON:
{
  "summary": "Brief description of file's purpose and content (max 200 chars)",
  "key_elements": ["function1", "class1", "interface1"], 
  "purpose": "Role of this file in the project (max 100 chars)",
  "dependencies": ["file1.go", "file2.go"]
}`
}

// buildFileAnalysisUserPrompt создает пользовательский промпт для анализа файла
func (g *LLMContextGenerator) buildFileAnalysisUserPrompt(request ContextGenerationRequest) string {
	prompt := fmt.Sprintf(`Project: %s (%s)
File: %s
Token budget: %d tokens

`, request.ProjectName, request.Language, request.FilePath, request.TokenBudget)

	if request.FileContent != "" {
		prompt += fmt.Sprintf("File content:\n```\n%s\n```\n\n", request.FileContent)
	} else {
		prompt += "File content: (large file - use MCP tools to read if needed)\n\n"
	}

	if len(request.FileList) > 0 {
		prompt += fmt.Sprintf("Project files for context:\n%s\n\n", strings.Join(request.FileList[:min(10, len(request.FileList))], "\n"))
	}

	prompt += "Please analyze this file and provide a structured description following the JSON format."
	return prompt
}

// UpdateFileContext обновляет контекст для конкретного файла
func (g *LLMContextGenerator) UpdateFileContext(ctx context.Context, projectContext *ProjectContextLLM, filePath, newContent string) error {
	log.Printf("🔄 Updating LLM context for file: %s", filePath)

	// Проверяем, нужно ли обновление (изменился ли размер существенно)
	if existingContext, exists := projectContext.Files[filePath]; exists {
		sizeDiff := abs(len(newContent) - existingContext.Size)
		if sizeDiff < len(newContent)/10 { // Если изменение меньше 10%
			log.Printf("⏭️ Skipping update for %s - minimal changes", filePath)
			return nil
		}
	}

	// Определяем доступный токен бюджет
	tokenBudget := (projectContext.TokensLimit - projectContext.TokensUsed) / 2 // Половина доступного бюджета

	// Генерируем новый контекст для файла
	newFileContext, err := g.generateFileContext(ctx, projectContext, filePath, newContent, tokenBudget)
	if err != nil {
		return fmt.Errorf("failed to update file context: %w", err)
	}

	// Обновляем контекст проекта
	if oldContext, exists := projectContext.Files[filePath]; exists {
		projectContext.TokensUsed -= oldContext.TokensUsed // Удаляем старые токены
	}

	projectContext.Files[filePath] = *newFileContext
	projectContext.TokensUsed += newFileContext.TokensUsed

	log.Printf("✅ Updated context for %s: %d tokens", filePath, newFileContext.TokensUsed)
	return nil
}

// RemoveFileContext удаляет файл из контекста
func (g *LLMContextGenerator) RemoveFileContext(projectContext *ProjectContextLLM, filePath string) {
	if existingContext, exists := projectContext.Files[filePath]; exists {
		projectContext.TokensUsed -= existingContext.TokensUsed
		delete(projectContext.Files, filePath)
		projectContext.TotalFiles--
		log.Printf("🗑️ Removed context for %s", filePath)
	}
}

// Вспомогательные методы

func (g *LLMContextGenerator) detectMainLanguage(files map[string]string) string {
	langCounts := make(map[string]int)

	for path := range files {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			langCounts["go"]++
		case ".js", ".ts":
			langCounts["javascript"]++
		case ".py":
			langCounts["python"]++
		case ".java":
			langCounts["java"]++
		case ".cpp", ".cc", ".cxx":
			langCounts["cpp"]++
		case ".c":
			langCounts["c"]++
		case ".rs":
			langCounts["rust"]++
		}
	}

	maxCount := 0
	mainLang := "unknown"
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			mainLang = lang
		}
	}

	return mainLang
}

func (g *LLMContextGenerator) analyzeProjectStructure(files map[string]string, structure *ProjectStructure) {
	dirCounts := make(map[string]int)
	fileTypeCounts := make(map[string]int)
	langMap := make(map[string]string)

	for path := range files {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = "root"
		}
		dirCounts[dir]++

		ext := strings.ToLower(filepath.Ext(path))
		fileTypeCounts[ext]++

		switch ext {
		case ".go":
			langMap[ext] = "Go"
		case ".js":
			langMap[ext] = "JavaScript"
		case ".ts":
			langMap[ext] = "TypeScript"
		case ".py":
			langMap[ext] = "Python"
		case ".java":
			langMap[ext] = "Java"
		case ".md":
			langMap[ext] = "Markdown"
		case ".json":
			langMap[ext] = "JSON"
		default:
			langMap[ext] = "Other"
		}
	}

	for dir, count := range dirCounts {
		structure.Directories = append(structure.Directories, Directory{
			Path:      dir,
			FileCount: count,
			Purpose:   g.guessDirPurpose(dir),
		})
	}

	for ext, count := range fileTypeCounts {
		if ext != "" {
			structure.FileTypes = append(structure.FileTypes, FileType{
				Extension: ext,
				Count:     count,
				Language:  langMap[ext],
			})
		}
	}
}

func (g *LLMContextGenerator) guessDirPurpose(dir string) string {
	lowerDir := strings.ToLower(dir)

	switch {
	case strings.Contains(lowerDir, "cmd") || strings.Contains(lowerDir, "main"):
		return "Application entry points"
	case strings.Contains(lowerDir, "internal"):
		return "Internal application code"
	case strings.Contains(lowerDir, "pkg"):
		return "Library code"
	case strings.Contains(lowerDir, "api"):
		return "API definitions and handlers"
	case strings.Contains(lowerDir, "web"):
		return "Web interface"
	case strings.Contains(lowerDir, "test"):
		return "Test files"
	case dir == "root":
		return "Project root files"
	default:
		return "Source code"
	}
}

func (g *LLMContextGenerator) extractDependencies(files map[string]string, language string) []string {
	if language == "go" {
		return g.extractGoDependencies(files)
	}
	return nil
}

func (g *LLMContextGenerator) extractGoDependencies(files map[string]string) []string {
	if goMod, exists := files["go.mod"]; exists {
		return g.parseGoModDependencies(goMod)
	}
	return nil
}

func (g *LLMContextGenerator) parseGoModDependencies(goMod string) []string {
	var deps []string
	lines := strings.Split(goMod, "\n")
	inRequire := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}

		if inRequire && line == ")" {
			break
		}

		if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps = append(deps, parts[0])
			}
		}
	}

	sort.Strings(deps)
	return deps
}

func (g *LLMContextGenerator) sortFilesByImportance(files map[string]string) []string {
	type fileInfo struct {
		path  string
		score int
	}

	var fileList []fileInfo
	for path := range files {
		score := g.calculateFileImportance(path, files[path])
		fileList = append(fileList, fileInfo{path: path, score: score})
	}

	sort.Slice(fileList, func(i, j int) bool {
		return fileList[i].score > fileList[j].score
	})

	result := make([]string, len(fileList))
	for i, fi := range fileList {
		result[i] = fi.path
	}
	return result
}

func (g *LLMContextGenerator) calculateFileImportance(path, content string) int {
	score := 0
	lowerPath := strings.ToLower(path)

	// main.go файлы - высший приоритет
	if strings.Contains(lowerPath, "main.go") {
		score += 1000
	}

	// Файлы с main функцией
	if strings.Contains(content, "func main(") {
		score += 500
	}

	// API и web файлы важны
	if strings.Contains(lowerPath, "api") ||
		strings.Contains(lowerPath, "web") ||
		strings.Contains(lowerPath, "handler") {
		score += 100
	}

	// .go файлы имеют приоритет
	if strings.HasSuffix(lowerPath, ".go") {
		score += 50
	}

	return score
}

func (g *LLMContextGenerator) getFileType(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	default:
		return "other"
	}
}

func (g *LLMContextGenerator) isCodeFile(fileType string) bool {
	codeTypes := []string{"go", "javascript", "typescript", "python", "java", "cpp", "c", "rust"}
	for _, ct := range codeTypes {
		if fileType == ct {
			return true
		}
	}
	return false
}

func (g *LLMContextGenerator) generateGenericSummary(content, fileType string) string {
	lines := len(strings.Split(content, "\n"))

	switch fileType {
	case "markdown":
		if strings.HasPrefix(content, "# ") {
			firstLine := strings.Split(content, "\n")[0]
			return strings.TrimPrefix(firstLine, "# ")
		}
		return fmt.Sprintf("Markdown documentation (%d lines)", lines)
	case "json":
		if strings.Contains(content, `"dependencies"`) {
			return "Package dependencies configuration"
		}
		if strings.Contains(content, `"scripts"`) {
			return "Build scripts configuration"
		}
		return "JSON configuration file"
	default:
		return fmt.Sprintf("%s file (%d lines)", fileType, lines)
	}
}

// GenerateContextWithPreloadedFiles генерирует контекст используя уже готовый список файлов
// Оптимизированная версия для параллельного выполнения
func (g *LLMContextGenerator) GenerateContextWithPreloadedFiles(ctx context.Context, projectName string, fileList []string, fileContentMap map[string]string) (*ProjectContextLLM, error) {
	log.Printf("🧠 [PARALLEL] Starting optimized LLM context generation for '%s' with %d preloaded files (limit: %d tokens)", projectName, len(fileList), g.maxTokens)
	start := time.Now()

	context := &ProjectContextLLM{
		ProjectName: projectName,
		GeneratedAt: time.Now(),
		TotalFiles:  len(fileList),
		Files:       make(map[string]LLMFileContext),
		TokensLimit: g.maxTokens,
		Structure: ProjectStructure{
			Directories: make([]Directory, 0),
			FileTypes:   make([]FileType, 0),
		},
	}
	log.Printf("🧠 [PARALLEL] Context structure initialized (%.2fs)", time.Since(start).Seconds())

	// 1. Определяем язык проекта по файлам
	step1Start := time.Now()
	context.Language = g.detectMainLanguageFromList(fileList)
	log.Printf("🧠 [PARALLEL] Main language detected: %s (%.2fs)", context.Language, time.Since(step1Start).Seconds())

	// 2. Анализируем структуру проекта
	step2Start := time.Now()
	g.analyzeProjectStructureFromList(fileList, &context.Structure)
	log.Printf("🧠 [PARALLEL] Project structure analyzed: %d dirs, %d file types (%.2fs)", len(context.Structure.Directories), len(context.Structure.FileTypes), time.Since(step2Start).Seconds())

	// 3. Генерируем описание проекта
	step3Start := time.Now()
	log.Printf("🧠 [PARALLEL] Starting LLM project description generation...")
	if err := g.generateProjectDescriptionFromList(ctx, context, fileList); err != nil {
		log.Printf("⚠️ [PARALLEL] Failed to generate project description: %v (%.2fs)", err, time.Since(step3Start).Seconds())
		context.Description = fmt.Sprintf("%s project", projectName)
	} else {
		log.Printf("🧠 [PARALLEL] Project description generated: '%s' (%.2fs)", context.Description, time.Since(step3Start).Seconds())
	}

	// 4. Извлекаем зависимости
	step4Start := time.Now()
	context.Dependencies = g.extractDependenciesFromMap(fileContentMap, context.Language)
	log.Printf("🧠 [PARALLEL] Dependencies extracted: %d deps (%.2fs)", len(context.Dependencies), time.Since(step4Start).Seconds())

	// 5. Сортируем файлы по важности
	step5Start := time.Now()
	sortedFiles := g.sortFilesByImportanceFromList(fileList, fileContentMap)
	log.Printf("🧠 [PARALLEL] Files sorted by importance: %d files (%.2fs)", len(sortedFiles), time.Since(step5Start).Seconds())
	if len(sortedFiles) > 0 {
		log.Printf("🧠 [PARALLEL] Top 5 files: %v", sortedFiles[:min(5, len(sortedFiles))])
	}

	// 6. Генерируем контекст для каждого файла с учетом лимита токенов
	tokenBudget := g.maxTokens - g.tokenEstimator.EstimateTokens(context.Description) - 500 // Резерв для метаданных
	log.Printf("🧠 [PARALLEL] Starting individual file context generation. Token budget: %d tokens", tokenBudget)
	processedFiles := 0

	for i, filePath := range sortedFiles {
		if tokenBudget <= 0 {
			log.Printf("⚠️ [PARALLEL] Token budget exhausted after %d files, skipping remaining %d files", processedFiles, len(sortedFiles)-i)
			break
		}

		fileStart := time.Now()
		log.Printf("🧠 [PARALLEL] Processing file %d/%d: %s (budget: %d tokens)", i+1, len(sortedFiles), filePath, tokenBudget)

		// Получаем содержимое файла из переданной карты
		fileContent, exists := fileContentMap[filePath]
		if !exists {
			log.Printf("⚠️ [PARALLEL] File content not found in map: %s, skipping", filePath)
			continue
		}

		fileBudget := tokenBudget / 4 // 1/4 бюджета на файл
		if fileBudget < 50 {
			fileBudget = 50 // Минимальный бюджет
		}

		fileContext, err := g.generateFileContext(ctx, context, filePath, fileContent, fileBudget)
		if err != nil {
			log.Printf("⚠️ [PARALLEL] Failed to generate context for %s: %v (%.2fs)", filePath, err, time.Since(fileStart).Seconds())
			continue
		}

		context.Files[filePath] = *fileContext
		tokenBudget -= fileContext.TokensUsed
		context.TokensUsed += fileContext.TokensUsed
		processedFiles++
		log.Printf("🧠 [PARALLEL] ✅ File processed: %s (%d tokens used, %d remaining, %.2fs)", filePath, fileContext.TokensUsed, tokenBudget, time.Since(fileStart).Seconds())
	}

	log.Printf("✅ [PARALLEL] LLM context generation completed: %d/%d files processed, %d/%d tokens used (%.2fs total)",
		len(context.Files), len(fileList), context.TokensUsed, context.TokensLimit, time.Since(start).Seconds())
	return context, nil
}

// generateProjectDescriptionFromList генерирует описание проекта используя список файлов
func (g *LLMContextGenerator) generateProjectDescriptionFromList(ctx context.Context, context *ProjectContextLLM, fileList []string) error {
	start := time.Now()
	log.Printf("🧠 [DESC-LIST] Starting project description generation from file list...")

	systemPrompt := `You are a code analysis assistant. Analyze the project structure and generate a concise description.

Your task is to:
1. Understand the project's main purpose based on file structure and names
2. Identify the primary technology/framework being used
3. Generate a concise description (max 100 characters)

Respond with JSON:
{
  "description": "Brief project description",
  "language": "primary programming language"
}`

	userPrompt := fmt.Sprintf(`Project: %s
Language: %s
Total files: %d

File structure:
%s

Please analyze this project structure and provide a concise description.`,
		context.ProjectName,
		context.Language,
		len(fileList),
		strings.Join(fileList[:min(20, len(fileList))], "\n")) // Первые 20 файлов

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	log.Printf("🧠 [DESC-LIST] Prompt prepared, sending to LLM... (%.2fs)", time.Since(start).Seconds())

	llmStart := time.Now()
	response, err := g.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ [DESC-LIST] LLM request failed after %.2fs: %v", time.Since(llmStart).Seconds(), err)
		return fmt.Errorf("LLM request failed: %w", err)
	}
	log.Printf("🧠 [DESC-LIST] LLM response received (%.2fs), parsing...", time.Since(llmStart).Seconds())

	var result struct {
		Description string `json:"description"`
		Language    string `json:"language"`
	}

	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		log.Printf("⚠️ [DESC-LIST] JSON parsing failed, using raw response: %s", response.Content[:min(100, len(response.Content))])
		context.Description = strings.TrimSpace(response.Content)
		if len(context.Description) > 100 {
			context.Description = context.Description[:100] + "..."
		}
		log.Printf("🧠 [DESC-LIST] Description set from raw response: '%s' (%.2fs total)", context.Description, time.Since(start).Seconds())
		return nil
	}

	context.Description = result.Description
	if result.Language != "" && result.Language != "unknown" {
		context.Language = result.Language
	}
	log.Printf("🧠 [DESC-LIST] ✅ Description parsed successfully: '%s' (%.2fs total)", context.Description, time.Since(start).Seconds())

	return nil
}

// detectMainLanguageFromList определяет основной язык по списку файлов
func (g *LLMContextGenerator) detectMainLanguageFromList(fileList []string) string {
	langCounts := make(map[string]int)

	for _, path := range fileList {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			langCounts["go"]++
		case ".js", ".ts":
			langCounts["javascript"]++
		case ".py":
			langCounts["python"]++
		case ".java":
			langCounts["java"]++
		case ".cpp", ".cc", ".cxx":
			langCounts["cpp"]++
		case ".c":
			langCounts["c"]++
		case ".rs":
			langCounts["rust"]++
		}
	}

	maxCount := 0
	mainLang := "unknown"
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			mainLang = lang
		}
	}

	return mainLang
}

// analyzeProjectStructureFromList анализирует структуру проекта по списку файлов
func (g *LLMContextGenerator) analyzeProjectStructureFromList(fileList []string, structure *ProjectStructure) {
	dirCounts := make(map[string]int)
	fileTypeCounts := make(map[string]int)
	langMap := make(map[string]string)

	for _, path := range fileList {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = "root"
		}
		dirCounts[dir]++

		ext := strings.ToLower(filepath.Ext(path))
		fileTypeCounts[ext]++

		switch ext {
		case ".go":
			langMap[ext] = "Go"
		case ".js":
			langMap[ext] = "JavaScript"
		case ".ts":
			langMap[ext] = "TypeScript"
		case ".py":
			langMap[ext] = "Python"
		case ".java":
			langMap[ext] = "Java"
		case ".md":
			langMap[ext] = "Markdown"
		case ".json":
			langMap[ext] = "JSON"
		default:
			langMap[ext] = "Other"
		}
	}

	for dir, count := range dirCounts {
		structure.Directories = append(structure.Directories, Directory{
			Path:      dir,
			FileCount: count,
			Purpose:   g.guessDirPurpose(dir),
		})
	}

	for ext, count := range fileTypeCounts {
		if ext != "" {
			structure.FileTypes = append(structure.FileTypes, FileType{
				Extension: ext,
				Count:     count,
				Language:  langMap[ext],
			})
		}
	}
}

// extractDependenciesFromMap извлекает зависимости из карты файлов
func (g *LLMContextGenerator) extractDependenciesFromMap(fileMap map[string]string, language string) []string {
	if language == "go" {
		if goMod, exists := fileMap["go.mod"]; exists {
			return g.parseGoModDependencies(goMod)
		}
	}
	return nil
}

// sortFilesByImportanceFromList сортирует файлы по важности используя список и карту содержимого
func (g *LLMContextGenerator) sortFilesByImportanceFromList(fileList []string, fileContentMap map[string]string) []string {
	type fileInfo struct {
		path  string
		score int
	}

	var files []fileInfo
	for _, path := range fileList {
		content := fileContentMap[path] // Может быть пустым, это нормально
		score := g.calculateFileImportance(path, content)
		files = append(files, fileInfo{path: path, score: score})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].score > files[j].score
	})

	result := make([]string, len(files))
	for i, fi := range files {
		result[i] = fi.path
	}
	return result
}

// Вспомогательные функции
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
