package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const registrationID = "io.github.ssh-tunnel-manager.desktop.autostart"
const fileMarker = "Managed by Portway: " + registrationID

type fileRegistration struct {
	path    string
	content []byte
	reason  string
}

func fileBackend(system, executable, home, config string) fileRegistration {
	r := fileRegistration{reason: "unsupported_platform"}
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n") {
		r.reason = "invalid_executable"
		return r
	}
	switch system {
	case "darwin":
		r.path = filepath.Join(home, "Library", "LaunchAgents", registrationID+".plist")
		macos := filepath.Dir(executable)
		contents := filepath.Dir(macos)
		app := filepath.Dir(contents)
		if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(strings.ToLower(app), ".app") {
			r.reason = "app_bundle_required"
			return r
		}
		var escaped bytes.Buffer
		_ = xml.EscapeText(&escaped, []byte(app))
		r.content = []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!-- %s -->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>/usr/bin/open</string><string>-g</string><string>-j</string><string>-a</string><string>%s</string><string>--args</string><string>--autostart</string></array>
<key>RunAtLoad</key><true/>
<key>LimitLoadToSessionType</key><string>Aqua</string>
</dict></plist>
`, fileMarker, registrationID, escaped.String()))
	case "linux":
		r.path = filepath.Join(config, "autostart", "portway.desktop")
		// Escape Exec arguments, then the desktop-file string layer. No shell is used.
		argument := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$", "%", "%%").Replace(executable)
		argument = strings.ReplaceAll(argument, "\\", "\\\\")
		r.content = []byte(fmt.Sprintf("# %s\n[Desktop Entry]\nType=Application\nVersion=1.0\nName=Portway\nExec=\"%s\" --autostart\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", fileMarker, argument))
	default:
		return r
	}
	r.reason = ""
	return r
}

func (r fileRegistration) read() ([]byte, error) {
	if r.path == "" {
		return nil, nil
	}
	info, err := os.Lstat(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return nil, fmt.Errorf("unexpected startup entry at %s", r.path)
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(data, []byte(fileMarker)) {
		return nil, fmt.Errorf("startup entry is not managed by Portway: %s", r.path)
	}
	return data, nil
}

func (r fileRegistration) status() (Status, error) {
	data, err := r.read()
	return Status{Supported: r.reason == "", Reason: r.reason, Enabled: data != nil,
		NeedsUpdate: data != nil && !bytes.Equal(data, r.content)}, err
}

func (r fileRegistration) set(enabled bool) error {
	data, err := r.read()
	if err != nil {
		return err
	}
	if !enabled {
		if data == nil {
			return nil
		}
		return os.Remove(r.path)
	}
	if r.reason != "" {
		return errUnsupported
	}
	if bytes.Equal(data, r.content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".portway-autostart-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(r.content); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), r.path)
}
