#!/bin/bash

# Локальная имитация CI процесса
set -e

echo "🚀 AI Chatter - Local CI Pipeline"
echo "=================================="

# Переменные
START_TIME=$(date +%s)
FAILED_TESTS=""
COVERAGE_THRESHOLD=75

# Функция для логирования с timestamp
log() {
    echo "[$(date +'%H:%M:%S')] $1"
}

# Функция для проверки exit code
check_result() {
    if [ $? -eq 0 ]; then
        log "✅ $1 - PASSED"
    else
        log "❌ $1 - FAILED"
        FAILED_TESTS="${FAILED_TESTS}$1\n"
        return 1
    fi
}

# Очистка предыдущих артефактов
cleanup() {
    log "🧹 Cleaning up previous artifacts..."
    rm -f coverage.out *.prof *.log
    rm -f ai-chatter notion-mcp-server test-custom-mcp
}

# Проверка окружения
check_environment() {
    log "🔍 Checking environment..."
    
    echo "Go version: $(go version)"
    echo "OS: $(uname -s)"
    echo "Architecture: $(uname -m)"
    echo "Working directory: $(pwd)"
    echo "Available memory: $(free -h 2>/dev/null || echo 'N/A (not Linux)')"
    echo "Available disk: $(df -h . | tail -1)"
    
    # Проверяем Go модули
    if [ ! -f go.mod ]; then
        log "❌ go.mod not found! Run from project root."
        exit 1
    fi
    
    log "✅ Environment check passed"
}

# Загрузка зависимостей
download_deps() {
    log "📦 Downloading dependencies..."
    go mod download
    check_result "Dependencies download"
}

# Проверка форматирования
check_formatting() {
    log "🎨 Checking code formatting..."
    
    # Проверяем gofmt
    UNFORMATTED=$(gofmt -l .)
    if [ -n "$UNFORMATTED" ]; then
        log "❌ Code formatting issues found:"
        echo "$UNFORMATTED"
        log "💡 Run: go fmt ./..."
        return 1
    fi
    
    check_result "Code formatting"
}

# Статический анализ (если есть golangci-lint)
static_analysis() {
    if command -v golangci-lint >/dev/null 2>&1; then
        log "🔍 Running static analysis..."
        golangci-lint run ./...
        check_result "Static analysis"
    else
        log "⚠️ golangci-lint not found, skipping static analysis"
        log "💡 Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
    fi
}

# Сборка
build_applications() {
    log "🔨 Building applications..."
    
    go build -o ai-chatter cmd/bot/main.go
    check_result "Bot build"
    
    go build -o notion-mcp-server cmd/notion-mcp-server/main.go  
    check_result "MCP server build"
    
    go build -o test-custom-mcp cmd/test-custom-mcp/main.go
    check_result "Test MCP build"
    
    # Проверяем что файлы созданы
    for binary in ai-chatter notion-mcp-server test-custom-mcp; do
        if [ ! -f "$binary" ]; then
            log "❌ Binary $binary not created"
            return 1
        fi
    done
    
    log "✅ All binaries built successfully"
}

# Unit тесты
run_unit_tests() {
    log "🧪 Running unit tests..."
    
    # Запускаем с coverage и race detection
    go test -race -coverprofile=coverage.out ./...
    check_result "Unit tests"
    
    # Анализируем coverage
    if [ -f coverage.out ]; then
        COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        log "📊 Code coverage: ${COVERAGE}%"
        
        if (( $(echo "$COVERAGE < $COVERAGE_THRESHOLD" | bc -l) )); then
            log "⚠️ Coverage below threshold (${COVERAGE}% < ${COVERAGE_THRESHOLD}%)"
        else
            log "✅ Coverage meets threshold (${COVERAGE}% >= ${COVERAGE_THRESHOLD}%)"
        fi
        
        # Генерируем HTML отчёт
        go tool cover -html=coverage.out -o coverage.html
        log "📋 Coverage report: coverage.html"
    fi
}

