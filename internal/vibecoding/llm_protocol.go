package vibecoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"ai-chatter/internal/llm"
)

// VibeCodingRequest представляет запрос к LLM для вайбкодинга
type VibeCodingRequest struct {
	Action  string                 `json:"action"`            // "analyze", "generate_code", "answer_question", "autonomous_work"
	Context VibeCodingContext      `json:"context"`           // Контекст сессии
	Query   string                 `json:"query"`             // Вопрос или запрос пользователя
	Options map[string]interface{} `json:"options,omitempty"` // Дополнительные опции
}

// VibeCodingContext содержит контекст сессии
type VibeCodingContext struct {
	ProjectName     string            `json:"project_name"`
	Language        string            `json:"language"`
	Files           map[string]string `json:"files"`
	GeneratedFiles  map[string]string `json:"generated_files,omitempty"`
	SessionDuration string            `json:"session_duration"`
}

// VibeCodingResponse представляет ответ от LLM
type VibeCodingResponse struct {
	Status      string                 `json:"status"`                // "success", "error", "partial"
	Response    string                 `json:"response"`              // Основной ответ
	Code        map[string]string      `json:"code,omitempty"`        // Сгенерированный код: filename -> content
	Suggestions []string               `json:"suggestions,omitempty"` // Предложения для дальнейших действий
	Error       string                 `json:"error,omitempty"`       // Сообщение об ошибке
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Дополнительные метаданные
}

// VibeCodingLLMClient обертка для LLM клиента с JSON протоколом
type VibeCodingLLMClient struct {
	llmClient  llm.Client
	maxRetries int
	mcpClient  *VibeCodingMCPClient // MCP клиент для прямого доступа к файлам
}

// NewVibeCodingLLMClient создает новый клиент с JSON протоколом
func NewVibeCodingLLMClient(llmClient llm.Client) *VibeCodingLLMClient {
	return &VibeCodingLLMClient{
		llmClient:  llmClient,
		maxRetries: 3,
		mcpClient:  nil, // будет установлен через SetMCPClient
	}
}

// SetMCPClient устанавливает MCP клиент для прямого доступа к файлам
func (c *VibeCodingLLMClient) SetMCPClient(mcpClient *VibeCodingMCPClient) {
	c.mcpClient = mcpClient
}

