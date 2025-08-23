package vibecoding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-chatter/internal/codevalidation"
)

// TestHandleSessionsWithNilAnalysis тестирует handleSessions когда session.Analysis == nil
func TestHandleSessionsWithNilAnalysis(t *testing.T) {
	// Создаем менеджер сессий без веб-сервера
	sessionManager := NewSessionManagerWithoutWebServer()

	// Создаем сессию с nil Analysis (такое может быть если сессия создана но не инициализирована)
	session := &VibeCodingSession{
		UserID:         123,
		ProjectName:    "test-project",
		StartTime:      time.Now(),
		Files:          make(map[string]string),
		GeneratedFiles: make(map[string]string),
		Analysis:       nil, // Специально nil для тестирования
	}

	// Добавляем сессию напрямую в менеджер
	sessionManager.sessions[123] = session

	// Создаем веб-сервер
	webServer := NewWebServer(sessionManager, 8081)

	// Создаем тестовый HTTP запрос
	req, err := http.NewRequest("GET", "/api/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ResponseRecorder для записи ответа
	rr := httptest.NewRecorder()

	// Вызываем handleSessions - здесь НЕ должно быть паники
	webServer.handleSessions(rr, req)

	// Проверяем что запрос прошел успешно (код 200)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Парсим JSON ответ
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Проверяем структуру ответа
	sessions, ok := response["sessions"].([]interface{})
	if !ok {
		t.Fatal("Expected 'sessions' field to be an array")
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// Проверяем первую сессию
	sessionData := sessions[0].(map[string]interface{})

	// Проверяем что language установлен в "Unknown" вместо паники
	language, ok := sessionData["language"].(string)
	if !ok {
		t.Fatal("Expected 'language' field to be a string")
	}

	if language != "Unknown" {
		t.Errorf("Expected language to be 'Unknown', got '%s'", language)
	}

	// Проверяем другие поля
	if sessionData["user_id"] != float64(123) { // JSON unmarshaling converts numbers to float64
		t.Errorf("Expected user_id to be 123, got %v", sessionData["user_id"])
	}

	if sessionData["project_name"] != "test-project" {
		t.Errorf("Expected project_name to be 'test-project', got %v", sessionData["project_name"])
	}
}

// TestHandleSessionsWithValidAnalysis тестирует handleSessions с валидным Analysis
func TestHandleSessionsWithValidAnalysis(t *testing.T) {
	// Создаем менеджер сессий без веб-сервера
	sessionManager := NewSessionManagerWithoutWebServer()

	// Создаем сессию с валидным Analysis
	session := &VibeCodingSession{
		UserID:         456,
		ProjectName:    "python-project",
		StartTime:      time.Now(),
		Files:          make(map[string]string),
		GeneratedFiles: make(map[string]string),
		Analysis: &codevalidation.CodeAnalysisResult{
			Language: "Python",
		},
	}

	// Добавляем сессию
	sessionManager.sessions[456] = session

	// Создаем веб-сервер
	webServer := NewWebServer(sessionManager, 8082)

	// Создаем тестовый HTTP запрос
	req, err := http.NewRequest("GET", "/api/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ResponseRecorder
	rr := httptest.NewRecorder()

	// Вызываем handleSessions
	webServer.handleSessions(rr, req)

	// Проверяем успешный ответ
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Парсим JSON ответ
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Проверяем что есть сессии
	sessions := response["sessions"].([]interface{})
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// Проверяем что language правильно установлен
	sessionData := sessions[0].(map[string]interface{})
	language := sessionData["language"].(string)

	if language != "Python" {
		t.Errorf("Expected language to be 'Python', got '%s'", language)
	}
}
