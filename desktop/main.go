package main

import (
	"context"
	"encoding/json"
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

	"ssh-tunnel-manager/desktop/platform"
	"ssh-tunnel-manager/internal/daemon"
)

const applicationTitle = "Portway"
const applicationVersion = "0.1.0"

type application struct {
	mu       sync.RWMutex
	ctx      context.Context
	service  *daemon.Service
	problem  error
	ready    chan struct{}
	cancel   context.CancelFunc
	done     <-chan struct{}
	nativeMu sync.Mutex
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
	// Wails owns this in-process transport. Its webview uses a custom origin,
	// not the public HTTP daemon's http/https origin; do not weaken HTTP checks.
	r = r.Clone(r.Context())
	r.Header.Del("Origin")
	service.ServeHTTP(w, r)
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
}

type menuStarter func(string, func(), func(), func()) error

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
	executable, _ := os.Executable()
	service.Logger().Info("desktop native menus initializing", "app", applicationTitle, "platform", runtime.GOOS, "executable", executable)
	if err := menus.start(applicationTitle, a.show, func() { wruntime.Quit(ctx) }, func() { platform.OpenDirectory(service.ConfigDirectory()) }); err != nil {
		service.Logger().Error("desktop native menus failed", "error", err)
		return fmt.Errorf("cannot initialize desktop menus: %w", err)
	}
	service.Logger().Info("desktop native menus ready", "app", applicationTitle, "tray", platform.HasTray())
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var sampler traySampler
		for {
			menus.update(sampler.sample(service.TunnelViews(), time.Now()))
			select {
			case <-a.done:
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
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
		BackgroundColour:  options.NewRGB(243, 245, 247),
		// Wails leaves the native zoom button disabled when Mac options are nil.
		Mac: &mac.Options{DisableZoom: false, About: &mac.AboutInfo{
			Title:   applicationTitle,
			Message: "Version " + applicationVersion + "\n\nSSH tunnel manager for local, remote and SOCKS5 forwarding.",
		}},
		Windows:     desktopWindowsOptions(),
		AssetServer: &assetserver.Options{Assets: daemon.WebAssets(), Handler: http.HandlerFunc(a.serve)},
		OnStartup: func(ctx context.Context) {
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
				_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.ErrorDialog,
					Title: applicationTitle, Message: "Cannot start Portway.\n\n" + err.Error() + "\n\nIf a CLI daemon is running, stop it before opening the desktop app.", Buttons: []string{"OK"}})
				wruntime.Quit(ctx)
			}
		}, OnDomReady: a.domReady, OnShutdown: a.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "ssh-tunnel-manager-desktop", OnSecondInstanceLaunch: func(options.SecondInstanceData) { a.show() }},
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
