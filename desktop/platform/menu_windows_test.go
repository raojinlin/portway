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
	snapshot := TraySnapshot{Summary: "Running 1", Traffic: "1024 B/s", Lines: []TrayLine{{Name: "test&line", Title: "test&line running", Details: []string{"RX 1024 B", "Connections 2"}}}}
	popup, err := newWindowsTrayPopup("Portway", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyMenu.Call(popup.handle)
	line := popup.lines["test&line"]
	if line.submenu == 0 || line.position != 3 {
		t.Fatal("missing line submenu")
	}
	read := func(menu, position uintptr) string {
		buffer := make([]uint16, 256)
		trayUser32.NewProc("GetMenuStringW").Call(menu, position, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), mfByPosition)
		return windows.UTF16ToString(buffer)
	}
	if got := read(popup.handle, 3); got != "test&&line running" {
		t.Fatalf("wrong menu label: %q", got)
	}
	snapshot.Traffic = "2048 B/s"
	snapshot.Lines[0].Details[0] = "RX 2048 B"
	popup.refresh(snapshot)
	if read(popup.handle, 1) != "2048 B/s" || read(line.submenu, 0) != "RX 2048 B" {
		t.Fatal("open menu did not update in place")
	}
	snapshot.Lines = nil
	popup.refresh(snapshot)
	if read(line.submenu, 0) != "" {
		t.Fatal("deleted line retains stale details")
	}
}

func TestWindowsTrayEmptyPopup(t *testing.T) {
	popup, err := newWindowsTrayPopup("Portway", TraySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	defer destroyMenu.Call(popup.handle)
	count, _, _ := trayUser32.NewProc("GetMenuItemCount").Call(popup.handle)
	if count != 8 || len(popup.lines) != 0 {
		t.Fatalf("unexpected empty menu: %d items", count)
	}
}
