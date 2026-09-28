package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-tunnel-manager/desktop/autostart"
	"ssh-tunnel-manager/desktop/platform"
	"ssh-tunnel-manager/internal/daemon"
)

const applicationTitle = "Portway"

var applicationVersion = "0.1.0"

type application struct {
	mu                 sync.RWMutex
	ctx                context.Context
	service            *daemon.Service
	problem            error
	ready              chan struct{}
	cancel             context.CancelFunc
	done               <-chan struct{}
	nativeMu           sync.Mutex
	language           string
	pendingLogs        bool
	pendingMCP         bool
	pendingConnections *connectionNavigation
	trayPending        map[string]bool
	trayRefresh        chan struct{}
	saveSkill          func([]byte) (string, error)
}

func (a *application) startup(ctx context.Context) {
	backendCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.ctx, a.cancel, a.done = ctx, cancel, backendCtx.Done()
	a.mu.Unlock()
	service, err := daemon.Open(backendCtx, daemon.Options{Desktop: true})
	a.mu.Lock()
	a.service, a.problem = service, err
	a.mu.Unlock()
	close(a.ready)
}

func (a *application) serve(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" && origin != "wails://wails" && origin != "http://wails.localhost" {
		http.Error(w, "untrusted desktop origin", http.StatusForbidden)
		return
	}
	a.mu.RLock()
	service, problem := a.service, a.problem
	a.mu.RUnlock()
	if service == nil {
		message := "Desktop backend is starting"
		if problem != nil {
			message = problem.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	if r.URL.Path == "/api/desktop/autostart" {
		autostartHandler(autostart.Get, autostart.Set).ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/api/desktop/skills/portway/save" {
		a.savePortwaySkill(w, r)
		return
	}
	// Wails owns this in-process transport. Its webview uses a custom origin,
	// not the public HTTP daemon's http/https origin; do not weaken HTTP checks.
	r = r.Clone(r.Context())
	r.Header.Del("Origin")
	service.ServeHTTP(w, r)
}

func (a *application) savePortwaySkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	archive, err := daemon.BuiltinPortwaySkillArchive()
	if err != nil {
		http.Error(w, "package Portway Skill: "+err.Error(), http.StatusInternalServerError)
		return
	}
	save := a.saveSkill
	if save == nil {
		save = a.saveSkillWithDialog
	}
	path, err := save(archive)
	if err != nil {
		http.Error(w, "save Portway Skill: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if path == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": path})
}

