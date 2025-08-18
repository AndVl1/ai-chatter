#!/bin/bash

# Скрипт для тестирования Docker сборки
set -e

echo "🐳 Тестирование Docker сборки AI Chatter бота..."
echo "================================================================"

# Проверяем что Docker доступен
if ! command -v docker &> /dev/null; then
    echo "❌ Docker не найден. Установите Docker для запуска теста."
    exit 1
fi

# Проверяем что docker-compose доступен
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose не найден. Установите docker-compose для запуска теста."
    exit 1
fi

echo "✅ Docker и docker-compose найдены"

# Сборка образа
echo "🔨 Собираем Docker образ..."
docker-compose build ai-chatter-bot

echo "✅ Docker образ успешно собран!"

# Проверяем что образ создался
IMAGE_ID=$(docker images ai-chatter-ai-chatter-bot -q | head -n1)
if [ -z "$IMAGE_ID" ]; then
    echo "❌ Образ не найден после сборки"
    exit 1
fi

echo "✅ Образ создан: $IMAGE_ID"

# Показываем размер образа
SIZE=$(docker images ai-chatter-ai-chatter-bot --format "table {{.Size}}" | tail -n1)
echo "📦 Размер образа: $SIZE"

echo ""
echo "🎉 Docker сборка прошла успешно!"
echo ""
echo "Для запуска бота используйте:"
echo "docker-compose up -d"
echo ""
echo "Не забудьте настроить переменные окружения в .env файле:"
echo "- TELEGRAM_BOT_TOKEN"
echo "- NOTION_TOKEN" 
echo "- NOTION_PARENT_PAGE_ID"
echo "- ADMIN_USER_ID"
echo "- OPENAI_API_KEY (или другой LLM провайдер)"
