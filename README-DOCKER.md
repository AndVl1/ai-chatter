# 🐳 Docker Setup для AI Chatter

Этот документ описывает, как запустить AI Chatter в Docker контейнере.

## Быстрый старт

### 1. Настройка переменных окружения

Скопируйте и настройте переменные окружения:
```bash
cp env.example .env
```

Обязательно настройте следующие переменные в `.env`:
```env
# Telegram
TELEGRAM_BOT_TOKEN=your_telegram_bot_token
ADMIN_USER_ID=your_telegram_user_id

# LLM Provider (выберите один)
OPENAI_API_KEY=sk-your_openai_key     # для OpenAI/OpenRouter
# или
YANDEX_OAUTH_TOKEN=your_yandex_token  # для YandexGPT

# Notion (для отчётов и сохранения диалогов)
NOTION_TOKEN=secret_your_notion_token
NOTION_PARENT_PAGE_ID=your-page-id
```

### 2. Запуск

```bash
# Сборка и запуск
docker-compose up -d

# Просмотр логов
docker-compose logs -f ai-chatter-bot

# Остановка
docker-compose down
```

## Архитектура

Docker setup включает:
- **AI Chatter Bot** - основной Telegram бот
- **Custom MCP Server** - встроенный Notion MCP сервер
- **Volumes** - персистентное хранение данных и логов

```
┌─────────────────────────────────┐
│         Docker Container        │
│  ┌─────────────┐ ┌─────────────┐│
│  │ AI Chatter  │ │ Notion MCP  ││ 
│  │    Bot      │◄─┤   Server    ││
│  └─────────────┘ └─────────────┘│
│                                 │
│  📁 /app/data   📁 /app/logs    │
└─────────────────────────────────┘
          │              │
    ┌──────────┐   ┌──────────┐
    │ ./data   │   │ ./logs   │
    │ (host)   │   │ (host)   │
    └──────────┘   └──────────┘
```

## Переменные окружения

### Обязательные
- `TELEGRAM_BOT_TOKEN` - токен Telegram бота
- `ADMIN_USER_ID` - ID администратора  
- `OPENAI_API_KEY` или `YANDEX_OAUTH_TOKEN` - API ключ LLM провайдера

### Notion (рекомендуемые)
- `NOTION_TOKEN` - Integration token для Notion
- `NOTION_PARENT_PAGE_ID` - ID родительской страницы

### Опциональные
- `LLM_PROVIDER` - `openai` (по умолчанию) или `yandex`
- `OPENAI_MODEL` - модель OpenAI (по умолчанию `gpt-3.5-turbo`)
- `MESSAGE_PARSE_MODE` - `HTML` (по умолчанию), `Markdown`, или `MarkdownV2`

## Volumes

- `./data:/app/data` - пользовательские данные (allowlist, pending, настройки)
- `./logs:/app/logs` - логи в JSONL формате

## Новые возможности

### 📊 Система отчётности (только для админа)

```bash
# Ручной отчёт за последние сутки
/report

# Автоматические отчёты каждый день в 21:00 UTC
# (админ получает уведомления автоматически)
```

### 🤖 Автоматические Notion функции

LLM автоматически может:
- Сохранять диалоги в Notion (`save_dialog_to_notion`)
- Искать прошлые разговоры (`search_notion`)  
- Создавать новые страницы (`create_notion_page`)
- Создавать отчёты с правильной структурой страниц

### 🔗 Последовательные действия

LLM может выполнять сложные workflows:
1. Поискать страницу "Reports"
2. Создать её если не найдена  
3. Создать отчёт как подстраницу
4. Уведомить пользователя с ссылкой

## Отладка

### Просмотр логов
```bash
# Логи всех сервисов
docker-compose logs -f

# Только логи бота
docker-compose logs -f ai-chatter-bot

# Последние 100 строк
docker-compose logs --tail=100 ai-chatter-bot
```

### Проверка статуса
```bash
# Статус контейнеров
docker-compose ps

# Информация о ресурсах
docker stats
```

### Вход в контейнер
```bash
# Bash в контейнере
docker-compose exec ai-chatter-bot sh

# Проверка процессов
docker-compose exec ai-chatter-bot ps aux
```

### Rebuild
```bash
# Пересборка образа после изменений
docker-compose build --no-cache
docker-compose up -d
```

## Тестирование Docker сборки

```bash
# Тест сборки (без запуска)
./scripts/docker-build-test.sh
```

## Troubleshooting

### Проблема: контейнер не запускается
```bash
# Проверьте логи
docker-compose logs ai-chatter-bot

# Проверьте переменные окружения
docker-compose config
```

### Проблема: нет доступа к Notion
- Убедитесь что `NOTION_TOKEN` правильно настроен
- Проверьте что интеграция добавлена к нужным страницам
- Проверьте что `NOTION_PARENT_PAGE_ID` существует

### Проблема: отчёты не генерируются
- Убедитесь что пользователь является админом (`ADMIN_USER_ID`)
- Проверьте что есть логи для анализа в `./logs/log.jsonl`
- Проверьте что Notion интеграция настроена

### Проблема: большой размер образа
```bash
# Очистка неиспользуемых образов
docker system prune -a

# Анализ размера слоёв
docker history ai-chatter-ai-chatter-bot
```

## Production использование

### Systemd сервис
```ini
# /etc/systemd/system/ai-chatter.service
[Unit]
Description=AI Chatter Bot Docker
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/ai-chatter
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

### Логротация
```bash
# /etc/logrotate.d/ai-chatter
/opt/ai-chatter/logs/*.jsonl {
    daily
    missingok
    rotate 30
    compress
    notifempty
    sharedscripts
    postrotate
        docker-compose -f /opt/ai-chatter/docker-compose.yml restart ai-chatter-bot
    endscript
}
```

### Мониторинг
```bash
# Health check
curl -f http://localhost:8080/health || docker-compose restart ai-chatter-bot

# Мониторинг ресурсов
docker stats --no-stream ai-chatter-bot
```

## Benefits

- ✅ **Простое развёртывание** - один docker-compose команда
- ✅ **Изоляция** - контейнеризованная среда
- ✅ **Персистентность** - сохранение данных между перезапусками
- ✅ **Автоматическая настройка** - MCP сервер настраивается автоматически
- ✅ **Production ready** - оптимизировано для продакшена
- ✅ **Легкий upgrade** - обновление через rebuild образа
