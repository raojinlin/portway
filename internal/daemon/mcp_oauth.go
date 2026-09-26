package daemon

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	mcpOAuthScope            = "portway"
	mcpOAuthOfflineScope     = "offline_access"
	oauthAuthorizationExpiry = 5 * time.Minute
	oauthAccessTokenExpiry   = time.Hour
	oauthRefreshTokenExpiry  = 30 * 24 * time.Hour
	oauthClientExpiry        = 365 * 24 * time.Hour
)

type oauthSignedClaims struct {
	Kind         string   `json:"kind"`
	Expires      int64    `json:"exp"`
	Audience     string   `json:"aud,omitempty"`
	ClientHash   string   `json:"client_hash,omitempty"`
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	Nonce        string   `json:"nonce,omitempty"`
}

type oauthAuthorization struct {
	ClientID    string
	ClientName  string
	RedirectURI string
	Challenge   string
	Scope       string
	State       string
	Expires     time.Time
}

type mcpOAuthServer struct {
	mu             sync.Mutex
	baseURL        string
	resourceURL    string
	secret         []byte
	logger         *slog.Logger
	pending        map[string]oauthAuthorization
	authorizeCodes map[string]oauthAuthorization
}

type oauthClientRegistrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
}

type oauthClientRegistrationResponse struct {
	oauthClientRegistrationRequest
	ClientID         string `json:"client_id"`
	ClientIDIssuedAt int64  `json:"client_id_issued_at"`
}

