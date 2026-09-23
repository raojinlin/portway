package platform

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmContextMenu = 0x007B
	wmTimer       = 0x0113
	wmLeftUp      = 0x0202
	wmLeftDouble  = 0x0203
	wmRightUp     = 0x0205
	ninSelect     = 0x0400
	ninKeySelect  = 0x0401
	wmTray        = 0x8001
	wmTrayUpdate  = 0x8002
	mfDisabled    = 0x0001
	mfPopup       = 0x0010
	mfByPosition  = 0x0400
	mfSeparator   = 0x0800
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4
)

var (
	trayUser32            = windows.NewLazySystemDLL("user32.dll")
	trayShell32           = windows.NewLazySystemDLL("shell32.dll")
	registerClass         = trayUser32.NewProc("RegisterClassExW")
	unregisterClass       = trayUser32.NewProc("UnregisterClassW")
	createWindow          = trayUser32.NewProc("CreateWindowExW")
	defWindowProc         = trayUser32.NewProc("DefWindowProcW")
	destroyWindow         = trayUser32.NewProc("DestroyWindow")
	getMessage            = trayUser32.NewProc("GetMessageW")
	translateMessage      = trayUser32.NewProc("TranslateMessage")
	dispatchMessage       = trayUser32.NewProc("DispatchMessageW")
	postMessage           = trayUser32.NewProc("PostMessageW")
	postQuitMessage       = trayUser32.NewProc("PostQuitMessage")
	registerWindowMessage = trayUser32.NewProc("RegisterWindowMessageW")
	loadIcon              = trayUser32.NewProc("LoadIconW")
	setTimer              = trayUser32.NewProc("SetTimer")
	killTimer             = trayUser32.NewProc("KillTimer")
	createPopupMenu       = trayUser32.NewProc("CreatePopupMenu")
	destroyMenu           = trayUser32.NewProc("DestroyMenu")
	appendMenu            = trayUser32.NewProc("AppendMenuW")
	modifyMenu            = trayUser32.NewProc("ModifyMenuW")
	trackPopupMenu        = trayUser32.NewProc("TrackPopupMenuEx")
	endMenu               = trayUser32.NewProc("EndMenu")
	getCursorPos          = trayUser32.NewProc("GetCursorPos")
	setForegroundWindow   = trayUser32.NewProc("SetForegroundWindow")
	notifyIcon            = trayShell32.NewProc("Shell_NotifyIconW")
	winTrayMu             sync.RWMutex
	winTray               *windowsTray
)

type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          trayPoint
	Private        uint32
}
type trayWindowClass struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type trayNotifyIcon struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

type windowsTray struct {
	title                 string
	show, quit, directory func()
	window                atomic.Uintptr
	stopping              atomic.Bool
	ready                 chan error
	done                  chan struct{}
	snapshotMu            sync.Mutex
	snapshot              TraySnapshot
	// Everything below is owned exclusively by the tray message-loop thread.
	icon           trayNotifyIcon
	taskbarCreated uint32
	version4       bool
	popup          *windowsTrayPopup
}

func Start(title string, show, quit, directory func()) error {
	winTrayMu.Lock()
	if winTray != nil {
		winTrayMu.Unlock()
		return fmt.Errorf("Windows tray already started")
	}
	t := &windowsTray{title: title, show: show, quit: quit, directory: directory,
		ready: make(chan error, 1), done: make(chan struct{})}
	winTray = t
	winTrayMu.Unlock()
	go t.run()
	return <-t.ready
}

func Update(snapshot TraySnapshot) {
	winTrayMu.RLock()
	t := winTray
	winTrayMu.RUnlock()
	if t == nil || t.stopping.Load() {
		return
	}
	t.snapshotMu.Lock()
	t.snapshot = snapshot
	t.snapshotMu.Unlock()
	if hwnd := t.window.Load(); hwnd != 0 {
		postMessage.Call(hwnd, wmTrayUpdate, 0, 0)
	}
}

func Stop() {
	winTrayMu.RLock()
	t := winTray
	winTrayMu.RUnlock()
	if t == nil {
		return
	}
	if !t.stopping.Swap(true) {
		if hwnd := t.window.Load(); hwnd != 0 {
			postMessage.Call(hwnd, wmClose, 0, 0)
		}
	}
	<-t.done
}

func (t *windowsTray) current() TraySnapshot {
	t.snapshotMu.Lock()
	defer t.snapshotMu.Unlock()
	return t.snapshot
}