// ProcessRequest обрабатывает запрос через JSON протокол
func (c *VibeCodingLLMClient) ProcessRequest(ctx context.Context, request VibeCodingRequest) (*VibeCodingResponse, error) {
	log.Printf("🧠 Processing VibeCoding request: action=%s, query_length=%d", request.Action, len(request.Query))

	var systemPrompt string
	var userPrompt string

	switch request.Action {
	case "answer_question":
		systemPrompt, userPrompt = c.buildQuestionPrompts(request)
	case "generate_code":
		systemPrompt, userPrompt = c.buildCodeGenerationPrompts(request)
	case "analyze":
		systemPrompt, userPrompt = c.buildAnalysisPrompts(request)
	case "autonomous_work":
		return c.processAutonomousWork(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported action: %s", request.Action)
	}

	// Отправляем запрос с retry логикой
	var lastError error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		response, err := c.sendRequestWithRetry(ctx, systemPrompt, userPrompt, attempt)
		if err == nil {
			return response, nil
		}

		lastError = err
		log.Printf("⚠️ Attempt %d failed: %v", attempt, err)

		// Если это последняя попытка или ошибка не связана с JSON парсингом, выходим
		if attempt == c.maxRetries || !isJSONParsingError(err) {
			break
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries, lastError)
}

// sendRequestWithRetry отправляет запрос к LLM с обработкой JSON ответа
func (c *VibeCodingLLMClient) sendRequestWithRetry(ctx context.Context, systemPrompt, userPrompt string, attempt int) (*VibeCodingResponse, error) {
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	if attempt > 1 {
		// Добавляем инструкцию для повторной попытки
		retryInstruction := "IMPORTANT: The previous response was not valid JSON. Please ensure your response is a properly formatted JSON object according to the schema."
		messages = append(messages, llm.Message{Role: "user", Content: retryInstruction})
	}

	log.Printf("🔄 Sending request to LLM (attempt %d)", attempt)

	llmResponse, err := c.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	log.Printf("📝 Received LLM response length: %d characters", len(llmResponse.Content))

	// Парсим JSON ответ
	response, err := c.parseJSONResponse(llmResponse.Content)
	if err != nil {
		log.Printf("❌ JSON parsing failed: %v", err)
		log.Printf("Raw response: %s", llmResponse.Content)

		// Пытаемся исправить JSON если это возможно
		if fixedResponse, fixErr := c.tryFixJSON(ctx, llmResponse.Content, attempt); fixErr == nil {
			return fixedResponse, nil
		}

		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Валидируем ответ
	if err := c.validateResponse(response); err != nil {
		return nil, fmt.Errorf("invalid response structure: %w", err)
	}

	return response, nil
}

// buildQuestionPrompts строит промпты для ответов на вопросы
func (c *VibeCodingLLMClient) buildQuestionPrompts(request VibeCodingRequest) (string, string) {
	systemPrompt := `You are an expert software development assistant in VibeCoding mode - an interactive development session.

Your task is to provide helpful, practical answers about the code project. Always respond with valid JSON matching this exact schema:

{
  "status": "success|error|partial",
  "response": "your main answer/explanation",
  "code": {
    "filename.ext": "code content if you're suggesting code changes"
  },
  "suggestions": ["list of follow-up suggestions or next steps"],
  "error": "error message if status is error",
  "metadata": {
    "reasoning": "brief explanation of your approach"
  }
}

Guidelines:
- Be concise but informative
- If suggesting code, include it in the "code" field with appropriate filenames
- Provide actionable suggestions in the "suggestions" field
- Use "status": "success" for normal responses
- Only use "status": "error" if you cannot process the request
- Use "status": "partial" if you can partially answer but need more information`

	userPrompt := fmt.Sprintf(`PROJECT CONTEXT:
Project: %s
Language: %s
Session Duration: %s
Files in project: %d
Generated files: %d

AVAILABLE FILES:
%s

USER QUESTION:
%s

Please provide a helpful response about this code project.`,
		request.Context.ProjectName,
		request.Context.Language,
		request.Context.SessionDuration,
		len(request.Context.Files),
		len(request.Context.GeneratedFiles),
		c.formatFileList(request.Context.Files),
		request.Query)

	return systemPrompt, userPrompt
}

// buildCodeGenerationPrompts строит промпты для генерации кода
func (c *VibeCodingLLMClient) buildCodeGenerationPrompts(request VibeCodingRequest) (string, string) {
	systemPrompt := `You are an expert code generator in VibeCoding mode. Generate high-quality, working code based on user requests.

Respond with valid JSON matching this exact schema:

{
  "status": "success|error|partial",
  "response": "explanation of what you generated",
  "code": {
    "filename.ext": "complete code content"
  },
  "suggestions": ["suggestions for testing, improvement, or next steps"],
  "error": "error message if status is error",
  "metadata": {
    "language": "programming language used",
    "approach": "brief description of your approach"
  }
}

Guidelines:
- Generate complete, working code
- Follow the project's language conventions
- Include appropriate comments
- Suggest testing approaches
- Consider error handling and edge cases`

	userPrompt := fmt.Sprintf(`PROJECT CONTEXT:
Project: %s
Language: %s
Existing files: %d

EXISTING CODE:
%s

CODE GENERATION REQUEST:
%s

Generate the requested code following best practices for %s.`,
		request.Context.ProjectName,
		request.Context.Language,
		len(request.Context.Files),
		c.formatCodeContext(request.Context.Files),
		request.Query,
		request.Context.Language)

	return systemPrompt, userPrompt
}

// buildAnalysisPrompts строит промпты для анализа проекта
func (c *VibeCodingLLMClient) buildAnalysisPrompts(request VibeCodingRequest) (string, string) {
	systemPrompt := `You are a code analysis expert. Analyze the provided project and give insights.

Respond with valid JSON matching this exact schema:

{
  "status": "success|error",
  "response": "your analysis summary",
  "suggestions": ["actionable recommendations"],
  "metadata": {
    "complexity": "low|medium|high",
    "quality": "assessment of code quality",
    "issues": ["list of identified issues"]
  }
}

Focus on:
- Code structure and organization
- Potential improvements
- Best practices compliance
- Security considerations
- Performance implications`

	userPrompt := fmt.Sprintf(`ANALYSIS REQUEST:
Project: %s (%s)
%s

FILES TO ANALYZE:
%s`,
		request.Context.ProjectName,
		request.Context.Language,
		request.Query,
		c.formatCodeContext(request.Context.Files))

	return systemPrompt, userPrompt
}

// parseJSONResponse парсит JSON ответ от LLM
func (c *VibeCodingLLMClient) parseJSONResponse(content string) (*VibeCodingResponse, error) {
	// Очищаем контент от лишних символов
	content = strings.TrimSpace(content)

	// Ищем JSON блок если есть markdown форматирование
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			content = strings.TrimSpace(content[start : start+end])
		}
	} else if strings.Contains(content, "```") {
		// Пытаемся извлечь JSON из обычных блоков кода
		start := strings.Index(content, "```") + 3
		end := strings.Index(content[start:], "```")
		if end > 0 {
			candidate := strings.TrimSpace(content[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				content = candidate
			}
		}
	}

	var response VibeCodingResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// tryFixJSON пытается исправить некорректный JSON
func (c *VibeCodingLLMClient) tryFixJSON(ctx context.Context, rawContent string, attempt int) (*VibeCodingResponse, error) {
	log.Printf("🔧 Attempting to fix JSON response (attempt %d)", attempt)

	fixPrompt := `The following response should be valid JSON but has formatting issues. Please fix it and return only the corrected JSON:

BROKEN JSON:
` + rawContent + `

Return only the corrected JSON object, no other text.`

	messages := []llm.Message{
		{Role: "system", Content: "You are a JSON formatter. Fix the provided JSON and return only valid JSON."},
		{Role: "user", Content: fixPrompt},
	}

	fixResponse, err := c.llmClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to fix JSON: %w", err)
	}

	// Пытаемся парсить исправленный JSON
	return c.parseJSONResponse(fixResponse.Content)
}

// validateResponse проверяет корректность структуры ответа
func (c *VibeCodingLLMClient) validateResponse(response *VibeCodingResponse) error {
	if response.Status == "" {
		return fmt.Errorf("status field is required")
	}

	validStatuses := []string{"success", "error", "partial"}
	isValidStatus := false
	for _, status := range validStatuses {
		if response.Status == status {
			isValidStatus = true
			break
		}
	}
	if !isValidStatus {
		return fmt.Errorf("invalid status: %s, must be one of %v", response.Status, validStatuses)
	}

	if response.Status == "error" && response.Error == "" {
		return fmt.Errorf("error field is required when status is error")
	}

	if response.Status == "success" && response.Response == "" {
		return fmt.Errorf("response field is required when status is success")
	}

	return nil
}

// formatFileList форматирует список файлов для контекста
func (c *VibeCodingLLMClient) formatFileList(files map[string]string) string {
	var result strings.Builder
	fileCount := 0
	maxFiles := 5 // Ограничиваем количество файлов в контексте

	for filename := range files {
		if fileCount >= maxFiles {
			result.WriteString("... and more files\n")
			break
		}
		result.WriteString(fmt.Sprintf("- %s\n", filename))
		fileCount++
	}

	return result.String()
}

// formatCodeContext форматирует код для контекста
func (c *VibeCodingLLMClient) formatCodeContext(files map[string]string) string {
	var result strings.Builder
	fileCount := 0
	maxFiles := 3 // Ограничиваем количество файлов с кодом

	for filename, content := range files {
		if fileCount >= maxFiles {
			result.WriteString("... (additional files not shown)\n")
			break
		}

		result.WriteString(fmt.Sprintf("=== %s ===\n", filename))

		// Ограничиваем размер файла
		if len(content) > 1000 {
			result.WriteString(content[:1000])
			result.WriteString("\n... (content truncated)\n")
		} else {
			result.WriteString(content)
		}
		result.WriteString("\n\n")
		fileCount++
	}

	return result.String()
}

// processAutonomousWork обрабатывает запрос на автономную работу с MCP инструментами
func (c *VibeCodingLLMClient) processAutonomousWork(ctx context.Context, request VibeCodingRequest) (*VibeCodingResponse, error) {
	if c.mcpClient == nil {
		return &VibeCodingResponse{
			Status: "error",
			Error:  "MCP client not available for autonomous work",
		}, nil
	}

	log.Printf("🤖 Starting autonomous work: %s", request.Query)

	// Извлекаем userID из контекста (из опций запроса)
	userID, ok := request.Options["user_id"].(int64)
	if !ok {
		// Попытка конвертации из float64 (JSON unmarshaling)
		if userIDFloat, ok := request.Options["user_id"].(float64); ok {
			userID = int64(userIDFloat)
		} else {
			return &VibeCodingResponse{
				Status: "error",
				Error:  "user_id required in options for autonomous work",
			}, nil
		}
	}

	// Создаем системный промпт для автономной работы с MCP инструментами
	systemPrompt := c.buildMCPSystemPrompt()
	userPrompt := c.buildMCPUserPrompt(request, userID)

	maxSteps := 10 // Максимальное количество шагов автономной работы
	var executionLog []string
	var allGeneratedCode = make(map[string]string)

	for step := 1; step <= maxSteps; step++ {
		log.Printf("🔄 Autonomous work step %d/%d", step, maxSteps)

		// Отправляем запрос к LLM
		messages := []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}

		// Добавляем предыдущие шаги в контекст
		if len(executionLog) > 0 {
			historyPrompt := "PREVIOUS EXECUTION STEPS:\n" + strings.Join(executionLog, "\n\n")
			messages = append(messages, llm.Message{Role: "assistant", Content: historyPrompt})
		}

		llmResponse, err := c.llmClient.Generate(ctx, messages)
		if err != nil {
			executionLog = append(executionLog, fmt.Sprintf("Step %d ERROR: LLM request failed: %v", step, err))
			break
		}

		// Парсим ответ LLM на предмет MCP команд
		stepResult, shouldContinue, err := c.processMCPStep(ctx, llmResponse.Content, userID, step)
		if err != nil {
			executionLog = append(executionLog, fmt.Sprintf("Step %d ERROR: %v", step, err))
			break
		}

		executionLog = append(executionLog, stepResult)

		// Если LLM указал, что работа завершена
		if !shouldContinue {
			log.Printf("✅ Autonomous work completed at step %d", step)
			break
		}

		// Проверяем, не достигли ли максимума шагов
		if step == maxSteps {
			executionLog = append(executionLog, "⚠️ Reached maximum number of steps")
			log.Printf("⚠️ Autonomous work reached maximum steps (%d)", maxSteps)
		}
	}

	return &VibeCodingResponse{
		Status:   "success",
		Response: "Autonomous work completed",
		Code:     allGeneratedCode,
		Suggestions: []string{
			"Review the generated code",
			"Run tests to verify functionality",
			"Consider additional improvements",
		},
		Metadata: map[string]interface{}{
			"execution_log":  executionLog,
			"steps_executed": len(executionLog),
		},
	}, nil
}