var oauthApprovalPage = template.Must(template.New("oauth-approval").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Authorize Portway</title>
  <style>
    :root { color-scheme:light; --bg:#f3f5f7; --surface:#fff; --surface-2:#f5f6f8; --ink:#24292f; --ink-2:#59636f; --ink-3:#687380; --rule:#dfe2e6; --accent:#315f9b; --red:#bf3d35; --red-bg:#fff2f0; --sans:'IBM Plex Sans','PingFang SC','Microsoft YaHei',sans-serif; --mono:'IBM Plex Mono','SFMono-Regular',Consolas,monospace; }
    @media (prefers-color-scheme:dark) { :root { color-scheme:dark; --bg:#181a1d; --surface:#202328; --surface-2:#22252a; --ink:#e4e7eb; --ink-2:#abb2bc; --ink-3:#929ba7; --rule:#363a40; --accent:#82a9e2; --red:#eb817b; --red-bg:#302426; } }
    * { box-sizing: border-box; }
    body { margin:0; min-height:100vh; display:grid; place-items:center; padding:24px; color:var(--ink); background:var(--bg); font:15px/1.55 var(--sans); -webkit-font-smoothing:antialiased; }
    main { width:min(560px,100%); border:1px solid var(--rule); border-radius:8px; background:var(--surface); }
    header { display:flex; align-items:center; justify-content:space-between; gap:16px; padding:14px 18px; border-bottom:1px solid var(--rule); }
    .brand { display:flex; align-items:center; gap:10px; font-size:14px; font-weight:600; }
    .brand-mark { display:grid; place-items:center; width:28px; height:28px; border:1px solid var(--rule); border-radius:6px; background:var(--surface-2); color:var(--accent); font:600 10px/1 var(--mono); }
    .status { display:inline-flex; align-items:center; gap:7px; color:var(--ink-2); font-size:12px; white-space:nowrap; }
    .status::before { width:7px; height:7px; border-radius:50%; background:var(--accent); content:''; }
    .content { padding:28px; }
    .eyebrow { color:var(--accent); font:600 11px/1.4 var(--mono); letter-spacing:.08em; text-transform:uppercase; }
    h1 { margin:9px 0 10px; font-size:24px; font-weight:600; line-height:1.25; }
    p { margin:0; color:var(--ink-2); }
    dl { display:grid; grid-template-columns:88px minmax(0,1fr); gap:11px 16px; margin:24px 0; padding:16px; border:1px solid var(--rule); border-radius:6px; background:var(--surface-2); }
    dt { color:var(--ink-3); font-size:12px; }
    dd { margin:0; color:var(--ink-2); font:12px/1.55 var(--mono); overflow-wrap:anywhere; }
    dd:last-child { margin-bottom:0; }
    .actions { display:flex; gap:10px; justify-content:flex-end; margin-top:20px; }
    label { display:grid; gap:7px; color:var(--ink-2); font-size:13px; font-weight:500; }
    .hint { color:var(--ink-3); font-size:12px; font-weight:400; }
    input[type=password] { width:100%; min-height:44px; padding:10px 12px; border:1px solid var(--rule); border-radius:3px; outline:none; background:var(--surface); color:var(--ink); font:14px var(--mono); }
    input[type=password]:focus-visible { border-color:var(--accent); outline:2px solid var(--accent); outline-offset:2px; }
    .error { margin:0 0 16px; padding:10px 12px; border-left:3px solid var(--red); background:var(--red-bg); color:var(--red); font-size:13px; }
    button { min-height:44px; padding:9px 16px; border:1px solid var(--rule); border-radius:3px; background:var(--surface); color:var(--ink); font:500 14px var(--sans); cursor:pointer; }
    button:hover { border-color:var(--accent); color:var(--accent); }
    button:focus-visible { outline:2px solid var(--accent); outline-offset:3px; }
    button.primary { border-color:var(--accent); background:var(--accent); color:var(--surface); }
    button.primary:hover { filter:brightness(.94); color:var(--surface); }
    @media (max-width:520px) { body { padding:16px; } .content { padding:22px 18px; } dl { grid-template-columns:1fr; gap:4px; } dd:not(:last-child) { margin-bottom:8px; } .actions { flex-direction:column-reverse; } button { width:100%; } }
  </style>
</head>
<body><main>
  <header><div class="brand"><span class="brand-mark" aria-hidden="true">PW</span>Portway</div><span class="status">MCP OAuth request</span></header>
  <section class="content">
    <div class="eyebrow">Local access confirmation</div>
    <h1>Authorize this agent?</h1>
    <p>The client will receive access to the MCP tools allowed by your current Portway permission settings.</p>
    <dl><dt>Client</dt><dd>{{.ClientName}}</dd><dt>Scope</dt><dd>{{.Scope}}</dd><dt>Callback</dt><dd>{{.RedirectURI}}</dd></dl>
    <form method="post" action="/oauth/authorize">
      <input type="hidden" name="request_id" value="{{.RequestID}}">
      {{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
      <label for="approval-secret">Portway approval code<span class="hint">Copy this code from the MCP section in Portway settings.</span></label>
      <input id="approval-secret" type="password" name="approval_secret" required autocomplete="current-password" autofocus>
      <div class="actions"><button name="decision" value="deny" formnovalidate>Deny</button><button class="primary" name="decision" value="approve">Authorize</button></div>
    </form>
  </section>
</main></body></html>`))

var oauthCallbackPage = template.Must(template.New("oauth-callback-result").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} - Portway</title>
  <style>
    :root { color-scheme:light; --bg:#f3f5f7; --surface:#fff; --surface-2:#f5f6f8; --ink:#24292f; --ink-2:#59636f; --ink-3:#687380; --rule:#dfe2e6; --accent:#315f9b; --green:#287b4b; --red:#bf3d35; --sans:'IBM Plex Sans','PingFang SC','Microsoft YaHei',sans-serif; --mono:'IBM Plex Mono','SFMono-Regular',Consolas,monospace; }
    @media (prefers-color-scheme:dark) { :root { color-scheme:dark; --bg:#181a1d; --surface:#202328; --surface-2:#22252a; --ink:#e4e7eb; --ink-2:#abb2bc; --ink-3:#929ba7; --rule:#363a40; --accent:#82a9e2; --green:#69b68a; --red:#eb817b; } }
    * { box-sizing:border-box; }
    body { margin:0; min-height:100vh; display:grid; place-items:center; padding:24px; background:var(--bg); color:var(--ink); font:15px/1.55 var(--sans); -webkit-font-smoothing:antialiased; }
    main { width:min(520px,100%); border:1px solid var(--rule); border-radius:8px; background:var(--surface); }
    header { display:flex; align-items:center; justify-content:space-between; gap:16px; padding:14px 18px; border-bottom:1px solid var(--rule); }
    .brand { display:flex; align-items:center; gap:10px; font-size:14px; font-weight:600; }
    .brand-mark { display:grid; place-items:center; width:28px; height:28px; border:1px solid var(--rule); border-radius:6px; background:var(--surface-2); color:var(--accent); font:600 10px/1 var(--mono); }
    .status { display:inline-flex; align-items:center; gap:7px; color:var(--ink-2); font-size:12px; white-space:nowrap; }
    .status::before { width:7px; height:7px; border-radius:50%; background:var(--status-color); content:''; }
    .result--success { --status-color:var(--green); }
    .result--denied { --status-color:var(--ink-3); }
    .result--failed { --status-color:var(--red); }
    .content { padding:32px 28px; }
    .eyebrow { color:var(--status-color); font:600 11px/1.4 var(--mono); letter-spacing:.08em; text-transform:uppercase; }
    h1 { margin:9px 0 10px; font-size:24px; font-weight:600; line-height:1.25; }
    p { margin:0; color:var(--ink-2); }
    .detail { margin-top:20px; padding:12px 14px; border-left:3px solid var(--status-color); background:var(--surface-2); color:var(--ink-2); font-size:13px; }
    .actions { display:flex; margin-top:22px; }
    a { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:9px 16px; border:1px solid var(--accent); border-radius:3px; background:var(--accent); color:var(--surface); font-weight:500; text-decoration:none; }
    a:hover { filter:brightness(.94); }
    a:focus-visible { outline:2px solid var(--accent); outline-offset:3px; }
    @media (max-width:520px) { body { padding:16px; } .content { padding:26px 18px; } .actions a { width:100%; } }
  </style>
</head>
<body><main class="result--{{.Tone}}">
  <header><div class="brand"><span class="brand-mark" aria-hidden="true">PW</span>Portway</div><span class="status">{{.Status}}</span></header>
  <section class="content" aria-live="polite">
    <div class="eyebrow">MCP OAuth</div>
    <h1>{{.Heading}}</h1>
    <p>{{.Message}}</p>
    <div class="detail">{{.Detail}}</div>
    {{if .RetryURL}}<div class="actions"><a href="{{.RetryURL}}" rel="noreferrer">Try callback again</a></div>{{end}}
  </section>
</main></body></html>`))

func newMCPOAuthServer(addr, secret string, logger *slog.Logger) *mcpOAuthServer {
	baseURL := "http://" + addr
	return &mcpOAuthServer{
		baseURL: baseURL, resourceURL: baseURL + "/mcp", secret: []byte(secret), logger: logger,
		pending: map[string]oauthAuthorization{}, authorizeCodes: map[string]oauthAuthorization{},
	}
}

func (s *mcpOAuthServer) matches(addr, secret string) bool {
	return s != nil && s.baseURL == "http://"+addr && subtle.ConstantTimeCompare(s.secret, []byte(secret)) == 1
}

func randomOAuthValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *mcpOAuthServer) sign(prefix string, claims oauthSignedClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return prefix + encoded + "." + signature, nil
}

func (s *mcpOAuthServer) verify(value, prefix string) (oauthSignedClaims, error) {
	var claims oauthSignedClaims
	if !strings.HasPrefix(value, prefix) {
		return claims, errors.New("unexpected token type")
	}
	encoded, signature, ok := strings.Cut(strings.TrimPrefix(value, prefix), ".")
	if !ok || encoded == "" || signature == "" {
		return claims, errors.New("malformed token")
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return claims, errors.New("malformed signature")
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(encoded))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return claims, errors.New("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return claims, errors.New("malformed claims")
	}
	if claims.Expires <= time.Now().Unix() {
		return claims, errors.New("expired token")
	}
	return claims, nil
}

func oauthClientHash(clientID string) string {
	digest := sha256.Sum256([]byte(clientID))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func writeOAuthJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	writeOAuthJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func (s *mcpOAuthServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource": s.resourceURL, "resource_name": "Portway MCP",
		"authorization_servers":    []string{s.baseURL},
		"scopes_supported":         []string{mcpOAuthScope, mcpOAuthOfflineScope},
		"bearer_methods_supported": []string{"header"},
	})
}

func (s *mcpOAuthServer) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/oauth/authorize",
		"token_endpoint":                        s.baseURL + "/oauth/token",
		"registration_endpoint":                 s.baseURL + "/oauth/register",
		"scopes_supported":                      []string{mcpOAuthScope, mcpOAuthOfflineScope},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func validOAuthRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Port() == "" || u.Fragment != "" || u.RawQuery != "" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func onlyContains(values []string, allowed ...string) bool {
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return false
		}
	}
	return true
}

