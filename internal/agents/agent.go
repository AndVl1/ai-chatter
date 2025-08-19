package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-chatter/internal/gmail"
	"ai-chatter/internal/llm"
	"ai-chatter/internal/notion"
)

const (
	// MaxRetryAttempts максимальное количество попыток исправления
	MaxRetryAttempts = 5
)

// Agent представляет агента для работы с MCP серверами
type Agent struct {
	name      string
	llmClient llm.Client
}

// NewAgent создает нового агента
func NewAgent(name string, llmClient llm.Client) *Agent {
	return &Agent{
		name:      name,
		llmClient: llmClient,
	}
}

// AgentMessage представляет сообщение между агентами
type AgentMessage struct {
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Type      string                 `json:"type"`
	Content   string                 `json:"content"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ValidationResponse ответ валидации от агента
type ValidationResponse struct {
	IsValid           bool        `json:"is_valid"`
	Message           string      `json:"message"`
	SuggestedAction   string      `json:"suggested_action,omitempty"`
	CorrectionRequest string      `json:"correction_request,omitempty"` // Запрос на исправление для исходного агента
	SpecificIssues    interface{} `json:"specific_issues,omitempty"`    // Конкретные проблемы для исправления (строка или массив)
}

// GetSpecificIssuesString возвращает specific_issues как строку
func (v *ValidationResponse) GetSpecificIssuesString() string {
	if v.SpecificIssues == nil {
		return ""
	}

	switch issues := v.SpecificIssues.(type) {
	case string:
		return issues
	case []interface{}:
		var result []string
		for _, item := range issues {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return strings.Join(result, "; ")
	case []string:
		return strings.Join(issues, "; ")
	default:
		// Попытка преобразования в JSON и обратно в строку
		if jsonBytes, err := json.Marshal(issues); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", issues)
	}
}

// ProgressCallback интерфейс для уведомлений о прогрессе
type ProgressCallback interface {
	UpdateProgress(step string, status string) // step - название шага, status - статус (in_progress, completed, error)
}

// ProgressStep представляет шаг выполнения
type ProgressStep struct {
	Name        string
	Description string
	Status      string // pending, in_progress, completed, error
}

// GmailSummaryWorkflow координирует работу агентов для создания Gmail summary
type GmailSummaryWorkflow struct {
	gmailAgent   *Agent
	notionAgent  *Agent
	gmailClient  *gmail.GmailMCPClient
	notionClient *notion.MCPClient
}

// NewGmailSummaryWorkflow создает новый рабочий процесс
func NewGmailSummaryWorkflow(
	gmailLLM llm.Client,
	notionLLM llm.Client,
	gmailClient *gmail.GmailMCPClient,
	notionClient *notion.MCPClient,
) *GmailSummaryWorkflow {
	return &GmailSummaryWorkflow{
		gmailAgent:   NewAgent("gmail-agent", gmailLLM),
		notionAgent:  NewAgent("notion-agent", notionLLM),
		gmailClient:  gmailClient,
		notionClient: notionClient,
	}
}

// ProcessGmailSummaryRequest обрабатывает запрос на создание Gmail summary
func (w *GmailSummaryWorkflow) ProcessGmailSummaryRequest(ctx context.Context, userQuery string) (string, error) {
	return w.ProcessGmailSummaryRequestWithProgress(ctx, userQuery, nil)
}

// ProcessGmailSummaryRequestWithProgress обрабатывает запрос с callback для прогресса
func (w *GmailSummaryWorkflow) ProcessGmailSummaryRequestWithProgress(ctx context.Context, userQuery string, progressCallback ProgressCallback) (string, error) {
	log.Printf("🔄 Starting Gmail summary workflow for query: %s", userQuery)

	// Шаг 1: Gmail агент собирает данные с retry механизмом
	if progressCallback != nil {
		progressCallback.UpdateProgress("gmail_data", "in_progress")
	}
	gmailData, err := w.collectGmailDataWithRetries(ctx, userQuery)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("gmail_data", "error")
		}
		return "", fmt.Errorf("failed to collect Gmail data: %w", err)
	}
	if progressCallback != nil {
		progressCallback.UpdateProgress("gmail_data", "completed")
	}

	// Валидация уже выполняется внутри collectGmailDataWithRetries
	if progressCallback != nil {
		progressCallback.UpdateProgress("validate_data", "completed")
	}
	log.Printf("✅ Gmail data collected and validated successfully")

	// Шаг 3: Notion агент ищет или создает папку "Gmail summaries"
	if progressCallback != nil {
		progressCallback.UpdateProgress("notion_setup", "in_progress")
	}
	summariesPageID, err := w.ensureGmailSummariesPage(ctx)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("notion_setup", "error")
		}
		return "", fmt.Errorf("failed to ensure Gmail summaries page: %w", err)
	}
	if progressCallback != nil {
		progressCallback.UpdateProgress("notion_setup", "completed")
	}

	// Шаг 4: Создание саммари с retry механизмом
	if progressCallback != nil {
		progressCallback.UpdateProgress("generate_summary", "in_progress")
	}
	summaryTitle, summaryContent, err := w.generateSummaryWithRetries(ctx, gmailData, userQuery)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("generate_summary", "error")
		}
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}
	if progressCallback != nil {
		progressCallback.UpdateProgress("generate_summary", "completed")
	}

	// Валидация уже выполняется внутри generateSummaryWithRetries
	if progressCallback != nil {
		progressCallback.UpdateProgress("validate_summary", "completed")
	}
	log.Printf("✅ Summary generated and validated successfully")

	// Шаг 6: Создание страницы в Notion
	if progressCallback != nil {
		progressCallback.UpdateProgress("create_notion_page", "in_progress")
	}
	pageURL, err := w.createNotionPage(ctx, summariesPageID, summaryTitle, summaryContent)
	if err != nil {
		if progressCallback != nil {
			progressCallback.UpdateProgress("create_notion_page", "error")
		}
		return "", fmt.Errorf("failed to create Notion page: %w", err)
	}
	if progressCallback != nil {
		progressCallback.UpdateProgress("create_notion_page", "completed")
	}

	log.Printf("✅ Gmail summary workflow completed successfully")
	return pageURL, nil
}

// collectGmailData собирает данные из Gmail через агента
func (w *GmailSummaryWorkflow) collectGmailData(ctx context.Context, userQuery string) (string, error) {
	log.Printf("📧 Gmail agent collecting data for query: %s", userQuery)

	// Создаем системный промпт для Gmail агента
	systemPrompt := `You are a Gmail data collection agent. Your task is to search Gmail and collect relevant emails based on user queries.

User query: "` + userQuery + `"

Your tasks:
1. Analyze the user query to determine appropriate Gmail search parameters
2. Search Gmail for relevant emails
3. Extract and summarize the most important information
4. Focus on unread, important, or recent emails that match the user's intent

Return your findings in a structured format that can be easily processed by other agents.`

	// Определяем параметры поиска на основе запроса пользователя через AI агента с retry
	searchQuery, err := w.buildGmailSearchQueryWithRetries(ctx, userQuery)
	if err != nil {
		return "", fmt.Errorf("failed to build Gmail search query: %w", err)
	}

	// Ищем email через Gmail MCP
	result := w.gmailClient.SearchEmails(ctx, searchQuery, 20, "today")
	if !result.Success {
		return "", fmt.Errorf("Gmail search failed: %s", result.Message)
	}

	// Генерируем запрос к LLM для анализа найденных писем
	var dataContent string
	if len(result.Emails) > 0 {
		dataContent = fmt.Sprintf("Gmail Search Query Used: %s\nFound %d emails:\n\n", searchQuery, len(result.Emails))
		for i, email := range result.Emails {
			importance := ""
			if email.IsImportant {
				importance = " [IMPORTANT]"
			}
			unread := ""
			if email.IsUnread {
				unread = " [UNREAD]"
			}
			dataContent += fmt.Sprintf("%d. From: %s%s%s\n", i+1, email.From, importance, unread)
			dataContent += fmt.Sprintf("   Subject: %s\n", email.Subject)
			dataContent += fmt.Sprintf("   Date: %s\n", email.Date.Format("2006-01-02 15:04"))
			dataContent += fmt.Sprintf("   Snippet: %s\n\n", email.Snippet)
		}
	} else {
		dataContent = fmt.Sprintf("Gmail Search Query Used: %s\nNo emails found for the specified query.", searchQuery)
	}

	// Запрос к LLM для анализа данных
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: dataContent},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate Gmail analysis: %w", err)
	}

	return response.Content, nil
}

// collectGmailDataWithRetries собирает данные из Gmail с retry механизмом при валидации
func (w *GmailSummaryWorkflow) collectGmailDataWithRetries(ctx context.Context, userQuery string) (string, error) {
	var lastData string
	var lastValidationResponse ValidationResponse

	for attempt := 1; attempt <= MaxRetryAttempts; attempt++ {
		log.Printf("📧 Gmail data collection attempt %d/%d", attempt, MaxRetryAttempts)

		// Собираем данные (возможно, с исправлениями)
		data, err := w.collectGmailDataAttempt(ctx, userQuery, lastValidationResponse.CorrectionRequest, attempt)
		if err != nil {
			return "", fmt.Errorf("failed to collect Gmail data on attempt %d: %w", attempt, err)
		}

		lastData = data

		// Валидируем собранные данные
		isValid, validationMessage, validationResp, err := w.validateGmailDataWithCorrection(ctx, data, userQuery)
		if err != nil {
			return "", fmt.Errorf("failed to validate Gmail data on attempt %d: %w", attempt, err)
		}

		lastValidationResponse = validationResp

		if isValid {
			log.Printf("✅ Gmail data validation successful on attempt %d", attempt)
			return data, nil
		}

		log.Printf("❌ Gmail data validation failed on attempt %d: %s", attempt, validationMessage)

		if attempt == MaxRetryAttempts {
			return "", fmt.Errorf("Gmail data validation failed after %d attempts: %s", MaxRetryAttempts, validationMessage)
		}

		if validationResp.CorrectionRequest != "" {
			log.Printf("🔄 Retry with correction: %s", validationResp.CorrectionRequest)
		}
	}

	return lastData, nil
}

// collectGmailDataAttempt собирает данные с учетом возможных исправлений
func (w *GmailSummaryWorkflow) collectGmailDataAttempt(ctx context.Context, userQuery, correctionRequest string, attempt int) (string, error) {
	log.Printf("📧 Gmail agent collecting data for query: %s (attempt %d)", userQuery, attempt)

	// Создаем системный промпт для Gmail агента с учетом исправлений
	systemPrompt := `You are a Gmail data collection agent. Your task is to search Gmail and collect relevant emails based on user queries.

User query: "` + userQuery + `"

Your tasks:
1. Analyze the user query to determine appropriate Gmail search parameters
2. Search Gmail for relevant emails using proper Gmail search operators
3. Extract and summarize the most important information
4. Focus on emails that match the user's specific intent

IMPORTANT - Gmail Search Context:
- The system has automatically built a Gmail search query based on the user request
- Different requests use different search operators (in:spam, in:inbox, is:unread, etc.)
- Your analysis should focus on the emails found by this targeted search
- If looking at spam folder (in:spam), focus on why emails might be there
- If looking at specific time periods, prioritize chronological relevance
- If looking at unread emails, emphasize their urgency and importance

CRITICAL - Spam Folder Analysis:
- **SPAM SEARCHES ARE EXPECTED TO BE EMPTY**: Most spam searches will return 0 emails
- **THIS IS NORMAL AND CORRECT**: Spam folder is designed to filter out unwanted emails
- **POSITIVE RESULT**: Finding no useful emails in spam means the spam filter is working properly
- **ANALYSIS APPROACH**: If spam folder is empty, explain this is the expected and desired outcome
- **USER EDUCATION**: Inform that empty spam results indicate good email filtering
- **NO FALSE ALARMS**: Do not report empty spam as a problem - it's a feature, not a bug

Return your findings in a structured format with:
- Email count and source (which folder/filter was used)
- Categorization by importance/urgency
- Key themes and patterns
- Actionable items or follow-up needed
- Any unusual patterns or concerns (especially for spam analysis)

IMPORTANT - Response Guidelines for Empty Results:
- **SPAM FOLDER**: If spam search returns 0 emails, report this as POSITIVE news - "Great! Your spam folder is clean, which means Gmail's spam filtering is working effectively."
- **OTHER FOLDERS**: Empty results in inbox/sent/drafts may indicate need for broader search criteria
- **ALWAYS INCLUDE**: Search query used and explanation of why results are empty
- **USER EDUCATION**: Help users understand when empty results are good vs when they need adjustment`

	if correctionRequest != "" && attempt > 1 {
		systemPrompt += fmt.Sprintf(`

IMPORTANT - CORRECTION NEEDED:
Previous attempt failed validation with this feedback: "%s"
Please improve your analysis and data collection based on this feedback. Focus on addressing the specific issues mentioned.`, correctionRequest)
	}

	// Определяем параметры поиска на основе запроса пользователя через AI агента с retry
	searchQuery, err := w.buildGmailSearchQueryWithRetries(ctx, userQuery)
	if err != nil {
		return "", fmt.Errorf("failed to build Gmail search query: %w", err)
	}

	// Ищем email через Gmail MCP
	result := w.gmailClient.SearchEmails(ctx, searchQuery, 20, "today")
	if !result.Success {
		return "", fmt.Errorf("Gmail search failed: %s", result.Message)
	}

	// Генерируем запрос к LLM для анализа найденных писем
	var dataContent string
	if len(result.Emails) > 0 {
		dataContent = fmt.Sprintf("Gmail Search Query Used: %s\nFound %d emails:\n\n", searchQuery, len(result.Emails))
		for i, email := range result.Emails {
			importance := ""
			if email.IsImportant {
				importance = " [IMPORTANT]"
			}
			unread := ""
			if email.IsUnread {
				unread = " [UNREAD]"
			}
			dataContent += fmt.Sprintf("%d. From: %s%s%s\n", i+1, email.From, importance, unread)
			dataContent += fmt.Sprintf("   Subject: %s\n", email.Subject)
			dataContent += fmt.Sprintf("   Date: %s\n", email.Date.Format("2006-01-02 15:04"))
			dataContent += fmt.Sprintf("   Snippet: %s\n\n", email.Snippet)
		}
	} else {
		dataContent = fmt.Sprintf("Gmail Search Query Used: %s\nNo emails found for the specified query.", searchQuery)
	}

	// Запрос к LLM для анализа данных
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: dataContent},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate Gmail analysis: %w", err)
	}

	return response.Content, nil
}

// validateGmailData валидирует собранные Gmail данные
func (w *GmailSummaryWorkflow) validateGmailData(ctx context.Context, gmailData, originalQuery string) (bool, string, error) {
	log.Printf("🔍 Validating Gmail data")

	systemPrompt := `You are a validation agent. Your task is to validate whether the collected Gmail data adequately addresses the user's original query.

Original user query: "` + originalQuery + `"

Evaluate the Gmail data and respond with JSON in this exact format:
{
  "is_valid": true/false, // boolean value
  "message": "explanation of validation result",
  "suggested_action": "what to do if validation fails (optional)"
}

Validation criteria:
- Does the data contain relevant emails for the user's query?
- Is there enough information to create a meaningful summary?
- Are important/unread emails properly identified?`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Gmail data to validate:\n\n" + gmailData},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return false, "", fmt.Errorf("failed to validate Gmail data: %w", err)
	}

	var validation ValidationResponse
	if err := json.Unmarshal([]byte(response.Content), &validation); err != nil {
		// Если не удалось распарсить JSON, считаем данные валидными
		log.Printf("⚠️ Failed to parse validation response: %v", err)
		log.Printf("📄 Response content: %s", response.Content)
		return false, "", fmt.Errorf("could not parse JSON string. Input string: %s: %v", response.Content, err)
	}

	return validation.IsValid, validation.Message, nil
}

// validateGmailDataWithCorrection валидирует данные с возможностью корректирующих запросов
func (w *GmailSummaryWorkflow) validateGmailDataWithCorrection(ctx context.Context, gmailData, originalQuery string) (bool, string, ValidationResponse, error) {
	log.Printf("🔍 Validating Gmail data with correction support")

	systemPrompt := `You are a validation agent. Your task is to validate whether the collected Gmail data adequately addresses the user's original query.

Original user query: "` + originalQuery + `"

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "is_valid": true,
  "message": "explanation of validation result",
  "suggested_action": "what to do if validation fails (optional)",
  "correction_request": "specific instructions for improving the data collection (if validation fails)",
  "specific_issues": "detailed list of problems found as a single string (if validation fails)"
}

