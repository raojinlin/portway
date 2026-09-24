package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ssh-tunnel-manager/desktop/platform"
	"ssh-tunnel-manager/internal/daemon"
)

func TestNativeMenusStartWithoutDOMReady(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{})}
	defer closeTestBackend(a)
	started := false
	updates := make(chan platform.TraySnapshot, 1)
	err := a.startDesktop(context.Background(), nativeMenus{
		start: func(title string, actions platform.TrayActions) error {
			if title != "Portway" || actions.Show == nil || actions.Quit == nil || actions.Directory == nil || actions.Logs == nil || actions.Copy == nil || actions.SetEnabled == nil || actions.Connections == nil {
				t.Fatal("missing native title or menu actions")
			}
			if a.service == nil {
				t.Fatal("menus started before backend initialization")
			}
			started = true
			return nil
		},
		update: func(value platform.TraySnapshot) {
			select {
			case updates <- value:
			default:
			}
		},
	})
	if err != nil || !started {
		t.Fatalf("native startup without DOMReady: started=%v err=%v", started, err)
	}
	select {
	case value := <-updates:
		if value.Summary != "暂无线路" || len(value.Lines) != 0 {
			t.Fatalf("unexpected menu summary: %+v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("menu summary was not updated")
	}
	assertDesktopLog(t, a, "desktop native menus ready")
}

func TestLanguageEvent(t *testing.T) {
	a := &application{language: "zh"}
	for _, data := range [][]interface{}{nil, {nil}, {1}, {"fr"}, {"en", "zh"}} {
		a.languageEvent(data...)
		if a.language != "zh" {
			t.Fatal("invalid language event accepted")
		}
	}
	a.languageEvent("en")
	if a.language != "en" {
		t.Fatal("English switch ignored")
	}
	a.languageEvent("zh")
	if a.language != "zh" {
		t.Fatal("Chinese switch ignored")
	}
}

func TestTrayActionRefreshAndFailureCleanup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{}), trayRefresh: make(chan struct{}, 1)}
	if err := a.changeTunnel("missing", true); err == nil {
		t.Fatal("uninitialized backend accepted action")
	}
	a.startup(context.Background())
	defer closeTestBackend(a)
	if err := a.changeTunnel("missing", true); err == nil {
		t.Fatal("missing tunnel accepted action")
	}
	if len(a.trayPending) != 0 {
		t.Fatal("failed action left menu busy")
	}
	select {
	case <-a.trayRefresh:
	default:
		t.Fatal("action did not request an immediate menu refresh")
	}
	a.trayPending["missing"] = true
	if err := a.changeTunnel("missing", false); err != nil {
		t.Fatal("duplicate in-flight action was not suppressed")
	}
	assertDesktopLog(t, a, "desktop tunnel action failed")
}

func TestLogNavigationRemainsPendingUntilApplied(t *testing.T) {
	a := &application{}
	a.openLogs() // No frontend context yet; retain the request for its ready event.
	if !a.pendingLogs {
		t.Fatal("startup log navigation was dropped")
	}
	a.navigationApplied("unknown")
	if !a.pendingLogs {
		t.Fatal("invalid acknowledgement cleared navigation")
	}
	a.navigationApplied("activity")
	if a.pendingLogs {
		t.Fatal("applied navigation would replay on reload")
	}
}

func TestConnectionNavigationKeepsLatestTargetUntilApplied(t *testing.T) {
	a := &application{}
	a.openConnections("first", false)
	a.openConnections("socks / 中文", true)
	a.navigationApplied(map[string]interface{}{"page": "connections", "name": "first", "showHistory": false})
	if a.pendingConnections == nil || a.pendingConnections.Name != "socks / 中文" || !a.pendingConnections.ShowHistory {
		t.Fatal("stale acknowledgement discarded latest connection target")
	}
	a.navigationApplied("activity")
	if a.pendingConnections == nil {
		t.Fatal("log acknowledgement discarded connections")
	}
	a.navigationApplied(map[string]interface{}{"page": "connections", "name": "socks / 中文", "showHistory": true})
	if a.pendingConnections != nil {
		t.Fatal("applied connection navigation retained")
	}
	a.openLogs()
	a.openConnections("stopped", true)
	if a.pendingLogs || a.pendingConnections == nil {
		t.Fatal("connections did not replace log navigation")
	}
	a.openLogs()
	if !a.pendingLogs || a.pendingConnections != nil {
		t.Fatal("logs did not replace connection navigation")
	}
}