func (t *windowsTray) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(t.done)
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(2, nil, &instance); err != nil {
		t.ready <- err
		return
	}
	className, _ := windows.UTF16PtrFromString("Portway.TrayWindow")
	class := trayWindowClass{WndProc: windows.NewCallback(t.windowProc), Instance: uintptr(instance), ClassName: className}
	class.Size = uint32(unsafe.Sizeof(class))
	if result, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		t.ready <- fmt.Errorf("register tray window: %w", err)
		return
	}
	defer unregisterClass.Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
	// A hidden top-level window receives TaskbarCreated broadcasts; HWND_MESSAGE does not.
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		t.ready <- fmt.Errorf("create tray window: %w", err)
		return
	}
	t.window.Store(hwnd)
	defer func() {
		if t.window.Swap(0) != 0 {
			destroyWindow.Call(hwnd)
		}
	}()
	t.icon = trayNotifyIcon{Window: hwnd, ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: wmTray}
	t.icon.Size = uint32(unsafe.Sizeof(t.icon))
	// Wails packages the app icon as resource 3. LoadIcon returns a shared handle.
	t.icon.Icon, _, _ = loadIcon.Call(uintptr(instance), 3)
	if t.icon.Icon == 0 {
		t.icon.Icon, _, _ = loadIcon.Call(0, 32512)
	}
	copy(t.icon.Tip[:], trayUTF16(t.title, len(t.icon.Tip)))
	message, _ := windows.UTF16PtrFromString("TaskbarCreated")
	id, _, _ := registerWindowMessage.Call(uintptr(unsafe.Pointer(message)))
	t.taskbarCreated = uint32(id)
	if t.icon.Icon == 0 || !t.addIcon() {
		t.ready <- fmt.Errorf("cannot create Windows notification-area icon")
		return
	}
	defer notifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.icon)))
	t.ready <- nil
	var msg trayMessage
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *windowsTray) addIcon() bool {
	if result, _, _ := notifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.icon))); result == 0 {
		return false
	}
	t.icon.Version = 4
	result, _, _ := notifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&t.icon)))
	t.version4 = result != 0
	killTimer.Call(t.icon.Window, 1)
	return true
}

func (t *windowsTray) update() {
	snapshot := t.current()
	t.icon.Tip = [128]uint16{}
	copy(t.icon.Tip[:], trayUTF16(t.title+"\n"+snapshot.Summary+"\n"+snapshot.Traffic, len(t.icon.Tip)))
	if result, _, _ := notifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.icon))); result == 0 {
		if !t.addIcon() {
			setTimer.Call(t.icon.Window, 1, 2000, 0)
		}
	} else {
		killTimer.Call(t.icon.Window, 1)
	}
	if t.popup != nil {
		t.popup.refresh(snapshot)
	}
}

func (t *windowsTray) windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if t.taskbarCreated != 0 && message == t.taskbarCreated {
		if !t.stopping.Load() && !t.addIcon() {
			setTimer.Call(hwnd, 1, 2000, 0)
		}
		return 0
	}
	switch message {
	case wmTray:
		if t.stopping.Load() {
			return 0
		}
		switch uint32(lParam & 0xFFFF) {
		case ninSelect, ninKeySelect, wmLeftUp, wmLeftDouble:
			go t.show()
		case wmContextMenu:
			if t.version4 {
				point := trayContextPoint(wParam)
				t.openMenu(hwnd, &point)
			} else {
				t.openMenu(hwnd, nil)
			}
		case wmRightUp:
			t.openMenu(hwnd, nil)
		}
		return 0
	case wmTrayUpdate:
		if !t.stopping.Load() {
			t.update()
		}
		return 0
	case wmTimer:
		if wParam == 1 && !t.stopping.Load() {
			t.addIcon()
		}
		return 0
	case wmClose:
		if t.popup != nil {
			endMenu.Call()
		}
		notifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.icon)))
		destroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		t.window.Store(0)
		postQuitMessage.Call(0)
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func trayContextPoint(value uintptr) trayPoint {
	return trayPoint{X: int32(int16(value & 0xFFFF)), Y: int32(int16((value >> 16) & 0xFFFF))}
}