IMPORTANT JSON RULES:
- All field values must be strings (not arrays or objects)
- Use true or false for is_valid (boolean)
- If specific_issues is needed, write it as a single string with semicolon separation
- Do not use arrays or nested objects
- Do not wrap response in markdown code blocks

Validation criteria:
- Does the data contain relevant emails for the user's query?
- Is there enough information to create a meaningful summary?
- Are important/unread emails properly identified?
- Is the analysis structured and comprehensive?
- Are the most relevant emails highlighted?

CRITICAL - Folder-Specific Validation Rules:
- **SPAM FOLDER (in:spam)**: It is NORMAL and EXPECTED to find few or NO useful emails in spam
  * Empty spam results are VALID - spam folder is designed to contain unwanted emails
  * Agent should acknowledge spam search was performed correctly
  * NO correction needed if spam folder contains no useful emails
  * Focus on whether spam analysis explains why emails are there (if any)
- **INBOX/SENT/DRAFTS**: These should contain relevant emails for normal queries
- **UNREAD/IMPORTANT**: These filters should return focused, actionable results
- **TIME-FILTERED**: Results should match the requested time period exactly

CRITICAL - Time Period Verification (MOST IMPORTANT CHECK):
- ALWAYS verify if the Gmail search query used matches the EXACT requested time period
- NUMERIC DAY PARSING: If user asked for any number + "дня/дней/days", verify EXACT match:
  * "за последние 3 дня" (last 3 days) → search MUST use "newer_than:3d" (NEVER "newer_than:1d")
  * "за последние 2 дня" → search MUST use "newer_than:2d" (NEVER "newer_than:1d") 
  * "last 5 days" → search MUST use "newer_than:5d" (NEVER "newer_than:1d")
  * "последние 7 дней" → search MUST use "newer_than:7d" (NEVER "newer_than:1d")
