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
echo "║               + GitHub & RuStore интеграции                   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Показываем справку если запрос help
if [ "$1" = "help" ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    echo -e "${CYAN}📖 Использование:${NC}"
    echo -e "   ${YELLOW}./start-ai-chatter.sh${NC}              - Полный запуск (бот + VibeCoding + веб)"
    echo -e "   ${YELLOW}./start-ai-chatter.sh basic${NC}        - Только основной бот"
    echo -e "   ${YELLOW}./start-ai-chatter.sh vibecoding${NC}   - Бот + VibeCoding"
    echo -e "   ${YELLOW}./start-ai-chatter.sh [mode] --clean${NC} - С очисткой старых образов"
    echo -e "   ${YELLOW}./start-ai-chatter.sh [mode] --logs${NC}  - С показом логов"
    echo ""
    echo -e "${CYAN}🔌 MCP Интеграции:${NC}"
    echo -e "   📋 Notion - создание заметок и поиск"
    echo -e "   📧 Gmail - анализ писем и обобщение"
    echo -e "   📦 GitHub - работа с релизами (для /release_rc)"
    echo -e "   🏪 RuStore - публикация приложений (для /ai_release)"
    echo ""
    echo -e "${CYAN}⚙️ Настройка .env:${NC}"
    echo -e "   ${YELLOW}TELEGRAM_BOT_TOKEN${NC}=...     (обязательно)"
    echo -e "   ${YELLOW}GITHUB_TOKEN${NC}=...           (для GitHub интеграции)"
    echo -e "   ${YELLOW}RUSTORE_KEY${NC}=...            (для RuStore)"
    echo -e "   ${YELLOW}GMAIL_CREDENTIALS_JSON${NC}=... (для Gmail)"
    echo -e "   ${YELLOW}NOTION_TOKEN${NC}=...           (для Notion)"
    echo ""
    exit 0
fi

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

# Проверка дополнительных MCP токенов
check_mcp_tokens() {
    local missing_tokens=()
    
    # GitHub Token
    if ! grep -q "GITHUB_TOKEN=.*[^=]" .env 2>/dev/null; then
        missing_tokens+=("GITHUB_TOKEN")
    fi
    
    # RuStore credentials (новая упрощенная схема)
    if ! grep -q "RUSTORE_KEY=.*[^=]" .env 2>/dev/null; then
        missing_tokens+=("RUSTORE_KEY")
    fi
    
    # Gmail credentials
    if ! grep -q "GMAIL_CREDENTIALS_JSON.*[^=]" .env 2>/dev/null && ! grep -q "GMAIL_CREDENTIALS_JSON_PATH.*[^=]" .env 2>/dev/null; then
        missing_tokens+=("GMAIL_CREDENTIALS (JSON или PATH)")
    fi
    
    # Notion token
    if ! grep -q "NOTION_TOKEN=.*[^=]" .env 2>/dev/null; then
        missing_tokens+=("NOTION_TOKEN")
    fi
    
    if [ ${#missing_tokens[@]} -gt 0 ]; then
        warning "Некоторые MCP интеграции не настроены:"
        for token in "${missing_tokens[@]}"; do
            echo -e "   ⚠️  $token - соответствующая функциональность будет отключена"
        done
        echo ""
        echo -e "${CYAN}💡 Для полной функциональности добавьте в .env:${NC}"
        echo -e "   ${YELLOW}GITHUB_TOKEN${NC}=your_github_token_here          # для интеграции с GitHub"
        echo -e "   ${YELLOW}RUSTORE_KEY${NC}=your_rustore_api_token         # единый токен для RuStore API"
        echo -e "   ${YELLOW}GMAIL_CREDENTIALS_JSON${NC}='{...}'               # для Gmail интеграции"
        echo -e "   ${YELLOW}NOTION_TOKEN${NC}=your_notion_token               # для Notion интеграции"
        echo ""
    fi
}

# Вызываем проверку MCP токенов
check_mcp_tokens

# Функция для сборки MCP серверов
build_mcp_servers() {
    log "🔧 Сборка MCP серверов..."
    
    # Проверяем наличие Go
    if ! command -v go &> /dev/null; then
        warning "Go не установлен. MCP серверы не будут собраны."
        warning "Установите Go для полной функциональности: https://golang.org/dl/"
        return 1
    fi
    
    # Используем отдельный скрипт сборки
    if [ -f "scripts/build-mcp-servers.sh" ]; then
        ./scripts/build-mcp-servers.sh
    else
        # Fallback к прямой сборке
        mkdir -p ./bin
        log "   📋 Notion MCP Server..."
        go build -o ./bin/notion-mcp-server ./cmd/notion-mcp-server/ 2>/dev/null || true
        log "   📧 Gmail MCP Server..."
        go build -o ./bin/gmail-mcp-server ./cmd/gmail-mcp-server/ 2>/dev/null || true
        log "   📦 GitHub MCP Server..."
        go build -o ./bin/github-mcp-server ./cmd/github-mcp-server/ 2>/dev/null || true
        log "   🏪 RuStore MCP Server..."
        go build -o ./bin/rustore-mcp-server ./cmd/rustore-mcp-server/ 2>/dev/null || true
        log "   🔥 VibeCoding MCP Server..."
        go build -o ./bin/vibecoding-mcp-server ./cmd/vibecoding-mcp-server/ 2>/dev/null || true
        log "   🌐 VibeCoding MCP HTTP Server..."
        go build -o ./bin/vibecoding-mcp-http-server ./cmd/vibecoding-mcp-http-server/ 2>/dev/null || true
        log "   🤖 AI Chatter Bot..."
        go build -o ./bin/ai-chatter-bot ./cmd/bot/ 2>/dev/null || true
        success "MCP серверы собраны!"
    fi
}

# Функция для отображения статуса MCP интеграций
show_mcp_status() {
    # Проверяем Notion
    if grep -q "NOTION_TOKEN=.*[^=]" .env 2>/dev/null && [ -f "./bin/notion-mcp-server" ]; then
        echo -e "   📋 Notion: ${GREEN}✅ Готов${NC} (создание заметок, поиск)"
    else
        echo -e "   📋 Notion: ${YELLOW}⚠️ Не настроен${NC}"
    fi
    
    # Проверяем Gmail
    if (grep -q "GMAIL_CREDENTIALS_JSON.*[^=]" .env 2>/dev/null || grep -q "GMAIL_CREDENTIALS_JSON_PATH.*[^=]" .env 2>/dev/null) && [ -f "./bin/gmail-mcp-server" ]; then
        echo -e "   📧 Gmail: ${GREEN}✅ Готов${NC} (поиск писем, анализ)"
    else
        echo -e "   📧 Gmail: ${YELLOW}⚠️ Не настроен${NC}"
    fi
    
    # Проверяем GitHub
    if grep -q "GITHUB_TOKEN=.*[^=]" .env 2>/dev/null && [ -f "./bin/github-mcp-server" ]; then
        echo -e "   📦 GitHub: ${GREEN}✅ Готов${NC} (релизы, скачивание AAB)"
    else
        echo -e "   📦 GitHub: ${YELLOW}⚠️ Не настроен${NC} (для /release_rc)"
    fi
    
    # Проверяем RuStore (новая схема с RUSTORE_KEY)
    if grep -q "RUSTORE_KEY=.*[^=]" .env 2>/dev/null && [ -f "./bin/rustore-mcp-server" ]; then
        echo -e "   🏪 RuStore: ${GREEN}✅ Готов${NC} (публикация приложений через /ai_release)"
    else
        echo -e "   🏪 RuStore: ${YELLOW}⚠️ Не настроен${NC} (для /ai_release)"
    fi
}

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
    
    # Сборка MCP серверов с очисткой
    log "Сборка MCP серверов с очисткой..."
    rm -rf ./bin
    build_mcp_servers
else
    # Сборка образов
    log "Сборка Docker образов..."
    docker-compose -f $COMPOSE_FILE build
    
    # Сборка MCP серверов
    log "Сборка MCP серверов..."
    build_mcp_servers
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
        echo -e "${CYAN}🔌 MCP Интеграции:${NC}"
        show_mcp_status
        echo ""
    elif [ "$MODE" = "basic" ]; then
        echo -e "${CYAN}📋 Активные сервисы:${NC}"
        echo -e "   🤖 Telegram Bot: Активен"
        echo ""
        echo -e "${CYAN}🔌 MCP Интеграции:${NC}"
        show_mcp_status
        echo ""
    fi
    
    echo -e "${CYAN}🔧 Полезные команды:${NC}"
    echo -e "   Логи:        ${YELLOW}docker-compose -f $COMPOSE_FILE logs -f${NC}"
    echo -e "   Остановка:   ${YELLOW}docker-compose -f $COMPOSE_FILE down${NC}"
    echo -e "   Статус:      ${YELLOW}docker-compose -f $COMPOSE_FILE ps${NC}"
    echo -e "   Пересборка:  ${YELLOW}./start-ai-chatter.sh --clean${NC}"
    echo -e "   MCP Серверы: ${YELLOW}./scripts/build-mcp-servers.sh${NC}"
    echo ""
    
    echo -e "${CYAN}🤖 Новые команды бота:${NC}"
    echo -e "   📦 ${YELLOW}/release_rc${NC}     - Публикация Release Candidate в RuStore"
    echo -e "   📧 ${YELLOW}/gmail_summary${NC}  - Анализ и обобщение писем Gmail"
    echo -e "   📋 ${YELLOW}/notion_save${NC}    - Сохранение в Notion"
    echo -e "   📋 ${YELLOW}/notion_search${NC}  - Поиск в Notion"
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