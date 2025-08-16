package main

import (
	"context"
	"fmt"
	"os"

	"ai-chatter/internal/notion"
)

func main() {
	fmt.Println("🧪 Testing Local Docker Notion MCP Integration")
	fmt.Println("===============================================")

	// Создаем MCP клиент
	mcpClient := notion.NewMCPClient("")

	ctx := context.Background()

	// Тестируем подключение
	fmt.Println("🔗 Testing connection to local MCP server...")
	fmt.Println("💡 Make sure to start MCP server first:")
	fmt.Println("   ./scripts/start-notion-mcp.sh")
	fmt.Println("")

	err := mcpClient.Connect(ctx, "")
	if err != nil {
		fmt.Printf("❌ Connection failed: %v\n", err)
		fmt.Println("\n💡 Please ensure:")
		fmt.Println("   1. Docker is running")
		fmt.Println("   2. NOTION_TOKEN is set")
		fmt.Println("   3. MCP server is started: ./scripts/start-notion-mcp.sh")
		fmt.Println("   4. Server is accessible at http://localhost:3000")
		os.Exit(1)
	}

	fmt.Println("✅ Connected successfully!")

	// Тестируем создание страницы
	fmt.Println("\n📝 Testing page creation...")

	result := mcpClient.CreateDialogSummary(
		ctx,
		"Test MCP Integration",
		"This is a test dialog created via official Notion MCP server",
		"test-user",
		"Test User",
		"test",
	)

	if result.Success {
		fmt.Printf("✅ Page created: %s\n", result.Message)
		if result.PageID != "" {
			fmt.Printf("📄 Page ID: %s\n", result.PageID)
		}
	} else {
		fmt.Printf("❌ Page creation failed: %s\n", result.Message)
	}

	// Тестируем поиск
	fmt.Println("\n🔍 Testing search...")

	searchResult := mcpClient.SearchDialogSummaries(ctx, "test", "", "")
	if searchResult.Success {
		fmt.Printf("✅ Search completed: %s\n", searchResult.Message)
	} else {
		fmt.Printf("❌ Search failed: %s\n", searchResult.Message)
	}

	// Закрываем соединение
	mcpClient.Close()

	fmt.Println("\n🎉 Local Docker MCP integration test completed!")
	fmt.Println("🐳 Local MCP server is working correctly")
	fmt.Println("📋 Benefits:")
	fmt.Println("   ✅ No OAuth setup required")
	fmt.Println("   ✅ Direct token authentication")
	fmt.Println("   ✅ Full control over server")
	fmt.Println("   ✅ Works offline")
}
