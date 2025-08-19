#!/bin/bash

# Test script for Gmail MCP server
echo "🧪 Testing Gmail MCP Server..."

# Check if credentials are configured
if [ -z "$GMAIL_CREDENTIALS_JSON" ] && [ -z "$GMAIL_CREDENTIALS_JSON_PATH" ]; then
    echo "❌ Gmail credentials not configured"
    echo "Please set either GMAIL_CREDENTIALS_JSON or GMAIL_CREDENTIALS_JSON_PATH"
    echo ""
    echo "To setup:"
    echo "1. Create credentials.json from Google Cloud Console"
    echo "2. Run: ./gmail-auth-helper credentials.json"
    echo "3. Copy output to .env file"
    exit 1
fi

# Build the Gmail MCP server
echo "🔨 Building Gmail MCP server..."
go build -o gmail-mcp-server cmd/gmail-mcp-server/main.go

if [ $? -ne 0 ]; then
    echo "❌ Failed to build Gmail MCP server"
    exit 1
fi

echo "✅ Gmail MCP server built successfully"

# Test the server in dry-run mode
echo "🚀 Testing Gmail MCP server initialization..."

# Run the server with a timeout to see if it initializes
timeout 10s ./gmail-mcp-server &
SERVER_PID=$!

sleep 5

if kill -0 $SERVER_PID 2>/dev/null; then
    echo "✅ Gmail MCP server started successfully"
    kill $SERVER_PID 2>/dev/null
else
    echo "❌ Gmail MCP server failed to start"
    exit 1
fi

echo "✅ Gmail MCP server test completed successfully"
echo ""
echo "📋 Next steps:"
echo "1. Configure complete bot setup in .env"
echo "2. Run: ./ai-chatter"
echo "3. Test /gmail_summary command"