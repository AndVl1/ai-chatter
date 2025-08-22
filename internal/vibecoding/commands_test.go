package vibecoding

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ai-chatter/internal/codevalidation"
	"ai-chatter/internal/llm"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockLLMClient для тестирования
type MockLLMClient struct {
	responses   map[string]string
	callCount   int
	shouldError bool
}

func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		responses: make(map[string]string),
		callCount: 0,
	}
}

func (m *MockLLMClient) SetResponse(key, response string) {
	m.responses[key] = response
}

func (m *MockLLMClient) SetShouldError(shouldError bool) {
	m.shouldError = shouldError
}

func (m *MockLLMClient) Generate(ctx context.Context, messages []llm.Message) (llm.Response, error) {
	m.callCount++

	if m.shouldError {
		return llm.Response{}, fmt.Errorf("mock LLM error")
	}

	// Определяем тип запроса по содержимому
	var content string
	for _, msg := range messages {
		content += msg.Content + " "
	}

	// Для генерации промпта
	if strings.Contains(content, "test writing advisor") {
		response := `{
			"test_prompt": "Write comprehensive tests for Python using pytest framework. Ensure all imports are correct and functions exist.",
			"key_rules": [
				"Import only existing functions and classes",
				"Use pytest fixtures for setup",
				"Include proper assertions"
			],
			"testing_framework": "pytest",
			"file_naming": "test_*.py",
			"best_practices": [
				"Use descriptive test names",
				"Test edge cases",
				"Mock external dependencies"
			],
			"common_pitfalls": [
				"Testing non-existent functions",
				"Missing imports",
				"Incorrect assertions"
			]
		}`
		return llm.Response{Content: response}, nil
	}

	// Для валидации тестов
	if strings.Contains(content, "test reviewer and validator") {
		response := `{
			"status": "ok",
			"issues": [],
			"fixed_tests": {},
			"reasoning": "All tests look good",
			"suggestions": ["Add more edge cases"]
		}`
		return llm.Response{Content: response}, nil
	}

	// Для проверки подходящности команды
	if strings.Contains(content, "command suitable") {
		response := `{
			"is_suitable": true,
			"confidence": "high",
			"reasoning": "Command matches file type"
		}`
		return llm.Response{Content: response}, nil
	}

	// Для адаптации команды
	if strings.Contains(content, "Adapt this test command") {
		response := `{
			"adapted_command": "python -m pytest test_file.py -v",
			"changes_made": "Added specific file targeting",
			"reasoning": "Targeting specific test file"
		}`
		return llm.Response{Content: response}, nil
	}

	// Для определения тестового файла
	if strings.Contains(content, "test file") {
		response := `{
			"is_test_file": true,
			"confidence": "high",
			"reasoning": "File has test_ prefix"
		}`
		return llm.Response{Content: response}, nil
	}

	// По умолчанию
	return llm.Response{Content: "Mock response"}, nil
}

func (m *MockLLMClient) GenerateWithTools(ctx context.Context, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	return m.Generate(ctx, messages)
}

func (m *MockLLMClient) GetCallCount() int {
	return m.callCount
}

// MockTelegramSender для тестирования
type MockTelegramSender struct{}

func (m *MockTelegramSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{MessageID: 123}, nil
}

func (m *MockTelegramSender) GetFile(config tgbotapi.FileConfig) (tgbotapi.File, error) {
	return tgbotapi.File{}, nil
}

// MockMessageFormatter для тестирования
type MockMessageFormatter struct{}

func (m *MockMessageFormatter) EscapeText(text string) string {
	return text
}

func (m *MockMessageFormatter) ParseModeValue() string {
	return "MarkdownV2"
}

