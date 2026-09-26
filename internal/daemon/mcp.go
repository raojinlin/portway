package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Addr    string `json:"addr"`
	URL     string `json:"url"`
	Access  string `json:"access"`
	Auth    string `json:"auth"`
	Error   string `json:"error,omitempty"`
}

type mcpRuntime struct {
	mu         sync.RWMutex
	server     *http.Server
	listener   net.Listener
	handler    *mcp.StreamableHTTPHandler
	toolServer *mcp.Server
	authMode   string
	token      string
	oauth      *mcpOAuthServer
	addr       string
	logger     *slog.Logger
}

type mcpToolServerContextKey struct{}

type mcpStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *mcpStatusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *mcpStatusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *mcpStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (r *mcpRuntime) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.server.Shutdown(ctx)
	if err != nil {
		_ = r.listener.Close()
	}
	return err
}

func (r *mcpRuntime) update(owner *Server, config MCPConfig, logger *slog.Logger) {
	toolServer := buildMCPServer(owner, config, logger)
	r.mu.Lock()
	oauthServer := r.oauth
	if !mcpAuthHasOAuth(config.Auth) {
		oauthServer = nil
	} else if !oauthServer.matches(config.Addr, config.Token) {
		oauthServer = newMCPOAuthServer(config.Addr, config.Token, logger)
	}
	r.toolServer = toolServer
	r.authMode = config.Auth
	r.token = config.Token
	r.oauth = oauthServer
	r.mu.Unlock()
}

func mcpAuthHasToken(mode string) bool { return mode == "token" || mode == "both" }

func mcpAuthHasOAuth(mode string) bool { return mode == "oauth" || mode == "both" }

func (r *mcpRuntime) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	started := time.Now()
	statusWriter := &mcpStatusWriter{ResponseWriter: w}
	defer func() {
		status := statusWriter.status
		if status == 0 {
			status = http.StatusOK
		}
		r.logger.DebugContext(request.Context(), "MCP HTTP request", "method", request.Method, "path", request.URL.Path,
			"status", status, "remote", request.RemoteAddr, "elapsed", time.Since(started))
	}()
	r.mu.RLock()
	authMode, token, oauthServer, toolServer := r.authMode, r.token, r.oauth, r.toolServer
	r.mu.RUnlock()
	if ok, reason := authorizeMCP(statusWriter, request, authMode, token, oauthServer); !ok {
		r.logger.WarnContext(request.Context(), "MCP request rejected", "reason", reason, "method", request.Method,
			"remote", request.RemoteAddr, "elapsed", time.Since(started))
		return
	}
	request.Body = http.MaxBytesReader(statusWriter, request.Body, 1024*1024)
	request = request.WithContext(context.WithValue(request.Context(), mcpToolServerContextKey{}, toolServer))
	r.handler.ServeHTTP(statusWriter, request)
}

type tunnelNameInput struct {
	Name string `json:"name" jsonschema:"Tunnel name"`
}

type listTunnelsInput struct {
	State     string `json:"state,omitempty" jsonschema:"Optional runtime state filter: starting, running, stopped, or error"`
	Direction string `json:"direction,omitempty" jsonschema:"Optional forwarding direction filter: local, remote, or dynamic"`
}

type tunnelListOutput struct {
	Tunnels []TunnelView `json:"tunnels"`
	Total   int          `json:"total"`
}

type connectionsInput struct {
	Tunnel string `json:"tunnel,omitempty" jsonschema:"Optional tunnel name filter"`
	State  string `json:"state,omitempty" jsonschema:"Optional connection state filter"`
	Query  string `json:"query,omitempty" jsonschema:"Optional case-insensitive search text"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum rows to return, from 1 to 1000; defaults to 100"`
}

type logsInput struct {
	Tunnel string `json:"tunnel,omitempty" jsonschema:"Optional tunnel name filter"`
	Level  string `json:"level,omitempty" jsonschema:"Optional log level: debug, info, warn, or error"`
	Query  string `json:"query,omitempty" jsonschema:"Optional case-insensitive search text"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum rows to return, from 1 to 1000; defaults to 100"`
}