func (s *mcpOAuthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var registration oauthClientRegistrationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024))
	if err := decoder.Decode(&registration); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "registration must be a JSON object")
		return
	}
	if len(registration.RedirectURIs) == 0 || len(registration.RedirectURIs) > 8 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "one to eight redirect URIs are required")
		return
	}
	for _, redirectURI := range registration.RedirectURIs {
		if !validOAuthRedirect(redirectURI) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "only loopback HTTP redirect URIs are allowed")
			return
		}
	}
	if registration.TokenEndpointAuthMethod != "" && registration.TokenEndpointAuthMethod != "none" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "only public clients are supported")
		return
	}
	if !onlyContains(registration.GrantTypes, "authorization_code", "refresh_token") || !onlyContains(registration.ResponseTypes, "code") {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported grant or response type")
		return
	}
	registration.TokenEndpointAuthMethod = "none"
	if len(registration.GrantTypes) == 0 {
		registration.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(registration.ResponseTypes) == 0 {
		registration.ResponseTypes = []string{"code"}
	}
	if registration.ClientName == "" {
		registration.ClientName = "Local MCP client"
	}
	claims := oauthSignedClaims{Kind: "client", Expires: time.Now().Add(oauthClientExpiry).Unix(),
		ClientName: registration.ClientName, RedirectURIs: registration.RedirectURIs}
	clientID, err := s.sign("pw_client_", claims)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not register client")
		return
	}
	s.logger.InfoContext(r.Context(), "MCP OAuth client registered", "client", registration.ClientName, "redirects", len(registration.RedirectURIs))
	writeOAuthJSON(w, http.StatusCreated, oauthClientRegistrationResponse{
		oauthClientRegistrationRequest: registration, ClientID: clientID, ClientIDIssuedAt: time.Now().Unix(),
	})
}

