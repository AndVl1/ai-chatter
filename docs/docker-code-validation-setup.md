# Docker Code Validation Setup

Данный документ описывает настройку Docker-in-Docker для полноценной валидации кода в AI Chatter боте.

## 🐳 Docker-in-Docker Architecture

AI Chatter бот теперь поддерживает **полноценную валидацию кода** через Docker-in-Docker (DinD) setup:

### ✅ Что работает автоматически:
- **Smart Code Detection**: LLM автоматически обнаруживает код в сообщениях
- **Archive Processing**: Поддержка ZIP, TAR, TAR.GZ архивов с проектами
- **Progress Tracking**: Real-time обновления процесса валидации
- **Mock Mode Fallback**: Graceful режим анализа кода без Docker
- **Multi-Language Support**: Python, JavaScript, Go, Java и другие

### 🚀 Docker-in-Docker Features:
- **Real Code Execution**: Запуск линтеров, тестов, сборки в изолированных контейнерах
- **Language-Specific Images**: Автоматический выбор подходящих образов
- **Dependency Installation**: Автоматическая установка зависимостей (pip, npm, go mod)
- **Security Isolation**: Полная изоляция выполняемого кода
- **Resource Management**: Automatic cleanup контейнеров

## 🏗️ Architecture Overview

```
┌─────────────────────────────────────┐
│            Host Docker              │
│  ┌─────────────────────────────────┐│
│  │      AI Chatter Container       ││
│  │  ┌─────────────────────────────┐││
│  │  │     Docker Daemon (DinD)    │││
│  │  │  ┌─────────────────────────┐│││
│  │  │  │   Code Validation       ││││
│  │  │  │   (python:3.11-slim)    ││││
│  │  │  └─────────────────────────┘│││
│  │  └─────────────────────────────┘││
│  └─────────────────────────────────┘│
└─────────────────────────────────────┘
```

## 📋 Setup Requirements

### Prerequisites:
1. **Docker** >= 20.10.0
2. **Docker Compose** >= 2.0
3. **Host OS**: Linux/macOS with kernel support for privileged containers
4. **Hardware**: Минимум 2GB RAM, 1GB свободного места

## 🚀 Quick Setup

### 1. Standard Docker Compose Setup:

```bash
# Клонирование и запуск
git clone <repo-url>
cd ai-chatter
cp .env.example .env
# Настройте переменные окружения в .env

# Запуск с Docker-in-Docker support
docker-compose up -d
```

### 2. Проверка Docker-in-Docker:

```bash
# Проверка статуса контейнера
docker-compose logs ai-chatter-bot | head -20

# Ожидаемый успешный вывод:
# 🐳 Starting Docker daemon...
# ⏳ Waiting for Docker daemon to start...
# ✅ Docker daemon is ready
# 🤖 Starting AI Chatter bot with Docker support...
```

## ⚙️ Configuration

### Docker Compose Settings:

```yaml
services:
  ai-chatter-bot:
    # ✅ Критически важные настройки для DinD:
    privileged: true              # Требуется для Docker daemon
    cap_add:
      - SYS_ADMIN                # Системные привилегии
    
    # 📊 Лимиты ресурсов (рекомендуется)
    deploy:
      resources:
        limits:
          memory: 2G              # Максимум памяти
          cpus: '1.0'            # Максимум CPU
        reservations:
          memory: 512M            # Минимум памяти
          cpus: '0.5'            # Минимум CPU
    
    # 💾 Volumes для сохранения Docker образов
    volumes:
      - ai-chatter-docker-data:/var/lib/docker
```

### Environment Variables:

```bash
# Стандартные настройки (в .env)
TELEGRAM_BOT_TOKEN=your-bot-token
LLM_PROVIDER=openai
OPENAI_API_KEY=your-api-key

# Docker-specific (автоматически)
DOCKER_TLS_CERTDIR=              # Отключить TLS для внутреннего использования
DOCKER_HOST=unix:///var/run/docker.sock
```

## 🔧 Manual Docker Setup (Alternative)

Если Docker Compose не работает на вашей системе:

### 1. Build Image:
```bash
docker build -t ai-chatter-dind .
```

### 2. Run with DinD:
```bash
docker run -d \
  --name ai-chatter-bot \
  --privileged \
  --cap-add SYS_ADMIN \
  -v ai-chatter-data:/app/data \
  -v ai-chatter-logs:/app/logs \
  -v ai-chatter-docker:/var/lib/docker \
  --env-file .env \
  --restart unless-stopped \
  ai-chatter-dind
```