func (a *application) saveSkillWithDialog(archive []byte) (string, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return "", errors.New("desktop window is not ready")
	}
	path, err := wruntime.SaveFileDialog(ctx, wruntime.SaveDialogOptions{
		Title: "Save Portway Skill", DefaultFilename: "portway-skill.zip", CanCreateDirectories: true,
		Filters: []wruntime.FileFilter{{DisplayName: "ZIP archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (a *application) show() {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx != nil {
		wruntime.WindowShow(ctx)
		wruntime.WindowUnminimise(ctx)
	}
}

func (a *application) domReady(ctx context.Context) {
	wruntime.EventsEmit(ctx, "desktop:theme-ready")
	wruntime.EventsEmit(ctx, "desktop:language-ready")
}

func (a *application) openLogs() {
	a.mu.Lock()
	a.pendingLogs = true
	a.pendingMCP = false
	a.pendingConnections = nil
	a.mu.Unlock()
	a.show()
	a.sendNavigation()
}

func (a *application) openMCP() {
	a.mu.Lock()
	a.pendingLogs = false
	a.pendingMCP = true
	a.pendingConnections = nil
	a.mu.Unlock()
	a.show()
	a.sendNavigation()
}

type connectionNavigation struct {
	Page        string `json:"page"`
	Name        string `json:"name"`
	ShowHistory bool   `json:"showHistory"`
}

func (a *application) openConnections(name string, showHistory bool) {
	if name == "" {
		return
	}
	a.mu.Lock()
	a.pendingLogs = false
	a.pendingMCP = false
	a.pendingConnections = &connectionNavigation{Page: "connections", Name: name, ShowHistory: showHistory}
	a.mu.Unlock()
	a.show()
	a.sendNavigation()
}

func (a *application) sendNavigation() {
	a.mu.RLock()
	ctx, pendingLogs, pendingMCP, connections := a.ctx, a.pendingLogs, a.pendingMCP, a.pendingConnections
	a.mu.RUnlock()
	if ctx != nil && pendingLogs {
		wruntime.EventsEmit(ctx, "desktop:navigate", "activity")
	} else if ctx != nil && pendingMCP {
		wruntime.EventsEmit(ctx, "desktop:navigate", "mcp")
	} else if ctx != nil && connections != nil {
		wruntime.EventsEmit(ctx, "desktop:navigate", connections)
	}
}

func (a *application) navigationApplied(data ...interface{}) {
	if len(data) != 1 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if page, ok := data[0].(string); ok && page == "activity" {
		a.pendingLogs = false
	}
	if page, ok := data[0].(string); ok && page == "mcp" {
		a.pendingMCP = false
	}
	if page, ok := data[0].(map[string]interface{}); ok && a.pendingConnections != nil && page["page"] == "connections" && page["name"] == a.pendingConnections.Name && page["showHistory"] == a.pendingConnections.ShowHistory {
		a.pendingConnections = nil
	}
}

func (a *application) languageEvent(data ...interface{}) {
	if len(data) != 1 {
		return
	}
	language, ok := data[0].(string)
	if !ok || (language != "zh" && language != "en") {
		return
	}
	a.mu.Lock()
	a.language = language
	a.mu.Unlock()
}

type menuStarter func(string, platform.TrayActions) error

type nativeMenus struct {
	start  menuStarter
	update func(platform.TraySnapshot)
}

func (a *application) startDesktop(ctx context.Context, menus nativeMenus) error {
	a.startup(ctx)
	return a.setupWindow(ctx, menus)
}

func (a *application) setupWindow(ctx context.Context, menus nativeMenus) error {
	<-a.ready
	a.nativeMu.Lock()
	defer a.nativeMu.Unlock()
	a.mu.RLock()
	service, problem := a.service, a.problem
	a.mu.RUnlock()
	if problem != nil {
		return problem
	}
	select {
	case <-a.done:
		return nil
	default:
	}
	a.mu.Lock()
	a.trayRefresh = make(chan struct{}, 1)
	refresh := a.trayRefresh
	a.mu.Unlock()
	executable, _ := os.Executable()
	service.Logger().Info("desktop native menus initializing", "app", applicationTitle, "platform", runtime.GOOS, "executable", executable)
	if err := menus.start(applicationTitle, platform.TrayActions{
		Show: a.show, Quit: func() { wruntime.Quit(ctx) }, Directory: func() { platform.OpenDirectory(service.ConfigDirectory()) }, Logs: a.openLogs, MCP: a.openMCP,
		SetMCP: func(enabled bool) {
			if err := service.SetMCPEnabled(enabled); err != nil {
				service.Logger().Warn("desktop MCP action failed", "enabled", enabled, "error", err)
				a.mu.RLock()
				language := a.language
				a.mu.RUnlock()
				_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.ErrorDialog,
					Title: trayText(language, "MCP 操作失败", "MCP action failed"), Message: err.Error(),
					Buttons: []string{trayText(language, "好", "OK")}})
			}
			select {
			case a.trayRefresh <- struct{}{}:
			default:
			}
		},
		Connections: a.openConnections,
		SetEnabled: func(name string, enabled bool) {
			if err := a.changeTunnel(name, enabled); err != nil {
				a.mu.RLock()
				language := a.language
				a.mu.RUnlock()
				select {
				case <-a.done:
					return
				default:
				}
				_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.ErrorDialog,
					Title: trayText(language, "线路操作失败", "Tunnel action failed"), Message: name + "\n\n" + err.Error(),
					Buttons: []string{trayText(language, "好", "OK")}})
			}
		},
		Copy: func(text string) {
			if err := wruntime.ClipboardSetText(ctx, text); err != nil {
				service.Logger().Warn("clipboard write failed", "error", err)
			}
		},
	}); err != nil {
		service.Logger().Error("desktop native menus failed", "error", err)
		return fmt.Errorf("cannot initialize desktop menus: %w", err)
	}
	service.Logger().Info("desktop native menus ready", "app", applicationTitle, "tray", platform.HasTray())
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var sampler traySampler
		for {
			a.mu.RLock()
			sampler.language = a.language
			a.mu.RUnlock()
			snapshot := sampler.sample(service.TunnelViews(), time.Now())
			snapshot.MCP = trayMCPStatus(service.MCPStatus(), sampler.language)
			a.mu.RLock()
			for i := range snapshot.Lines {
				snapshot.Lines[i].Busy = a.trayPending[snapshot.Lines[i].Name]
			}
			a.mu.RUnlock()
			menus.update(snapshot)
			select {
			case <-a.done:
				return
			case <-ticker.C:
			case <-refresh:
			}
		}
	}()
	return nil
}

