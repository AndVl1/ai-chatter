# 🚀 AI Chatter - Quick Start

Быстрый запуск AI Chatter бота одной командой!

## ⚡ Мгновенный запуск

### 1. Самый простой способ
```bash
make start
```

### 2. Альтернативно через скрипт
```bash
./start-ai-chatter.sh
```

### 3. Только основной бот (без VibeCoding веб-интерфейса)
```bash
make start-basic
```

### 4. С VibeCoding, но без дополнительного веб-интерфейса
```bash
make start-vibe
```

## 🔧 Настройка (первый запуск)

1. **Склонируйте репозиторий:**
   ```bash
   git clone <repository-url>
   cd ai-chatter
   ```

2. **Создайте .env файл:**
   ```bash
   cp env.example .env
   ```

3. **Отредактируйте .env файл:**
   ```bash
   nano .env  # или любой другой редактор
   ```
   
   Обязательно установите:
   - `TELEGRAM_BOT_TOKEN=your_bot_token_here`
   
   Опционально для дополнительных функций:
   - `NOTION_PARENT_PAGE_ID=` (для Notion MCP)
   - `GMAIL_CREDENTIALS_JSON=` (для Gmail MCP)

4. **Запустите систему:**
   ```bash
   make start
   ```

## 📋 Доступные команды

### Основные команды
| Команда | Описание |
|---------|----------|
| `make start` | Запустить всю систему (бот + VibeCoding + веб-интерфейс) |
| `make start-basic` | Только Telegram бот |
| `make start-vibe` | Бот + VibeCoding (без внешнего веб-интерфейса) |
| `make stop` | Остановить всю систему |
| `make restart` | Перезапустить систему |
| `make status` | Показать статус контейнеров |
| `make logs` | Показать логи всех сервисов |

### Команды разработчика
| Команда | Описание |
|---------|----------|
| `make build` | Собрать приложения локально |
| `make test` | Запустить тесты |
| `make docker-build` | Собрать Docker образы |
| `make clean-docker` | Очистить Docker данные |

### Справка
```bash
make help  # Показать все доступные команды
```

## 🌐 Доступные сервисы

После запуска `make start` у вас будет доступно:

- **🤖 Telegram Bot**: Активен в Telegram
- **🌐 VibeCoding API**: http://localhost:8080
- **🎨 Веб-интерфейс VibeCoding**: http://localhost:3000

## 🔍 Мониторинг

### Просмотр логов
```bash
make logs  # Все сервисы
```

### Статус системы
```bash
make status
```

### Логи конкретного сервиса
```bash
docker-compose -f docker-compose.full.yml logs -f ai-chatter
docker-compose -f docker-compose.full.yml logs -f vibecoding-web
```

## 🛠️ Режимы запуска

### 1. Полная система (рекомендуется)
```bash
make start
```
Включает:
- ✅ Telegram бот
- ✅ VibeCoding с Docker поддержкой
- ✅ Notion MCP сервер
- ✅ Gmail MCP сервер
- ✅ Внешний веб-интерфейс на порту 3000
- ✅ VibeCoding API на порту 8080

### 2. Только бот
```bash
make start-basic
```
Включает:
- ✅ Telegram бот
- ✅ Notion MCP сервер
- ✅ Gmail MCP сервер
- ❌ VibeCoding веб-интерфейс

### 3. Бот + VibeCoding (без веб-интерфейса)
```bash
make start-vibe
```
Включает:
- ✅ Telegram бот
- ✅ VibeCoding с Docker поддержкой
- ✅ VibeCoding MCP сервер
- ❌ Внешний веб-интерфейс

## 🐳 Docker конфигурации

- `docker-compose.full.yml` - Полная система
- `docker-compose.vibecoding.yml` - VibeCoding + веб-интерфейс
- `docker-compose.yml` - Только основной бот

## 🔄 Обновление

```bash
git pull
make stop
make docker-build
make start
```

## ❗ Решение проблем

### Порт занят
```bash
make stop  # Остановить все контейнеры
lsof -ti:8080 | xargs kill -9  # Убить процессы на порту 8080
lsof -ti:3000 | xargs kill -9  # Убить процессы на порту 3000
make start
```

### Проблемы с Docker
```bash
make clean-docker  # Очистить Docker данные
make docker-build  # Пересобрать образы
make start
```

### Проблемы с .env
```bash
cp env.example .env
nano .env  # Отредактировать конфигурацию
make start
```

## 📞 Поддержка

- **Логи**: `make logs`
- **Статус**: `make status`
- **Остановка**: `make stop`
- **Справка**: `make help`

---

**🎉 Готово! AI Chatter запущен и готов к работе!**