func validOAuthScope(scope string) bool {
	fields := strings.Fields(scope)
	return slices.Contains(fields, mcpOAuthScope) && onlyContains(fields, mcpOAuthScope, mcpOAuthOfflineScope)
}

func secureStringEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (s *mcpOAuthServer) clientClaims(clientID string) (oauthSignedClaims, error) {
	claims, err := s.verify(clientID, "pw_client_")
	if err != nil || claims.Kind != "client" || len(claims.RedirectURIs) == 0 {
		return oauthSignedClaims{}, errors.New("invalid client")
	}
	return claims, nil
}

func (s *mcpOAuthServer) cleanupLocked(now time.Time) {
	for id, request := range s.pending {
		if request.Expires.Before(now) {
			delete(s.pending, id)
		}
	}
	for code, request := range s.authorizeCodes {
		if request.Expires.Before(now) {
			delete(s.authorizeCodes, code)
		}
	}
}

func (s *mcpOAuthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleAuthorizeDecision(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	clientID, redirectURI := query.Get("client_id"), query.Get("redirect_uri")
	client, err := s.clientClaims(clientID)
	if err != nil || !slices.Contains(client.RedirectURIs, redirectURI) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "unknown client or redirect URI")
		return
	}
	scope := strings.Join(strings.Fields(query.Get("scope")), " ")
	if scope == "" {
		scope = mcpOAuthScope
	}
	resource := query.Get("resource")
	if query.Get("response_type") != "code" || query.Get("state") == "" ||
		query.Get("code_challenge_method") != "S256" || len(query.Get("code_challenge")) != 43 ||
		!validOAuthScope(scope) || resource != "" && resource != s.resourceURL {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request is incomplete or unsupported")
		return
	}
	requestID, err := randomOAuthValue()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not create authorization request")
		return
	}
	authorization := oauthAuthorization{ClientID: clientID, ClientName: client.ClientName, RedirectURI: redirectURI,
		Challenge: query.Get("code_challenge"), Scope: scope, State: query.Get("state"), Expires: time.Now().Add(oauthAuthorizationExpiry)}
	s.mu.Lock()
	s.cleanupLocked(time.Now())
	s.pending[requestID] = authorization
	s.mu.Unlock()
	s.renderApproval(w, requestID, authorization, "")
}

func (s *mcpOAuthServer) renderApproval(w http.ResponseWriter, requestID string, authorization oauthAuthorization, message string) {
	writeOAuthHTMLHeaders(w, "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	_ = oauthApprovalPage.Execute(w, map[string]string{
		"RequestID": requestID, "ClientName": authorization.ClientName, "Scope": authorization.Scope,
		"RedirectURI": authorization.RedirectURI, "Error": message,
	})
}

