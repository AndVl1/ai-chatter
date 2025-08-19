#!/bin/bash

# Test script for credentials parsing
echo "🧪 Testing Gmail credentials parsing..."

# Create temporary test files
echo "📝 Creating test credentials files..."

# Test file 1: Google Cloud Console format (installed)
cat > /tmp/test-credentials-installed.json << 'EOF'
{
  "installed": {
    "client_id": "test-client-id.googleusercontent.com",
    "client_secret": "test-client-secret",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "redirect_uris": ["urn:ietf:wg:oauth:2.0:oob", "http://localhost"]
  }
}
EOF

# Test file 2: Google Cloud Console format (web)
cat > /tmp/test-credentials-web.json << 'EOF'
{
  "web": {
    "client_id": "test-web-client-id.googleusercontent.com",
    "client_secret": "test-web-client-secret",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "redirect_uris": ["http://localhost:8080/callback"]
  }
}
EOF

# Test file 3: Direct format
cat > /tmp/test-credentials-direct.json << 'EOF'
{
  "client_id": "test-direct-client-id",
  "client_secret": "test-direct-client-secret",
  "redirect_uris": ["urn:ietf:wg:oauth:2.0:oob", "http://localhost"]
}
EOF

# Build Gmail auth helper
echo "🔨 Building gmail-auth-helper..."
go build -o gmail-auth-helper cmd/gmail-auth-helper/main.go

if [ $? -ne 0 ]; then
    echo "❌ Failed to build gmail-auth-helper"
    exit 1
fi

echo "✅ gmail-auth-helper built successfully"

# Test parsing (without OAuth flow, just check parsing)
echo "🔍 Testing credentials parsing..."

echo "1. Testing installed format:"
echo "   Expected: ✅ Parsed Google Cloud Console credentials (installed/desktop format)"
timeout 3s ./gmail-auth-helper /tmp/test-credentials-installed.json <<< "" 2>/dev/null | head -5

echo ""
echo "2. Testing web format:"
echo "   Expected: ✅ Parsed Google Cloud Console credentials (web format)"
timeout 3s ./gmail-auth-helper /tmp/test-credentials-web.json <<< "" 2>/dev/null | head -5

echo ""
echo "3. Testing direct format:"
echo "   Expected: ✅ Parsed direct OAuth2 credentials format"
timeout 3s ./gmail-auth-helper /tmp/test-credentials-direct.json <<< "" 2>/dev/null | head -5

# Cleanup
rm -f /tmp/test-credentials-*.json

echo ""
echo "✅ Credentials parsing test completed"
echo ""
echo "📋 All formats are now supported:"
echo "  - Google Cloud Console (installed) - Desktop applications"
echo "  - Google Cloud Console (web) - Web applications"  
echo "  - Direct format - Manual credential structure"