func TestNativeMenuFailureIsReportedAndLogged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{})}
	defer closeTestBackend(a)
	failure := errors.New("status item unavailable")
	err := a.startDesktop(context.Background(), nativeMenus{
		start: func(string, platform.TrayActions) error { return failure },
	})
	if !errors.Is(err, failure) {
		t.Fatalf("lost native startup error: %v", err)
	}
	assertDesktopLog(t, a, "desktop native menus failed", failure.Error())
}

func TestNativeMenusDoNotStartAfterCancellation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{})}
	a.startup(context.Background())
	defer closeTestBackend(a)
	a.cancel()
	err := a.setupWindow(context.Background(), nativeMenus{
		start: func(string, platform.TrayActions) error { t.Fatal("menus created during shutdown"); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
}

func closeTestBackend(a *application) {
	if a.cancel != nil {
		a.cancel()
	}
	if a.service != nil {
		_ = a.service.Close()
	}
}

func assertDesktopLog(t *testing.T, a *application, messages ...string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.service.ConfigDirectory(), "logs", "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if !strings.Contains(string(data), message) {
			t.Fatalf("missing desktop log %q: %s", message, data)
		}
	}
}

func TestDesktopProductNameMatchesWindow(t *testing.T) {
	data, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Name string `json:"name"`
		Info struct {
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	a := &application{}
	title := a.windowOptions().Title
	if config.Name != title || config.Info.ProductName != title {
		t.Fatalf("desktop names disagree: window=%q app=%q product=%q", title, config.Name, config.Info.ProductName)
	}
	if config.Info.ProductVersion != applicationVersion {
		t.Fatalf("About version %q differs from package version %q", applicationVersion, config.Info.ProductVersion)
	}
}

func TestDesktopAboutMenuConfigured(t *testing.T) {
	a := &application{}
	options := a.windowOptions()
	if options.Mac == nil || options.Mac.About == nil {
		t.Fatal("macOS About menu is not configured")
	}
	about := options.Mac.About
	if about.Title != applicationTitle || !strings.Contains(about.Message, applicationVersion) || !strings.Contains(about.Message, "SOCKS5") {
		t.Fatalf("incomplete About information: %+v", about)
	}
}

func TestDesktopWindowAllowsZoom(t *testing.T) {
	a := &application{ready: make(chan struct{})}
	options := a.windowOptions()
	if options.Mac == nil || options.Mac.DisableZoom {
		t.Fatal("macOS zoom button must be enabled explicitly")
	}
	if options.DisableResize {
		t.Fatal("window must remain resizable")
	}
	if options.Fullscreen {
		t.Fatal("enabling zoom must not start the application in fullscreen")
	}
	wantTray := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	if options.HideWindowOnClose != wantTray {
		t.Fatalf("hide on close=%v, want %v on %s", options.HideWindowOnClose, wantTray, runtime.GOOS)
	}
}

func TestDesktopTransportAndConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{})}
	w := httptest.NewRecorder()
	a.serve(w, httptest.NewRequest("GET", "/api/config", nil))
	if w.Code != 503 {
		t.Fatal("uninitialized backend accepted request")
	}
	a.startup(context.Background())
	defer func() {
		a.cancel()
		if a.service != nil {
			a.service.Close()
		}
	}()
	if a.problem != nil {
		t.Fatal(a.problem)
	}
	w = httptest.NewRecorder()
	a.serve(w, httptest.NewRequest("GET", "/api/config", nil))
	var view struct {
		Config  daemon.FileConfig `json:"config"`
		Desktop bool              `json:"desktop"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || !view.Desktop {
		t.Fatalf("config: %s %v", w.Body.String(), err)
	}
	data, _ := json.Marshal(view.Config)
	for _, origin := range []string{"wails://wails", "http://wails.localhost", "null", "https://evil.example"} {
		req := httptest.NewRequest("PUT", "/api/config", strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		w = httptest.NewRecorder()
		a.serve(w, req)
		want := 200
		if origin == "https://evil.example" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("origin %s: %d %s", origin, w.Code, w.Body.String())
		}
		if req.Header.Get("Origin") != origin {
			t.Fatal("transport mutated original headers")
		}
	}
	if err := a.service.Close(); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	a.serve(w, httptest.NewRequest("GET", "/api/config", nil))
	if w.Code != 503 {
		t.Fatal("closed backend accepted request")
	}
}