func (a *application) changeTunnel(name string, enabled bool) error {
	a.mu.Lock()
	if a.trayPending[name] {
		a.mu.Unlock()
		return nil
	}
	service := a.service
	if service == nil {
		a.mu.Unlock()
		return fmt.Errorf("backend is not ready")
	}
	if a.trayPending == nil {
		a.trayPending = make(map[string]bool)
	}
	a.trayPending[name] = true
	refresh := a.trayRefresh
	a.mu.Unlock()
	notify := func() {
		select {
		case refresh <- struct{}{}:
		default:
		}
	}
	notify()
	defer func() {
		a.mu.Lock()
		delete(a.trayPending, name)
		a.mu.Unlock()
		notify()
	}()
	return service.SetTunnelEnabled(name, enabled)
}

func (a *application) shutdown(context.Context) {
	<-a.ready
	a.mu.RLock()
	service, cancel := a.service, a.cancel
	a.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	// Serialize shutdown with native startup so a queued start cannot recreate the tray.
	a.nativeMu.Lock()
	defer a.nativeMu.Unlock()
	platform.Stop()
	if service != nil {
		_ = service.Close()
	}
}

func (a *application) windowOptions() *options.App {
	return &options.App{
		Title: applicationTitle, Width: 1180, Height: 800, MinWidth: 760, MinHeight: 540,
		HideWindowOnClose: platform.HasTray(),
		StartHidden:       autostart.LaunchHidden(os.Args[1:], platform.HasTray()),
		BackgroundColour:  options.NewRGB(243, 245, 247),
		// Wails leaves the native zoom button disabled when Mac options are nil.
		Mac: &mac.Options{DisableZoom: false, About: &mac.AboutInfo{
			Title:   applicationTitle,
			Message: "Version " + applicationVersion + "\n\nSSH tunnel manager for local, remote and SOCKS5 forwarding.",
		}},
		Windows:     desktopWindowsOptions(),
		AssetServer: &assetserver.Options{Assets: daemon.WebAssets(), Handler: http.HandlerFunc(a.serve)},
		OnStartup: func(ctx context.Context) {
			a.mu.Lock()
			a.language = platform.SystemLanguage()
			a.mu.Unlock()
			wruntime.EventsOn(ctx, "desktop:language", a.languageEvent)
			wruntime.EventsOn(ctx, "desktop:navigation-ready", func(...interface{}) { a.sendNavigation() })
			wruntime.EventsOn(ctx, "desktop:navigation-applied", a.navigationApplied)
			wruntime.EventsOn(ctx, "desktop:theme", func(data ...interface{}) {
				applyThemeEvent(data, func(dark, followSystem bool, r, g, b uint8) {
					wruntime.WindowSetBackgroundColour(ctx, r, g, b, 255)
					if followSystem {
						wruntime.WindowSetSystemDefaultTheme(ctx)
					} else if dark {
						wruntime.WindowSetDarkTheme(ctx)
					} else {
						wruntime.WindowSetLightTheme(ctx)
					}
					platform.SetAppearance(dark, followSystem)
				})
			})
			// Native menus must not wait for webview navigation or external page resources.
			if err := a.startDesktop(ctx, nativeMenus{start: platform.Start, update: platform.Update}); err != nil {
				a.mu.RLock()
				language := a.language
				a.mu.RUnlock()
				message := trayText(language, "无法启动 Portway。\n\n", "Cannot start Portway.\n\n") + err.Error() +
					trayText(language, "\n\n如果 CLI daemon 正在运行，请先停止，再打开桌面应用。", "\n\nIf a CLI daemon is running, stop it before opening the desktop app.")
				_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.ErrorDialog,
					Title: applicationTitle, Message: message, Buttons: []string{trayText(language, "好", "OK")}})
				wruntime.Quit(ctx)
			}
		}, OnDomReady: a.domReady, OnShutdown: a.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "ssh-tunnel-manager-desktop", OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
			if !autostart.LaunchHidden(data.Args, platform.HasTray()) {
				a.show()
			}
		}},
	}
}

func main() {
	a := &application{ready: make(chan struct{})}
	err := wails.Run(a.windowOptions())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
