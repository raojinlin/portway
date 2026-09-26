package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectMCPTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "portway-test", Version: "1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestMCPToolsRespectAccessLevels(t *testing.T) {
	owner, _ := newTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		access string
		want   []string
	}{
		{"read", []string{"get_portway_skill", "get_tunnel", "list_active_connections", "list_connection_history", "list_tunnels", "search_runtime_logs"}},
		{"operate", []string{"get_portway_skill", "get_tunnel", "list_active_connections", "list_connection_history", "list_tunnels", "search_runtime_logs", "start_tunnel", "stop_tunnel"}},
		{"manage", []string{"create_tunnel", "delete_tunnel", "get_portway_skill", "get_tunnel", "list_active_connections", "list_connection_history", "list_tunnels", "search_runtime_logs", "start_tunnel", "stop_tunnel", "update_tunnel"}},
	}
	for _, tc := range cases {
		t.Run(tc.access, func(t *testing.T) {
			session := connectMCPTestClient(t, buildMCPServer(owner, MCPConfig{Access: tc.access}, logger))
			result, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(result.Tools))
			for _, tool := range result.Tools {
				got = append(got, tool.Name)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if len(got) != len(tc.want) {
				t.Fatalf("tools = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("tools = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestMCPListAndSkillTools(t *testing.T) {
	owner, _ := newTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	session := connectMCPTestClient(t, buildMCPServer(owner, MCPConfig{Access: "read"}, logger))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_tunnels", Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("list_tunnels: %+v %v", result, err)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil || len(structured) == 0 || structured[0] != '{' || !bytes.Contains(structured, []byte(`"tunnels"`)) {
		t.Fatalf("list_tunnels structured content must be an object: %s %v", structured, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_portway_skill", Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("get_portway_skill: %+v %v", result, err)
	}
}

func TestMCPAuditLogsProtocolSuccessAndFailureWithoutArguments(t *testing.T) {
	owner, _ := newTestServer(t)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	session := connectMCPTestClient(t, buildMCPServer(owner, MCPConfig{Access: "read"}, logger))
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_runtime_logs", Arguments: map[string]any{"query": "must-not-appear-in-audit"}}); err != nil || result.IsError {
		t.Fatalf("search_runtime_logs: %+v %v", result, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_tunnel", Arguments: map[string]any{"name": "missing-line"}})
	if err != nil || !result.IsError {
		t.Fatalf("get_tunnel failure: %+v %v", result, err)
	}

	logs := output.String()
	for _, want := range []string{`"msg":"MCP request"`, `"method":"initialize"`, `"method":"tools/list"`,
		`"msg":"MCP tool call"`, `"tool":"search_runtime_logs"`, `"result":"success"`,
		`"msg":"MCP tool call failed"`, `"tool":"get_tunnel"`, `"tunnel":"missing-line"`, `"result":"error"`} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing audit field %s in logs:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "must-not-appear-in-audit") {
		t.Fatal("MCP audit log included tool arguments")
	}
}

func TestMCPHTTPAuthenticationAndOriginProtection(t *testing.T) {
	owner, _ := newTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := buildMCPServer(owner, MCPConfig{Access: "read"}, logger)
	base := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	handler := mcpAuthHandler("01234567890123456789012345678901", base)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`

	request := func(authorization, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", io.NopCloser(strings.NewReader(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request("", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", got)
	}
	if got := request("01234567890123456789012345678901", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("raw token status = %d", got)
	}
	if got := request("Bearer 01234567890123456789012345678901", "https://example.com").Code; got != http.StatusForbidden {
		t.Fatalf("origin status = %d", got)
	}
	if got := request("Bearer 01234567890123456789012345678901", "").Code; got != http.StatusOK {
		t.Fatalf("authenticated status = %d", got)
	}
}
