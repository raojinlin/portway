package daemon

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const oauthTestSecret = "01234567890123456789012345678901"

func oauthRequest(t *testing.T, handler func(http.ResponseWriter, *http.Request), method, target, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func oauthResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode OAuth response %d %q: %v", response.Code, response.Body.String(), err)
	}
	return value
}

func registerOAuthTestClient(t *testing.T, server *mcpOAuthServer, callback string) string {
	t.Helper()
	registration := `{"redirect_uris":["` + callback + `"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],"response_types":["code"],"client_name":"Codex"}`
	response := oauthRequest(t, server.handleRegister, http.MethodPost, "/oauth/register", registration, "application/json")
	registered := oauthResponse(t, response)
	clientID, _ := registered["client_id"].(string)
	if response.Code != http.StatusCreated || clientID == "" {
		t.Fatalf("register: %d %s", response.Code, response.Body.String())
	}
	return clientID
}

func beginOAuthTestAuthorization(t *testing.T, server *mcpOAuthServer, clientID, callback, state string) (string, string) {
	t.Helper()
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	digest := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {callback},
		"state": {state}, "scope": {"portway offline_access"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"},
	}
	response := oauthRequest(t, server.handleAuthorize, http.MethodGet, "/oauth/authorize?"+query.Encode(), "", "")
	const marker = `name="request_id" value="`
	start := strings.Index(response.Body.String(), marker)
	if response.Code != http.StatusOK || start < 0 {
		t.Fatalf("authorize page: %d %s", response.Code, response.Body.String())
	}
	requestID := response.Body.String()[start+len(marker):]
	requestID = requestID[:strings.IndexByte(requestID, '"')]
	return requestID, verifier
}

func awaitOAuthCallback(t *testing.T, callbacks <-chan url.Values) url.Values {
	t.Helper()
	select {
	case query := <-callbacks:
		return query
	case <-time.After(2 * time.Second):
		t.Fatal("OAuth callback was not delivered")
		return nil
	}
}

func TestMCPOAuthAuthorizationCodePKCEFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := newMCPOAuthServer("127.0.0.1:7778", oauthTestSecret, logger)
	callbacks := make(chan url.Values, 1)
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callbacks <- r.URL.Query()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callbackServer.Close()
	callback := callbackServer.URL + "/callback/codex-test"
	clientID := registerOAuthTestClient(t, server, callback)
	requestID, verifier := beginOAuthTestAuthorization(t, server, clientID, callback, "state-123")

	wrongDecision := url.Values{"request_id": {requestID}, "decision": {"approve"}, "approval_secret": {"wrong"}}
	response := oauthRequest(t, server.handleAuthorize, http.MethodPost, "/oauth/authorize", wrongDecision.Encode(), "application/x-www-form-urlencoded")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "approval code is incorrect") {
		t.Fatalf("wrong approval code: %d %s", response.Code, response.Body.String())
	}
	decision := url.Values{"request_id": {requestID}, "decision": {"approve"}, "approval_secret": {oauthTestSecret}}
	response = oauthRequest(t, server.handleAuthorize, http.MethodPost, "/oauth/authorize", decision.Encode(), "application/x-www-form-urlencoded")
	if response.Code != http.StatusOK || response.Header().Get("Location") != "" || !strings.Contains(response.Body.String(), "Authorization complete") {
		t.Fatalf("approval completion page: %d %q %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	callbackQuery := awaitOAuthCallback(t, callbacks)
	if callbackQuery.Get("state") != "state-123" || callbackQuery.Get("code") == "" {
		t.Fatalf("approval callback: %v", callbackQuery)
	}

	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {callbackQuery.Get("code")},
		"redirect_uri": {callback}, "code_verifier": {verifier},
	}
	response = oauthRequest(t, server.handleToken, http.MethodPost, "/oauth/token", tokenForm.Encode(), "application/x-www-form-urlencoded")
	tokens := oauthResponse(t, response)
	accessToken, _ := tokens["access_token"].(string)
	refreshToken, _ := tokens["refresh_token"].(string)
	if response.Code != http.StatusOK || accessToken == "" || refreshToken == "" {
		t.Fatalf("token: %d %s", response.Code, response.Body.String())
	}
	authRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	authRequest.Header.Set("Authorization", "Bearer "+accessToken)
	if ok, reason := authorizeMCP(httptest.NewRecorder(), authRequest, "oauth", "ignored", server); !ok {
		t.Fatalf("OAuth access token rejected: %s", reason)
	}
	if ok, reason := authorizeMCP(httptest.NewRecorder(), authRequest, "both", oauthTestSecret, server); !ok {
		t.Fatalf("OAuth access token rejected in combined mode: %s", reason)
	}
	staticRequest := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	staticRequest.Header.Set("Authorization", "Bearer "+oauthTestSecret)
	staticResponse := httptest.NewRecorder()
	if ok, _ := authorizeMCP(staticResponse, staticRequest, "oauth", "ignored", server); ok ||
		!strings.Contains(staticResponse.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("static token accepted in OAuth mode or discovery challenge missing: %d %q", staticResponse.Code, staticResponse.Header().Get("WWW-Authenticate"))
	}
	if ok, reason := authorizeMCP(httptest.NewRecorder(), staticRequest, "both", oauthTestSecret, server); !ok {
		t.Fatalf("static token rejected in combined mode: %s", reason)
	}

	refreshForm := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {refreshToken}}
	response = oauthRequest(t, server.handleToken, http.MethodPost, "/oauth/token", refreshForm.Encode(), "application/x-www-form-urlencoded")
	if refreshed, _ := oauthResponse(t, response)["access_token"].(string); response.Code != http.StatusOK || refreshed == "" || refreshed == accessToken {
		t.Fatalf("refresh: %d %s", response.Code, response.Body.String())
	}
}