### 3. Monitor Startup:
```bash
# Проверка логов запуска
docker logs ai-chatter-bot -f
```

## 🐛 Troubleshooting

### Common Issues:

#### 1. "Docker daemon failed to start within 30 seconds"
```bash
# Проверка: достаточно ли ресурсов?
docker stats ai-chatter-bot

# Решение: увеличить memory limit в docker-compose.yml
deploy:
  resources:
    limits:
      memory: 4G  # Увеличить до 4GB
```

#### 2. "Permission denied" при запуске контейнеров
```bash
# Проверка: privileged mode включен?
docker inspect ai-chatter-bot | grep -i privileged
# Должно показать: "Privileged": true

# Решение: пересоздать контейнер с правильными настройками
docker-compose down
docker-compose up -d
```

#### 3. Контейнер запускается но code validation не работает
```bash
# Проверка: Docker daemon запущен внутри контейнера?
docker exec -it ai-chatter-bot docker info

# Если не работает - проверить логи
docker exec -it ai-chatter-bot cat /tmp/dockerd.log
```

### Fallback Mode:
Если Docker-in-Docker не запускается, бот автоматически переходит в **Mock Mode**:
- ✅ Code detection работает
- ✅ Project analysis работает  
- ✅ Progress tracking работает
- ⚠️ Реальное выполнение кода недоступно
- 💡 Пользователи получают анализ + рекомендации по установке Docker

## 📊 Performance Monitoring

### Resource Usage:
```bash
# Мониторинг ресурсов
docker stats ai-chatter-bot

# Ожидаемые значения:
# - Memory: 200-800MB (base) + 200-500MB per validation
# - CPU: 5-10% (idle) + 20-50% during validation
# - Network: Low (~1-5MB/hour)
```

### Storage Management:
```bash
# Проверка использования места Docker образами
docker exec -it ai-chatter-bot docker system df

# Очистка неиспользуемых образов (автоматически выполняется)
docker exec -it ai-chatter-bot docker system prune -f
```

## 🔒 Security Considerations

### Privileged Mode:
- ⚠️ **Внимание**: `privileged: true` дает контейнеру полный доступ к host системе
- ✅ **Безопасность**: Код выполняется в двойной изоляции (Host→DinD→Code Container)
- 🔧 **Рекомендация**: Запускать только на доверенных серверах

### Code Isolation:
- ✅ Каждая валидация кода запускается в отдельном контейнере
- ✅ Автоматическая очистка контейнеров после выполнения
- ✅ Ограничения по времени выполнения (timeout)
- ✅ Ограничения по размеру файлов (1MB per file, 50 files per archive)

## 🚀 Production Deployment

### Recommended Setup:
```yaml
# docker-compose.prod.yml
services:
  ai-chatter-bot:
    privileged: true
    restart: unless-stopped
    
    # Production resource limits
    deploy:
      resources:
        limits:
          memory: 3G
          cpus: '1.5'
        reservations:
          memory: 1G
          cpus: '0.5'
    
    # Health checks
    healthcheck:
      test: ["CMD", "docker", "info"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
    
    # Logging
    logging:
      driver: json-file
      options:
        max-size: "100m"
        max-file: "3"
```

### Monitoring & Alerts:
```bash
# Health check script
#!/bin/bash
if ! docker exec ai-chatter-bot docker info >/dev/null 2>&1; then
    echo "ALERT: Docker-in-Docker not working in ai-chatter-bot"
    # Send notification to admin
fi
```

## 🎯 User Experience

### When Docker-in-Docker Works:
1. **User sends code** → Auto-detected by LLM
2. **Real-time progress** → 5 steps with live updates
3. **Actual execution** → Linting, testing, building in isolated container
4. **Comprehensive results** → Real errors, warnings, suggestions
5. **Resource cleanup** → Automatic container removal

### When Docker-in-Docker Fails:
1. **User sends code** → Auto-detected by LLM  
2. **Real-time progress** → 5 steps with live updates
3. **Mock analysis** → LLM-based code analysis without execution
4. **Helpful results** → Analysis + recommendation to install Docker
5. **Graceful degradation** → Feature still useful for code review

## 📞 Support

При проблемах с Docker-in-Docker setup:

1. **Проверить системные требования**
2. **Убедиться что Docker Host поддерживает privileged containers**
3. **Проверить логи**: `docker-compose logs ai-chatter-bot`
4. **Fallback на mock mode** если проблемы критичны

Docker-in-Docker setup значительно улучшает функциональность валидации кода, но бот остается полностью работоспособным и без него.