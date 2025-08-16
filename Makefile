# AI Chatter - Makefile
.PHONY: help build test clean ci format integration cross coverage

# Переменные
GO_VERSION := $(shell go version | cut -d' ' -f3)
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT := $(shell git rev-parse --short HEAD)

# Цвета для вывода
BLUE := \033[34m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
NC := \033[0m # No Color

help: ## Показать эту справку
	@echo "$(BLUE)AI Chatter - Available commands:$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "$(GREEN)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Build info:$(NC)"
	@echo "  Go version: $(GO_VERSION)"
	@echo "  Git commit: $(GIT_COMMIT)"
	@echo "  Build time: $(BUILD_TIME)"

build: ## Собрать все приложения
	@echo "$(BLUE)🔨 Building applications...$(NC)"
	@go build -o ai-chatter cmd/bot/main.go
	@go build -o notion-mcp-server cmd/notion-mcp-server/main.go
	@go build -o test-custom-mcp cmd/test-custom-mcp/main.go
	@echo "$(GREEN)✅ Build completed$(NC)"

test: ## Запустить unit тесты
	@echo "$(BLUE)🧪 Running unit tests...$(NC)"
	@go test -race -v ./...

coverage: ## Запустить тесты с coverage
	@echo "$(BLUE)📊 Running tests with coverage...$(NC)"
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✅ Coverage report: coverage.html$(NC)"

integration: ## Запустить интеграционные тесты
	@echo "$(BLUE)🌐 Running integration tests...$(NC)"
	@./scripts/test-notion-integration.sh

format: ## Проверить и исправить форматирование кода
	@echo "$(BLUE)🎨 Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✅ Code formatted$(NC)"

clean: ## Очистить артефакты сборки
	@echo "$(BLUE)🧹 Cleaning up...$(NC)"
	@rm -f ai-chatter notion-mcp-server test-custom-mcp
	@rm -f coverage.out coverage.html *.prof *.log
	@echo "$(GREEN)✅ Cleanup completed$(NC)"

ci: ## Запустить полный CI pipeline локально
	@echo "$(BLUE)🚀 Running full CI pipeline...$(NC)"
	@./scripts/ci-local.sh

ci-fast: format build test ## Быстрая проверка (форматирование + сборка + unit тесты)
	@echo "$(GREEN)✅ Fast CI check completed$(NC)"

cross: ## Проверить cross-platform сборку
	@echo "$(BLUE)🌍 Cross-platform build check...$(NC)"
	@GOOS=linux GOARCH=amd64 go build -o /dev/null cmd/bot/main.go
	@GOOS=darwin GOARCH=amd64 go build -o /dev/null cmd/bot/main.go  
	@GOOS=windows GOARCH=amd64 go build -o /dev/null cmd/bot/main.go
	@echo "$(GREEN)✅ Cross-platform builds OK$(NC)"

deps: ## Обновить зависимости
	@echo "$(BLUE)📦 Updating dependencies...$(NC)"
	@go mod download
	@go mod tidy
	@echo "$(GREEN)✅ Dependencies updated$(NC)"

lint: ## Запустить линтер (если установлен)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(BLUE)🔍 Running linter...$(NC)"; \
		golangci-lint run ./...; \
		echo "$(GREEN)✅ Linting completed$(NC)"; \
	else \
		echo "$(YELLOW)⚠️ golangci-lint not installed$(NC)"; \
		echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

dev: build ## Запустить в режиме разработки
	@echo "$(BLUE)🚀 Starting development server...$(NC)"
	@echo "$(YELLOW)Loading .env file...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./ai-chatter

mcp-server: build ## Запустить MCP сервер
	@echo "$(BLUE)🔌 Starting MCP server...$(NC)"
	@if [ -f .env ]; then set -a && source .env && set +a; fi
	@./notion-mcp-server

install: build ## Установить в GOPATH/bin
	@echo "$(BLUE)📥 Installing to GOPATH/bin...$(NC)"
	@go install cmd/bot/main.go
	@go install cmd/notion-mcp-server/main.go
	@echo "$(GREEN)✅ Installation completed$(NC)"

docker-build: ## Собрать Docker образ
	@echo "$(BLUE)🐳 Building Docker image...$(NC)"
	@docker build -t ai-chatter:$(GIT_COMMIT) .
	@docker tag ai-chatter:$(GIT_COMMIT) ai-chatter:latest
	@echo "$(GREEN)✅ Docker image built$(NC)"

release: clean format build test cross ## Подготовить релиз
	@echo "$(BLUE)🏷️ Preparing release...$(NC)"
	@echo "$(YELLOW)Version: $(GIT_COMMIT)$(NC)"
	@echo "$(YELLOW)Build time: $(BUILD_TIME)$(NC)"
	@echo "$(GREEN)✅ Release ready$(NC)"

benchmark: ## Запустить бенчмарки
	@echo "$(BLUE)⚡ Running benchmarks...$(NC)"
	@go test -bench=. -benchmem ./... | tee benchmark-results.txt
	@echo "$(GREEN)✅ Benchmarks completed$(NC)"

profile-cpu: ## CPU профилирование
	@echo "$(BLUE)🔥 CPU profiling...$(NC)"
	@go test -cpuprofile=cpu.prof -run=TestMCPConnection ./internal/notion
	@if [ -f cpu.prof ]; then \
		echo "$(GREEN)📊 CPU profile: cpu.prof$(NC)"; \
		echo "View with: go tool pprof cpu.prof"; \
	fi

profile-mem: ## Memory профилирование  
	@echo "$(BLUE)🧠 Memory profiling...$(NC)"
	@go test -memprofile=mem.prof -run=TestMCPConnection ./internal/notion
	@if [ -f mem.prof ]; then \
		echo "$(GREEN)📊 Memory profile: mem.prof$(NC)"; \
		echo "View with: go tool pprof mem.prof"; \
	fi

# Aliases для удобства
all: ci ## Alias для 'ci'
check: ci-fast ## Alias для 'ci-fast'
fmt: format ## Alias для 'format'