func TestMCPOAuthCallbackFailureShowsRetryPage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	callback := "http://" + listener.Addr().String() + "/callback/codex-test"
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	server := newMCPOAuthServer("127.0.0.1:7778", oauthTestSecret, slog.New(slog.NewTextHandler(&logs, nil)))
	clientID := registerOAuthTestClient(t, server, callback)
	requestID, _ := beginOAuthTestAuthorization(t, server, clientID, callback, "state-retry")
	decision := url.Values{"request_id": {requestID}, "decision": {"approve"}, "approval_secret": {oauthTestSecret}}
	response := oauthRequest(t, server.handleAuthorize, http.MethodPost, "/oauth/authorize", decision.Encode(), "application/x-www-form-urlencoded")
	if response.Code != http.StatusBadGateway || response.Header().Get("Location") != "" ||
		!strings.Contains(response.Body.String(), "Could not reach your MCP client") ||
		!strings.Contains(response.Body.String(), "Try callback again") ||
		!strings.Contains(response.Body.String(), "state-retry") {
		t.Fatalf("callback failure page: %d %q %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if strings.Contains(logs.String(), "code=") || strings.Contains(logs.String(), "state-retry") {
		t.Fatalf("callback credentials leaked to logs: %s", logs.String())
	}
}

func TestMCPOAuthDenialDeliveredToCallback(t *testing.T) {
	callbacks := make(chan url.Values, 1)
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callbacks <- r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	server := newMCPOAuthServer("127.0.0.1:7778", oauthTestSecret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	callback := callbackServer.URL + "/callback/codex-test"
	clientID := registerOAuthTestClient(t, server, callback)
	requestID, _ := beginOAuthTestAuthorization(t, server, clientID, callback, "state-denied")
	decision := url.Values{"request_id": {requestID}, "decision": {"deny"}}
	response := oauthRequest(t, server.handleAuthorize, http.MethodPost, "/oauth/authorize", decision.Encode(), "application/x-www-form-urlencoded")
	if response.Code != http.StatusOK || response.Header().Get("Location") != "" || !strings.Contains(response.Body.String(), "Access denied") {
		t.Fatalf("denial page: %d %q %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	callbackQuery := awaitOAuthCallback(t, callbacks)
	if callbackQuery.Get("error") != "access_denied" || callbackQuery.Get("state") != "state-denied" {
		t.Fatalf("denial callback: %v", callbackQuery)
	}
}

func TestMCPOAuthRejectsUnsafeClients(t *testing.T) {
	server := newMCPOAuthServer("127.0.0.1:7778", oauthTestSecret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, callback := range []string{"https://example.com/callback", "http://127.0.0.1/callback", "http://user@127.0.0.1:1234/callback"} {
		body := `{"redirect_uris":["` + callback + `"],"token_endpoint_auth_method":"none"}`
		response := oauthRequest(t, server.handleRegister, http.MethodPost, "/oauth/register", body, "application/json")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unsafe redirect %q accepted: %d", callback, response.Code)
		}
	}
}

func TestMCPOAuthMetadata(t *testing.T) {
	server := newMCPOAuthServer("127.0.0.1:7778", oauthTestSecret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := oauthRequest(t, server.handleProtectedResourceMetadata, http.MethodGet, "/.well-known/oauth-protected-resource", "", "")
	metadata := oauthResponse(t, response)
	if response.Code != http.StatusOK || metadata["resource"] != "http://127.0.0.1:7778/mcp" {
		t.Fatalf("resource metadata: %d %s", response.Code, response.Body.String())
	}
	response = oauthRequest(t, server.handleAuthorizationServerMetadata, http.MethodGet, "/.well-known/oauth-authorization-server", "", "")
	metadata = oauthResponse(t, response)
	if response.Code != http.StatusOK || metadata["registration_endpoint"] != "http://127.0.0.1:7778/oauth/register" {
		t.Fatalf("authorization metadata: %d %s", response.Code, response.Body.String())
	}
}
