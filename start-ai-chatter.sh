#!/bin/bash

# AI Chatter - Startup Script
# Запуск всей системы одной командой

set -e

# Цвета для красивого вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Функция для красивого логирования
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Заголовок
echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║                      🤖 AI CHATTER BOT                       ║"
echo "║                    Полный запуск системы                      ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Проверка Docker
log "Проверяю Docker..."
if ! command -v docker &> /dev/null; then
    error "Docker не установлен! Пожалуйста, установите Docker."
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    error "Docker Compose не установлен! Пожалуйста, установите Docker Compose."
    exit 1
fi

# Проверка .env файла
log "Проверяю конфигурацию..."
if [ ! -f .env ]; then
    warning "Файл .env не найден!"
    log "Создаю .env файл из примера..."
    if [ -f env.example ]; then
        cp env.example .env
        echo -e "${YELLOW}⚠️  ВАЖНО: Отредактируйте файл .env и добавьте необходимые токены!${NC}"
        echo -e "   - TELEGRAM_BOT_TOKEN (обязательно)"
        echo -e "   - NOTION_PARENT_PAGE_ID (для Notion MCP)"
        echo -e "   - GMAIL_* параметры (для Gmail MCP)"
        echo ""
        echo -e "После редактирования .env запустите скрипт снова."
        exit 1
    else
        error "Файл env.example не найден! Создайте .env файл вручную."
        exit 1
    fi
fi

# Проверка TELEGRAM_BOT_TOKEN
if ! grep -q "TELEGRAM_BOT_TOKEN=.*[^=]" .env 2>/dev/null; then
    error "TELEGRAM_BOT_TOKEN не настроен в .env файле!"
    echo -e "Добавьте строку: ${YELLOW}TELEGRAM_BOT_TOKEN=your_bot_token_here${NC}"
    exit 1
fi

# Определение режима запуска
MODE="full"
if [ "$1" = "basic" ]; then
    MODE="basic"
    log "Режим: Только основной бот (без VibeCoding веб-интерфейса)"
elif [ "$1" = "vibecoding" ]; then
    MODE="vibecoding"
    log "Режим: VibeCoding с веб-интерфейсом"
else
    log "Режим: Полная система (бот + VibeCoding + веб-интерфейс)"
fi

# Выбор Docker Compose файла
COMPOSE_FILE="docker-compose.full.yml"
if [ "$MODE" = "basic" ]; then
    COMPOSE_FILE="docker-compose.yml"
elif [ "$MODE" = "vibecoding" ]; then
    COMPOSE_FILE="docker-compose.vibecoding.yml"
fi

log "Используется конфигурация: $COMPOSE_FILE"

# Остановка существующих контейнеров
log "Остановка существующих контейнеров..."
docker-compose -f $COMPOSE_FILE down --remove-orphans 2>/dev/null || true

# Очистка старых образов (опционально)
if [ "$2" = "--clean" ]; then
    warning "Удаление старых Docker образов..."
    docker system prune -f
    docker-compose -f $COMPOSE_FILE build --no-cache
else
    # Сборка образов
    log "Сборка Docker образов..."
    docker-compose -f $COMPOSE_FILE build
fi

# Запуск сервисов
log "Запуск сервисов..."
docker-compose -f $COMPOSE_FILE up -d

# Ожидание готовности сервисов
log "Ожидание готовности сервисов..."
sleep 5

# Проверка статуса сервисов
log "Проверка статуса сервисов..."
if docker-compose -f $COMPOSE_FILE ps | grep -q "Up"; then
    success "Сервисы запущены успешно!"
    
    echo ""
    echo -e "${GREEN}🎉 AI Chatter успешно запущен!${NC}"
    echo ""
    
    # Показываем доступные сервисы
    if [ "$MODE" = "full" ] || [ "$MODE" = "vibecoding" ]; then
        echo -e "${CYAN}📋 Доступные сервисы:${NC}"
        echo -e "   🤖 Telegram Bot: Активен"
        echo -e "   🌐 VibeCoding API: http://localhost:8080"
        echo -e "   🎨 Веб-интерфейс: http://localhost:3000"
        echo ""
    elif [ "$MODE" = "basic" ]; then
        echo -e "${CYAN}📋 Активные сервисы:${NC}"
        echo -e "   🤖 Telegram Bot: Активен"
        echo ""
    fi
    
    echo -e "${CYAN}🔧 Полезные команды:${NC}"
    echo -e "   Логи:        ${YELLOW}docker-compose -f $COMPOSE_FILE logs -f${NC}"
    echo -e "   Остановка:   ${YELLOW}docker-compose -f $COMPOSE_FILE down${NC}"
    echo -e "   Статус:      ${YELLOW}docker-compose -f $COMPOSE_FILE ps${NC}"
    echo ""
    
else
    error "Не удалось запустить некоторые сервисы!"
    echo -e "${YELLOW}Проверьте логи:${NC}"
    echo -e "   ${YELLOW}docker-compose -f $COMPOSE_FILE logs${NC}"
    exit 1
fi

# Опциональный мониторинг логов
if [ "$3" = "--logs" ]; then
    log "Показываю логи сервисов... (Ctrl+C для выхода)"
    sleep 2
    docker-compose -f $COMPOSE_FILE logs -f
fi