- If user asked for "несколько дней" (several days), verify appropriate multi-day period (3d+) was used
- REJECT as invalid if any numeric time period was converted to wrong number
- Flag ANY mismatch between requested time period and actual search parameters as VALIDATION FAILURE
- This is the MOST CRITICAL validation - time period errors are unacceptable

If validation fails, provide specific correction_request with detailed instructions on how to improve:
- What type of information is missing
- How to better analyze the emails
- What aspects need more focus
- Specific formatting or structure improvements needed
- Correct time period parameters if temporal parsing was wrong`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Gmail data to validate:\n\n" + gmailData},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return false, "", ValidationResponse{}, fmt.Errorf("failed to validate Gmail data: %w", err)
	}

	var validation ValidationResponse
	if err := json.Unmarshal([]byte(response.Content), &validation); err != nil {
		// Если не удалось распарсить JSON, это ошибка валидации
		log.Printf("⚠️ Failed to parse validation response as JSON: %v", err)
		log.Printf("📄 Response content: %s", response.Content)
		return false, "Invalid JSON response format from validation agent", ValidationResponse{
			IsValid:           false,
			Message:           "Invalid JSON response format from validation agent",
			CorrectionRequest: "Please respond with valid JSON in the exact format specified in the system prompt",
			SpecificIssues:    fmt.Sprintf("JSON parsing error: %v. Response content: %s", err, response.Content),
		}, nil
	}

	return validation.IsValid, validation.Message, validation, nil
}

// ensureGmailSummariesPage находит или создает страницу "Gmail summaries"
func (w *GmailSummaryWorkflow) ensureGmailSummariesPage(ctx context.Context) (string, error) {
	log.Printf("📋 Ensuring Gmail summaries page exists")

	// Сначала ищем существующую страницу
	searchResult := w.notionClient.SearchPagesWithID(ctx, "Gmail summaries", 5, true)
	if searchResult.Success && len(searchResult.Pages) > 0 {
		log.Printf("✅ Found existing Gmail summaries page: %s", searchResult.Pages[0].ID)
		return searchResult.Pages[0].ID, nil
	}

	// Если не найдено, получаем список доступных страниц для создания родительской
	availablePages := w.notionClient.ListAvailablePages(ctx, 10, "", true)
	if !availablePages.Success || len(availablePages.Pages) == 0 {
		return "", fmt.Errorf("no available parent pages found")
	}

	// Берем первую доступную страницу как родительскую
	parentPageID := availablePages.Pages[0].ID

	// Создаем страницу "Gmail summaries"
	createResult := w.notionClient.CreateFreeFormPage(ctx, "Gmail summaries",
		"This page contains Gmail email summaries generated automatically.",
		parentPageID, []string{"gmail", "summaries"})

	if !createResult.Success {
		return "", fmt.Errorf("failed to create Gmail summaries page: %s", createResult.Message)
	}

	log.Printf("✅ Created new Gmail summaries page: %s", createResult.PageID)
	return createResult.PageID, nil
}

// validateSummary валидирует созданное саммари
func (w *GmailSummaryWorkflow) validateSummary(ctx context.Context, title, content string) (bool, string, error) {
	log.Printf("🔍 Validating generated summary")

	systemPrompt := `You are a summary validation agent. Validate the quality and completeness of the generated Gmail summary.