func writeOAuthHTMLHeaders(w http.ResponseWriter, contentSecurityPolicy string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func oauthRedirectURL(target string, values url.Values) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	query := u.Query()
	for key, entries := range values {
		for _, value := range entries {
			query.Add(key, value)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func dialOAuthLoopback(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	} else if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, errors.New("OAuth callback address is not loopback")
	}
	return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(host, port))
}

func deliverOAuthCallback(ctx context.Context, target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return errors.New("invalid OAuth callback URL")
	}
	base := *u
	base.RawQuery, base.ForceQuery = "", false
	if !validOAuthRedirect(base.String()) {
		return errors.New("OAuth callback URL is not an allowed loopback address")
	}
	transport := &http.Transport{Proxy: nil, DialContext: dialOAuthLoopback, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("could not create OAuth callback request")
	}
	request.Header.Set("User-Agent", "Portway-OAuth-Callback/1.0")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("OAuth callback request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("callback returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (s *mcpOAuthServer) renderCallbackResult(w http.ResponseWriter, status int, tone, state, heading, message, detail, retryURL string) {
	writeOAuthHTMLHeaders(w, "default-src 'none'; style-src 'unsafe-inline'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(status)
	_ = oauthCallbackPage.Execute(w, map[string]string{
		"Title": heading, "Tone": tone, "Status": state, "Heading": heading,
		"Message": message, "Detail": detail, "RetryURL": retryURL,
	})
}

func (s *mcpOAuthServer) handleAuthorizeDecision(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid approval form")
		return
	}
	requestID := r.Form.Get("request_id")
	s.mu.Lock()
	authorization, ok := s.pending[requestID]
	s.mu.Unlock()
	if !ok || authorization.Expires.Before(time.Now()) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request expired")
		return
	}
	if r.Form.Get("decision") != "approve" {
		s.mu.Lock()
		delete(s.pending, requestID)
		s.mu.Unlock()
		callbackURL, err := oauthRedirectURL(authorization.RedirectURI, url.Values{"error": {"access_denied"}, "state": {authorization.State}})
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not create callback URL")
			return
		}
		if err := deliverOAuthCallback(r.Context(), callbackURL); err != nil {
			s.logger.WarnContext(r.Context(), "MCP OAuth denial callback delivery failed", "client", authorization.ClientName,
				"callback_host", callbackHost(authorization.RedirectURI), "error", err)
			s.renderCallbackResult(w, http.StatusBadGateway, "failed", "Action required", "Could not reach your MCP client",
				"Portway could not deliver the denied authorization result to the local callback listener.",
				"Keep the MCP login command running, then try the callback again.", callbackURL)
			return
		}
		s.logger.InfoContext(r.Context(), "MCP OAuth access denied", "client", authorization.ClientName,
			"callback_host", callbackHost(authorization.RedirectURI))
		s.renderCallbackResult(w, http.StatusOK, "denied", "Not connected", "Access denied",
			"Portway did not grant MCP access to this client.", "You can close this window and return to your terminal.", "")
		return
	}
	if !secureStringEqual(r.Form.Get("approval_secret"), string(s.secret)) {
		s.logger.WarnContext(r.Context(), "MCP OAuth approval rejected", "reason", "invalid_approval_code", "client", authorization.ClientName)
		s.renderApproval(w, requestID, authorization, "The approval code is incorrect. Try again.")
		return
	}
	s.mu.Lock()
	delete(s.pending, requestID)
	s.mu.Unlock()
	code, err := randomOAuthValue()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not create authorization code")
		return
	}
	authorization.Expires = time.Now().Add(oauthAuthorizationExpiry)
	s.mu.Lock()
	s.cleanupLocked(time.Now())
	s.authorizeCodes[code] = authorization
	s.mu.Unlock()
	s.logger.InfoContext(r.Context(), "MCP OAuth access approved", "client", authorization.ClientName, "scope", authorization.Scope)
	callbackURL, err := oauthRedirectURL(authorization.RedirectURI, url.Values{"code": {code}, "state": {authorization.State}})
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not create callback URL")
		return
	}
	if err := deliverOAuthCallback(r.Context(), callbackURL); err != nil {
		s.logger.WarnContext(r.Context(), "MCP OAuth callback delivery failed", "client", authorization.ClientName,
			"callback_host", callbackHost(authorization.RedirectURI), "error", err)
		s.renderCallbackResult(w, http.StatusBadGateway, "failed", "Action required", "Could not reach your MCP client",
			"Portway approved the request, but the local callback listener was unavailable.",
			"Keep the MCP login command running, then try the callback again.", callbackURL)
		return
	}
	s.logger.InfoContext(r.Context(), "MCP OAuth callback delivered", "client", authorization.ClientName,
		"callback_host", callbackHost(authorization.RedirectURI))
	s.renderCallbackResult(w, http.StatusOK, "success", "Connected", "Authorization complete",
		"Your MCP client received the authorization result successfully.",
		"You can close this window and return to your terminal.", "")
}

func callbackHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func (s *mcpOAuthServer) issueTokens(w http.ResponseWriter, clientID, scope string) {
	now := time.Now()
	nonce, err := randomOAuthValue()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not issue access token")
		return
	}
	common := oauthSignedClaims{Audience: s.resourceURL, ClientHash: oauthClientHash(clientID), Scope: scope, Nonce: nonce}
	accessClaims := common
	accessClaims.Kind, accessClaims.Expires = "access", now.Add(oauthAccessTokenExpiry).Unix()
	accessToken, err := s.sign("pw_access_", accessClaims)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not issue access token")
		return
	}
	response := map[string]any{"access_token": accessToken, "token_type": "Bearer",
		"expires_in": int(oauthAccessTokenExpiry.Seconds()), "scope": scope}
	if slices.Contains(strings.Fields(scope), mcpOAuthOfflineScope) {
		refreshClaims := common
		refreshClaims.Nonce, err = randomOAuthValue()
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not issue refresh token")
			return
		}
		refreshClaims.Kind, refreshClaims.Expires = "refresh", now.Add(oauthRefreshTokenExpiry).Unix()
		refreshToken, signErr := s.sign("pw_refresh_", refreshClaims)
		if signErr != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "could not issue refresh token")
			return
		}
		response["refresh_token"] = refreshToken
	}
	writeOAuthJSON(w, http.StatusOK, response)
}

func (s *mcpOAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token request must be form encoded")
		return
	}
	clientID := r.Form.Get("client_id")
	if _, err := s.clientClaims(clientID); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown OAuth client")
		return
	}
	if resource := r.Form.Get("resource"); resource != "" && resource != s.resourceURL {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this MCP server")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code := r.Form.Get("code")
		s.mu.Lock()
		authorization, ok := s.authorizeCodes[code]
		delete(s.authorizeCodes, code)
		s.mu.Unlock()
		if !ok || authorization.Expires.Before(time.Now()) || !secureStringEqual(authorization.ClientID, clientID) ||
			!secureStringEqual(authorization.RedirectURI, r.Form.Get("redirect_uri")) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
			return
		}
		verifier := r.Form.Get("code_verifier")
		digest := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(digest[:])
		if len(verifier) < 43 || len(verifier) > 128 || !secureStringEqual(challenge, authorization.Challenge) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
			return
		}
		s.logger.DebugContext(r.Context(), "MCP OAuth token issued", "grant", "authorization_code")
		s.issueTokens(w, clientID, authorization.Scope)
	case "refresh_token":
		claims, err := s.verify(r.Form.Get("refresh_token"), "pw_refresh_")
		if err != nil || claims.Kind != "refresh" || claims.Audience != s.resourceURL ||
			!secureStringEqual(claims.ClientHash, oauthClientHash(clientID)) || !validOAuthScope(claims.Scope) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or expired")
			return
		}
		s.logger.DebugContext(r.Context(), "MCP OAuth token issued", "grant", "refresh_token")
		s.issueTokens(w, clientID, claims.Scope)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "supported grants are authorization_code and refresh_token")
	}
}

func (s *mcpOAuthServer) authorizeToken(token string) bool {
	claims, err := s.verify(token, "pw_access_")
	return err == nil && claims.Kind == "access" && claims.Audience == s.resourceURL && validOAuthScope(claims.Scope)
}

func (s *mcpOAuthServer) challenge() string {
	return fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource", scope="%s"`, s.baseURL, mcpOAuthScope)
}
