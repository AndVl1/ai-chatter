package codevalidation

import (
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Mock bot interface for testing
type mockBot struct {
	sentMessages []tgbotapi.EditMessageTextConfig
	parseMode    string
}

func (m *mockBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if editMsg, ok := c.(tgbotapi.EditMessageTextConfig); ok {
		m.sentMessages = append(m.sentMessages, editMsg)
	}
	return tgbotapi.Message{}, nil
}

func (m *mockBot) ParseModeValue() string {
	if m.parseMode == "" {
		return "Markdown"
	}
	return m.parseMode
}

func TestNewCodeValidationProgressTracker(t *testing.T) {
	mockBot := &mockBot{}
	chatID := int64(12345)
	messageID := 67890
	filename := "test.py"
	language := "Python"

	tracker := NewCodeValidationProgressTracker(mockBot, chatID, messageID, filename, language)

	if tracker.chatID != chatID {
		t.Errorf("Expected chatID %d, got %d", chatID, tracker.chatID)
	}

	if tracker.messageID != messageID {
		t.Errorf("Expected messageID %d, got %d", messageID, tracker.messageID)
	}

	if tracker.filename != filename {
		t.Errorf("Expected filename %s, got %s", filename, tracker.filename)
	}

	if tracker.language != language {
		t.Errorf("Expected language %s, got %s", language, tracker.language)
	}

	// Check that all expected steps are initialized
	expectedSteps := []string{"code_analysis", "docker_setup", "install_deps", "copy_code", "run_validation"}
	for _, step := range expectedSteps {
		if _, exists := tracker.steps[step]; !exists {
			t.Errorf("Expected step %s to exist", step)
		}
	}

	// Check that all steps are initially pending
	for stepKey, step := range tracker.steps {
		if step.Status != "pending" {
			t.Errorf("Expected step %s to have status 'pending', got %s", stepKey, step.Status)
		}
	}
}

func TestProgressTracker_UpdateProgress(t *testing.T) {
	mockBot := &mockBot{}
	tracker := NewCodeValidationProgressTracker(mockBot, 12345, 67890, "test.py", "Python")

	// Test updating to in_progress
	tracker.UpdateProgress("code_analysis", "in_progress")

	step := tracker.steps["code_analysis"]
	if step.Status != "in_progress" {
		t.Errorf("Expected status 'in_progress', got %s", step.Status)
	}

	if step.StartTime.IsZero() {
		t.Error("Expected StartTime to be set")
	}

	// Check that a message was sent
	if len(mockBot.sentMessages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(mockBot.sentMessages))
	}

	// Test updating to completed
	time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	tracker.UpdateProgress("code_analysis", "completed")

	step = tracker.steps["code_analysis"]
	if step.Status != "completed" {
		t.Errorf("Expected status 'completed', got %s", step.Status)
	}

	if step.EndTime.IsZero() {
		t.Error("Expected EndTime to be set")
	}

	if step.EndTime.Before(step.StartTime) {
		t.Error("EndTime should be after StartTime")
	}

	// Check that another message was sent
	if len(mockBot.sentMessages) != 2 {
		t.Errorf("Expected 2 messages sent, got %d", len(mockBot.sentMessages))
	}
}

func TestProgressTracker_BuildProgressMessage(t *testing.T) {
	mockBot := &mockBot{}
	tracker := NewCodeValidationProgressTracker(mockBot, 12345, 67890, "test.py", "Python")

	// Test initial message
	message := tracker.buildProgressMessage()

	if !strings.Contains(message, "test.py") {
		t.Error("Expected message to contain filename")
	}

	if !strings.Contains(message, "Python") {
		t.Error("Expected message to contain language")
	}

	if !strings.Contains(message, "🔄 **Валидация кода в процессе...**") {
		t.Error("Expected message to contain progress header")
	}

	// Count emoji indicators for pending steps
	pendingCount := strings.Count(message, "⏳")
	if pendingCount != 5 { // 5 steps should be pending initially
		t.Errorf("Expected 5 pending steps, found %d", pendingCount)
	}

	// Test message with completed steps
	tracker.UpdateProgress("code_analysis", "completed")
	tracker.UpdateProgress("docker_setup", "in_progress")

	message = tracker.buildProgressMessage()

	if !strings.Contains(message, "✅") {
		t.Error("Expected message to contain completed step indicator")
	}

	if !strings.Contains(message, "🔄") {
		t.Error("Expected message to contain in-progress step indicator")
	}
}