# Интеграционные тесты
run_integration_tests() {
    log "🌐 Checking integration test setup..."
    
    if [ -n "$NOTION_TOKEN" ] && [ -n "$NOTION_TEST_PAGE_ID" ]; then
        log "🚀 Running integration tests..."
        chmod +x scripts/test-notion-integration.sh
        ./scripts/test-notion-integration.sh
        check_result "Integration tests"
    else
        log "⚠️ Skipping integration tests - environment not configured"
        log "💡 Set NOTION_TOKEN and NOTION_TEST_PAGE_ID to run integration tests"
    fi
}

# Проверка производительности
performance_check() {
    log "⚡ Running performance checks..."
    
    # Простой benchmark если есть
    if go test -bench=. ./... >/dev/null 2>&1; then
        log "📈 Running benchmarks..."
        go test -bench=. -benchmem ./... | tee benchmark-results.txt
        check_result "Benchmarks"
    else
        log "⚠️ No benchmarks found"
    fi
    
    # Проверяем размер бинарей
    log "📏 Binary sizes:"
    for binary in ai-chatter notion-mcp-server test-custom-mcp; do
        if [ -f "$binary" ]; then
            SIZE=$(du -h "$binary" | cut -f1)
            echo "  - $binary: $SIZE"
        fi
    done
}

# Cross-platform проверка
cross_platform_check() {
    log "🌍 Cross-platform build check..."
    
    # Проверяем сборку для разных платформ
    platforms=("linux/amd64" "darwin/amd64" "windows/amd64")
    
    for platform in "${platforms[@]}"; do
        GOOS=${platform%/*}
        GOARCH=${platform#*/}
        
        log "Building for $GOOS/$GOARCH..."
        env GOOS=$GOOS GOARCH=$GOARCH go build -o /dev/null cmd/bot/main.go
        check_result "Build for $GOOS/$GOARCH"
    done
}

# Финальный отчёт
final_report() {
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    echo ""
    echo "📊 CI Pipeline Summary"
    echo "======================"
    echo "Total duration: ${DURATION}s"
    echo "Timestamp: $(date)"
    echo ""
    
    if [ -z "$FAILED_TESTS" ]; then
        echo "🎉 All checks passed! Ready for deployment."
        echo ""
        echo "✅ Completed successfully:"
        echo "  - Environment check"
        echo "  - Dependencies download"  
        echo "  - Code formatting"
        echo "  - Application builds"
        echo "  - Unit tests"
        if [ -n "$NOTION_TOKEN" ]; then
            echo "  - Integration tests"
        fi
        echo "  - Performance checks"
        echo "  - Cross-platform builds"
        echo ""
        echo "📋 Generated artifacts:"
        echo "  - coverage.html (code coverage report)"
        echo "  - coverage.out (coverage data)"
        if [ -f benchmark-results.txt ]; then
            echo "  - benchmark-results.txt (performance data)"
        fi
        echo ""
        return 0
    else
        echo "❌ Some checks failed:"
        echo -e "$FAILED_TESTS"
        echo ""
        echo "🔧 Please fix the issues and run again."
        return 1
    fi
}

# Обработка аргументов
case "${1:-all}" in
    "env")
        check_environment
        ;;
    "deps")
        download_deps
        ;;
    "format") 
        check_formatting
        ;;
    "build")
        build_applications
        ;;
    "test")
        run_unit_tests
        ;;
    "integration")
        run_integration_tests
        ;;
    "performance")
        performance_check
        ;;
    "cross")
        cross_platform_check
        ;;
    "clean")
        cleanup
        ;;
    "all"|*)
        echo "Running full CI pipeline..."
        echo ""
        
        cleanup
        check_environment
        download_deps
        check_formatting
        static_analysis
        build_applications  
        run_unit_tests
        run_integration_tests
        performance_check
        cross_platform_check
        final_report
        exit $?
        ;;
esac
