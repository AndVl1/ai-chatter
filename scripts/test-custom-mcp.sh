#!/bin/bash

# Скрипт для тестирования кастомного Notion MCP сервера

set -e

echo "🧪 Testing Custom Notion MCP Server"
echo "===================================="

ENV_FILE="./.env"

# Check if the .env file exists and load it
if [ -f "$ENV_FILE" ]; then
  source "$ENV_FILE"
  echo "✅ Environment loaded from $ENV_FILE"
else
  echo "⚠️ Warning: .env file not found at $ENV_FILE"
fi

# Проверяем наличие NOTION_TOKEN
if [ -z "$NOTION_TOKEN" ]; then
    echo "❌ Error: NOTION_TOKEN environment variable is required"
    echo "💡 Please set it with your Notion integration token:"
    echo "   export NOTION_TOKEN=secret_xxxxx"
    echo ""
    echo "📖 How to get token:"
    echo "   1. Go to https://developers.notion.com/docs/authorization"
    echo "   2. Create new integration"
    echo "   3. Copy the integration token"
    exit 1
fi

echo "✅ NOTION_TOKEN is set"

# Проверяем наличие NOTION_TEST_PAGE_ID
if [ -z "$NOTION_TEST_PAGE_ID" ]; then
    echo "❌ Error: NOTION_TEST_PAGE_ID environment variable is required"
    echo "💡 Please set it with your Notion test page ID:"
    echo "   export NOTION_TEST_PAGE_ID=your-page-id"
    echo ""
    echo "📖 How to get page ID:"
    echo "   1. Open a page in Notion"
    echo "   2. Copy the page URL"
    echo "   3. Extract ID from URL: https://notion.so/workspace/Page-Name-{THIS_IS_THE_ID}"
    echo "   4. Give integration access: Share → Connect to integration"
    echo ""
    echo "📋 See docs/notion-parent-page-setup.md for detailed instructions"
    exit 1
fi

echo "✅ NOTION_TEST_PAGE_ID is set"

# Собираем проект
echo ""
echo "🔨 Building custom MCP server..."
go build -o notion-mcp-server cmd/notion-mcp-server/main.go
echo "✅ MCP server built successfully"

echo ""
echo "🔨 Building test client..."
go build -o test-custom-mcp cmd/test-custom-mcp/main.go
echo "✅ Test client built successfully"

# Запускаем тест
echo ""
echo "🚀 Running custom MCP integration test..."
echo "🔗 This will spawn our custom MCP server as subprocess"
echo ""

# Передаём переменные окружения в тест
NOTION_TOKEN="$NOTION_TOKEN" NOTION_TEST_PAGE_ID="$NOTION_TEST_PAGE_ID" ./test-custom-mcp

echo ""
echo "🎉 Custom MCP Server test completed successfully!"
echo "📋 Your custom Notion MCP server is working perfectly"
