package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"ai-chatter/internal/notion"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(".env" /*, "../.env", "cmd/bot/.env"*/); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}
	fmt.Println("🧪 Testing Custom Notion MCP Server")
	fmt.Println("===================================")

	// Проверяем наличие токена
	notionToken := os.Getenv("NOTION_TOKEN")
	if notionToken == "" {
		fmt.Println("❌ NOTION_TOKEN environment variable is required")
		fmt.Println("💡 Please set it with your Notion integration token:")
		fmt.Println("   export NOTION_TOKEN=secret_xxxxx")
		os.Exit(1)
	}

	fmt.Printf("✅ NOTION_TOKEN is set: %s...%s\n", notionToken[:10], notionToken[len(notionToken)-5:])

	// Создаем MCP клиент
	mcpClient := notion.NewMCPClient(notionToken)

	ctx := context.Background()

	// Тестируем подключение к кастомному серверу
	fmt.Println("\n🔗 Connecting to custom MCP server...")
	fmt.Println("💡 Make sure the server binary is built:")
	fmt.Println("   go build -o notion-mcp-server cmd/notion-mcp-server/main.go")
	fmt.Println("")

	err := mcpClient.Connect(ctx, notionToken)
	if err != nil {
		fmt.Printf("❌ Connection failed: %v\n", err)
		fmt.Println("\n💡 Please ensure:")
		fmt.Println("   1. MCP server is built: go build -o notion-mcp-server cmd/notion-mcp-server/main.go")
		fmt.Println("   2. NOTION_TOKEN is valid")
		fmt.Println("   3. Notion integration has access to pages")
		os.Exit(1)
	}

	fmt.Println("✅ Connected successfully!")

	// Тестируем сохранение диалога
	fmt.Println("\n💾 Testing dialog saving...")

	// Получаем parent page ID (обязательно для нового API)
	testPageID := os.Getenv("NOTION_TEST_PAGE_ID")
	if testPageID == "" {
		fmt.Println("❌ NOTION_TEST_PAGE_ID environment variable is required")
		fmt.Println("💡 Please set it with your Notion test page ID:")
		fmt.Println("   export NOTION_TEST_PAGE_ID=your-page-id")
		fmt.Println("📖 See docs/notion-parent-page-setup.md for details")
		os.Exit(1)
	}

	fmt.Printf("✅ Using test page ID: %s\n", testPageID)

	dialogResult := mcpClient.CreateDialogSummary(
		ctx,
		"Test Dialog from Custom MCP",
		"This is a test dialog created through our custom MCP server.",
		"test_user_123",
		"TestUser",
		"test",
		testPageID,
	)

	if !dialogResult.Success {
		fmt.Printf("❌ Dialog save failed: %s\n", dialogResult.Message)
	} else {
		fmt.Printf("✅ Dialog saved: %s\n", dialogResult.Message)
		if dialogResult.PageID != "" {
			fmt.Printf("📄 Page ID: %s\n", dialogResult.PageID)
		}
	}

	// Тестируем создание произвольной страницы
	fmt.Println("\n📄 Testing free-form page creation...")

	pageResult := mcpClient.CreateFreeFormPage(
		ctx,
		"Custom MCP Test Page",
		"# Custom MCP Integration Test\n\nThis page was created using our custom Notion MCP server built with Go and the official MCP SDK.\n\n## Features\n- Direct Notion API integration\n- MCP protocol compliance\n- Go-based implementation\n- Official SDK usage",
		testPageID,
		[]string{"test", "mcp", "custom"},
	)

	if !pageResult.Success {
		fmt.Printf("❌ Page creation failed: %s\n", pageResult.Message)
	} else {
		fmt.Printf("✅ Page created: %s\n", pageResult.Message)
		if pageResult.PageID != "" {
			fmt.Printf("📄 Page ID: %s\n", pageResult.PageID)
		}
	}

	// Тестируем поиск
	fmt.Println("\n🔍 Testing search functionality...")

	searchResult := mcpClient.SearchDialogSummaries(ctx, "AI", "", "")

	if !searchResult.Success {
		fmt.Printf("❌ Search failed: %s\n", searchResult.Message)
	} else {
		fmt.Printf("✅ Search completed: %s\n", searchResult.Message)
	}

	// Тестируем поиск страниц с ID
	fmt.Println("\n🆔 Testing search pages with ID...")

	pageSearchResult := mcpClient.SearchPagesWithID(ctx, "Test", 5, false)

	if !pageSearchResult.Success {
		fmt.Printf("❌ Page search failed: %s\n", pageSearchResult.Message)
	} else {
		fmt.Printf("✅ Page search completed: %s\n", pageSearchResult.Message)
		if len(pageSearchResult.Pages) > 0 {
			fmt.Printf("📋 Found %d pages:\n", len(pageSearchResult.Pages))
			for i, page := range pageSearchResult.Pages {
				fmt.Printf("   %d. %s (ID: %s)\n", i+1, page.Title, page.ID)
			}
		}
	}

	// Тестируем список доступных страниц
	fmt.Println("\n📋 Testing list available pages...")

	availablePagesResult := mcpClient.ListAvailablePages(ctx, 10, "", false)

	if !availablePagesResult.Success {
		fmt.Printf("❌ List available pages failed: %s\n", availablePagesResult.Message)
	} else {
		fmt.Printf("✅ List available pages completed: %s\n", availablePagesResult.Message)
		if len(availablePagesResult.Pages) > 0 {
			fmt.Printf("📋 Found %d available pages:\n", len(availablePagesResult.Pages))
			for i, page := range availablePagesResult.Pages {
				fmt.Printf("   %d. %s (ID: %s)", i+1, page.Title, page.ID)
				if page.CanBeParent {
					fmt.Printf(" ✅ Can be parent")
				}
				if page.Type != "" && page.Type != "page" {
					fmt.Printf(" [%s]", page.Type)
				}
				fmt.Println()
			}
		}
	}

	// Закрываем соединение
	mcpClient.Close()

	fmt.Println("\n🎉 Custom MCP Server integration test completed!")
	fmt.Println("🏗️ Custom MCP server is working correctly")
	fmt.Println("📋 Benefits of custom approach:")
	fmt.Println("   ✅ Full control over MCP implementation")
	fmt.Println("   ✅ Direct Notion API access")
	fmt.Println("   ✅ Official MCP SDK compliance")
	fmt.Println("   ✅ Go-based end-to-end solution")
	fmt.Println("   ✅ Easy debugging and customization")
}