type mcpTunnelInput struct {
	Name                     string `json:"name" jsonschema:"Unique tunnel name"`
	ServiceIcon              string `json:"service_icon,omitempty" jsonschema:"Built-in service icon ID or auto"`
	Direction                string `json:"direction,omitempty" jsonschema:"Forwarding direction: local, remote, or dynamic; defaults to local"`
	LocalListen              string `json:"local_listen,omitempty" jsonschema:"Local listen address for local and dynamic forwarding"`
	SSHAddress               string `json:"ssh_address" jsonschema:"SSH host:port or SSH config Host alias"`
	SSHUser                  string `json:"ssh_user,omitempty" jsonschema:"Optional SSH user; prefer SSH config"`
	SSHKeyPath               string `json:"ssh_key_path,omitempty" jsonschema:"Optional private key path on the Portway host"`
	SSHConfigPath            string `json:"ssh_config_path,omitempty" jsonschema:"Optional SSH config path"`
	RemoteListen             string `json:"remote_listen,omitempty" jsonschema:"Server-side listen address for remote forwarding"`
	ForwardAddress           string `json:"forward_address,omitempty" jsonschema:"Fixed target for local and remote forwarding"`
	KnownHostsPath           string `json:"known_hosts_path,omitempty" jsonschema:"Optional known_hosts path"`
	TrustNewHostKey          bool   `json:"trust_new_host_key,omitempty" jsonschema:"Trust and record a previously unseen host key"`
	InsecureSkipHostKeyCheck bool   `json:"insecure_skip_host_key_check,omitempty" jsonschema:"Disable host-key verification; unsafe"`
	KeepAlive                string `json:"keep_alive,omitempty" jsonschema:"Go duration such as 30s or 1m"`
	ReconnectDelay           string `json:"reconnect_delay,omitempty" jsonschema:"Go duration such as 3s or 500ms"`
}

func (in mcpTunnelInput) request() TunnelRequest {
	return TunnelRequest{
		Name: in.Name, ServiceIcon: in.ServiceIcon, Direction: in.Direction, LocalListen: in.LocalListen,
		SSHAddress: in.SSHAddress, SSHUser: in.SSHUser, SSHKeyPath: in.SSHKeyPath, SSHConfigPath: in.SSHConfigPath,
		RemoteListen: in.RemoteListen, ForwardAddress: in.ForwardAddress, KnownHostsPath: in.KnownHostsPath,
		TrustNewHostKey: in.TrustNewHostKey, InsecureSkipHostKeyCheck: in.InsecureSkipHostKeyCheck,
		KeepAlive: in.KeepAlive, ReconnectDelay: in.ReconnectDelay,
	}
}

func boolPointer(value bool) *bool { return &value }

func annotations(title string, readOnly, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: readOnly, DestructiveHint: boolPointer(destructive),
		IdempotentHint: idempotent, OpenWorldHint: boolPointer(false)}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func normalizedLimit(value int) (int, error) {
	if value == 0 {
		return 100, nil
	}
	if value < 1 || value > 1000 {
		return 0, fmt.Errorf("limit must be between 1 and 1000")
	}
	return value, nil
}

