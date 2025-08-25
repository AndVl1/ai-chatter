# GitHub Token Setup Guide

## 🔑 GitHub Personal Access Token для AI Chatter

Для работы команд `/release_rc` и `/ai_release` необходим GitHub Personal Access Token с соответствующими разрешениями.

## 📝 Пошаговая инструкция

### 1. Создание Classic Personal Access Token

1. **Перейдите в настройки GitHub:**
   - Откройте https://github.com/settings/tokens
   - Или: Profile → Settings → Developer settings → Personal access tokens → Tokens (classic)

2. **Создайте новый токен:**
   - Нажмите **"Generate new token"** → **"Generate new token (classic)"**
   - **Note:** Введите описание, например: "AI Chatter Bot - Release Management"
   - **Expiration:** Выберите срок действия (рекомендуется 90 дней или 1 год)

3. **Выберите разрешения:**
   ```
   ✅ repo                    # Полный доступ к репозиториям
     ✅ repo:status           # Доступ к статусу коммитов
     ✅ repo_deployment       # Доступ к развертываниям
     ✅ public_repo           # Доступ к публичным репозиториям
     ✅ repo:invite           # Доступ к приглашениям
     ✅ security_events       # Доступ к событиям безопасности
   ```

4. **Создайте токен:**
   - Нажмите **"Generate token"**
   - **⚠️ Важно:** Скопируйте токен немедленно! Он больше не будет показан.

### 2. Alternative: Fine-grained Personal Access Token

1. **Создайте Fine-grained токен:**
   - Перейдите в **Personal access tokens** → **Fine-grained tokens**
   - Нажмите **"Generate new token"**

2. **Настройте доступ:**
   - **Repository access:** "Selected repositories" → выберите нужные репозитории
   - **Repository permissions:**
     - **Contents:** Read
     - **Metadata:** Read
     - **Pull requests:** Read (опционально)

## 🔧 Настройка в AI Chatter

### 1. Локальная разработка

Добавьте токен в файл `.env`:
```bash
# GitHub Integration
GITHUB_TOKEN=ghp_your_actual_token_here
```

### 2. Docker

Добавьте токен в файл `.env` или передайте через переменную окружения:
```bash
docker run -e GITHUB_TOKEN=ghp_your_token_here ai-chatter:latest
```

### 3. Docker Compose

```yaml
environment:
  - GITHUB_TOKEN=ghp_your_token_here
```

## 🚨 Форматы токенов

### Classic Personal Access Token
```
ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```
- Длина: 40 символов
- Префикс: `ghp_`

### Fine-grained Personal Access Token  
```
github_pat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```
- Длина: ~93 символа
- Префикс: `github_pat_`

### OAuth Token
```
gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```
- Длина: 40 символов  
- Префикс: `gho_`

## ✅ Проверка настройки

После настройки токена в логах вы увидите:
```
🔍 Bot: Checking GitHub token...
📦 Bot: GITHUB_TOKEN available: true
🔑 Bot: GitHub token: ghp_...xxxx (length: 40)
✅ GitHub MCP client connected successfully
```

## ❌ Типичные ошибки

### Error 401: Bad credentials
```
❌ GitHub API error 401: {"message":"Bad credentials"}
```

**Причины:**
- Неверный токен
- Токен истек
- Недостаточные разрешения
- Токен не передан

**Решение:**
1. Проверьте токен в `.env` файле
2. Убедитесь что токен не истек
3. Проверьте разрешения `repo`
4. Перезапустите бот после изменения `.env`

### Error 403: Rate limit exceeded
```
❌ GitHub API error 403: {"message":"API rate limit exceeded"}
```

**Причины:**
- Токен не настроен (используется public API)
- Превышен лимит для токена

**Решение:**
1. Настройте GITHUB_TOKEN
2. Для public API: 60 запросов/час
3. Для авторизованных запросов: 5000 запросов/час

## 🔐 Безопасность

- **Никогда не коммитьте токены в git**
- **Используйте `.env` файлы (они в .gitignore)**
- **Регулярно меняйте токены**
- **Предоставляйте минимальные необходимые разрешения**
- **Следите за сроком действия токенов**

## 📞 Поддержка

Если проблемы остаются:
1. Проверьте логи бота
2. Убедитесь что токен правильно скопирован
3. Проверьте разрешения репозитория
4. Создайте новый токен для тестирования