// buildMCPSystemPrompt создает системный промпт для работы с MCP инструментами
func (c *VibeCodingLLMClient) buildMCPSystemPrompt() string {
	return `You are an autonomous coding assistant with access to MCP tools for direct file manipulation. 
You can read, write, delete files, execute commands, run tests, and validate code without user interaction.

AVAILABLE MCP TOOLS:
- vibe_list_files(user_id): List all files in session
- vibe_read_file(user_id, filename): Read file content
- vibe_write_file(user_id, filename, content, generated=true): Write/update file
- vibe_delete_file(user_id, filename): Delete file
- vibe_execute_command(user_id, command): Execute shell command
- vibe_validate_code(user_id, filename=""): Validate code syntax
- vibe_run_tests(user_id, test_file=""): Run tests
- vibe_get_session_info(user_id): Get session information

RESPONSE FORMAT:
Respond with a JSON object containing your action plan:

{
  "action": "continue|complete",
  "reasoning": "explain what you're doing and why",
  "mcp_calls": [
    {
      "tool": "tool_name",
      "params": {"param1": "value1", "param2": "value2"},
      "purpose": "why you're calling this tool"
    }
  ],
  "next_step": "description of what to do next (if action is continue)",
  "summary": "summary of work completed (if action is complete)"
}

GUIDELINES:
- Start by understanding the current project state (list files, read key files)
- Make incremental changes and validate them
- Test your changes after implementation
- Fix any errors you encounter
- Work autonomously without asking for user input
- Use "action": "complete" when the task is finished
- Use "action": "continue" if more work is needed
- Be methodical and careful with file operations
- Always validate generated code before considering work complete`
}