// TestGenerateTestWritingPrompt тестирует генерацию специализированного промпта
func TestGenerateTestWritingPrompt(t *testing.T) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	session := &VibeCodingSession{
		ProjectName: "test-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language:     "Python",
			DockerImage:  "python:3.11-slim",
			WorkingDir:   "/workspace",
			TestCommands: []string{"python -m pytest"},
		},
		Files: map[string]string{
			"main.py":  "def hello(): return 'world'",
			"utils.py": "import os\ndef get_env(): return os.getenv('TEST')",
		},
	}

	ctx := context.Background()
	prompt, err := handler.generateTestWritingPrompt(ctx, session)

	if err != nil {
		t.Fatalf("generateTestWritingPrompt() failed: %v", err)
	}

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}

	// Проверяем, что промпт содержит ключевые элементы
	expectedElements := []string{
		"pytest",
		"test_*.py",
		"Import only existing functions",
		"Python",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("Prompt missing expected element: %s", element)
		}
	}

	// Проверяем, что LLM был вызван
	if mockLLM.GetCallCount() != 1 {
		t.Errorf("Expected 1 LLM call, got %d", mockLLM.GetCallCount())
	}
}

// TestGenerateTestWritingPrompt_LLMError тестирует обработку ошибок LLM
func TestGenerateTestWritingPrompt_LLMError(t *testing.T) {
	// Создаем LLM клиент, который всегда возвращает ошибку
	mockLLM := NewMockLLMClient()
	mockLLM.SetShouldError(true)

	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	session := &VibeCodingSession{
		ProjectName: "test-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
		Files: map[string]string{
			"main.py": "def hello(): return 'world'",
		},
	}

	ctx := context.Background()
	_, err := handler.generateTestWritingPrompt(ctx, session)

	if err == nil {
		t.Error("Expected error from LLM failure, got nil")
	}
}

// TestExtractProjectStructureForPrompt тестирует извлечение структуры проекта
func TestExtractProjectStructureForPrompt(t *testing.T) {
	handler := &VibeCodingHandler{}

	files := map[string]string{
		"main.py":   "import os\nimport sys\ndef main(): pass",
		"utils.py":  "from typing import List\nimport json",
		"config.js": "const express = require('express');",
		"README.md": "# Project",
	}

	result := handler.extractProjectStructureForPrompt(files)

	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Проверяем, что результат содержит структуру файлов
	expectedFiles := []string{"main.py", "utils.py", "config.js", "README.md"}
	for _, filename := range expectedFiles {
		if !strings.Contains(result, filename) {
			t.Errorf("Result missing file: %s", filename)
		}
	}

	// Проверяем, что результат содержит обнаруженные зависимости
	expectedDeps := []string{"import os", "import sys", "from typing import List"}
	for _, dep := range expectedDeps {
		if !strings.Contains(result, dep) {
			t.Errorf("Result missing dependency: %s", dep)
		}
	}
}

// TestExtractFunctionsAndClasses тестирует извлечение функций и классов
func TestExtractFunctionsAndClasses(t *testing.T) {
	handler := &VibeCodingHandler{}

	files := map[string]string{
		"main.py": `
def hello_world():
    return "Hello"

class Calculator:
    def add(self, a, b):
        return a + b

def process_data(data):
    return data.upper()
`,
		"utils.go": `
package main

func processFile() error {
    return nil
}

type Config struct {
    Name string
}

func (c *Config) GetName() string {
    return c.Name
}
`,
	}

	result := handler.extractFunctionsAndClasses(files)

	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Проверяем функции Python
	expectedFunctions := []string{"hello_world", "add", "process_data"}
	for _, fn := range expectedFunctions {
		if !strings.Contains(result, fn) {
			t.Errorf("Result missing function: %s", fn)
		}
	}

	// Проверяем классы Python
	expectedClasses := []string{"Calculator"}
	for _, cls := range expectedClasses {
		if !strings.Contains(result, cls) {
			t.Errorf("Result missing class: %s", cls)
		}
	}

	// Проверяем функции Go
	expectedGoFunctions := []string{"processFile", "GetName"}
	for _, fn := range expectedGoFunctions {
		if !strings.Contains(result, fn) {
			t.Errorf("Result missing Go function: %s", fn)
		}
	}

	// Проверяем структуры Go
	expectedGoStructs := []string{"Config"}
	for _, st := range expectedGoStructs {
		if !strings.Contains(result, st) {
			t.Errorf("Result missing Go struct: %s", st)
		}
	}
}

