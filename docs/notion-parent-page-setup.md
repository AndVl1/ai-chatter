# 📋 Настройка NOTION_PARENT_PAGE_ID

## Обязательная настройка для MCP интеграции

Согласно официальному API Notion, при создании страниц **обязательно** нужно указывать родительскую страницу через `parent_page_id`. Наша интеграция следует этому требованию.

## Как получить Parent Page ID

### Метод 1: Из URL страницы в браузере

1. **Откройте Notion** в браузере
2. **Перейдите на страницу**, которая будет родительской для ваших новых страниц
3. **Скопируйте URL** из адресной строки

URL будет выглядеть так:
```
https://www.notion.so/workspace/My-Page-Name-12345678901234567890123456789012
```

4. **ID страницы** - это последние 32 символа:
```
12345678901234567890123456789012
```

Или в формате с дефисами:
```
12345678-9012-3456-7890-123456789012
```

### Метод 2: Через API запрос

```bash
# Поиск страниц в workspace
curl -X POST 'https://api.notion.com/v1/search' \
  -H 'Authorization: Bearer secret_your_token' \
  -H 'Content-Type: application/json' \
  -H 'Notion-Version: 2022-06-28' \
  --data '{
    "query": "название вашей страницы"
  }'
```

В ответе найдите нужную страницу и скопируйте её `id`.

### Метод 3: Создание новой страницы

1. **Создайте новую страницу** в Notion для ваших диалогов
2. **Назовите её**, например: "AI Chatter Dialogs"
3. **Дайте доступ интеграции** к этой странице:
   - Нажмите "Share" → "Connect to integration" → выберите вашу интеграцию
4. **Скопируйте ID** из URL

## Настройка переменной окружения

### В .env файле:
```bash
NOTION_PARENT_PAGE_ID=12345678-9012-3456-7890-123456789012
```

### Или экспорт:
```bash
export NOTION_PARENT_PAGE_ID=12345678-9012-3456-7890-123456789012
```

## Проверка настройки

### 1. Тест подключения
```bash
# Убедитесь что переменные установлены
echo $NOTION_TOKEN
echo $NOTION_PARENT_PAGE_ID

# Запустите тест
./scripts/test-custom-mcp.sh
```

### 2. Ожидаемый результат при правильной настройке:
```
✅ Dialog saved: Dialog 'Test Dialog from Custom MCP' saved to Notion
✅ Page created: Successfully created page 'Custom MCP Test Page' in Notion
```

### 3. Ошибки при неправильной настройке:

**Если NOTION_PARENT_PAGE_ID не установлен:**
```
❌ parent_page_id is required - get it from your Notion workspace
```

**Если parent page ID неверный:**
```
❌ Failed to create page: Notion API error 400: {"object":"error","status":400,"code":"validation_error","message":"parent page not found"}
```

**Если интеграция не имеет доступа к странице:**
```
❌ Failed to create page: Notion API error 403: {"object":"error","status":403,"code":"unauthorized","message":"integration does not have access to the parent page"}
```

## Решение проблем

### Проблема: "parent page not found"

**Причина:** Неверный ID страницы или страница была удалена

**Решение:**
1. Проверьте правильность ID
2. Убедитесь что страница существует
3. Проверьте формат ID (32 символа или с дефисами)

### Проблема: "integration does not have access"

**Причина:** Интеграция не подключена к странице

**Решение:**
1. Откройте родительскую страницу в Notion
2. Нажмите "Share" (или три точки → "Settings" → "Share")
3. "Connect to integration" → выберите вашу интеграцию
4. Убедитесь что интеграция появилась в списке

### Проблема: "invalid parent object"

**Причина:** Неправильная структура parent в API

**Решение:** Убедитесь что используете обновлённую версию MCP сервера с правильным форматом:
```json
{
  "parent": {
    "type": "page_id",
    "page_id": "ваш-parent-page-id"
  }
}
```

## Пример полной настройки

```bash
# .env file
NOTION_TOKEN=secret_abc123def456
NOTION_PARENT_PAGE_ID=550e8400-e29b-41d4-a716-446655440000

# Тест
export NOTION_TOKEN=secret_abc123def456
export NOTION_PARENT_PAGE_ID=550e8400-e29b-41d4-a716-446655440000
./test-custom-mcp
```

## Безопасность

⚠️ **Важно**: 
- Никогда не коммитьте реальные токены в git
- Используйте `.env` файл и добавьте его в `.gitignore`
- Parent page ID не является секретом, но лучше тоже держать в `.env`

## Структура страниц

После настройки ваши диалоги будут создаваться как дочерние страницы:

```
📄 AI Chatter Dialogs (parent page)
  ├── 📝 Dialog: Обсуждение MCP
  ├── 📝 Dialog: Вопросы по API
  ├── 📄 Custom MCP Test Page
  └── 📄 Другие созданные страницы
```

Это обеспечивает организацию и позволяет легко найти все созданные ботом страницы.
