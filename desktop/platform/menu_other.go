//go:build !darwin && !windows

package platform

func Start(title string, show, quit, directory func()) error { return nil }
func Update(snapshot TraySnapshot)                           {}
func Stop()                                                  {}