Validation criteria:
- Is the title descriptive and appropriate?
- Is the content well-structured and informative?
- Does it properly highlight important information?
- Is the markdown formatting correct?

Return JSON:
{
  "is_valid": true/false, // boolean value
  "message": "validation feedback",
  "suggested_action": "improvements needed if invalid"
}`

	summaryData := fmt.Sprintf("Title: %s\n\nContent:\n%s", title, content)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: summaryData},
	}

	response, err := w.notionAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return false, "", fmt.Errorf("failed to validate summary: %w", err)
	}

	var validation ValidationResponse
	if err := json.Unmarshal([]byte(response.Content), &validation); err != nil {
		// Если не удалось распарсить JSON, считаем саммари валидным
		log.Printf("⚠️ Failed to parse validation response, assuming valid: %v", err)
		log.Printf("📄 Response content: %s", response.Content)
		return false, "Validation response could not be parsed, assuming valid", fmt.Errorf("could not parse JSON string: %v", err)
	}

	return validation.IsValid, validation.Message, nil
}

// generateSummaryWithRetries генерирует summary с retry механизмом при валидации
func (w *GmailSummaryWorkflow) generateSummaryWithRetries(ctx context.Context, gmailData, userQuery string) (string, string, error) {
	var lastTitle, lastContent string
	var lastValidationResponse ValidationResponse

	for attempt := 1; attempt <= MaxRetryAttempts; attempt++ {
		log.Printf("📝 Summary generation attempt %d/%d", attempt, MaxRetryAttempts)

		// Генерируем summary (возможно, с исправлениями)
		title, content, err := w.generateSummaryAttempt(ctx, gmailData, userQuery, lastValidationResponse.CorrectionRequest, attempt)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate summary on attempt %d: %w", attempt, err)
		}

		lastTitle, lastContent = title, content

		// Валидируем summary
		isValid, validationMessage, validationResp, err := w.validateSummaryWithCorrection(ctx, title, content)
		if err != nil {
			return "", "", fmt.Errorf("failed to validate summary on attempt %d: %w", attempt, err)
		}

		lastValidationResponse = validationResp

		if isValid {
			log.Printf("✅ Summary validation successful on attempt %d", attempt)
			return title, content, nil
		}

		log.Printf("❌ Summary validation failed on attempt %d: %s", attempt, validationMessage)

		if attempt == MaxRetryAttempts {
			return "", "", fmt.Errorf("summary validation failed after %d attempts: %s", MaxRetryAttempts, validationMessage)
		}

		if validationResp.CorrectionRequest != "" {
			log.Printf("🔄 Retry with correction: %s", validationResp.CorrectionRequest)
		}
	}

	return lastTitle, lastContent, nil
}

// generateSummaryAttempt генерирует summary с учетом возможных исправлений
func (w *GmailSummaryWorkflow) generateSummaryAttempt(ctx context.Context, gmailData, userQuery, correctionRequest string, attempt int) (string, string, error) {
	log.Printf("📝 Generating summary for Gmail data (attempt %d)", attempt)

	systemPrompt := `You are a summary generation agent. Create a concise and informative summary of Gmail emails based on the user's query and collected data.

