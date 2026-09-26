package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct {
	token string
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func saveServiceConfig(t *testing.T, service *Service, config FileConfig) (configView, int) {
	t.Helper()
	data, _ := json.Marshal(config)
	request := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(data)))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	service.ServeHTTP(w, request)
	var view configView
	if w.Code < 300 {
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
	}
	return view, w.Code
}

func mcpToolCount(t *testing.T, endpoint, token string) int {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "portway-service-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearerTransport{token: token}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return len(tools.Tools)
}

func mcpInitializeStatus(endpoint, token string) (int, error) {
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	request, _ := http.NewRequest(http.MethodPost, endpoint, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	return response.StatusCode, nil
}

func TestServiceLifecycleAndExclusiveOwnership(t *testing.T) {
	_, path := testFileConfig(t)
	s, err := Open(context.Background(), Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.ConfigDirectory() != filepath.Dir(path) {
		t.Fatal("wrong config directory")
	}
	if duplicate, err := Open(context.Background(), Options{}); err == nil {
		duplicate.Close()
		t.Fatal("CLI and desktop share ownership")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/config", nil))
	var view configView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || !view.Desktop {
		t.Fatalf("desktop config: %s %v", w.Body.String(), err)
	}
	view.Config.LogLevel = "debug"
	body, _ := json.Marshal(view.Config)
	req := httptest.NewRequest("PUT", "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/tunnels/test/start", nil))
	if w.Code != 503 {
		t.Fatalf("closed backend accepted request: %d", w.Code)
	}
	reopened, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.server.savedConfig.LogLevel != "debug" {
		t.Fatal("desktop config not persisted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceStartsAuthenticatedMCPListener(t *testing.T) {
	c, path := testFileConfig(t)
	c.MCP.Enabled = true
	c.MCP.Addr = availableAddress(t)
	c.MCP.Token = "01234567890123456789012345678901"
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	service, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if !service.server.mcpStatus.Running || service.server.mcpStatus.URL != "http://"+c.MCP.Addr+"/mcp" {
		t.Fatalf("MCP status: %+v", service.server.mcpStatus)
	}
	status, err := mcpInitializeStatus(service.server.mcpStatus.URL, "")
	if err != nil || status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated MCP status = %d, %v", status, err)
	}
	if count := mcpToolCount(t, service.server.mcpStatus.URL, c.MCP.Token); count != 8 {
		t.Fatalf("MCP tools = %d", count)
	}
	skillResponse, err := http.Get("http://" + c.MCP.Addr + builtinSkillDownloadPath)
	if err != nil {
		t.Fatal(err)
	}
	defer skillResponse.Body.Close()
	if skillResponse.StatusCode != http.StatusOK || skillResponse.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("MCP skill download: %d %q", skillResponse.StatusCode, skillResponse.Header.Get("Content-Type"))
	}
}

func TestServiceHotAppliesMCPConfiguration(t *testing.T) {
	c, _ := testFileConfig(t)
	service, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	c.MCP = MCPConfig{Enabled: true, Addr: availableAddress(t), Access: "read", Auth: "token", Token: "11111111111111111111111111111111"}
	view, status := saveServiceConfig(t, service, c)
	if status != http.StatusOK || view.RestartRequired || !view.MCPStatus.Running {
		t.Fatalf("enable MCP: status=%d view=%+v", status, view)
	}
	endpoint := view.MCPStatus.URL
	if count := mcpToolCount(t, endpoint, c.MCP.Token); count != 6 {
		t.Fatalf("read tools = %d", count)
	}
	runtime := service.mcp

	oldToken := c.MCP.Token
	c.MCP.Access = "manage"
	c.MCP.Token = "22222222222222222222222222222222"
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || view.RestartRequired || service.mcp != runtime {
		t.Fatalf("same-address update restarted listener: status=%d view=%+v", status, view)
	}
	if got, err := mcpInitializeStatus(endpoint, oldToken); err != nil || got != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, %v", got, err)
	}
	if count := mcpToolCount(t, endpoint, c.MCP.Token); count != 11 {
		t.Fatalf("manage tools = %d", count)
	}
	c.MCP.Auth = "both"
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || service.mcp != runtime || view.MCPStatus.Auth != "both" {
		t.Fatalf("combined auth hot update restarted listener: status=%d view=%+v", status, view)
	}
	if count := mcpToolCount(t, endpoint, c.MCP.Token); count != 11 {
		t.Fatalf("static token failed in combined auth mode: tools=%d", count)
	}
	metadata, err := http.Get("http://" + c.MCP.Addr + "/.well-known/oauth-protected-resource")
	if err != nil || metadata.StatusCode != http.StatusOK {
		t.Fatalf("OAuth metadata: %+v %v", metadata, err)
	}
	metadata.Body.Close()
	c.MCP.Auth = "oauth"
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || service.mcp != runtime || view.MCPStatus.Auth != "oauth" {
		t.Fatalf("OAuth hot update restarted listener: status=%d view=%+v", status, view)
	}
	if got, err := mcpInitializeStatus(endpoint, c.MCP.Token); err != nil || got != http.StatusUnauthorized {
		t.Fatalf("static token accepted in OAuth-only mode: status=%d err=%v", got, err)
	}
	c.MCP.Auth = "token"
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || service.mcp != runtime {
		t.Fatalf("token auth hot update restarted listener: status=%d view=%+v", status, view)
	}
	c.MCP.Addr = availableAddress(t)
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || view.RestartRequired || service.mcp == runtime || view.MCPStatus.URL == endpoint {
		t.Fatalf("address switch failed: status=%d view=%+v", status, view)
	}
	if _, err := mcpInitializeStatus(endpoint, c.MCP.Token); err == nil {
		t.Fatal("old MCP endpoint still accepts connections after address switch")
	}
	endpoint = view.MCPStatus.URL
	if count := mcpToolCount(t, endpoint, c.MCP.Token); count != 11 {
		t.Fatalf("switched MCP tools = %d", count)
	}

	c.MCP.Enabled = false
	view, status = saveServiceConfig(t, service, c)
	if status != http.StatusOK || view.RestartRequired || view.MCPStatus.Running || service.mcp != nil {
		t.Fatalf("disable MCP: status=%d view=%+v", status, view)
	}
	if _, err := mcpInitializeStatus(endpoint, c.MCP.Token); err == nil {
		t.Fatal("disabled MCP endpoint still accepts connections")
	}
}

func TestServiceSetMCPEnabledPersistsAndHotApplies(t *testing.T) {
	c, path := testFileConfig(t)
	c.MCP = MCPConfig{Addr: availableAddress(t), Access: "operate", Auth: "token"}
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	service, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	if err := service.SetMCPEnabled(true); err != nil {
		t.Fatal(err)
	}
	status := service.MCPStatus()
	if !status.Enabled || !status.Running || service.server.savedConfig.MCP.Token == "" {
		t.Fatalf("MCP was not enabled: status=%+v config=%+v", status, service.server.savedConfig.MCP)
	}
	if err := service.SetMCPEnabled(false); err != nil {
		t.Fatal(err)
	}
	status = service.MCPStatus()
	if status.Enabled || status.Running || service.server.savedConfig.MCP.Enabled {
		t.Fatalf("MCP was not disabled: status=%+v config=%+v", status, service.server.savedConfig.MCP)
	}
}

func TestServiceKeepsOldMCPWhenNewAddressFails(t *testing.T) {
	c, path := testFileConfig(t)
	c.MCP = MCPConfig{Enabled: true, Addr: availableAddress(t), Access: "operate", Auth: "token", Token: "11111111111111111111111111111111"}
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	service, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	oldEndpoint := service.server.mcpStatus.URL

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	next := c
	next.MCP.Addr = busy.Addr().String()
	_, status := saveServiceConfig(t, service, next)
	if status != http.StatusInternalServerError {
		t.Fatalf("busy address save status = %d", status)
	}
	if service.server.mcpStatus.URL != oldEndpoint || !service.server.mcpStatus.Running {
		t.Fatalf("old MCP status was replaced: %+v", service.server.mcpStatus)
	}
	if count := mcpToolCount(t, oldEndpoint, c.MCP.Token); count != 8 {
		t.Fatalf("old MCP tools = %d", count)
	}
}

func TestServiceRollsBackMCPWhenConfigWriteFails(t *testing.T) {
	c, path := testFileConfig(t)
	c.MCP = MCPConfig{Enabled: true, Addr: availableAddress(t), Access: "read", Auth: "token", Token: "11111111111111111111111111111111"}
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	service, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	endpoint := service.server.mcpStatus.URL

	failingPath := filepath.Join(t.TempDir(), "directory.yaml")
	if err := os.Mkdir(failingPath, 0o700); err != nil {
		t.Fatal(err)
	}
	service.server.mu.Lock()
	service.server.configPath = failingPath
	service.server.mu.Unlock()
	next := c
	next.MCP.Access = "manage"
	next.MCP.Token = "22222222222222222222222222222222"
	_, status := saveServiceConfig(t, service, next)
	if status != http.StatusInternalServerError {
		t.Fatalf("failed write status = %d", status)
	}
	if service.server.mcpStatus.Access != "read" || !service.server.mcpStatus.Running {
		t.Fatalf("MCP status was not rolled back: %+v", service.server.mcpStatus)
	}
	if got, err := mcpInitializeStatus(endpoint, next.MCP.Token); err != nil || got != http.StatusUnauthorized {
		t.Fatalf("new token status = %d, %v", got, err)
	}
	if count := mcpToolCount(t, endpoint, c.MCP.Token); count != 6 {
		t.Fatalf("rolled-back tools = %d", count)
	}
}

func TestServiceStartupFailureReleasesLocks(t *testing.T) {
	c, _ := testFileConfig(t)
	if err := os.MkdirAll(filepath.Dir(c.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.StatePath, []byte("invalid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), Options{}); err == nil {
		s.Close()
		t.Fatal("invalid state accepted")
	}
	if err := os.WriteFile(c.StatePath, []byte(`{"tunnels":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatalf("startup failure leaked resources: %v", err)
	}
	defer s.Close()
}
