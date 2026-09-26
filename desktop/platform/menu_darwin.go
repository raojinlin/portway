package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework CoreText -framework UniformTypeIdentifiers
#include <stdlib.h>
int stmTrayStart(const char *name);
void stmTrayUpdate(const char *title);
void stmTrayStop(void);
void stmSetAppearance(int dark, int followSystem);
int stmSystemChinese(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

var events = make(chan int, 8)
var tunnelEvents = make(chan TunnelAction, 8)

type connectionEvent struct {
	name    string
	history bool
}

var connectionEvents = make(chan connectionEvent, 8)
var stopped = make(chan struct{})
var stopOnce sync.Once
var startOnce sync.Once
var startErr error

func SystemLanguage() string {
	if C.stmSystemChinese() != 0 {
		return "zh"
	}
	return "en"
}

//export stmMenuAction
func stmMenuAction(action C.int) {
	select {
	case events <- int(action):
	default:
	}
}

//export stmTunnelAction
func stmTunnelAction(name *C.char, enabled C.int) {
	select {
	case tunnelEvents <- TunnelAction{Name: C.GoString(name), Enabled: enabled != 0}:
	default:
	}
}

//export stmConnectionsAction
func stmConnectionsAction(name *C.char, history C.int) {
	select {
	case connectionEvents <- connectionEvent{name: C.GoString(name), history: history != 0}:
	default:
	}
}

// AppKit work is dispatched to the main thread without replacing Wails' app delegate.
func Start(title string, actions TrayActions) error {
	startOnce.Do(func() {
		name := C.CString(title)
		defer C.free(unsafe.Pointer(name))
		if result := C.stmTrayStart(name); result != 0 {
			startErr = fmt.Errorf("AppKit menu initialization failed (code %d: 1=application menu unavailable, 2=status item unavailable)", int(result))
			return
		}
		go func() {
			for {
				select {
				case action := <-events:
					switch action {
					case 1:
						actions.Show()
					case 2:
						actions.Directory()
					case 3:
						actions.Quit()
					case 4:
						actions.Logs()
					case 5:
						if actions.MCP != nil {
							actions.MCP()
						}
					case 6:
						if actions.SetMCP != nil {
							go actions.SetMCP(true)
						}
					case 7:
						if actions.SetMCP != nil {
							go actions.SetMCP(false)
						}
					}
				case action := <-tunnelEvents:
					if actions.SetEnabled != nil {
						go actions.SetEnabled(action.Name, action.Enabled)
					}
				case action := <-connectionEvents:
					if actions.Connections != nil {
						go actions.Connections(action.name, action.history)
					}
				case <-stopped:
					return
				}
			}
		}()
	})
	return startErr
}

func Update(snapshot TraySnapshot) {
	data, _ := json.Marshal(snapshot)
	value := C.CString(string(data))
	defer C.free(unsafe.Pointer(value))
	C.stmTrayUpdate(value)
}

func Stop() { stopOnce.Do(func() { close(stopped); C.stmTrayStop() }) }

func SetAppearance(dark, followSystem bool) {
	value := C.int(0)
	if dark {
		value = 1
	}
	system := C.int(0)
	if followSystem {
		system = 1
	}
	C.stmSetAppearance(value, system)
}