func (t *windowsTray) openMenu(hwnd uintptr, anchor *trayPoint) {
	if t.popup != nil {
		return
	}
	popup, err := newWindowsTrayPopup(t.title, t.current())
	if err != nil {
		go t.show()
		return
	}
	t.popup = popup
	defer func() { t.popup = nil; destroyMenu.Call(popup.handle) }()
	var point trayPoint
	if anchor != nil && (anchor.X != -1 || anchor.Y != -1) {
		point = *anchor
	} else {
		getCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	}
	setForegroundWindow.Call(hwnd)
	command, _, _ := trackPopupMenu.Call(popup.handle, 0x100|0x80|0x02, uintptr(point.X), uintptr(point.Y), hwnd, 0)
	postMessage.Call(hwnd, 0, 0, 0)
	if t.stopping.Load() {
		return
	}
	// Wails callbacks must not block this thread (Quit waits for tray shutdown).
	switch command {
	case 1:
		go t.show()
	case 2:
		go t.directory()
	case 3:
		go t.quit()
	}
}

type windowsLineMenu struct {
	position uintptr
	submenu  uintptr
	details  int
}
type windowsTrayPopup struct {
	handle uintptr
	lines  map[string]windowsLineMenu
}

func appendWindowsMenu(menu, flags, id uintptr, text string) error {
	label, _ := windows.UTF16PtrFromString(windowsMenuText(text))
	if result, _, err := appendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(label))); result == 0 {
		return fmt.Errorf("append tray menu: %w", err)
	}
	return nil
}

func newWindowsTrayPopup(title string, snapshot TraySnapshot) (_ *windowsTrayPopup, err error) {
	handle, _, callErr := createPopupMenu.Call()
	if handle == 0 {
		return nil, fmt.Errorf("create popup menu: %w", callErr)
	}
	popup := &windowsTrayPopup{handle: handle, lines: make(map[string]windowsLineMenu)}
	defer func() {
		if err != nil {
			destroyMenu.Call(handle)
		}
	}()
	if snapshot.Summary == "" {
		snapshot.Summary = title
	}
	for _, text := range []string{snapshot.Summary, snapshot.Traffic} {
		if err = appendWindowsMenu(handle, mfDisabled, 0, text); err != nil {
			return nil, err
		}
	}
	if err = appendWindowsMenu(handle, mfSeparator, 0, ""); err != nil {
		return nil, err
	}
	if len(snapshot.Lines) == 0 {
		if err = appendWindowsMenu(handle, mfDisabled, 0, "暂无线路"); err != nil {
			return nil, err
		}
	}
	for i, line := range snapshot.Lines {
		submenu, _, callErr := createPopupMenu.Call()
		if submenu == 0 {
			return nil, fmt.Errorf("create line submenu: %w", callErr)
		}
		if err = appendWindowsMenu(handle, mfPopup, submenu, line.Title); err != nil {
			destroyMenu.Call(submenu)
			return nil, err
		}
		popup.lines[line.Name] = windowsLineMenu{position: uintptr(3 + i), submenu: submenu, details: len(line.Details)}
		for _, detail := range line.Details {
			if err = appendWindowsMenu(submenu, 0, 0, detail); err != nil {
				return nil, err
			}
		}
	}
	if err = appendWindowsMenu(handle, mfSeparator, 0, ""); err != nil {
		return nil, err
	}
	for i, text := range []string{"打开 " + title, "打开配置目录", "退出 " + title} {
		if err = appendWindowsMenu(handle, 0, uintptr(i+1), text); err != nil {
			return nil, err
		}
	}
	return popup, nil
}

func updateWindowsMenu(menu, position, flags, id uintptr, text string) {
	label, _ := windows.UTF16PtrFromString(windowsMenuText(text))
	modifyMenu.Call(menu, position, mfByPosition|flags, id, uintptr(unsafe.Pointer(label)))
}

func (p *windowsTrayPopup) refresh(snapshot TraySnapshot) {
	updateWindowsMenu(p.handle, 0, mfDisabled, 0, snapshot.Summary)
	updateWindowsMenu(p.handle, 1, mfDisabled, 0, snapshot.Traffic)
	current := make(map[string]TrayLine, len(snapshot.Lines))
	for _, line := range snapshot.Lines {
		current[line.Name] = line
	}
	// Keep the open menu's structure stable; new/removed lines reconcile on reopen.
	for name, menu := range p.lines {
		line, exists := current[name]
		flags := uintptr(mfPopup)
		if !exists {
			flags |= mfDisabled
			line.Title = name + " · 已移除"
		}
		updateWindowsMenu(p.handle, menu.position, flags, menu.submenu, line.Title)
		for i := 0; i < menu.details; i++ {
			text := ""
			if i < len(line.Details) {
				text = line.Details[i]
			}
			updateWindowsMenu(menu.submenu, uintptr(i), 0, 0, text)
		}
	}
}
