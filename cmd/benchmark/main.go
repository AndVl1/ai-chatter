package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// OpenRouterRequest структура запроса к OpenRouter API
type OpenRouterRequest struct {
	Model       string                 `json:"model"`
	Messages    []OpenRouterMessage    `json:"messages"`
	Temperature *float32               `json:"temperature,omitempty"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// OpenRouterMessage сообщение для OpenRouter API
type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenRouterResponse ответ от OpenRouter API
type OpenRouterResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		//PromptCost       float64 `json:"prompt_cost"`
		//CompletionCost   float64 `json:"completion_cost"`
		//TotalCost        float64 `json:"total_cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// OpenRouterGenerationResponse ответ от OpenRouter Generation API для получения детальной стоимости
type OpenRouterGenerationResponse struct {
	Data struct {
		ID                     string  `json:"id"`
		Model                  string  `json:"model"`
		CreatedAt              string  `json:"created_at"`
		TokensPrompt           int     `json:"tokens_prompt"`
		TokensCompletion       int     `json:"tokens_completion"`
		NativeTokensPrompt     int     `json:"native_tokens_prompt"`
		NativeTokensCompletion int     `json:"native_tokens_completion"`
		Usage                  float64 `json:"usage"`
		TotalCost              float64 `json:"total_cost"`
		Cancelled              bool    `json:"cancelled"`
		FinishReason           string  `json:"finish_reason"`
		GenerationTime         int     `json:"generation_time"`
		Latency                int     `json:"latency"`
	} `json:"data"`
}

// DetailedCostInfo подробная информация о стоимости запроса
type DetailedCostInfo struct {
	RequestID        string  `json:"request_id"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	PromptCost       float64 `json:"prompt_cost"`
	CompletionCost   float64 `json:"completion_cost"`
	TotalCost        float64 `json:"total_cost"`
	PromptPrice      float64 `json:"prompt_price"`     // Цена за 1K промпт токенов
	CompletionPrice  float64 `json:"completion_price"` // Цена за 1K completion токенов
	NativeCost       float64 `json:"native_cost"`      // Стоимость в нативной валюте модели
}

// BenchmarkResult результат одного теста
type BenchmarkResult struct {
	Parameter        string            `json:"parameter"`
	Value            interface{}       `json:"value"`
	Response         string            `json:"response"`
	Tokens           int               `json:"tokens"`
	Cost             float64           `json:"cost"`
	Duration         time.Duration     `json:"duration"`
	ResponseLength   int               `json:"response_length"`
	DetailedCostInfo *DetailedCostInfo `json:"detailed_cost_info,omitempty"` // Детальная стоимость от Generation API
}

// AnalysisResult результат анализа от LLM
type AnalysisResult struct {
	QualityScore   float64     `json:"quality_score"`
	CostEfficiency float64     `json:"cost_efficiency"`
	ResponseTime   float64     `json:"response_time"`
	Analysis       string      `json:"analysis"`
	Recommendation string      `json:"recommendation"`
	BestValue      interface{} `json:"best_value"`
}

// ParallelTestResult результат параллельного выполнения теста
type ParallelTestResult struct {
	Result BenchmarkResult
	Error  error
}

const (
	// Модель для тестирования (можно изменить)
	TEST_MODEL = "qwen/qwen3-coder"

	// Базовый URL OpenRouter
	OPENROUTER_BASE_URL = "https://openrouter.ai/api/v1/chat/completions"

	// URL для получения детальной информации о стоимости запроса
	OPENROUTER_GENERATION_URL = "https://openrouter.ai/api/v1/generation"
)

var (
	// Параметры для тестирования температуры
	temperatureValues = []float32{0.0, 0.3, 0.7, 1.0, 1.2}

	// Параметры для тестирования max_tokens
	maxTokensValues = []int{100, 500, 1000, 2500}

	// HTTP клиент с увеличенным таймаутом
	httpClient = &http.Client{Timeout: 120 * time.Second}

	// API ключ OpenRouter
	apiKey string
)

func main() {
	log.Printf("🚀 Starting LLM Benchmark Tool")

	// Загружаем переменные окружения
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Получаем API ключ
	apiKey = os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatalf("❌ OPENAI_API_KEY not set in environment")
	}

	log.Printf("🔑 Using OpenRouter API Key: %s...%s", apiKey[:8], apiKey[len(apiKey)-8:])
	log.Printf("🎯 Testing model: %s", TEST_MODEL)

	ctx := context.Background()

	// Шаг 1: Генерируем тестовый вопрос
	log.Printf("\n🧠 Step 1: Generating test question...")
	testQuestion, err := generateTestQuestion(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to generate test question: %v", err)
	}

	log.Printf("✅ Generated test question: %s", testQuestion)

	// Шаг 2: Тестируем разные значения температуры параллельно
	log.Printf("\n🌡️ Step 2: Testing different temperature values in parallel...")
	totalStart := time.Now()
	temperatureResults, err := benchmarkTemperature(ctx, testQuestion)
	totalTempDuration := time.Since(totalStart)
	if err != nil {
		log.Fatalf("❌ Temperature benchmark failed: %v", err)
	}
	log.Printf("⏱️ Total temperature tests completed in: %v", totalTempDuration)

	// Анализируем результаты температуры
	log.Printf("\n📊 Analyzing temperature results...")
	tempAnalysis, err := analyzeResults(ctx, "temperature", temperatureResults, testQuestion)
	if err != nil {
		log.Printf("⚠️ Temperature analysis failed: %v", err)
	} else {
		printAnalysis("Temperature", tempAnalysis)
	}

	// Шаг 3: Тестируем разные значения max_tokens параллельно
	log.Printf("\n🔢 Step 3: Testing different max_tokens values in parallel...")
	totalStart2 := time.Now()
	maxTokensResults, err := benchmarkMaxTokens(ctx, testQuestion)
	totalTokensDuration := time.Since(totalStart2)
	if err != nil {
		log.Fatalf("❌ MaxTokens benchmark failed: %v", err)
	}
	log.Printf("⏱️ Total max_tokens tests completed in: %v", totalTokensDuration)

	// Анализируем результаты max_tokens
	log.Printf("\n📊 Analyzing max_tokens results...")
	tokensAnalysis, err := analyzeResults(ctx, "max_tokens", maxTokensResults, testQuestion)
	if err != nil {
		log.Printf("⚠️ MaxTokens analysis failed: %v", err)
	} else {
		printAnalysis("Max Tokens", tokensAnalysis)
	}

	// Выводим итоговую статистику
	log.Printf("\n📈 ИТОГОВАЯ СТАТИСТИКА")
	log.Printf("═══════════════════════")
	printSummaryStats("Temperature", temperatureResults)
	printSummaryStats("Max Tokens", maxTokensResults)

	log.Printf("\n🎉 Benchmark completed successfully!")
}

// generateTestQuestion генерирует тестовый вопрос с помощью LLM
func generateTestQuestion(ctx context.Context) (string, error) {
	prompt := `Сгенерируй интересный тестовый вопрос для бенчмарка LLM модели. 
Вопрос должен быть:
1. Достаточно сложным, чтобы показать разницу в качестве ответов при разных параметрах
2. Не слишком специфичным (доступным для общей модели)
3. Требующим развернутого ответа (но не более 5000 токенов)
4. Позволяющим оценить креативность, логику и полноту ответа

Пример хорошего вопроса: "Объясни, как искусственный интеллект может изменить образование в ближайшие 10 лет, какие будут преимущества и риски?"

Ответь только одним вопросом, без дополнительных объяснений.`

	request := OpenRouterRequest{
		Model: TEST_MODEL,
		Messages: []OpenRouterMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: float32Ptr(0.7),
		MaxTokens:   intPtr(200),
	}

	response, err := makeOpenRouterRequest(ctx, request)
	if err != nil {
		return "", fmt.Errorf("failed to generate test question: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices received")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// benchmarkTemperature тестирует разные значения температуры параллельно
func benchmarkTemperature(ctx context.Context, testQuestion string) ([]BenchmarkResult, error) {
	log.Printf("  🚀 Starting %d parallel temperature tests...", len(temperatureValues))

	// Канал для сбора результатов
	resultsChan := make(chan ParallelTestResult, len(temperatureValues))
	var wg sync.WaitGroup

	// Запускаем параллельные тесты
	for i, temp := range temperatureValues {
		wg.Add(1)
		go func(temperature float32, index int) {
			defer wg.Done()

			// Добавляем небольшую задержку между стартами для уменьшения нагрузки на API
			time.Sleep(time.Duration(index) * 300 * time.Millisecond)

			log.Printf("    🌡️ Starting temperature test: %.1f", temperature)

			start := time.Now()

			request := OpenRouterRequest{
				Model: TEST_MODEL,
				Messages: []OpenRouterMessage{
					{Role: "user", Content: testQuestion},
				},
				Temperature: float32Ptr(temperature),
				MaxTokens:   intPtr(5000), // Фиксированный max_tokens для теста температуры
			}

			// Создаем контекст с таймаутом для каждого запроса
			requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()

			response, err := makeOpenRouterRequest(requestCtx, request)
			duration := time.Since(start)

			if err != nil {
				log.Printf("    ❌ Failed temperature %.1f: %v", temperature, err)
				resultsChan <- ParallelTestResult{Error: fmt.Errorf("temperature %.1f failed: %w", temperature, err)}
				return
			}

			if len(response.Choices) == 0 {
				log.Printf("    ❌ No response for temperature %.1f", temperature)
				resultsChan <- ParallelTestResult{Error: fmt.Errorf("no response for temperature %.1f", temperature)}
				return
			}

			// Получаем детальную информацию о стоимости
			var detailedCost *DetailedCostInfo
			if response.ID != "" {
				if cost, err := getDetailedCostInfo(ctx, response.ID); err != nil {
					log.Printf("    ⚠️ Failed to get detailed cost for temperature %.1f: %v", temperature, err)
				} else {
					detailedCost = cost
				}
			}

			result := BenchmarkResult{
				Parameter:        "temperature",
				Value:            temperature,
				Response:         response.Choices[0].Message.Content,
				Tokens:           response.Usage.TotalTokens,
				Cost:             detailedCost.TotalCost,
				Duration:         duration,
				ResponseLength:   len(response.Choices[0].Message.Content),
				DetailedCostInfo: detailedCost,
			}

			// Логируем базовую информацию
			log.Printf("    ✅ Temperature %.1f completed: %d tokens, $%.6f, %v, %d chars",
				temperature, result.Tokens, result.Cost, duration, result.ResponseLength)

			// Логируем детальную стоимость, если доступна
			if detailedCost != nil {
				log.Printf("    💰 Detailed cost: Prompt $%.6f (%d tok), Completion $%.6f (%d tok), Total $%.6f",
					detailedCost.PromptCost, detailedCost.PromptTokens,
					detailedCost.CompletionCost, detailedCost.CompletionTokens,
					detailedCost.TotalCost)
				log.Printf("    📊 Pricing: $%.6f/1K prompt, $%.6f/1K completion",
					detailedCost.PromptPrice, detailedCost.CompletionPrice)
			}

			resultsChan <- ParallelTestResult{Result: result, Error: nil}

		}(temp, i)
	}

	// Ждем завершения всех goroutines
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Собираем результаты
	var results []BenchmarkResult
	var errors []error

	for testResult := range resultsChan {
		if testResult.Error != nil {
			errors = append(errors, testResult.Error)
		} else {
			results = append(results, testResult.Result)
		}
	}

	log.Printf("  ✅ Temperature tests completed: %d successful, %d failed", len(results), len(errors))

	if len(results) == 0 {
		return nil, fmt.Errorf("all temperature tests failed: %v", errors)
	}

	// Сортируем результаты по значению температуры для красивого вывода
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Value.(float32) > results[j].Value.(float32) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// benchmarkMaxTokens тестирует разные значения max_tokens параллельно
func benchmarkMaxTokens(ctx context.Context, testQuestion string) ([]BenchmarkResult, error) {
	log.Printf("  🚀 Starting %d parallel max_tokens tests...", len(maxTokensValues))

	// Канал для сбора результатов
	resultsChan := make(chan ParallelTestResult, len(maxTokensValues))
	var wg sync.WaitGroup

	// Запускаем параллельные тесты
	for i, maxTokens := range maxTokensValues {
		wg.Add(1)
		go func(maxTokensValue int, index int) {
			defer wg.Done()

			// Добавляем небольшую задержку между стартами
			time.Sleep(time.Duration(index) * 400 * time.Millisecond)

			log.Printf("    🔢 Starting max_tokens test: %d", maxTokensValue)

			start := time.Now()

			request := OpenRouterRequest{
				Model: TEST_MODEL,
				Messages: []OpenRouterMessage{
					{Role: "user", Content: testQuestion},
				},
				Temperature: float32Ptr(0.7), // Фиксированная температура для теста max_tokens
				MaxTokens:   intPtr(maxTokensValue),
			}

			// Создаем контекст с таймаутом для каждого запроса
			requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()

			response, err := makeOpenRouterRequest(requestCtx, request)
			duration := time.Since(start)

			if err != nil {
				log.Printf("    ❌ Failed max_tokens %d: %v", maxTokensValue, err)
				resultsChan <- ParallelTestResult{Error: fmt.Errorf("max_tokens %d failed: %w", maxTokensValue, err)}
				return
			}

			if len(response.Choices) == 0 {
				log.Printf("    ❌ No response for max_tokens %d", maxTokensValue)
				resultsChan <- ParallelTestResult{Error: fmt.Errorf("no response for max_tokens %d", maxTokensValue)}
				return
			}

			// Получаем детальную информацию о стоимости
			var detailedCost *DetailedCostInfo
			if response.ID != "" {
				if cost, err := getDetailedCostInfo(ctx, response.ID); err != nil {
					log.Printf("    ⚠️ Failed to get detailed cost for max_tokens %d: %v", maxTokensValue, err)
				} else {
					detailedCost = cost
				}
			}

			result := BenchmarkResult{
				Parameter:        "max_tokens",
				Value:            maxTokensValue,
				Response:         response.Choices[0].Message.Content,
				Tokens:           response.Usage.TotalTokens,
				Cost:             detailedCost.TotalCost,
				Duration:         duration,
				ResponseLength:   len(response.Choices[0].Message.Content),
				DetailedCostInfo: detailedCost,
			}

			// Логируем базовую информацию
			log.Printf("    ✅ max_tokens %d completed: %d tokens, $%.6f, %v, %d chars",
				maxTokensValue, result.Tokens, result.Cost, duration, result.ResponseLength)

			// Логируем детальную стоимость, если доступна
			if detailedCost != nil {
				log.Printf("    💰 Detailed cost: Prompt $%.6f (%d tok), Completion $%.6f (%d tok), Total $%.6f",
					detailedCost.PromptCost, detailedCost.PromptTokens,
					detailedCost.CompletionCost, detailedCost.CompletionTokens,
					detailedCost.TotalCost)
				log.Printf("    📊 Pricing: $%.6f/1K prompt, $%.6f/1K completion",
					detailedCost.PromptPrice, detailedCost.CompletionPrice)
			}

			resultsChan <- ParallelTestResult{Result: result, Error: nil}

		}(maxTokens, i)
	}

	// Ждем завершения всех goroutines
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Собираем результаты
	var results []BenchmarkResult
	var errors []error

	for testResult := range resultsChan {
		if testResult.Error != nil {
			errors = append(errors, testResult.Error)
		} else {
			results = append(results, testResult.Result)
		}
	}

	log.Printf("  ✅ Max_tokens tests completed: %d successful, %d failed", len(results), len(errors))

	if len(results) == 0 {
		return nil, fmt.Errorf("all max_tokens tests failed: %v", errors)
	}

	// Сортируем результаты по значению max_tokens для красивого вывода
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Value.(int) > results[j].Value.(int) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// analyzeResults анализирует результаты с помощью LLM
func analyzeResults(ctx context.Context, parameter string, results []BenchmarkResult, originalQuestion string) (*AnalysisResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to analyze")
	}

	// Формируем промпт для анализа
	var analysisPrompt strings.Builder
	analysisPrompt.WriteString(fmt.Sprintf("Проанализируй результаты бенчмарка LLM для параметра %s.\n\n", parameter))
	analysisPrompt.WriteString(fmt.Sprintf("Исходный вопрос: %s\n\n", originalQuestion))
	analysisPrompt.WriteString("Результаты тестов:\n\n")

	for i, result := range results {
		analysisPrompt.WriteString(fmt.Sprintf("=== Тест %d ===\n", i+1))
		analysisPrompt.WriteString(fmt.Sprintf("Параметр: %v\n", result.Value))
		analysisPrompt.WriteString(fmt.Sprintf("Токены: %d\n", result.Tokens))
		analysisPrompt.WriteString(fmt.Sprintf("Стоимость: $%.6f\n", result.Cost))
		analysisPrompt.WriteString(fmt.Sprintf("Время: %v\n", result.Duration))
		analysisPrompt.WriteString(fmt.Sprintf("Длина ответа: %d символов\n", result.ResponseLength))

		// Добавляем детальную информацию о стоимости, если доступна
		if result.DetailedCostInfo != nil {
			analysisPrompt.WriteString(fmt.Sprintf("Детальная стоимость:\n"))
			analysisPrompt.WriteString(fmt.Sprintf("  - Промпт: $%.6f (%d токенов)\n",
				result.DetailedCostInfo.PromptCost, result.DetailedCostInfo.PromptTokens))
			analysisPrompt.WriteString(fmt.Sprintf("  - Ответ: $%.6f (%d токенов)\n",
				result.DetailedCostInfo.CompletionCost, result.DetailedCostInfo.CompletionTokens))
			analysisPrompt.WriteString(fmt.Sprintf("  - Цена за 1K: промпт $%.6f, ответ $%.6f\n",
				result.DetailedCostInfo.PromptPrice, result.DetailedCostInfo.CompletionPrice))
		}

		analysisPrompt.WriteString(fmt.Sprintf("Ответ: %s\n\n", truncateString(result.Response, 300)))
	}

	analysisPrompt.WriteString(`Проанализируй результаты по следующим критериям:
1. Качество ответов (полнота, точность, креативность) - оценка от 1 до 10
2. Экономическая эффективность (соотношение качества к стоимости) - оценка от 1 до 10
3. Скорость ответа - оценка от 1 до 10
4. Общий анализ влияния параметра на результат
5. Рекомендация по оптимальному значению параметра

Ответь в JSON формате:
{
  "quality_score": [1-10],
  "cost_efficiency": [1-10], 
  "response_time": [1-10],
  "analysis": "детальный анализ результатов",
  "recommendation": "рекомендация по использованию",
  "best_value": "оптимальное_значение_параметра"
}`)

	request := OpenRouterRequest{
		Model: TEST_MODEL,
		Messages: []OpenRouterMessage{
			{Role: "user", Content: analysisPrompt.String()},
		},
		Temperature: float32Ptr(0.3), // Низкая температура для аналитического ответа
		MaxTokens:   intPtr(1500),
	}

	response, err := makeOpenRouterRequest(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze results: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no analysis response received")
	}

	// Парсим JSON ответ
	analysisContent := response.Choices[0].Message.Content
	log.Printf("🔍 Raw analysis response: %s", truncateString(analysisContent, 200))

	// Ищем JSON в ответе
	jsonStart := strings.Index(analysisContent, "{")
	jsonEnd := strings.LastIndex(analysisContent, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON found in analysis response")
	}

	jsonContent := analysisContent[jsonStart : jsonEnd+1]
	log.Printf("🔍 Extracted JSON: %s", truncateString(jsonContent, 300))

	// Пробуем разные варианты парсинга JSON
	var analysis AnalysisResult
	if err := json.Unmarshal([]byte(jsonContent), &analysis); err != nil {
		// Пробуем исправить возможные проблемы с JSON
		// LLM иногда возвращает массивы вместо чисел, берем первое значение
		fixedContent := jsonContent

		// Ищем и заменяем массивы оценок на первое значение
		fixedContent = fixArrayValue(fixedContent, "quality_score")
		fixedContent = fixArrayValue(fixedContent, "cost_efficiency")
		fixedContent = fixArrayValue(fixedContent, "response_time")

		if err2 := json.Unmarshal([]byte(fixedContent), &analysis); err2 != nil {
			return nil, fmt.Errorf("failed to parse analysis JSON: %w (original: %v)", err2, err)
		}
		log.Printf("✅ Fixed JSON parsing issue")
	}

	return &analysis, nil
}

// makeOpenRouterRequest выполняет запрос к OpenRouter API
func makeOpenRouterRequest(ctx context.Context, request OpenRouterRequest) (*OpenRouterResponse, error) {
	// Добавляем метаданные для отслеживания
	if request.Metadata == nil {
		request.Metadata = make(map[string]interface{})
	}
	request.Metadata["title"] = "ai-chatter-benchmark"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", OPENROUTER_BASE_URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/AndVl1/ai-chatter")
	req.Header.Set("X-Title", "ai-chatter-benchmark")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var openRouterResponse OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&openRouterResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if openRouterResponse.Error != nil {
		return nil, fmt.Errorf("OpenRouter API error: %s", openRouterResponse.Error.Message)
	}

	return &openRouterResponse, nil
}

// getDetailedCostInfo получает детальную информацию о стоимости запроса
func getDetailedCostInfo(ctx context.Context, requestID string) (*DetailedCostInfo, error) {
	if requestID == "" {
		return nil, fmt.Errorf("request ID is empty")
	}

	// Увеличиваем задержку для бесплатных моделей
	time.Sleep(2 * time.Second)

	generationURL := fmt.Sprintf("%s?id=%s", OPENROUTER_GENERATION_URL, requestID)

	req, err := http.NewRequestWithContext(ctx, "GET", generationURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create generation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/AndVl1/ai-chatter")
	req.Header.Set("X-Title", "ai-chatter-benchmark")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("generation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Для бесплатных моделей Generation API может быть недоступен
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("generation data not available (possibly free model)")
		}
		return nil, fmt.Errorf("generation API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var generationResponse OpenRouterGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&generationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode generation response: %w", err)
	}

	// Вычисляем цены за 1K токенов на основе общих данных
	var promptPrice, completionPrice float64
	if generationResponse.Data.TokensPrompt > 0 && generationResponse.Data.TotalCost > 0 {
		// Приблизительно разделяем стоимость между prompt и completion
		totalTokens := generationResponse.Data.TokensPrompt + generationResponse.Data.TokensCompletion
		if totalTokens > 0 {
			promptRatio := float64(generationResponse.Data.TokensPrompt) / float64(totalTokens)
			completionRatio := float64(generationResponse.Data.TokensCompletion) / float64(totalTokens)

			promptCost := generationResponse.Data.TotalCost * promptRatio
			completionCost := generationResponse.Data.TotalCost * completionRatio

			promptPrice = (promptCost / float64(generationResponse.Data.TokensPrompt)) * 1000
			completionPrice = (completionCost / float64(generationResponse.Data.TokensCompletion)) * 1000
		}
	}

	return &DetailedCostInfo{
		RequestID:        requestID,
		Model:            generationResponse.Data.Model,
		PromptTokens:     generationResponse.Data.TokensPrompt,
		CompletionTokens: generationResponse.Data.TokensCompletion,
		TotalTokens:      generationResponse.Data.TokensPrompt + generationResponse.Data.TokensCompletion,
		PromptCost:       generationResponse.Data.TotalCost * (float64(generationResponse.Data.TokensPrompt) / float64(generationResponse.Data.TokensPrompt+generationResponse.Data.TokensCompletion)),
		CompletionCost:   generationResponse.Data.TotalCost * (float64(generationResponse.Data.TokensCompletion) / float64(generationResponse.Data.TokensPrompt+generationResponse.Data.TokensCompletion)),
		TotalCost:        generationResponse.Data.TotalCost,
		PromptPrice:      promptPrice,
		CompletionPrice:  completionPrice,
		NativeCost:       generationResponse.Data.Usage, // usage кажется более подходящим для native cost
	}, nil
}

// printAnalysis выводит результат анализа в консоль
func printAnalysis(parameterName string, analysis *AnalysisResult) {
	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("📊 АНАЛИЗ РЕЗУЛЬТАТОВ: %s\n", strings.ToUpper(parameterName))
	fmt.Printf(strings.Repeat("=", 60) + "\n\n")

	fmt.Printf("🎯 ОЦЕНКИ:\n")
	fmt.Printf("  Качество ответов:    %.1f/10\n", analysis.QualityScore)
	fmt.Printf("  Экономичность:       %.1f/10\n", analysis.CostEfficiency)
	fmt.Printf("  Скорость ответа:     %.1f/10\n", analysis.ResponseTime)

	fmt.Printf("\n🔍 ДЕТАЛЬНЫЙ АНАЛИЗ:\n")
	fmt.Printf("%s\n", wrapText(analysis.Analysis, 60))

	fmt.Printf("\n💡 РЕКОМЕНДАЦИЯ:\n")
	fmt.Printf("%s\n", wrapText(analysis.Recommendation, 60))

	fmt.Printf("\n⭐ ОПТИМАЛЬНОЕ ЗНАЧЕНИЕ: %v\n", analysis.BestValue)
}

// Вспомогательные функции
func float32Ptr(v float32) *float32 { return &v }
func intPtr(v int) *int             { return &v }

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// fixArrayValue исправляет JSON, заменяя массивы на первое значение
func fixArrayValue(jsonStr, fieldName string) string {
	// Используем простую строковую замену для безопасности
	start := strings.Index(jsonStr, fmt.Sprintf(`"%s": [`, fieldName))
	if start == -1 {
		return jsonStr
	}

	// Находим конец массива
	openBracket := strings.Index(jsonStr[start:], "[")
	if openBracket == -1 {
		return jsonStr
	}

	closeBracket := strings.Index(jsonStr[start+openBracket:], "]")
	if closeBracket == -1 {
		return jsonStr
	}

	// Извлекаем содержимое массива
	arrayContent := jsonStr[start+openBracket+1 : start+openBracket+closeBracket]

	// Берем первое значение
	firstValue := strings.Split(strings.TrimSpace(arrayContent), ",")[0]
	firstValue = strings.TrimSpace(firstValue)

	// Заменяем весь массив на первое значение
	before := jsonStr[:start]
	after := jsonStr[start+openBracket+closeBracket+1:]
	fieldDeclaration := fmt.Sprintf(`"%s": %s`, fieldName, firstValue)

	return before + fieldDeclaration + after
}

func wrapText(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		if currentLine.Len() > 0 && currentLine.Len()+len(word)+1 > width {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}

// printSummaryStats выводит сводную статистику по результатам
func printSummaryStats(parameterName string, results []BenchmarkResult) {
	if len(results) == 0 {
		log.Printf("  %s: No results", parameterName)
		return
	}

	// Вычисляем статистики
	var totalCost float64
	var totalDuration time.Duration
	var avgTokens, avgLength int
	var minCost, maxCost float64
	var minDuration, maxDuration time.Duration

	minCost = results[0].Cost
	maxCost = results[0].Cost
	minDuration = results[0].Duration
	maxDuration = results[0].Duration

	for _, result := range results {
		totalCost += result.Cost
		totalDuration += result.Duration
		avgTokens += result.Tokens
		avgLength += result.ResponseLength

		if result.Cost < minCost {
			minCost = result.Cost
		}
		if result.Cost > maxCost {
			maxCost = result.Cost
		}
		if result.Duration < minDuration {
			minDuration = result.Duration
		}
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
	}

	avgTokens /= len(results)
	avgLength /= len(results)
	avgCost := totalCost / float64(len(results))
	avgDuration := totalDuration / time.Duration(len(results))

	log.Printf("\n📊 %s Results:", parameterName)
	log.Printf("  Tests: %d", len(results))
	log.Printf("  Avg Tokens: %d", avgTokens)
	log.Printf("  Avg Cost: $%.6f", avgCost)
	log.Printf("  Avg Duration: %v", avgDuration)
	log.Printf("  Avg Length: %d chars", avgLength)
	log.Printf("  Cost Range: $%.6f - $%.6f", minCost, maxCost)
	log.Printf("  Duration Range: %v - %v", minDuration, maxDuration)

	// Находим лучший по соотношению цена/качество
	bestValue := results[0].Value
	bestRatio := float64(results[0].ResponseLength) / math.Max(results[0].Cost*1000000, 0.1) // Избегаем деления на 0

	for _, result := range results {
		ratio := float64(result.ResponseLength) / math.Max(result.Cost*1000000, 0.1)
		if ratio > bestRatio {
			bestRatio = ratio
			bestValue = result.Value
		}
	}

	log.Printf("  Best Value (chars/cost): %v", bestValue)

	// Выводим информацию о ценах модели, если доступна
	for _, result := range results {
		if result.DetailedCostInfo != nil {
			log.Printf("\n💰 Model Pricing Information (%s):", result.DetailedCostInfo.Model)
			log.Printf("  Prompt Price: $%.6f per 1K tokens", result.DetailedCostInfo.PromptPrice)
			log.Printf("  Completion Price: $%.6f per 1K tokens", result.DetailedCostInfo.CompletionPrice)
			log.Printf("  Example: 1000 prompt + 1000 completion tokens = $%.6f",
				result.DetailedCostInfo.PromptPrice+result.DetailedCostInfo.CompletionPrice)
			break // Показываем только один раз для модели
		}
	}
}
