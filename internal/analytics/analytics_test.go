package analytics

import (
	"testing"
	"time"

	"ai-chatter/internal/storage"
)

func TestAnalyzeDailyLogs(t *testing.T) {
	// Тестовая дата
	testDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	// Создаем тестовые события
	events := []storage.Event{
		// События в целевой день
		{
			Timestamp:         testDate.Add(2 * time.Hour),
			UserID:            123,
			UserMessage:       "Привет",
			AssistantResponse: "Привет! Как дела?",
			MCPFunctionCalls:  []string{"save_dialog_to_notion"},
		},
		{
			Timestamp:         testDate.Add(4 * time.Hour),
			UserID:            123,
			UserMessage:       "Создай страницу",
			AssistantResponse: "Создаю страницу",
			MCPFunctionCalls:  []string{"create_notion_page", "search_pages_with_id"},
		},
		{
			Timestamp:         testDate.Add(6 * time.Hour),
			UserID:            456,
			UserMessage:       "Найди информацию",
			AssistantResponse: "Ищу...",
			MCPFunctionCalls:  []string{"search_pages_with_id"},
		},
		// События в другой день (не должны учитываться)
		{
			Timestamp:         testDate.AddDate(0, 0, 1),
			UserID:            789,
			UserMessage:       "Завтрашнее сообщение",
			AssistantResponse: "Ответ",
			MCPFunctionCalls:  []string{"save_dialog_to_notion"},
		},
		// Системные события без UserMessage (не должны учитываться)
		{
			Timestamp:         testDate.Add(8 * time.Hour),
			UserID:            123,
			UserMessage:       "",
			AssistantResponse: "[system]",
		},
	}

	// Анализируем события
	stats := AnalyzeDailyLogs(events, testDate)

	// Проверяем результаты
	if stats.Date != "2024-01-15" {
		t.Errorf("Expected date '2024-01-15', got '%s'", stats.Date)
	}

	if stats.TotalMessages != 3 {
		t.Errorf("Expected 3 total messages, got %d", stats.TotalMessages)
	}

	if stats.UniqueUsers != 2 {
		t.Errorf("Expected 2 unique users, got %d", stats.UniqueUsers)
	}

	if stats.MCPFunctionCallsTotal != 4 {
		t.Errorf("Expected 4 total MCP function calls, got %d", stats.MCPFunctionCallsTotal)
	}

	// Проверяем функции по типам
	expectedFunctions := map[string]int{
		"save_dialog_to_notion": 1,
		"create_notion_page":    1,
		"search_pages_with_id":  2,
	}

	for funcName, expectedCount := range expectedFunctions {
		if count, exists := stats.MCPFunctionsByType[funcName]; !exists || count != expectedCount {
			t.Errorf("Expected %d calls to %s, got %d", expectedCount, funcName, count)
		}
	}

	// Проверяем статистику пользователей
	if len(stats.UserStats) != 2 {
		t.Errorf("Expected 2 users in stats, got %d", len(stats.UserStats))
	}

	// Проверяем статистику пользователя 123
	user123Stats, exists := stats.UserStats[123]
	if !exists {
		t.Error("Expected stats for user 123")
	} else {
		if user123Stats.Messages != 2 {
			t.Errorf("Expected 2 messages for user 123, got %d", user123Stats.Messages)
		}
		if user123Stats.MCPFunctionCalls != 3 {
			t.Errorf("Expected 3 MCP calls for user 123, got %d", user123Stats.MCPFunctionCalls)
		}
	}

	// Проверяем статистику пользователя 456
	user456Stats, exists := stats.UserStats[456]
	if !exists {
		t.Error("Expected stats for user 456")
	} else {
		if user456Stats.Messages != 1 {
			t.Errorf("Expected 1 message for user 456, got %d", user456Stats.Messages)
		}
		if user456Stats.MCPFunctionCalls != 1 {
			t.Errorf("Expected 1 MCP call for user 456, got %d", user456Stats.MCPFunctionCalls)
		}
	}
}

func TestAnalyzeDailyLogsEmptyData(t *testing.T) {
	testDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	events := []storage.Event{}

	stats := AnalyzeDailyLogs(events, testDate)

	if stats.Date != "2024-01-15" {
		t.Errorf("Expected date '2024-01-15', got '%s'", stats.Date)
	}

	if stats.TotalMessages != 0 {
		t.Errorf("Expected 0 total messages, got %d", stats.TotalMessages)
	}

	if stats.UniqueUsers != 0 {
		t.Errorf("Expected 0 unique users, got %d", stats.UniqueUsers)
	}

	if stats.MCPFunctionCallsTotal != 0 {
		t.Errorf("Expected 0 total MCP function calls, got %d", stats.MCPFunctionCallsTotal)
	}
}

func TestGenerateReportSummary(t *testing.T) {
	stats := &DailyStats{
		Date:                  "2024-01-15",
		TotalMessages:         5,
		UniqueUsers:           2,
		MCPFunctionCallsTotal: 3,
		MCPFunctionsByType: map[string]int{
			"save_dialog_to_notion": 2,
			"create_notion_page":    1,
		},
		UserStats: map[int64]UserStats{
			123: {
				UserID:           123,
				Messages:         3,
				MCPFunctionCalls: 2,
			},
			456: {
				UserID:           456,
				Messages:         2,
				MCPFunctionCalls: 1,
			},
		},
	}

	summary := stats.GenerateReportSummary()

	// Проверяем что в резюме есть основная информация
	expectedStrings := []string{
		"2024-01-15",
		"5", // total messages
		"2", // unique users
		"3", // total MCP calls
		"save_dialog_to_notion",
		"create_notion_page",
		"Пользователь 123",
		"Пользователь 456",
	}

	for _, expected := range expectedStrings {
		if !contains(summary, expected) {
			t.Errorf("Expected summary to contain '%s', but it didn't. Summary: %s", expected, summary)
		}
	}
}

func TestToJSON(t *testing.T) {
	stats := &DailyStats{
		Date:                  "2024-01-15",
		TotalMessages:         1,
		UniqueUsers:           1,
		MCPFunctionCallsTotal: 1,
		MCPFunctionsByType: map[string]int{
			"test_function": 1,
		},
		UserStats: map[int64]UserStats{
			123: {
				UserID:   123,
				Messages: 1,
			},
		},
	}

	jsonStr, err := stats.ToJSON()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !contains(jsonStr, "2024-01-15") {
		t.Errorf("Expected JSON to contain date, got: %s", jsonStr)
	}

	if !contains(jsonStr, "test_function") {
		t.Errorf("Expected JSON to contain function name, got: %s", jsonStr)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 1; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
