#!/bin/bash

# Build script for multi-MCP setup
echo "🔨 Building AI Chatter with multi-MCP support..."

# Build main bot
echo "📱 Building main bot..."
go build -o ai-chatter cmd/bot/main.go

# Build Notion MCP server
echo "📝 Building Notion MCP server..."
go build -o notion-mcp-server cmd/notion-mcp-server/main.go

# Build Gmail MCP server  
echo "📧 Building Gmail MCP server..."
go build -o gmail-mcp-server cmd/gmail-mcp-server/main.go

# Build Gmail auth helper
echo "🔑 Building Gmail auth helper..."
go build -o gmail-auth-helper cmd/gmail-auth-helper/main.go

# Set executable permissions
chmod +x ai-chatter notion-mcp-server gmail-mcp-server gmail-auth-helper

echo "✅ Build completed!"
echo ""
echo "📋 Built binaries:"
echo "  - ai-chatter (main bot)"
echo "  - notion-mcp-server (Notion MCP)"
echo "  - gmail-mcp-server (Gmail MCP)"
echo "  - gmail-auth-helper (Gmail OAuth2 setup)"
echo ""
echo "🚀 To run locally:"
echo "  ./ai-chatter"
echo ""
echo "🐳 To run with Docker:"
echo "  docker-compose up -d --build"
echo ""
echo "🔑 To setup Gmail OAuth2:"
echo "  1. Create credentials.json from Google Cloud Console"
echo "  2. Run: ./gmail-auth-helper credentials.json"
echo "  3. Copy output to .env file"
echo ""
echo "⚙️  Don't forget to configure .env file with:"
echo "  - GMAIL_CREDENTIALS_JSON (for Gmail integration)"
echo "  - GMAIL_REFRESH_TOKEN (for automated auth)"
echo "  - NOTION_TOKEN (for Notion integration)"
echo "  - Other required variables from .env.example"