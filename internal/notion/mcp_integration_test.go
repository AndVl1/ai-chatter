package notion

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMCPIntegration проводит полноценный интеграционный тест с реальным Notion API
// Требует переменные окружения:
// - NOTION_TOKEN: токен интеграции Notion
// - NOTION_TEST_PAGE_ID: ID страницы для создания тестовых подстраниц
func TestMCPIntegration(t *testing.T) {
	// Проверяем наличие необходимых переменных окружения
	notionToken := os.Getenv("NOTION_TOKEN")
	testPageID := os.Getenv("NOTION_TEST_PAGE_ID")

	if notionToken == "" {
		t.Skip("NOTION_TOKEN not set, skipping integration test")
	}

	if testPageID == "" {
		t.Skip("NOTION_TEST_PAGE_ID not set, skipping integration test")
	}

	// Создаем MCP клиент
	mcpClient := NewMCPClient(notionToken)
	ctx := context.Background()

	// Подключаемся к MCP серверу
	err := mcpClient.Connect(ctx, notionToken)
	if err != nil {
		t.Fatalf("Failed to connect to MCP server: %v", err)
	}
	defer mcpClient.Close()

	// Уникальный суффикс для тестовых страниц
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	testSuffix := fmt.Sprintf("Test_%s", timestamp)

	t.Run("CreateDialogSummary", func(t *testing.T) {
		title := fmt.Sprintf("Integration Test Dialog %s", testSuffix)
		content := `# Test Dialog Summary

**User:** Привет! Как дела?
**Assistant:** Привет! Дела отлично, спасибо! Как у вас дела?
**User:** Тоже хорошо! Можешь сохранить этот диалог?
**Assistant:** Конечно, сохраняю диалог в Notion.

## Test Information
- Test run: ` + timestamp + `
- Purpose: Integration testing of MCP Notion integration
- Expected: Page should be created successfully`

		result := mcpClient.CreateDialogSummary(
			ctx,
			title,
			content,
			"test_user_123",
			"IntegrationTestUser",
			"integration_test",
			testPageID,
		)

		if !result.Success {
			t.Errorf("CreateDialogSummary failed: %s", result.Message)
			return
		}

		t.Logf("✅ Dialog created successfully: %s", result.Message)

		// Проверяем что вернулся page ID
		if result.PageID == "" {
			t.Error("Expected PageID in result, got empty string")
		} else {
			t.Logf("📄 Created page ID: %s", result.PageID)
		}

		// Проверяем что в сообщении есть упоминание об успехе
		if !strings.Contains(result.Message, "saved") && !strings.Contains(result.Message, "created") {
			t.Errorf("Expected success message, got: %s", result.Message)
		}
	})

	t.Run("CreateFreeFormPage", func(t *testing.T) {
		title := fmt.Sprintf("Integration Test Free Page %s", testSuffix)
		content := fmt.Sprintf(`# Integration Test Page

This page was created during integration testing of the Notion MCP server.

## Test Details
- **Timestamp:** %s
- **Test Type:** Free-form page creation
- **Parent Page ID:** %s
- **Test Framework:** Go testing package

## Features Tested
- [x] MCP server connection
- [x] Notion API authentication  
- [x] Page creation with parent
- [x] Content formatting
- [x] Tags assignment

## Expected Results
- Page should be created under the specified parent
- Content should be properly formatted
- Tags should be assigned correctly
- Page ID should be returned

---
*This is an automated test page and can be safely deleted.*`, timestamp, testPageID)

		result := mcpClient.CreateFreeFormPage(
			ctx,
			title,
			content,
			testPageID,
			[]string{"integration-test", "mcp", "automated", timestamp},
		)

		if !result.Success {
			t.Errorf("CreateFreeFormPage failed: %s", result.Message)
			return
		}

		t.Logf("✅ Free-form page created successfully: %s", result.Message)

		// Проверяем что вернулся page ID
		if result.PageID == "" {
			t.Error("Expected PageID in result, got empty string")
		} else {
			t.Logf("📄 Created page ID: %s", result.PageID)
		}

		// Проверяем что в сообщении есть упоминание об успехе
		if !strings.Contains(result.Message, "created") && !strings.Contains(result.Message, "Successfully") {
			t.Errorf("Expected success message, got: %s", result.Message)
		}
	})

	t.Run("SearchWorkspace", func(t *testing.T) {
		// Ищем только что созданные страницы
		searchQuery := fmt.Sprintf("Integration Test %s", testSuffix)

		result := mcpClient.SearchWorkspace(ctx, searchQuery, "", []string{})

		if !result.Success {
			t.Errorf("SearchWorkspace failed: %s", result.Message)
			return
		}

		t.Logf("✅ Search completed: %s", result.Message)

		// Проверяем что поиск нашёл результаты
		// (может быть пустым если индексация ещё не завершилась)
		if strings.Contains(result.Message, "found 0") {
			t.Logf("⚠️  Search returned 0 results (indexing may be in progress)")
		} else {
			t.Logf("🔍 Search found results in workspace")
		}
	})

	t.Run("SearchPagesWithID", func(t *testing.T) {
		// Ищем страницы по названию "Test" (могут быть созданы предыдущими тестами)
		searchResult := mcpClient.SearchPagesWithID(ctx, "Test", 5, false)

		if !searchResult.Success {
			t.Errorf("Page search failed: %s", searchResult.Message)
			return
		}

		t.Logf("✅ Page search completed: %s", searchResult.Message)
		t.Logf("📋 Found %d pages", len(searchResult.Pages))

		for i, page := range searchResult.Pages {
			t.Logf("   %d. %s (ID: %s)", i+1, page.Title, page.ID)

			// Проверяем что ID не пустой
			if page.ID == "" {
				t.Errorf("Page ID is empty for page: %s", page.Title)
			}

			// Проверяем что Title не пустой
			if page.Title == "" {
				t.Errorf("Page title is empty for ID: %s", page.ID)
			}
		}

		// Тестируем точное совпадение с только что созданной страницей
		exactTitle := fmt.Sprintf("Custom Test Page %s", testSuffix)
		exactSearchResult := mcpClient.SearchPagesWithID(ctx, exactTitle, 1, true)
		if exactSearchResult.Success {
			t.Logf("✅ Exact match search: found %d pages", len(exactSearchResult.Pages))
		} else {
			t.Logf("⚠️  Exact match search failed (page may not be indexed yet): %s", exactSearchResult.Message)
		}
	})

	t.Run("ListAvailablePages", func(t *testing.T) {
		// Получаем список доступных страниц
		listResult := mcpClient.ListAvailablePages(ctx, 10, "", false)

		if !listResult.Success {
			t.Errorf("List available pages failed: %s", listResult.Message)
			return
		}

		t.Logf("✅ List available pages completed: %s", listResult.Message)
		t.Logf("📋 Found %d available pages", len(listResult.Pages))

		for i, page := range listResult.Pages {
			t.Logf("   %d. %s (ID: %s, CanBeParent: %t)", i+1, page.Title, page.ID, page.CanBeParent)

			// Проверяем что ID не пустой
			if page.ID == "" {
				t.Errorf("Page ID is empty for page: %s", page.Title)
			}

			// Проверяем что Title не пустой
			if page.Title == "" {
				t.Errorf("Page title is empty for ID: %s", page.ID)
			}
		}

		// Если нашли страницы, можем протестировать создание подстраницы
		if len(listResult.Pages) > 0 {
			parentPage := listResult.Pages[0]
			if parentPage.CanBeParent {
				t.Logf("🧪 Testing subpage creation under: %s", parentPage.Title)

				subPageTitle := fmt.Sprintf("Sub Page Test %s", testSuffix)
				subPageContent := "This is a test subpage created under a specific parent."

				createResult := mcpClient.CreateFreeFormPage(ctx, subPageTitle, subPageContent, parentPage.ID, nil)

				if createResult.Success {
					t.Logf("✅ Subpage created successfully under %s", parentPage.Title)
				} else {
					t.Logf("⚠️  Subpage creation failed: %s", createResult.Message)
				}
			}
		}

		// Тестируем фильтр parent_only
		parentOnlyResult := mcpClient.ListAvailablePages(ctx, 5, "", true)
		if parentOnlyResult.Success {
			t.Logf("✅ Parent-only filter: found %d pages", len(parentOnlyResult.Pages))
			for _, page := range parentOnlyResult.Pages {
				if !page.CanBeParent {
					t.Errorf("Page %s marked as cannot be parent but returned in parent_only filter", page.Title)
				}
			}
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// Тест с некорректным parent page ID
		invalidPageID := "invalid-page-id-format"

		result := mcpClient.CreateFreeFormPage(
			ctx,
			"This Should Fail",
			"This page creation should fail due to invalid parent page ID",
			invalidPageID,
			[]string{"error-test"},
		)

		if result.Success {
			t.Error("Expected CreateFreeFormPage to fail with invalid parent page ID, but it succeeded")
		} else {
			t.Logf("✅ Error handling works correctly: %s", result.Message)
		}

		// Проверяем что ошибка содержит полезную информацию
		if !strings.Contains(result.Message, "error") && !strings.Contains(result.Message, "Error") {
			t.Errorf("Expected error message to contain 'error', got: %s", result.Message)
		}
	})

	t.Logf("🎉 Integration test completed successfully! Created test pages with suffix: %s", testSuffix)
}

// TestMCPConnection проверяет базовое подключение к MCP серверу
func TestMCPConnection(t *testing.T) {
	notionToken := os.Getenv("NOTION_TOKEN")
	if notionToken == "" {
		t.Skip("NOTION_TOKEN not set, skipping connection test")
	}

	mcpClient := NewMCPClient(notionToken)
	ctx := context.Background()

	// Тест подключения
	err := mcpClient.Connect(ctx, notionToken)
	if err != nil {
		t.Fatalf("Failed to connect to MCP server: %v", err)
	}

	// Проверяем что session создалась
	if mcpClient.session == nil {
		t.Error("Expected session to be created, but it's nil")
	}

	// Тест отключения
	err = mcpClient.Close()
	if err != nil {
		t.Errorf("Failed to close MCP connection: %v", err)
	}

	t.Log("✅ MCP connection test passed")
}

// TestRequiredEnvironmentVariables проверяет что все необходимые переменные документированы
func TestRequiredEnvironmentVariables(t *testing.T) {
	requiredVars := []string{
		"NOTION_TOKEN",
		"NOTION_TEST_PAGE_ID",
	}

	missing := make([]string, 0)
	for _, envVar := range requiredVars {
		if os.Getenv(envVar) == "" {
			missing = append(missing, envVar)
		}
	}

	if len(missing) > 0 {
		t.Logf("⚠️  Missing environment variables for full integration testing: %v", missing)
		t.Logf("📖 To run full integration tests, set these variables:")
		for _, envVar := range missing {
			switch envVar {
			case "NOTION_TOKEN":
				t.Logf("   %s=secret_your_notion_integration_token", envVar)
			case "NOTION_TEST_PAGE_ID":
				t.Logf("   %s=12345678-90ab-cdef-1234-567890abcdef", envVar)
			}
		}
		t.Skip("Integration test environment not fully configured")
	}

	t.Log("✅ All required environment variables are set")
}

// Helper function to run integration tests with proper logging
func init() {
	// Настраиваем логирование для тестов
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