User query: "` + userQuery + `"

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

CRITICAL – RESPONSE LANGUAGE:
You MUST use user's original language for generated summary.

{
  "title": "Summary title",
  "content": "Markdown formatted summary content"
}

IMPORTANT JSON RULES:
- All field values must be strings (not arrays or objects)
- Do not use arrays or nested objects
- Do not wrap response in markdown code blocks
- Make sure all quotes and newlines in content are properly escaped for JSON
- Use \\n for line breaks in the content field

Your task:
1. Generate a descriptive title for this summary
2. Create a well-structured summary in markdown format
3. Highlight important/unread emails
4. Group related emails if applicable
5. Include actionable insights or follow-up suggestions

JSON CONTENT FORMATTING:
- Use \\n for line breaks in markdown content
- Escape quotes properly: \\"
- Keep markdown formatting simple (headers, lists, bold)
- Ensure the content field is a single valid JSON string`

	if correctionRequest != "" && attempt > 1 {
		systemPrompt += fmt.Sprintf(`

IMPORTANT - CORRECTION NEEDED:
Previous attempt failed validation with this feedback: "%s"
Please improve your summary generation based on this feedback. Focus on addressing the specific issues mentioned.`, correctionRequest)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Gmail data to summarize:\n\n" + gmailData},
	}

	response, err := w.notionAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate summary: %w", err)
	}

	var summaryResult struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := json.Unmarshal([]byte(response.Content), &summaryResult); err != nil {
		// Если не удалось распарсить JSON, возвращаем специальный маркер для валидации
		log.Printf("⚠️ Failed to parse summary response as JSON: %v", err)
		log.Printf("📄 Response content: %s", response.Content)
		// Возвращаем специальные значения, которые валидатор распознает как JSON ошибку
		return "INVALID_JSON_ERROR", "INVALID_JSON_ERROR: " + response.Content, nil
	}

	// Добавляем дату к заголовку от AI
	titleWithDate := fmt.Sprintf("%s: %s", summaryResult.Title, time.Now().Format("02/01/2006"))
	return titleWithDate, summaryResult.Content, nil
}

// validateSummaryWithCorrection валидирует summary с возможностью корректирующих запросов
func (w *GmailSummaryWorkflow) validateSummaryWithCorrection(ctx context.Context, title, content string) (bool, string, ValidationResponse, error) {
	log.Printf("🔍 Validating generated summary with correction support")

	// Проверяем на JSON ошибки до отправки в LLM
	if title == "INVALID_JSON_ERROR" {
		log.Printf("📋 Detected JSON parsing error, creating correction request")
		return false, "Invalid JSON response format from summary generation", ValidationResponse{
			IsValid:           false,
			Message:           "Summary generation returned invalid JSON format",
			CorrectionRequest: "Please respond with valid JSON in the exact format specified: {\"title\": \"Summary title\", \"content\": \"Markdown content\"}. Do not include markdown code blocks (```json) around your response. Return only the raw JSON object.",
			SpecificIssues:    "JSON parsing failed. The response must be a valid JSON object with 'title' and 'content' fields.",
		}, nil
	}

	systemPrompt := `You are a summary validation agent. Validate the quality and completeness of the generated Gmail summary.

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "is_valid": true, // strictly boolean
  "message": "validation feedback",
  "suggested_action": "improvements needed if invalid",
  "correction_request": "specific instructions for improving the summary (if validation fails)",
  "specific_issues": "detailed list of problems found as a single string (if validation fails)"
}

IMPORTANT JSON RULES:
- All field values must be strings (not arrays or objects)
- Use true or false for is_valid (boolean)
- If specific_issues is needed, write it as a single string with semicolon separation
- Do not use arrays or nested objects
- Do not wrap response in markdown code blocks

Validation criteria:
- Is the title descriptive and appropriate?
- Is the content well-structured and informative?
- Does it properly highlight important information?
- Is the markdown formatting correct?
- Is the summary comprehensive and actionable?
- Are key insights clearly presented?
- Is user's original language is used for the summary?

If validation fails, provide specific correction_request with detailed instructions on how to improve:
- What content is missing or inadequate
- How to improve structure and formatting
- What insights need to be added or clarified
- Specific markdown formatting issues to fix`

	summaryData := fmt.Sprintf("Title: %s\n\nContent:\n%s", title, content)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: summaryData},
	}

	response, err := w.notionAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return false, "", ValidationResponse{}, fmt.Errorf("failed to validate summary: %w", err)
	}

	var validation ValidationResponse
	if err := json.Unmarshal([]byte(response.Content), &validation); err != nil {
		// Если не удалось распарсить JSON, это ошибка валидации
		log.Printf("⚠️ Failed to parse validation response as JSON: %v", err)
		log.Printf("📄️ Response content: %s", response.Content)
		return false, "Invalid JSON response format from validation agent", ValidationResponse{
			IsValid:           false,
			Message:           "Invalid JSON response format from validation agent",
			CorrectionRequest: "Please respond with valid JSON in the exact format specified in the system prompt",
			SpecificIssues:    fmt.Sprintf("JSON parsing error: %v. Response content: %s", err, response.Content),
		}, nil
	}

	return validation.IsValid, validation.Message, validation, nil
}