func buildMCPServer(owner *Server, config MCPConfig, logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "portway", Version: "1.0.0"}, nil)
	server.AddReceivingMiddleware(mcpAuditMiddleware(logger))

	mcp.AddTool(server, &mcp.Tool{Name: "list_tunnels", Description: "List saved Portway tunnels with live state and non-secret configuration.",
		Annotations: annotations("List tunnels", true, false, true)},
		func(_ context.Context, _ *mcp.CallToolRequest, input listTunnelsInput) (*mcp.CallToolResult, any, error) {
			if input.State != "" && input.State != "starting" && input.State != "running" && input.State != "stopped" && input.State != "error" {
				return nil, nil, fmt.Errorf("state must be starting, running, stopped, or error")
			}
			if input.Direction != "" && input.Direction != "local" && input.Direction != "remote" && input.Direction != "dynamic" {
				return nil, nil, fmt.Errorf("direction must be local, remote, or dynamic")
			}
			views := owner.tunnelViews()
			filtered := make([]TunnelView, 0, len(views))
			for _, view := range views {
				if input.State != "" && view.State != input.State || input.Direction != "" && view.Direction != input.Direction {
					continue
				}
				filtered = append(filtered, view)
			}
			return textResult(fmt.Sprintf("Found %d tunnel(s).", len(filtered))), tunnelListOutput{Tunnels: filtered, Total: len(filtered)}, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "get_tunnel", Description: "Get one Portway tunnel's live state and non-secret configuration.",
		Annotations: annotations("Get tunnel", true, false, true)},
		func(_ context.Context, _ *mcp.CallToolRequest, input tunnelNameInput) (*mcp.CallToolResult, any, error) {
			owner.mu.Lock()
			defer owner.mu.Unlock()
			view, ok := owner.viewLocked(input.Name)
			if !ok {
				return nil, nil, fmt.Errorf("tunnel %q not found", input.Name)
			}
			return textResult(fmt.Sprintf("Tunnel %s is %s.", view.Name, view.State)), view, nil
		})

	addConnectionsTool := func(name, description string, history bool) {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: description,
			Annotations: annotations(strings.ReplaceAll(name, "_", " "), true, false, true)},
			func(_ context.Context, _ *mcp.CallToolRequest, input connectionsInput) (*mcp.CallToolResult, any, error) {
				limit, err := normalizedLimit(input.Limit)
				if err != nil {
					return nil, nil, err
				}
				if input.State != "" && ((!history && input.State != "connecting" && input.State != "connected") || (history && input.State != "closed" && input.State != "failed")) {
					return nil, nil, fmt.Errorf("invalid connection state")
				}
				view := owner.connectionsView(history, input.Tunnel, input.State, input.Query, limit)
				return textResult(fmt.Sprintf("Found %d connection record(s).", view.Total)), view, nil
			})
	}
	addConnectionsTool("list_active_connections", "List active Portway forwarding connections, optionally filtered by tunnel, state, or text.", false)
	addConnectionsTool("list_connection_history", "List completed SOCKS5 connection history, optionally filtered by tunnel, state, or text.", true)

	mcp.AddTool(server, &mcp.Tool{Name: "search_runtime_logs", Description: "Search Portway runtime logs without exposing SSH passwords.",
		Annotations: annotations("Search runtime logs", true, false, true)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input logsInput) (*mcp.CallToolResult, any, error) {
			limit, err := normalizedLimit(input.Limit)
			if err != nil {
				return nil, nil, err
			}
			level := strings.ToUpper(input.Level)
			if level != "" && level != "DEBUG" && level != "INFO" && level != "WARN" && level != "ERROR" {
				return nil, nil, fmt.Errorf("level must be debug, info, warn, or error")
			}
			owner.mu.Lock()
			cfg, path := owner.effectiveConfig, owner.configPath
			owner.mu.Unlock()
			view := logView{Entries: []logEntry{}, Enabled: cfg.LogFile != "", Limit: limit}
			if view.Enabled {
				view.Path, err = configPath(cfg.LogFile, filepath.Dir(path))
				if err == nil {
					err = readLogView(ctx, &view, cfg.LogMaxBackups, level, input.Tunnel, input.Query)
				}
			}
			if err != nil {
				return nil, nil, fmt.Errorf("read runtime log: %w", err)
			}
			return textResult(fmt.Sprintf("Found %d log entries.", len(view.Entries))), view, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "get_portway_skill", Description: "Download the built-in Portway skill as files for installation in the agent's own skills directory.",
		Annotations: annotations("Get Portway skill", true, false, true)},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			bundle, _, err := loadBuiltinSkill()
			if err != nil {
				return nil, nil, err
			}
			return textResult("Downloaded the Portway skill package. Install the returned files in the agent's own skills directory."), bundle, nil
		})

	if config.Access == "operate" || config.Access == "manage" {
		for _, operation := range []struct {
			name, description string
			enabled           bool
		}{
			{"start_tunnel", "Start a saved Portway tunnel and persist it as enabled.", true},
			{"stop_tunnel", "Stop a Portway tunnel while preserving its saved definition.", false},
		} {
			op := operation
			mcp.AddTool(server, &mcp.Tool{Name: op.name, Description: op.description,
				Annotations: annotations(strings.ReplaceAll(op.name, "_", " "), false, false, true)},
				func(_ context.Context, _ *mcp.CallToolRequest, input tunnelNameInput) (*mcp.CallToolResult, any, error) {
					view, _, err := owner.setTunnelEnabled(input.Name, op.enabled)
					if err != nil {
						return nil, nil, err
					}
					return textResult(fmt.Sprintf("Tunnel %s is now %s.", view.Name, view.State)), view, nil
				})
		}
	}

	if config.Access == "manage" {
		mcp.AddTool(server, &mcp.Tool{Name: "create_tunnel", Description: "Create, persist, and start a Portway tunnel. SSH passwords are intentionally unsupported.",
			Annotations: annotations("Create tunnel", false, false, false)},
			func(_ context.Context, _ *mcp.CallToolRequest, input mcpTunnelInput) (*mcp.CallToolResult, any, error) {
				view, _, err := owner.createTunnel(input.request())
				if err != nil {
					return nil, nil, err
				}
				return textResult(fmt.Sprintf("Created tunnel %s; connection starts asynchronously.", view.Name)), view, nil
			})
		mcp.AddTool(server, &mcp.Tool{Name: "update_tunnel", Description: "Replace a Portway tunnel's non-secret configuration. Supply the complete configuration; running tunnels restart when needed.",
			Annotations: annotations("Update tunnel", false, false, true)},
			func(_ context.Context, _ *mcp.CallToolRequest, input mcpTunnelInput) (*mcp.CallToolResult, any, error) {
				view, _, err := owner.updateTunnel(input.Name, input.request())
				if err != nil {
					return nil, nil, err
				}
				return textResult(fmt.Sprintf("Updated tunnel %s.", view.Name)), view, nil
			})
		mcp.AddTool(server, &mcp.Tool{Name: "delete_tunnel", Description: "Permanently stop and delete a Portway tunnel. Confirm with the user before calling.",
			Annotations: annotations("Delete tunnel", false, true, true)},
			func(_ context.Context, _ *mcp.CallToolRequest, input tunnelNameInput) (*mcp.CallToolResult, any, error) {
				if _, err := owner.deleteTunnel(input.Name); err != nil {
					return nil, nil, err
				}
				return textResult(fmt.Sprintf("Deleted tunnel %s.", input.Name)), map[string]string{"name": input.Name, "status": "deleted"}, nil
			})
	}
	return server
}

func mcpAuditMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			started := time.Now()
			result, err := next(ctx, method, request)
			elapsed := time.Since(started)
			if method != "tools/call" {
				if err != nil {
					logger.WarnContext(ctx, "MCP request failed", "method", method, "error", err, "elapsed", elapsed)
				} else {
					logger.DebugContext(ctx, "MCP request", "method", method, "elapsed", elapsed)
				}
				return result, err
			}

			tool, tunnel := mcpAuditTarget(request)
			fields := []any{"tool", tool, "result", "success", "elapsed", elapsed}
			if tunnel != "" {
				fields = append(fields, "tunnel", tunnel)
			}
			toolErr := err
			if toolErr == nil {
				if callResult, ok := result.(*mcp.CallToolResult); ok {
					toolErr = callResult.GetError()
				}
			}
			if toolErr != nil {
				fields[3] = "error"
				fields = append(fields, "error", toolErr)
				logger.WarnContext(ctx, "MCP tool call failed", fields...)
			} else {
				logger.InfoContext(ctx, "MCP tool call", fields...)
			}
			return result, err
		}
	}
}

func mcpAuditTarget(request mcp.Request) (tool, tunnel string) {
	params, ok := request.GetParams().(*mcp.CallToolParamsRaw)
	if !ok || params == nil {
		return "", ""
	}
	tool = params.Name
	var selector struct {
		Name   string `json:"name"`
		Tunnel string `json:"tunnel"`
	}
	if err := json.Unmarshal(params.Arguments, &selector); err == nil {
		if selector.Name != "" {
			tunnel = selector.Name
		} else {
			tunnel = selector.Tunnel
		}
	}
	return tool, tunnel
}