// buildMCPUserPrompt создает пользовательский промпт для MCP работы
func (c *VibeCodingLLMClient) buildMCPUserPrompt(request VibeCodingRequest, userID int64) string {
	prompt := fmt.Sprintf(`AUTONOMOUS WORK REQUEST:
User ID: %d
Project: %s (%s)
Task: %s

`, userID, request.Context.ProjectName, request.Context.Language, request.Query)

	// Добавляем сжатый контекст проекта если передан через опции
	if contextInterface, exists := request.Options["project_context"]; exists {
		if context, ok := contextInterface.(*ProjectContext); ok && context != nil {
			prompt += "PROJECT CONTEXT (COMPRESSED):\n"
			prompt += fmt.Sprintf("Language: %s | Total files: %d\n", context.Language, context.TotalFiles)

			if context.Description != "" {
				prompt += fmt.Sprintf("Description: %s\n", context.Description)
			}

			// Показываем ключевые файлы
			for i, file := range context.Files {
				if i >= 8 { // Ограничиваем для избежания переполнения контекста
					prompt += fmt.Sprintf("... and %d more files (use vibe_list_files to see all)\n", len(context.Files)-8)
					break
				}

				prompt += fmt.Sprintf("\n%s (%s):\n", file.Path, file.Type)
				if file.Summary != "" {
					prompt += fmt.Sprintf("  Summary: %s\n", file.Summary)
				}

				// Ключевые функции и структуры
				if len(file.Functions) > 0 {
					funcNames := make([]string, 0, len(file.Functions))
					for j, fn := range file.Functions {
						if j >= 3 { // Показываем только первые 3
							funcNames = append(funcNames, "...")
							break
						}
						funcNames = append(funcNames, fn.Name)
					}
					prompt += fmt.Sprintf("  Functions: %s\n", strings.Join(funcNames, ", "))
				}

				if len(file.Structs) > 0 {
					structNames := make([]string, 0, len(file.Structs))
					for j, st := range file.Structs {
						if j >= 3 {
							structNames = append(structNames, "...")
							break
						}
						structNames = append(structNames, st.Name)
					}
					prompt += fmt.Sprintf("  Structs: %s\n", strings.Join(structNames, ", "))
				}
			}

			prompt += "\nIMPORTANT: This is only a summary. Use vibe_read_file to get full file content when needed.\n\n"
		}
	}

	prompt += `Please work autonomously to complete this task using the available MCP tools. 
Start by assessing the current project state and then proceed with the implementation.

Remember to:
1. Understand the existing codebase first (use vibe_read_file for full content)
2. Implement changes incrementally
3. Test and validate your work
4. Fix any issues you encounter
5. Provide a summary when complete`

	return prompt
}

