#!/bin/bash

# Скрипт для запуска локального Notion MCP сервера

set -e

ENV_FILE="./.env"

# Check if the .env file exists
if [ -f "$ENV_FILE" ]; then
  # Source the .env file to load variables
  source "$ENV_FILE"
else
  echo "Error: .env file not found at $ENV_FILE"
  exit 1
fi

echo "🐳 Starting Local Notion MCP Server"
echo "===================================="

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

# Проверяем наличие Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed"
    echo "📖 Please install Docker: https://docs.docker.com/get-docker/"
    exit 1
fi

echo "✅ Docker is available"

# Формируем заголовки для MCP
HEADERS="{\"Authorization\": \"Bearer $NOTION_TOKEN\", \"Notion-Version\": \"2022-06-28\"}"

echo "🚀 Starting Notion MCP server..."
echo "🔗 Server will be available at: http://localhost:3000"
echo "🛑 Press Ctrl+C to stop"
echo ""

# Запускаем Docker контейнер
sudo docker run --rm -i \
    --name ai-chatter-notion-mcp \
    -p 3000:3000 \
    -e "$NOTION_TOKEN" \
    mcp/notion:latest

echo ""
echo "🛑 Notion MCP server stopped"
