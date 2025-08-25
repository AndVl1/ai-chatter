//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Упрощенная версия для быстрого демо
const (
	DEMO_MODEL     = "openai/gpt-4o-mini"
	OPENROUTER_URL = "https://openrouter.ai/api/v1/chat/completions"
)

type DemoRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature *float32 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

type DemoResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		TotalTokens      int     `json:"total_tokens"`
		PromptCost       float64 `json:"prompt_cost"`
		CompletionCost   float64 `json:"completion_cost"`
		TotalCost        float64 `json:"total_cost"`
	} `json:"usage"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		runQuickDemo()
	} else {
		log.Printf("Используйте: go run cmd/benchmark/demo.go demo")
		log.Printf("Для полного бенчмарка: go run cmd/benchmark/main.go")
	}
}

func runQuickDemo() {
	log.Printf("🚀 LLM Benchmark Demo")

	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env not found")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatalf("❌ OPENAI_API_KEY required")
	}

	log.Printf("🎯 Demo with model: %s", DEMO_MODEL)

	testQuestion := "Кратко объясни разницу между ИИ и машинным обучением"

	// Тест 2 температур параллельно
	temps := []float32{0.0, 1.0}
	log.Printf("\n🌡️ Parallel Temperature Test:")

	type TempResult struct {
		Temp     float32
		Response *DemoResponse
		Duration time.Duration
		Error    error
	}

	tempChan := make(chan TempResult, len(temps))
	var wg sync.WaitGroup

	for i, temp := range temps {
		wg.Add(1)
		go func(temperature float32, index int) {
			defer wg.Done()
			time.Sleep(time.Duration(index) * 200 * time.Millisecond) // Стaggered start

			start := time.Now()
			response, err := makeRequest(testQuestion, &temperature, intPtr(200))
			duration := time.Since(start)

			tempChan <- TempResult{
				Temp:     temperature,
				Response: response,
				Duration: duration,
				Error:    err,
			}
		}(temp, i)
	}

	go func() {
		wg.Wait()
		close(tempChan)
	}()

	for result := range tempChan {
		if result.Error != nil {
			log.Printf("  ❌ temp %.1f: %v", result.Temp, result.Error)
			continue
		}

		log.Printf("  ✅ temp %.1f: %d tokens (p:%d, c:%d), $%.6f, %v, %d chars",
			result.Temp, result.Response.Usage.TotalTokens,
			result.Response.Usage.PromptTokens, result.Response.Usage.CompletionTokens,
			result.Response.Usage.TotalCost, result.Duration, len(result.Response.Choices[0].Message.Content))
		log.Printf("     Cost breakdown: Prompt $%.6f, Completion $%.6f",
			result.Response.Usage.PromptCost, result.Response.Usage.CompletionCost)
		log.Printf("     Response: %s...", truncate(result.Response.Choices[0].Message.Content, 100))
	}

	// Тест max_tokens параллельно
	tokens := []int{50, 200}
	log.Printf("\n🔢 Parallel MaxTokens Test:")

	type TokenResult struct {
		MaxTokens int
		Response  *DemoResponse
		Duration  time.Duration
		Error     error
	}

	tokenChan := make(chan TokenResult, len(tokens))
	var wg2 sync.WaitGroup

	for i, maxTokens := range tokens {
		wg2.Add(1)
		go func(maxTokensValue int, index int) {
			defer wg2.Done()
			time.Sleep(time.Duration(index) * 250 * time.Millisecond) // Staggered start

			start := time.Now()
			response, err := makeRequest(testQuestion, float32Ptr(0.7), &maxTokensValue)
			duration := time.Since(start)

			tokenChan <- TokenResult{
				MaxTokens: maxTokensValue,
				Response:  response,
				Duration:  duration,
				Error:     err,
			}
		}(maxTokens, i)
	}

	go func() {
		wg2.Wait()
		close(tokenChan)
	}()

	for result := range tokenChan {
		if result.Error != nil {
			log.Printf("  ❌ tokens %d: %v", result.MaxTokens, result.Error)
			continue
		}

		log.Printf("  ✅ tokens %d: %d actual tokens (p:%d, c:%d), $%.6f, %v, %d chars",
			result.MaxTokens, result.Response.Usage.TotalTokens,
			result.Response.Usage.PromptTokens, result.Response.Usage.CompletionTokens,
			result.Response.Usage.TotalCost, result.Duration, len(result.Response.Choices[0].Message.Content))
		log.Printf("     Cost breakdown: Prompt $%.6f, Completion $%.6f",
			result.Response.Usage.PromptCost, result.Response.Usage.CompletionCost)
		log.Printf("     Response: %s...", truncate(result.Response.Choices[0].Message.Content, 100))
	}

	log.Printf("\n🎉 Demo completed! Run full benchmark with main.go for detailed analysis")
}

func makeRequest(question string, temp *float32, maxTokens *int) (*DemoResponse, error) {
	request := DemoRequest{
		Model: DEMO_MODEL,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: question},
		},
		Temperature: temp,
		MaxTokens:   maxTokens,
	}

	jsonData, _ := json.Marshal(request)
	req, _ := http.NewRequest("POST", OPENROUTER_URL, bytes.NewBuffer(jsonData))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response DemoResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

func float32Ptr(v float32) *float32 { return &v }
func intPtr(v int) *int             { return &v }
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
