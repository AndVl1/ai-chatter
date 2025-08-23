#!/bin/bash

# Скрипт для запуска VibeCoding веб-интерфейса

set -e

echo "🚀 Starting VibeCoding Web Interface with MCP communication"

# Проверяем что Go установлен
if ! command -v go &> /dev/null; then
    echo "❌ Go is required but not installed"
    exit 1
fi

# Проверяем что Docker установлен
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is required but not installed"
    exit 1
fi

# Проверяем что docker-compose установлен
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose is required but not installed"
    exit 1
fi

# Переходим в корневую директорию проекта
cd "$(dirname "$0")/.."

# Собираем VibeCoding MCP сервер
echo "🔧 Building VibeCoding MCP server..."
go build -o ./cmd/vibecoding-mcp-server/vibecoding-mcp-server ./cmd/vibecoding-mcp-server/

if [ ! -f "./cmd/vibecoding-mcp-server/vibecoding-mcp-server" ]; then
    echo "❌ Failed to build VibeCoding MCP server"
    exit 1
fi

echo "✅ VibeCoding MCP server built successfully"

# Создаем необходимые директории
mkdir -p /tmp/vibecoding-mcp

# Запускаем docker-compose
echo "🐳 Starting VibeCoding containers..."
docker-compose -f docker-compose.vibecoding.yml up --build -d

# Проверяем статус контейнеров
echo "📋 Checking container status..."
docker-compose -f docker-compose.vibecoding.yml ps

echo ""
echo "✅ VibeCoding Web Interface started successfully!"
echo ""
echo "🌐 Web Interface: http://localhost:3000"
echo "📋 API Status: http://localhost:3000/api/status"
echo ""
echo "📜 To view logs:"
echo "   docker-compose -f docker-compose.vibecoding.yml logs -f"
echo ""
echo "🛑 To stop:"
echo "   docker-compose -f docker-compose.vibecoding.yml down"
echo ""