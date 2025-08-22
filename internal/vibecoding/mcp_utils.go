package vibecoding

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseUserID парсит user_id из различных типов
func ParseUserID(userIDArg interface{}) (int64, error) {
	var userID int64
	switch v := userIDArg.(type) {
	case float64:
		userID = int64(v)
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid user_id string format: %w", err)
		}
		userID = parsed
	default:
		return 0, fmt.Errorf("user_id must be a number or string, got %T", v)
	}
	return userID, nil
}

// FormatFileList форматирует список файлов для вывода
func FormatFileList(userID int64, fileList []string) string {
	var fileListText strings.Builder
	fileListText.WriteString(fmt.Sprintf("📁 VibeCoding workspace files for user %d:\n\n", userID))

	if len(fileList) == 0 {
		fileListText.WriteString("No files found in workspace\n")
	} else {
		for i, file := range fileList {
			fileListText.WriteString(fmt.Sprintf("%d. %s\n", i+1, file))
		}
	}

	return fileListText.String()
}

// FormatSessionInfo форматирует информацию о сессии
func FormatSessionInfo(userID int64, session *VibeCodingSession) string {
	return fmt.Sprintf(`📊 VibeCoding Session Info for User %d

**Project:** %s
**Container ID:** %s  
**Test Command:** %s
**Start Time:** %s
**Files:** %d
**Generated Files:** %d`,
		userID,
		session.ProjectName,
		session.ContainerID,
		session.TestCommand,
		session.StartTime.Format(time.RFC3339),
		len(session.Files),
		len(session.GeneratedFiles))
}