// createNotionPage создает страницу саммари в Notion
func (w *GmailSummaryWorkflow) createNotionPage(ctx context.Context, parentPageID, title, content string) (string, error) {
	log.Printf("📄 Creating Notion page: %s", title)

	result := w.notionClient.CreateFreeFormPage(ctx, title, content, parentPageID, []string{"gmail", "summary", "auto-generated"})
	if !result.Success {
		return "", fmt.Errorf("failed to create Notion page: %s", result.Message)
	}

	// Формируем URL страницы
	pageURL := fmt.Sprintf("https://www.notion.so/%s", result.PageID)
	log.Printf("✅ Created Notion page: %s", pageURL)

	return pageURL, nil
}

// GmailSearchQueryResponse представляет ответ агента для создания поискового запроса
type GmailSearchQueryResponse struct {
	Query       string `json:"query"`
	Explanation string `json:"explanation"`
	Reasoning   string `json:"reasoning"`
}

// buildGmailSearchQuery создает поисковый запрос для Gmail через AI агента
func (w *GmailSummaryWorkflow) buildGmailSearchQuery(ctx context.Context, userQuery string) (string, error) {
	log.Printf("🤖 AI agent building Gmail search query for: %s", userQuery)

	systemPrompt := `You are a Gmail search query generation agent. Your task is to convert user requests into precise Gmail search operators.

User query: "` + userQuery + `"

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "query": "Gmail search operators string",
  "explanation": "Brief explanation of what the query searches for",
  "reasoning": "Why these specific operators were chosen"
}

IMPORTANT JSON RULES:
- All field values must be strings (not arrays or objects)
- Do not use arrays or nested objects
- Do not wrap response in markdown code blocks
- Make sure all quotes are properly escaped in JSON

Your tasks:
1. Analyze the user query to understand their intent
2. Generate appropriate Gmail search operators
3. Handle time periods, folders, email status, and other criteria accurately

Available Gmail search operators:
- Folders: in:inbox, in:sent, in:drafts, in:spam, in:trash
- Time: newer_than:Xd (X days), older_than:Xd, newer_than:Xm (X months)
- Status: is:unread, is:read, is:important, is:starred
- Combinations: Use spaces for AND, OR for alternatives, parentheses for grouping

CRITICAL - Time Period Handling (EXACT PARSING REQUIRED):
- "last 3 days" / "последние 3 дня" = "newer_than:3d" (NEVER use 1d)
- "last 2 days" / "последние 2 дня" = "newer_than:2d" (NEVER use 1d)
- "за последние 5 дней" / "последние 5 дней" = "newer_than:5d" (NEVER use 1d)
- "last 3 days" / "last 5 days" = "newer_than:3d" / "newer_than:5d"
- "сегодня/today" ONLY = "newer_than:1d"
- "вчера/yesterday" = "older_than:1d newer_than:2d"
- "неделя/week" = "newer_than:7d"
- "месяц/month" = "newer_than:30d"
- "несколько дней" / "пару дней" = "newer_than:3d" (default 3 days for vague terms)
- PARSE NUMERIC VALUES: Extract any number mentioned (1,2,3,4,5,etc) and use newer_than:Nd
- DOUBLE CHECK: If query contains any number + "дня/дней/days", use that exact number for newer_than:Nd

Languages: Support both Russian and English queries equally well.

Examples:
- "за последние 3 дня" → {"query": "(is:unread OR is:important) newer_than:3d", ...}
- "spam folder" → {"query": "in:spam", ...}
- "important unread emails today" → {"query": "is:important is:unread newer_than:1d", ...}`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Generate Gmail search query for: \"%s\"", userQuery)},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate Gmail search query: %w", err)
	}

	var queryResp GmailSearchQueryResponse
	if err := json.Unmarshal([]byte(response.Content), &queryResp); err != nil {
		log.Printf("⚠️ Failed to parse search query response as JSON: %v", err)
		log.Printf("📄 Response content: %s", response.Content)
		// Возвращаем ошибку, чтобы вызвать retry на более высоком уровне
		return "", fmt.Errorf("invalid JSON response format from search query generation: %v", err)
	}

	log.Printf("🔍 AI generated Gmail query: %s", queryResp.Query)
	log.Printf("📝 Explanation: %s", queryResp.Explanation)
	log.Printf("🧠 Reasoning: %s", queryResp.Reasoning)

	// Валидируем сгенерированный запрос
	if isValid, suggestion := w.validateGmailSearchQuery(ctx, queryResp.Query, userQuery); !isValid {
		log.Printf("⚠️ Generated query validation failed: %s", suggestion)
		// Используем fallback запрос при неудачной валидации
		fallbackQuery := "(is:unread OR is:important) newer_than:7d"
		log.Printf("🔄 Using fallback query: %s", fallbackQuery)
		return fallbackQuery, nil
	}

	return queryResp.Query, nil
}

