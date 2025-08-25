# RuStore API v1 Migration Guide

## 🚀 Обновление до актуальной версии RuStore API

AI Chatter обновлен для работы с актуальной версией [RuStore API v1](https://www.rustore.ru/help/work-with-rustore-api/api-upload-publication-app/create-draft-version).

## 📋 Основные изменения

### 1. **Endpoint обновлен**
```bash
# Старый (неактуальный):
POST /application/{app_id}/version

# Новый (актуальный):
POST /public/v1/application/{packageName}/version
```

### 2. **Изменения параметров**

#### Основной идентификатор:
- ❌ `app_id` → ✅ `package_name` (например: `com.myapp.example`)

#### Новые обязательные заголовки:
- ❌ `Authorization: Bearer {token}` → ✅ `Public-Token: {token}`

### 3. **Новые поля запроса**

#### Основная информация:
```json
{
  "appName": "Название приложения",
  "appType": "GAMES или MAIN",
  "categories": ["health", "news"],
  "ageLegal": "6+",
  "shortDescription": "Краткое описание (80 символов)",
  "fullDescription": "Полное описание (4000 символов)",
  "whatsNew": "Что нового (5000 символов)",
  "moderInfo": "Комментарий для модератора (180 символов)"
}
```

#### Коммерческие параметры:
```json
{
  "priceValue": 8799,  // Цена в копейках (87.99 руб = 8799)
  "seoTagIds": [100, 102]  // ID SEO тегов (макс 5)
}
```

#### Параметры публикации:
```json
{
  "publishType": "MANUAL|INSTANTLY|DELAYED",
  "publishDateTime": "2022-07-08T13:24:41.8328711+03:00",  // Для DELAYED
  "partialValue": 5  // Процент частичной публикации: 5, 10, 25, 50, 75, 100
}
```

### 4. **Новый формат ответа**

#### Старый формат:
```json
{
  "body": {
    "versionId": "243242",
    "versionName": "1.0.6",
    "versionCode": 42,
    "status": "draft"
  }
}
```

#### Новый формат:
```json
{
  "code": "OK",
  "message": null,
  "body": 243242,  // Version ID как число
  "timestamp": "2023-07-27T10:28:59.039649+03:00"
}
```

## 🔧 Обновления в AI Chatter

### MCP Server (`cmd/rustore-mcp-server/main.go`)
- ✅ Обновлен endpoint на `/public/v1/application/{packageName}/version`
- ✅ Заголовок `Public-Token` вместо `Authorization`
- ✅ Новые параметры в `RuStoreCreateDraftParams`
- ✅ Обновленная структура ответа `RuStoreDraftResponse`

### MCP Client (`internal/rustore/mcp.go`)
- ✅ Обновлена структура `CreateDraftParams`
- ✅ Поддержка всех новых полей API v1
- ✅ Динамическое формирование запроса (только непустые поля)
- ✅ Обратная совместимость с существующими клиентами

## 🎯 Практическое использование

### Команда `/release_rc`
Теперь поддерживает все новые поля RuStore API v1:

```
/release_rc
📦 Репозиторий: AndVl1/SnakeGame
🏪 Package Name: com.andvl1.snakegame
📱 App Type: GAMES
🏷️ Categories: arcade, puzzle
🔞 Age Rating: 12+
💰 Price: Free
📄 Descriptions: Auto-generated
🚀 Publish Type: MANUAL
```

### Команда `/ai_release`
AI Agent теперь может заполнить все поля автоматически:

```
/ai_release
🤖 AI будет заполнять:
✅ App Name, Type, Categories
✅ Age Rating, Descriptions
✅ SEO Tags, Price Value
✅ Publish Type, Partial Value
```

## ⚙️ Настройка

### Environment Variables
```bash
# RuStore API v1 настройки
RUSTORE_COMPANY_ID=your_company_id
RUSTORE_KEY_ID=your_key_id  
RUSTORE_KEY_SECRET=your_key_secret
RUSTORE_MCP_SERVER_PATH=./bin/rustore-mcp-server
```

### Получение токена авторизации
Следуйте [официальной документации RuStore](https://www.rustore.ru/help/work-with-rustore-api/api-authorization-token) для получения токена.

## 📚 Полная документация

- [RuStore API v1 - Создание черновика](https://www.rustore.ru/help/work-with-rustore-api/api-upload-publication-app/create-draft-version)
- [Список категорий приложений](https://www.rustore.ru/help/work-with-rustore-api/api-upload-publication-app/app-category-list)
- [Список поисковых тегов](https://www.rustore.ru/help/work-with-rustore-api/api-upload-publication-app/app-tag-list)

## 🔄 Обратная совместимость

Все существующие интеграции продолжат работать благодаря автоматическому маппингу:
- `app_id` → `package_name`
- Старые поля переводятся в новые
- Метаданные ответов адаптируются

## 🎉 Преимущества обновления

1. **Актуальность**: Соответствие последней версии RuStore API
2. **Расширенность**: Поддержка всех новых возможностей
3. **Гибкость**: Новые параметры публикации и коммерциализации
4. **AI-готовность**: Полная интеграция с AI Release System
5. **Надежность**: Правильные заголовки и форматы запросов

Теперь AI Chatter полностью готов к работе с современной RuStore платформой! 🚀