func authorizeMCP(w http.ResponseWriter, request *http.Request, authMode, token string, oauthServer *mcpOAuthServer) (bool, string) {
	if request.Header.Get("Origin") != "" {
		http.Error(w, "browser origins are not allowed", http.StatusForbidden)
		return false, "browser_origin"
	}
	scheme, provided, ok := strings.Cut(request.Header.Get("Authorization"), " ")
	authorized := false
	if ok && strings.EqualFold(scheme, "Bearer") {
		if mcpAuthHasToken(authMode) {
			authorized = len(provided) == len(token) && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
		}
		if !authorized && mcpAuthHasOAuth(authMode) {
			authorized = oauthServer != nil && oauthServer.authorizeToken(provided)
		}
	}
	if !authorized {
		if mcpAuthHasOAuth(authMode) && oauthServer != nil {
			w.Header().Set("WWW-Authenticate", oauthServer.challenge())
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false, "unauthorized"
	}
	return true, ""
}

func mcpAuthHandler(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if ok, _ := authorizeMCP(w, request, "token", token, nil); !ok {
			return
		}
		request.Body = http.MaxBytesReader(w, request.Body, 1024*1024)
		next.ServeHTTP(w, request)
	})
}

func startMCP(owner *Server, config MCPConfig, logger *slog.Logger) (*mcpRuntime, error) {
	listener, err := net.Listen("tcp", config.Addr)
	if err != nil {
		return nil, err
	}
	runtime := &mcpRuntime{listener: listener, addr: config.Addr, logger: logger}
	runtime.update(owner, config, logger)
	runtime.handler = mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		if toolServer, ok := request.Context().Value(mcpToolServerContextKey{}).(*mcp.Server); ok {
			return toolServer
		}
		return nil
	},
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: logger})
	mux := http.NewServeMux()
	mux.Handle("/mcp", runtime)
	mux.HandleFunc("GET "+builtinSkillDownloadPath, owner.handleDownloadSkill)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		mux.HandleFunc(path, runtime.serveOAuth(func(server *mcpOAuthServer, w http.ResponseWriter, r *http.Request) {
			server.handleProtectedResourceMetadata(w, r)
		}))
	}
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-authorization-server/mcp"} {
		mux.HandleFunc(path, runtime.serveOAuth(func(server *mcpOAuthServer, w http.ResponseWriter, r *http.Request) {
			server.handleAuthorizationServerMetadata(w, r)
		}))
	}
	mux.HandleFunc("/oauth/register", runtime.serveOAuth(func(server *mcpOAuthServer, w http.ResponseWriter, r *http.Request) {
		server.handleRegister(w, r)
	}))
	mux.HandleFunc("/oauth/authorize", runtime.serveOAuth(func(server *mcpOAuthServer, w http.ResponseWriter, r *http.Request) {
		server.handleAuthorize(w, r)
	}))
	mux.HandleFunc("/oauth/token", runtime.serveOAuth(func(server *mcpOAuthServer, w http.ResponseWriter, r *http.Request) {
		server.handleToken(w, r)
	}))
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	runtime.server = httpServer
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("MCP server failed", "error", serveErr)
		}
	}()
	return runtime, nil
}

func (r *mcpRuntime) serveOAuth(handler func(*mcpOAuthServer, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		r.mu.RLock()
		oauthServer := r.oauth
		r.mu.RUnlock()
		if oauthServer == nil {
			http.NotFound(w, request)
			return
		}
		handler(oauthServer, w, request)
	}
}