// buildGmailSearchQueryWithRetries создает поисковый запрос с retry механизмом для JSON ошибок
func (w *GmailSummaryWorkflow) buildGmailSearchQueryWithRetries(ctx context.Context, userQuery string) (string, error) {
	for attempt := 1; attempt <= MaxRetryAttempts; attempt++ {
		log.Printf("🔍 Gmail search query generation attempt %d/%d", attempt, MaxRetryAttempts)

		query, err := w.buildGmailSearchQuery(ctx, userQuery)
		if err != nil {
			log.Printf("❌ Gmail search query generation failed on attempt %d: %v", attempt, err)

			if attempt == MaxRetryAttempts {
				// После всех попыток используем fallback
				fallbackQuery := "(is:unread OR is:important) newer_than:7d"
				log.Printf("🔄 Using fallback query after %d attempts: %s", MaxRetryAttempts, fallbackQuery)
				return fallbackQuery, nil
			}

			// Продолжаем попытки
			continue
		}

		// Успешно сгенерирован запрос
		log.Printf("✅ Gmail search query generation successful on attempt %d", attempt)
		return query, nil
	}

	// Этот код никогда не должен выполниться, но для безопасности
	fallbackQuery := "(is:unread OR is:important) newer_than:7d"
	log.Printf("🔄 Using fallback query as final resort: %s", fallbackQuery)
	return fallbackQuery, nil
}