func TestProgressTracker_SetFinalResult(t *testing.T) {
	mockBot := &mockBot{}
	tracker := NewCodeValidationProgressTracker(mockBot, 12345, 67890, "test.py", "Python")

	// Set up some completed steps
	tracker.UpdateProgress("code_analysis", "completed")
	tracker.UpdateProgress("docker_setup", "completed")
	tracker.UpdateProgress("copy_code", "completed")
	tracker.UpdateProgress("run_validation", "completed")

	result := &ValidationResult{
		Success:     true,
		Output:      "All tests passed",
		Errors:      []string{},
		Warnings:    []string{"Minor style issue"},
		ExitCode:    0,
		Duration:    "2.5s",
		Suggestions: []string{"Consider adding more tests"},
	}

	initialMessageCount := len(mockBot.sentMessages)
	tracker.SetFinalResult(result)

	// Check that a final message was sent
	if len(mockBot.sentMessages) <= initialMessageCount {
		t.Error("Expected final message to be sent")
	}

	// Verify that buildFinalMessage is called by checking the content
	// (we can't easily test buildFinalMessage directly due to mutex)
	lastMessage := mockBot.sentMessages[len(mockBot.sentMessages)-1]
	messageText := lastMessage.Text

	if !strings.Contains(messageText, "✅ **Валидация кода успешно завершена**") {
		t.Error("Expected success message in final result")
	}

	if !strings.Contains(messageText, "test.py") {
		t.Error("Expected filename in final message")
	}

	if !strings.Contains(messageText, "2.5s") {
		t.Error("Expected duration in final message")
	}
}

func TestProgressTracker_BuildFinalMessage_Success(t *testing.T) {
	mockBot := &mockBot{}
	tracker := NewCodeValidationProgressTracker(mockBot, 12345, 67890, "script.js", "JavaScript")

	// Mark all steps as completed
	steps := []string{"code_analysis", "docker_setup", "install_deps", "copy_code", "run_validation"}
	for _, step := range steps {
		tracker.UpdateProgress(step, "completed")
	}

	result := &ValidationResult{
		Success:     true,
		Output:      "Tests: 5 passed\nLinting: no issues",
		Errors:      []string{},
		Warnings:    []string{},
		ExitCode:    0,
		Duration:    "3.2s",
		Suggestions: []string{"Add more unit tests", "Consider type checking"},
	}

	message := tracker.buildFinalMessage(result)

	// Check success indicators
	if !strings.Contains(message, "✅ **Валидация кода успешно завершена**") {
		t.Error("Expected success header")
	}

	if !strings.Contains(message, "🎉 **Все проверки пройдены успешно!**") {
		t.Error("Expected success celebration message")
	}

	// Check file info
	if !strings.Contains(message, "script.js") {
		t.Error("Expected filename in message")
	}

	if !strings.Contains(message, "JavaScript") {
		t.Error("Expected language in message")
	}

	// Check execution info
	if !strings.Contains(message, "3.2s") {
		t.Error("Expected duration in message")
	}

	if !strings.Contains(message, "Exit Code:** 0") {
		t.Error("Expected exit code in message")
	}

	// Check suggestions
	if !strings.Contains(message, "💡 **Рекомендации:**") {
		t.Error("Expected suggestions section")
	}

	if !strings.Contains(message, "Add more unit tests") {
		t.Error("Expected first suggestion")
	}

	// Check step completion indicators
	completedCount := strings.Count(message, "✅")
	if completedCount < 5 { // At least 5 for the completed steps
		t.Errorf("Expected at least 5 completed step indicators, got %d", completedCount)
	}
}

func TestProgressTracker_BuildFinalMessage_Failure(t *testing.T) {
	mockBot := &mockBot{}
	tracker := NewCodeValidationProgressTracker(mockBot, 12345, 67890, "broken.py", "Python")

	// Mark some steps as completed, one as error
	tracker.UpdateProgress("code_analysis", "completed")
	tracker.UpdateProgress("docker_setup", "completed")
	tracker.UpdateProgress("copy_code", "completed")
	tracker.UpdateProgress("run_validation", "error")

	result := &ValidationResult{
		Success:     false,
		Output:      "Tests failed: 2 passed, 3 failed",
		Errors:      []string{"Syntax error in line 15", "Undefined variable 'x'"},
		Warnings:    []string{"Unused import 'os'"},
		ExitCode:    1,
		Duration:    "1.8s",
		Suggestions: []string{"Fix syntax errors", "Check variable definitions"},
	}

	message := tracker.buildFinalMessage(result)

	// Check failure indicators
	if !strings.Contains(message, "❌ **Валидация кода завершена с ошибками**") {
		t.Error("Expected failure header")
	}

	if !strings.Contains(message, "❌ **Обнаружены проблемы:**") {
		t.Error("Expected problems section")
	}

	// Check error details
	if !strings.Contains(message, "Syntax error in line 15") {
		t.Error("Expected first error message")
	}

	if !strings.Contains(message, "Undefined variable &#39;x&#39;") {
		t.Error("Expected second error message")
	}

	// Check warnings
	if !strings.Contains(message, "⚠️ **Предупреждения:**") {
		t.Error("Expected warnings section")
	}

	if !strings.Contains(message, "Unused import &#39;os&#39;") {
		t.Error("Expected warning message")
	}

	// Check exit code
	if !strings.Contains(message, "Exit Code:** 1") {
		t.Error("Expected non-zero exit code")
	}

	// Check that we have error indicators
	if !strings.Contains(message, "❌") {
		t.Error("Expected error indicators in step list")
	}
}
