#!/bin/bash

# Скрипт для отладки MCP сервера

set -e

echo "🐛 Debug: Testing MCP Server Manually"
echo "====================================="

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
    exit 1
fi

echo "✅ NOTION_TOKEN is set"

# Собираем сервер
echo ""
echo "🔨 Building MCP server..."
go build -o notion-mcp-server cmd/notion-mcp-server/main.go
echo "✅ MCP server built successfully"

echo ""
echo "🧪 Testing MCP server with manual input..."
echo "💡 This will start the server and wait for JSON-RPC input"
echo "🛑 Press Ctrl+C to stop"
echo ""

# Экспортируем токен и запускаем сервер
export NOTION_TOKEN="$NOTION_TOKEN"
echo "🚀 Starting MCP server with NOTION_TOKEN..."

# Запускаем сервер в фоне и отправляем тестовый запрос
(
  sleep 2
  echo "📤 Sending initialize request..."
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}'
  sleep 1
  echo "📤 Sending tools/list request..."
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
  sleep 1
) | ./notion-mcp-server
