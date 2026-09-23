package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <stdlib.h>
int stmTrayStart(const char *name);
void stmTrayUpdate(const char *title);
void stmTrayStop(void);
void stmSetAppearance(int dark, int followSystem);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

var events = make(chan int, 8)
var stopped = make(chan struct{})
var stopOnce sync.Once
var startOnce sync.Once
var startErr error

//export stmMenuAction
func stmMenuAction(action C.int) {
	select {
	case events <- int(action):
	default:
	}
}

// AppKit work is dispatched to the main thread without replacing Wails' app delegate.
func Start(title string, show, quit, directory func()) error {
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
						show()
					case 2:
						directory()
					case 3:
						quit()
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
