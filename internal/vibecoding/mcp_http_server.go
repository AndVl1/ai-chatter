package vibecoding

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// VibeCodingMCPHTTPServer HTTP-based MCP server for VibeCoding
type VibeCodingMCPHTTPServer struct {
	sessionManager *SessionManager
	port           int
}

// NewVibeCodingMCPHTTPServer creates a new HTTP MCP server
func NewVibeCodingMCPHTTPServer(sessionManager *SessionManager, port int) *VibeCodingMCPHTTPServer {
	return &VibeCodingMCPHTTPServer{
		sessionManager: sessionManager,
		port:           port,
	}
}

// Start starts the HTTP MCP server
func (s *VibeCodingMCPHTTPServer) Start(ctx context.Context) error {
	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "vibecoding-http-mcp",
		Version: "1.0.0",
	}, nil)

	// Create VibeCoding MCP server instance
	vibeCodingServer := NewVibeCodingMCPHTTPServerInstance(s.sessionManager)

	// Register all VibeCoding tools
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_list_files",
		Description: "Lists all files in the VibeCoding workspace for the specified user",
	}, vibeCodingServer.ListFiles)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_read_file",
		Description: "Reads the content of a specific file in the VibeCoding workspace",
	}, vibeCodingServer.ReadFile)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_write_file",
		Description: "Writes content to a file in the VibeCoding workspace",
	}, vibeCodingServer.WriteFile)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_execute_command",
		Description: "Executes a command in the VibeCoding container environment",
	}, vibeCodingServer.ExecuteCommand)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_validate_code",
		Description: "Validates code syntax and compilation in the VibeCoding environment",
	}, vibeCodingServer.ValidateCode)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_run_tests",
		Description: "Runs tests in the VibeCoding environment",
	}, vibeCodingServer.RunTests)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibe_get_session_info",
		Description: "Gets information about the current VibeCoding session",
	}, vibeCodingServer.GetSessionInfo)

	log.Printf("🔗 VibeCoding MCP HTTP server registered %d tools", 7)

	// TODO: HTTP transport not yet available in MCP SDK
	// For now, we'll use stdio transport through subprocess
	// This will be updated when HTTP transport becomes available
	
	log.Printf("⚠️ HTTP transport not implemented yet - using stdio fallback")
	log.Printf("✅ VibeCoding MCP server initialized (stdio mode)")
	return nil
}

// Stop stops the HTTP MCP server
func (s *VibeCodingMCPHTTPServer) Stop(ctx context.Context) error {
	// TODO: Implement server shutdown when HTTP transport is available
	log.Printf("🔌 VibeCoding HTTP MCP server stop requested")
	return nil
}

// GetHTTPTransportURL returns the URL for HTTP MCP transport
func (s *VibeCodingMCPHTTPServer) GetHTTPTransportURL() string {
	return fmt.Sprintf("http://localhost:%d/mcp", s.port)
}

// NewVibeCodingMCPHTTPServerInstance creates server instance with tool implementations
func NewVibeCodingMCPHTTPServerInstance(sessionManager *SessionManager) *VibeCodingMCPHTTPServerInstance {
	return &VibeCodingMCPHTTPServerInstance{sessionManager: sessionManager}
}

// VibeCodingMCPHTTPServerInstance implements MCP tool handlers
type VibeCodingMCPHTTPServerInstance struct {
	sessionManager *SessionManager
}

// ListFiles implements vibe_list_files tool
func (s *VibeCodingMCPHTTPServerInstance) ListFiles(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	// TODO: Implement using existing VibeCoding server logic
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// ReadFile implements vibe_read_file tool  
func (s *VibeCodingMCPHTTPServerInstance) ReadFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// WriteFile implements vibe_write_file tool
func (s *VibeCodingMCPHTTPServerInstance) WriteFile(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// ExecuteCommand implements vibe_execute_command tool
func (s *VibeCodingMCPHTTPServerInstance) ExecuteCommand(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// ValidateCode implements vibe_validate_code tool
func (s *VibeCodingMCPHTTPServerInstance) ValidateCode(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// RunTests implements vibe_run_tests tool
func (s *VibeCodingMCPHTTPServerInstance) RunTests(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}

// GetSessionInfo implements vibe_get_session_info tool
func (s *VibeCodingMCPHTTPServerInstance) GetSessionInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[any], error) {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "🔧 VibeCoding HTTP MCP tools not yet implemented"},
		},
	}, nil
}