// validateGmailSearchQuery валидирует сгенерированный Gmail поисковый запрос
func (w *GmailSummaryWorkflow) validateGmailSearchQuery(ctx context.Context, generatedQuery, originalUserQuery string) (bool, string) {
	log.Printf("🔍 Validating generated Gmail search query")

	systemPrompt := `You are a Gmail search query validation agent. Your task is to validate whether a generated Gmail search query accurately represents the user's original request.

Original user query: "` + originalUserQuery + `"
Generated Gmail query: "` + generatedQuery + `"

CRITICAL - RESPONSE FORMAT:
You MUST respond with valid JSON in this EXACT format. Do NOT include markdown code blocks. Return ONLY the raw JSON:

{
  "is_valid": true,
  "message": "Brief explanation of validation result",
  "suggested_action": "What should be corrected if validation fails"
}

IMPORTANT JSON RULES:
- All field values must be strings (not arrays or objects)
- Use true or false for is_valid (boolean)
- Do not use arrays or nested objects
- Do not wrap response in markdown code blocks

Validation criteria:
1. Time Period Accuracy (CRITICAL - MOST COMMON ERROR):
   - NUMERIC DAYS: If user asked for any number + "дня/дней/days" (2,3,4,5,etc), the query MUST use that EXACT number
   - "за последние 3 дня" → MUST contain "newer_than:3d" (NEVER "newer_than:1d")
   - "за последние 2 дня" → MUST contain "newer_than:2d" (NEVER "newer_than:1d")
   - "last 5 days" → MUST contain "newer_than:5d" (NEVER "newer_than:1d")
   - "последние 7 дней" → MUST contain "newer_than:7d" (NEVER "newer_than:1d")
   - "сегодня/today" (WITHOUT numbers) → MUST contain "newer_than:1d"
   - "месяц/month" → MUST contain "newer_than:30d"
   - REJECT as invalid if numeric time period is converted to wrong number

2. Folder Accuracy:
   - If user mentioned "спам/spam", query should contain "in:spam"
   - If user mentioned "отправленные/sent", query should contain "in:sent"
   - If user mentioned "черновики/drafts", query should contain "in:drafts"

3. Status Accuracy:
   - If user asked for "непрочитанные/unread", query should contain "is:unread"
   - If user asked for "важные/important", query should contain "is:important"

4. Spam Folder Context:
   - **SPAM QUERIES ARE ALWAYS VALID**: Searches in spam folder (in:spam) are always appropriate
   - **EMPTY SPAM IS CORRECT**: No results from spam folder is the expected and desired outcome
   - **DO NOT FLAG SPAM SEARCHES**: Never mark spam folder queries as invalid due to empty results
   - **SPAM = SUCCESS**: Empty spam means email filtering is working properly

5. Query Syntax:
   - Valid Gmail search operators
   - Proper use of parentheses and logical operators
   - No syntax errors `

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Validate this query mapping:\nUser: \"%s\"\nGenerated: \"%s\"", originalUserQuery, generatedQuery)},
	}

	response, err := w.gmailAgent.llmClient.Generate(ctx, messages)
	if err != nil {
		log.Printf("⚠️ Failed to validate Gmail search query: %v", err)
		return true, "" // При ошибке валидации считаем запрос валидным
	}

	var validation ValidationResponse
	if err := json.Unmarshal([]byte(response.Content), &validation); err != nil {
		log.Printf("⚠️ Failed to parse query validation response as JSON: %v", err)
		log.Printf("📄️ Response content: %s", response.Content)
		return false, fmt.Sprintf("invalid JSON response format from search query generation: %v", err)
	}

	if !validation.IsValid {
		log.Printf("❌ Query validation failed: %s", validation.Message)
		return false, validation.SuggestedAction
	}

	log.Printf("✅ Query validation successful: %s", validation.Message)
	return true, ""
}