// processMCPStep обрабатывает один шаг автономной работы
func (c *VibeCodingLLMClient) processMCPStep(ctx context.Context, llmResponse string, userID int64, step int) (string, bool, error) {
	// Парсим JSON ответ от LLM
	var stepResponse struct {
		Action    string `json:"action"` // "continue" или "complete"
		Reasoning string `json:"reasoning"`
		MCPCalls  []struct {
			Tool    string                 `json:"tool"`
			Params  map[string]interface{} `json:"params"`
			Purpose string                 `json:"purpose"`
		} `json:"mcp_calls"`
		NextStep string `json:"next_step"`
		Summary  string `json:"summary"`
	}

	if err := json.Unmarshal([]byte(llmResponse), &stepResponse); err != nil {
		// Пытаемся извлечь JSON из markdown блока
		if strings.Contains(llmResponse, "```json") {
			start := strings.Index(llmResponse, "```json") + 7
			end := strings.Index(llmResponse[start:], "```")
			if end > 0 {
				jsonContent := strings.TrimSpace(llmResponse[start : start+end])
				if err := json.Unmarshal([]byte(jsonContent), &stepResponse); err != nil {
					return fmt.Sprintf("Step %d: Failed to parse LLM response as JSON: %v", step, err), false, err
				}
			} else {
				return fmt.Sprintf("Step %d: Invalid JSON in markdown block", step), false, err
			}
		} else {
			return fmt.Sprintf("Step %d: Failed to parse LLM response: %v", step, err), false, err
		}
	}

	log.Printf("🎯 Step %d reasoning: %s", step, stepResponse.Reasoning)

	var stepLog strings.Builder
	stepLog.WriteString(fmt.Sprintf("Step %d: %s\n", step, stepResponse.Reasoning))

	// Выполняем MCP вызовы
	for i, mcpCall := range stepResponse.MCPCalls {
		log.Printf("🔧 Executing MCP call %d/%d: %s", i+1, len(stepResponse.MCPCalls), mcpCall.Tool)
		stepLog.WriteString(fmt.Sprintf("  MCP Call %d: %s - %s\n", i+1, mcpCall.Tool, mcpCall.Purpose))

		// Добавляем user_id если его нет в параметрах
		if mcpCall.Params["user_id"] == nil {
			mcpCall.Params["user_id"] = float64(userID) // JSON unmarshaling создает float64
		}

		// Выполняем MCP инструмент через клиент
		var result VibeCodingMCPResult
		var err error

		switch mcpCall.Tool {
		case "vibe_list_files":
			result = c.mcpClient.ListFiles(ctx, userID)
		case "vibe_read_file":
			filename := ""
			if f, ok := mcpCall.Params["filename"].(string); ok {
				filename = f
			}
			result = c.mcpClient.ReadFile(ctx, userID, filename)
		case "vibe_write_file":
			filename := ""
			content := ""
			generated := true
			if f, ok := mcpCall.Params["filename"].(string); ok {
				filename = f
			}
			if c, ok := mcpCall.Params["content"].(string); ok {
				content = c
			}
			if g, ok := mcpCall.Params["generated"].(bool); ok {
				generated = g
			}
			result = c.mcpClient.WriteFile(ctx, userID, filename, content, generated)
		case "vibe_execute_command":
			command := ""
			if cmd, ok := mcpCall.Params["command"].(string); ok {
				command = cmd
			}
			result = c.mcpClient.ExecuteCommand(ctx, userID, command)
		case "vibe_validate_code":
			filename := ""
			if f, ok := mcpCall.Params["filename"].(string); ok {
				filename = f
			}
			result = c.mcpClient.ValidateCode(ctx, userID, filename)
		case "vibe_run_tests":
			testFile := ""
			if f, ok := mcpCall.Params["test_file"].(string); ok {
				testFile = f
			}
			result = c.mcpClient.RunTests(ctx, userID, testFile)
		case "vibe_get_session_info":
			result = c.mcpClient.GetSessionInfo(ctx, userID)
		default:
			err = fmt.Errorf("unknown MCP tool: %s", mcpCall.Tool)
		}

		if err != nil {
			errorMsg := fmt.Sprintf("    ERROR: %v", err)
			stepLog.WriteString(errorMsg + "\n")
			log.Printf("❌ MCP call failed: %v", err)
		} else if !result.Success {
			errorMsg := fmt.Sprintf("    FAILED: %s", result.Message)
			stepLog.WriteString(errorMsg + "\n")
			log.Printf("❌ MCP call failed: %s", result.Message)
		} else {
			stepLog.WriteString("    SUCCESS\n")
			log.Printf("✅ MCP call successful")

			// Логируем результат для важных операций
			if mcpCall.Tool == "vibe_read_file" {
				if len(result.Message) > 200 {
					stepLog.WriteString(fmt.Sprintf("      Content: %s... (%d chars)\n", result.Message[:200], len(result.Message)))
				} else {
					stepLog.WriteString(fmt.Sprintf("      Content: %s\n", result.Message))
				}
			} else if mcpCall.Tool == "vibe_list_files" {
				stepLog.WriteString(fmt.Sprintf("      Total files: %d\n", result.TotalFiles))
			}
		}
	}

	// Определяем нужно ли продолжать
	shouldContinue := stepResponse.Action == "continue"

	if stepResponse.Action == "complete" {
		stepLog.WriteString(fmt.Sprintf("COMPLETED: %s\n", stepResponse.Summary))
	} else if stepResponse.NextStep != "" {
		stepLog.WriteString(fmt.Sprintf("Next: %s\n", stepResponse.NextStep))
	}

	return stepLog.String(), shouldContinue, nil
}

// isJSONParsingError проверяет, является ли ошибка ошибкой парсинга JSON
func isJSONParsingError(err error) bool {
	return strings.Contains(err.Error(), "failed to parse JSON") ||
		strings.Contains(err.Error(), "invalid character") ||
		strings.Contains(err.Error(), "unexpected end of JSON")
}
