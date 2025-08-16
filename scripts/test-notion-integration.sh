#!/bin/bash

# Скрипт для запуска интеграционных тестов с Notion MCP
set -e

echo "🧪 Notion MCP Integration Test"
echo "================================"

# Проверяем наличие .env файла
if [ -f ".env" ]; then
    echo "📄 Loading environment from .env file..."
    source .env
else
    echo "⚠️  No .env file found, using system environment variables"
fi

# Проверяем необходимые переменные
echo ""
echo "🔍 Checking environment variables..."

if [ -z "$NOTION_TOKEN" ]; then
    echo "❌ NOTION_TOKEN is not set"
    echo "   Please set your Notion integration token"
    echo "   Get it from: https://developers.notion.com"
    exit 1
else
    echo "✅ NOTION_TOKEN is set"
fi

if [ -z "$NOTION_TEST_PAGE_ID" ]; then
    echo "⚠️  NOTION_TEST_PAGE_ID is not set"
    echo "   Integration tests will be skipped"
    echo "   To run full tests, set this to a valid Notion page ID"
    echo ""
    echo "📖 How to get page ID:"
    echo "   1. Open a page in Notion"
    echo "   2. Copy the page URL"
    echo "   3. Extract the ID: https://notion.so/workspace/Page-Name-{THIS_IS_THE_ID}"
    echo "   4. Give integration access: Share → Connect to integration"
    echo ""
    echo "📋 See docs/notion-parent-page-setup.md for detailed instructions"
    echo ""
    TEST_WILL_SKIP=true
else
    echo "✅ NOTION_TEST_PAGE_ID is set: $NOTION_TEST_PAGE_ID"
    TEST_WILL_SKIP=false
fi

echo ""

# Запускаем MCP сервер в background если его нет
MCP_SERVER_PID=""
if ! pgrep -f "notion-mcp-server" > /dev/null; then
    echo "🚀 Starting MCP server in background..."
    
    # Собираем сервер если нужно
    if [ ! -f "./notion-mcp-server" ]; then
        echo "🔨 Building MCP server..."
        go build -o notion-mcp-server cmd/notion-mcp-server/main.go
    fi
    
    # Запускаем сервер
    ./notion-mcp-server 2>/dev/null &
    MCP_SERVER_PID=$!
    
    # Ждём запуска
    echo "⏳ Waiting for MCP server to start..."
    sleep 2
    
    if ! kill -0 $MCP_SERVER_PID 2>/dev/null; then
        echo "❌ Failed to start MCP server"
        exit 1
    fi
    
    echo "✅ MCP server started (PID: $MCP_SERVER_PID)"
else
    echo "✅ MCP server already running"
fi

echo ""

# Функция очистки
cleanup() {
    if [ -n "$MCP_SERVER_PID" ]; then
        echo ""
        echo "🧹 Stopping MCP server..."
        kill $MCP_SERVER_PID 2>/dev/null || true
        wait $MCP_SERVER_PID 2>/dev/null || true
        echo "✅ MCP server stopped"
    fi
}

# Регистрируем очистку при выходе
trap cleanup EXIT

# Запускаем тесты
echo "🧪 Running integration tests..."
echo ""

if [ "$TEST_WILL_SKIP" = "true" ]; then
    echo "⚠️  Running limited tests (NOTION_TEST_PAGE_ID not set)..."
    go test -v ./internal/notion -run "TestMCPConnection|TestRequiredEnvironmentVariables"
else
    echo "🚀 Running full integration tests..."
    go test -v ./internal/notion -run "TestMCP"
fi

TEST_EXIT_CODE=$?

echo ""

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "🎉 All tests passed!"
    
    if [ "$TEST_WILL_SKIP" = "true" ]; then
        echo ""
        echo "💡 To run full integration tests:"
        echo "   1. Set NOTION_TEST_PAGE_ID in your .env file"
        echo "   2. Make sure the integration has access to that page"
        echo "   3. Run this script again"
    fi
else
    echo "❌ Tests failed with exit code: $TEST_EXIT_CODE"
    
    echo ""
    echo "🔧 Troubleshooting:"
    echo "   1. Check that NOTION_TOKEN is valid"
    echo "   2. Verify NOTION_TEST_PAGE_ID points to an existing page"
    echo "   3. Ensure the integration has access to the test page"
    echo "   4. Check MCP server logs for errors"
fi

exit $TEST_EXIT_CODE
