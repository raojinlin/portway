package platform

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsTrayABILayout(t *testing.T) {
	classSize, messageSize, iconSize := uintptr(80), uintptr(48), uintptr(976)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		classSize, messageSize, iconSize = 48, 32, 956
	}
	if unsafe.Sizeof(trayWindowClass{}) != classSize || unsafe.Sizeof(trayMessage{}) != messageSize || unsafe.Sizeof(trayNotifyIcon{}) != iconSize {
		t.Fatalf("invalid Win32 structure layout: class=%d message=%d icon=%d", unsafe.Sizeof(trayWindowClass{}), unsafe.Sizeof(trayMessage{}), unsafe.Sizeof(trayNotifyIcon{}))
	}
}

func TestWindowsTrayContextPoint(t *testing.T) {
	point := trayContextPoint(0xFFECFFF6)
	if point.X != -10 || point.Y != -20 {
		t.Fatalf("incorrect coordinates on a left/top monitor: %+v", point)
	}
}

func TestWindowsTrayPopupRefresh(t *testing.T) {
	snapshot := TraySnapshot{Summary: "Running 1", Traffic: "1024 B/s", Lines: []TrayLine{{Name: "test&line", Enabled: true, Title: "test&line running", Details: []string{"RX 1024 B", "Connections 2"}, ProxyURL: "socks5h://127.0.0.1:1080", ProxyCommand: "$env:ALL_PROXY='socks5h://127.0.0.1:1080'"}}}
	popup, err := newWindowsTrayPopup("Portway", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyMenu.Call(popup.handle)
	line := popup.lines["test&line"]
	if line.submenu == 0 || line.position != 3 {
		t.Fatal("missing line submenu")
	}
	if line.copyID < 100 || popup.copies[line.copyID] != snapshot.Lines[0].ProxyURL || popup.copies[line.copyID+1] != snapshot.Lines[0].ProxyCommand {
		t.Fatal("missing proxy copy actions")
	}
	read := func(menu, position uintptr) string {
		buffer := make([]uint16, 256)
		trayUser32.NewProc("GetMenuStringW").Call(menu, position, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), mfByPosition)
		return windows.UTF16ToString(buffer)
	}
	state := func(menu, position uintptr) uintptr {
		value, _, _ := trayUser32.NewProc("GetMenuState").Call(menu, position, mfByPosition)
		return value
	}
	if got := read(popup.handle, 3); got != "test&&line running" {
		t.Fatalf("wrong menu label: %q", got)
	}
	if read(line.submenu, line.togglePosition) != "停止线路" || popup.actions[line.toggleID] != (TunnelAction{Name: "test&line", Enabled: false}) {
		t.Fatal("running tunnel is missing stop action")
	}
	if state(popup.handle, 0)&mfDisabled != 0 || state(line.submenu, 0)&mfDisabled != 0 {
		t.Fatal("statistics or line details use disabled text styling")
	}
	snapshot.Traffic = "2048 B/s"
	snapshot.Lines[0].Details[0] = "RX 2048 B"
	popup.refresh(snapshot)
	if read(popup.handle, 1) != "2048 B/s" || read(line.submenu, 0) != "RX 2048 B" {
		t.Fatal("open menu did not update in place")
	}
	snapshot.Language = "en"
	snapshot.Lines[0].ProxyURL = "socks5h://127.0.0.1:2080"
	snapshot.Lines[0].ProxyCommand = "$env:ALL_PROXY='socks5h://127.0.0.1:2080'"
	popup.refresh(snapshot)
	if read(line.submenu, line.copyPosition) != "Copy Proxy URL" || read(line.submenu, line.copyPosition+1) != "Copy Proxy Command" || popup.copies[line.copyID] != snapshot.Lines[0].ProxyURL || popup.copies[line.copyID+1] != snapshot.Lines[0].ProxyCommand {
		t.Fatal("proxy actions did not update language and payload")
	}
	if read(popup.handle, popup.actionsPosition) != "Open Portway" || read(popup.handle, popup.actionsPosition+1) != "Open Logs" || read(popup.handle, popup.actionsPosition+3) != "Quit Portway" {
		t.Fatal("open menu did not switch language")
	}
	proxyURL, proxyCommand := snapshot.Lines[0].ProxyURL, snapshot.Lines[0].ProxyCommand
	snapshot.Lines[0].Busy = true
	popup.refresh(snapshot)
	if _, exists := popup.actions[line.toggleID]; exists || read(line.submenu, line.togglePosition) != "Working..." || state(line.submenu, line.togglePosition)&mfDisabled == 0 {
		t.Fatal("busy tunnel action remains enabled")
	}
	snapshot.Lines[0].Busy = false
	snapshot.Lines[0].Enabled = false
	snapshot.Lines[0].ProxyURL, snapshot.Lines[0].ProxyCommand = "", ""
	popup.refresh(snapshot)
	count, _, _ := trayUser32.NewProc("GetMenuItemCount").Call(line.submenu)
	if count != line.copyPosition || popup.copies[line.copyID] != "" || popup.copies[line.copyID+1] != "" {
		t.Fatal("stopped line retains proxy actions")
	}
	if read(line.submenu, line.togglePosition) != "Start Tunnel" || !popup.actions[line.toggleID].Enabled {
		t.Fatal("stopped tunnel did not switch to start action")
	}
	snapshot.Lines[0].Enabled = true
	snapshot.Lines[0].ProxyURL, snapshot.Lines[0].ProxyCommand = proxyURL, proxyCommand
	popup.refresh(snapshot)
	count, _, _ = trayUser32.NewProc("GetMenuItemCount").Call(line.submenu)
	if count != line.copyPosition+2 || read(line.submenu, line.copyPosition) != "Copy Proxy URL" || popup.copies[line.copyID] != proxyURL || popup.copies[line.copyID+1] != proxyCommand {
		t.Fatal("restarted line did not restore proxy actions")
	}
	snapshot.Lines = nil
	popup.refresh(snapshot)
	if read(line.submenu, 0) != "" {
		t.Fatal("deleted line retains stale details")
	}
	count, _, _ = trayUser32.NewProc("GetMenuItemCount").Call(line.submenu)
	if popup.copies[line.copyID] != "" || popup.copies[line.copyID+1] != "" || count != line.copyPosition {
		t.Fatal("deleted line retains proxy actions")
	}
	if _, exists := popup.actions[line.toggleID]; exists || state(line.submenu, line.togglePosition)&mfDisabled == 0 {
		t.Fatal("deleted tunnel retains start/stop action")
	}
}

func TestWindowsTrayEmptyPopup(t *testing.T) {
	popup, err := newWindowsTrayPopup("Portway", TraySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	defer destroyMenu.Call(popup.handle)
	count, _, _ := trayUser32.NewProc("GetMenuItemCount").Call(popup.handle)
	if count != 9 || len(popup.lines) != 0 {
		t.Fatalf("unexpected empty menu: %d items", count)
	}
}
