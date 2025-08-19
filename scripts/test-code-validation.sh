#!/bin/bash

# Test script for code validation functionality
echo "🧪 Testing Code Validation Functionality"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed or not in PATH"
    exit 1
fi

echo "✅ Go is available"

# Check if Docker is installed (optional)
if command -v docker &> /dev/null; then
    echo "✅ Docker is available"
    if docker version &> /dev/null; then
        echo "✅ Docker daemon is running"
    else
        echo "⚠️  Docker is installed but daemon is not running"
        echo "   Code validation will work but won't actually execute Docker commands"
    fi
else
    echo "⚠️  Docker is not available"
    echo "   Code validation will work but won't actually execute Docker commands"
fi

echo ""
echo "🔧 Building project..."

# Build the main bot
if go build -o build/ai-chatter cmd/bot/main.go; then
    echo "✅ Main bot built successfully"
else
    echo "❌ Failed to build main bot"
    exit 1
fi

# Build the MCP server
if go build -o build/notion-mcp-server cmd/notion-mcp-server/main.go; then
    echo "✅ Notion MCP server built successfully"
else
    echo "❌ Failed to build Notion MCP server"
    exit 1
fi

echo ""
echo "🧪 Running tests..."

# Run all tests
if go test ./...; then
    echo "✅ All tests passed"
else
    echo "❌ Some tests failed"
    exit 1
fi

echo ""
echo "📊 Test Coverage Analysis..."

# Run tests with coverage
go test -coverprofile=coverage.out ./...

# Show coverage for key packages
echo ""
echo "📈 Coverage for code validation packages:"
go tool cover -func=coverage.out | grep -E "(codevalidation|telegram)" | grep -v "test"

echo ""
echo "🎉 Code validation functionality is ready!"
echo ""
echo "🚀 Key Features Implemented:"
echo "   ✅ Automatic code detection in messages"
echo "   ✅ Multi-language support (Python, JavaScript, Go, Java, etc.)"  
echo "   ✅ Docker CLI integration for code execution"
echo "   ✅ Archive support (ZIP, TAR, TAR.GZ)"
echo "   ✅ File upload handling from Telegram"
echo "   ✅ Real-time progress tracking with 5 steps"
echo "   ✅ LLM-powered project analysis"
echo "   ✅ Dependency installation automation"
echo "   ✅ Comprehensive validation (linting, testing, building)"
echo ""
echo "📝 To use:"
echo "   1. Set up your .env file with required tokens"
echo "   2. Start the bot: ./build/ai-chatter"
echo "   3. Send code in messages or upload files/archives"
echo "   4. Watch automatic validation in real-time!"

# Clean up
rm -f coverage.out

echo ""
echo "✨ Ready for deployment!"