// TestExtractNameFromDefinition тестирует извлечение имен из определений
func TestExtractNameFromDefinition(t *testing.T) {
	handler := &VibeCodingHandler{}

	testCases := []struct {
		line     string
		expected string
	}{
		{"def hello_world():", "hello_world"},
		{"def process_data(data, config=None):", "process_data"},
		{"class Calculator:", "Calculator"},
		{"class MyClass(BaseClass):", "MyClass"},
		{"func processFile() error {", "processFile"},
		{"func (c *Config) GetName() string {", "GetName"},
		{"type Config struct", "Config"},
		{"invalid line", ""},
	}

	for _, tc := range testCases {
		result := handler.extractNameFromDefinition(tc.line)
		if result != tc.expected {
			t.Errorf("extractNameFromDefinition(%q) = %q, expected %q", tc.line, result, tc.expected)
		}
	}
}

// TestValidateTestsWithLLM тестирует валидацию тестов через LLM
func TestValidateTestsWithLLM(t *testing.T) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	session := &VibeCodingSession{
		ProjectName: "test-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
		Files: map[string]string{
			"main.py": "def hello(): return 'world'",
		},
	}

	tests := map[string]string{
		"test_main.py": "import main\ndef test_hello(): assert main.hello() == 'world'",
	}

	ctx := context.Background()
	result, err := handler.validateTestsWithLLM(ctx, session, tests)

	if err != nil {
		t.Fatalf("validateTestsWithLLM() failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 validated test, got %d", len(result))
	}

	if _, exists := result["test_main.py"]; !exists {
		t.Error("Expected test_main.py in validated tests")
	}
}

// TestIsTestCommandSuitableForFile тестирует проверку подходящности команды для файла
func TestIsTestCommandSuitableForFile(t *testing.T) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	ctx := context.Background()
	result := handler.isTestCommandSuitableForFile(ctx, "python -m pytest", "test_main.py", "Python")

	if !result {
		t.Error("Expected command to be suitable for Python test file")
	}

	if mockLLM.GetCallCount() != 1 {
		t.Errorf("Expected 1 LLM call, got %d", mockLLM.GetCallCount())
	}
}

// TestAdaptTestCommandForFile тестирует адаптацию команды для файла
func TestAdaptTestCommandForFile(t *testing.T) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	ctx := context.Background()
	result := handler.adaptTestCommandForFile(ctx, "python -m pytest", "test_main.py", "Python")

	expected := "python -m pytest test_file.py -v"
	if result != expected {
		t.Errorf("adaptTestCommandForFile() = %q, expected %q", result, expected)
	}

	if mockLLM.GetCallCount() != 1 {
		t.Errorf("Expected 1 LLM call, got %d", mockLLM.GetCallCount())
	}
}

// TestIsTestFile тестирует определение тестовых файлов
func TestIsTestFile(t *testing.T) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	ctx := context.Background()

	testCases := []struct {
		filename string
		language string
		expected bool
	}{
		{"test_main.py", "Python", true},
		{"main_test.go", "Go", true},
		{"main.py", "Python", true}, // LLM возвращает true для всех в моке
	}

	for _, tc := range testCases {
		result := handler.isTestFile(ctx, tc.filename, tc.language)
		if result != tc.expected {
			t.Errorf("isTestFile(%q, %q) = %v, expected %v", tc.filename, tc.language, result, tc.expected)
		}
	}
}

