package analytics

import (
	"encoding/json"
	"fmt"
	"time"

	"ai-chatter/internal/storage"
)

// DailyStats содержит статистику за день
type DailyStats struct {
	Date                  string              `json:"date"`
	TotalMessages         int                 `json:"total_messages"`
	UniqueUsers           int                 `json:"unique_users"`
	MCPFunctionCallsTotal int                 `json:"mcp_function_calls_total"`
	MCPFunctionsByType    map[string]int      `json:"mcp_functions_by_type"`
	UserStats             map[int64]UserStats `json:"user_stats"`
}

// UserStats содержит статистику по пользователю
type UserStats struct {
	UserID             int64          `json:"user_id"`
	Messages           int            `json:"messages"`
	MCPFunctionCalls   int            `json:"mcp_function_calls"`
	MCPFunctionsByType map[string]int `json:"mcp_functions_by_type"`
}

// AnalyzeDailyLogs анализирует логи за указанную дату
func AnalyzeDailyLogs(events []storage.Event, targetDate time.Time) *DailyStats {
	// Нормализуем дату до начала дня
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	stats := &DailyStats{
		Date:               startOfDay.Format("2006-01-02"),
		MCPFunctionsByType: make(map[string]int),
		UserStats:          make(map[int64]UserStats),
	}

	uniqueUsers := make(map[int64]bool)

	for _, event := range events {
		// Проверяем, что событие произошло в нужный день
		if event.Timestamp.Before(startOfDay) || !event.Timestamp.Before(endOfDay) {
			continue
		}

		// Считаем только события с UserMessage (исключаем системные записи)
		if event.UserMessage != "" {
			stats.TotalMessages++
			uniqueUsers[event.UserID] = true

			// Инициализируем статистику пользователя если её нет
			userStat, exists := stats.UserStats[event.UserID]
			if !exists {
				userStat = UserStats{
					UserID:             event.UserID,
					MCPFunctionsByType: make(map[string]int),
				}
			}

			userStat.Messages++

			// Анализируем MCP function calls
			for _, funcName := range event.MCPFunctionCalls {
				stats.MCPFunctionCallsTotal++
				stats.MCPFunctionsByType[funcName]++
				userStat.MCPFunctionCalls++
				userStat.MCPFunctionsByType[funcName]++
			}

			stats.UserStats[event.UserID] = userStat
		}
	}

	stats.UniqueUsers = len(uniqueUsers)
	return stats
}

// GenerateReportSummary создает текстовое резюме для LLM
func (ds *DailyStats) GenerateReportSummary() string {
	summary := fmt.Sprintf(`Статистика использования AI Chatter за %s:

Общая активность:
- Всего сообщений: %d
- Уникальных пользователей: %d
- Всего вызовов MCP функций: %d

`, ds.Date, ds.TotalMessages, ds.UniqueUsers, ds.MCPFunctionCallsTotal)

	if len(ds.MCPFunctionsByType) > 0 {
		summary += "Использование MCP функций:\n"
		for funcName, count := range ds.MCPFunctionsByType {
			summary += fmt.Sprintf("- %s: %d раз\n", funcName, count)
		}
		summary += "\n"
	}

	summary += fmt.Sprintf("Активность пользователей (%d пользователей):\n", len(ds.UserStats))
	for userID, userStat := range ds.UserStats {
		summary += fmt.Sprintf("- Пользователь %d: %d сообщений", userID, userStat.Messages)
		if userStat.MCPFunctionCalls > 0 {
			summary += fmt.Sprintf(", %d MCP вызовов", userStat.MCPFunctionCalls)
		}
		summary += "\n"
	}

	return summary
}

// ToJSON сериализует статистику в JSON для детального анализа
func (ds *DailyStats) ToJSON() (string, error) {
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