// TestVibeCodingHandler_Creation тестирует создание обработчика
func TestVibeCodingHandler_Creation(t *testing.T) {
	mockSender := &MockTelegramSender{}
	mockFormatter := &MockMessageFormatter{}
	mockLLM := NewMockLLMClient()

	handler := NewVibeCodingHandler(mockSender, mockFormatter, mockLLM)

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.sessionManager == nil {
		t.Error("Expected non-nil session manager")
	}

	if handler.protocolClient == nil {
		t.Error("Expected non-nil protocol client")
	}

	if handler.awaitingAutoTask == nil {
		t.Error("Expected non-nil awaiting auto task map")
	}
}

// TestExecuteTestForValidation_NoCommands тестирует поведение когда нет команд тестирования
func TestExecuteTestForValidation_NoCommands(t *testing.T) {
	handler := &VibeCodingHandler{}

	session := &VibeCodingSession{
		Analysis: &codevalidation.CodeAnalysisResult{
			TestCommands: []string{}, // Нет команд тестирования
		},
	}

	ctx := context.Background()
	isValid, issue := handler.executeTestForValidation(ctx, session, "test_file.py")

	if isValid {
		t.Error("Expected test to be invalid when no test commands available")
	}

	if issue == nil {
		t.Error("Expected issue to be reported")
	}

	if issue.Type != "configuration_error" {
		t.Errorf("Expected configuration_error, got %s", issue.Type)
	}
}

// TestHandleInfoCommand_WithoutContext тестирует handleInfoCommand без контекста (проверка на панику)
func TestHandleInfoCommand_WithoutContext(t *testing.T) {
	mockSender := &MockTelegramSender{}
	mockFormatter := &MockMessageFormatter{}
	mockLLM := NewMockLLMClient()

	handler := &VibeCodingHandler{
		sender:    mockSender,
		formatter: mockFormatter,
		llmClient: mockLLM,
	}

	// Создаем сессию без контекста
	session := &VibeCodingSession{
		ProjectName: "test-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
		Files:          make(map[string]string),
		GeneratedFiles: make(map[string]string),
		TestCommand:    "python -m pytest",
		ContainerID:    "test-container",
		Context:        nil, // Нет контекста - это вызывало панику
	}

	// Это не должно вызывать панику
	err := handler.handleInfoCommand(123, session)

	if err != nil {
		t.Errorf("handleInfoCommand returned error: %v", err)
	}
}

// TestHandleInfoCommand_WithContext тестирует handleInfoCommand с контекстом
func TestHandleInfoCommand_WithContext(t *testing.T) {
	mockSender := &MockTelegramSender{}
	mockFormatter := &MockMessageFormatter{}
	mockLLM := NewMockLLMClient()

	handler := &VibeCodingHandler{
		sender:    mockSender,
		formatter: mockFormatter,
		llmClient: mockLLM,
	}

	// Создаем сессию с контекстом
	session := &VibeCodingSession{
		ProjectName: "test-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
		Files:          make(map[string]string),
		GeneratedFiles: make(map[string]string),
		TestCommand:    "python -m pytest",
		ContainerID:    "test-container",
		Context: &ProjectContextLLM{
			GeneratedAt: time.Now(),
			Files:       make(map[string]LLMFileContext),
		},
	}

	// Это тоже не должно вызывать панику
	err := handler.handleInfoCommand(123, session)

	if err != nil {
		t.Errorf("handleInfoCommand returned error: %v", err)
	}
}

// BenchmarkGenerateTestWritingPrompt бенчмарк для генерации промптов
func BenchmarkGenerateTestWritingPrompt(b *testing.B) {
	mockLLM := NewMockLLMClient()
	handler := &VibeCodingHandler{
		llmClient: mockLLM,
	}

	session := &VibeCodingSession{
		ProjectName: "benchmark-project",
		Analysis: &codevalidation.CodeAnalysisResult{
			Language:     "Python",
			TestCommands: []string{"python -m pytest"},
		},
		Files: map[string]string{
			"main.py": "def hello(): return 'world'",
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := handler.generateTestWritingPrompt(ctx, session)
		if err != nil {
			b.Fatalf("generateTestWritingPrompt() failed: %v", err)
		}
	}
}
