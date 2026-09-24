package autostart

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistrationLifecycle(t *testing.T) {
	for _, system := range []string{"darwin", "linux"} {
		t.Run(system, func(t *testing.T) {
			root := t.TempDir()
			r := fileBackend(system, filepath.Join(root, "Portway.app", "Contents", "MacOS", "portway"), root, root)
			check := func(enabled, update bool) {
				t.Helper()
				s, err := r.status()
				if err != nil || !s.Supported || s.Enabled != enabled || s.NeedsUpdate != update {
					t.Fatalf("status: %+v %v", s, err)
				}
			}
			check(false, false)
			for range 2 {
				if err := r.set(true); err != nil {
					t.Fatal(err)
				}
			}
			check(true, false)
			info, err := os.Stat(r.path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Fatalf("permissions: %v", info.Mode())
			}
			r.content = append(r.content, '\n')
			check(true, true)
			if err := r.set(true); err != nil {
				t.Fatal(err)
			}
			check(true, false)
			for range 2 {
				if err := r.set(false); err != nil {
					t.Fatal(err)
				}
			}
			check(false, false)
		})
	}
}

func TestMacPlistAndUnsupported(t *testing.T) {
	root := t.TempDir()
	r := fileBackend("darwin", filepath.Join(root, "A & B.app", "Contents", "MacOS", "portway"), root, root)
	var parsed any
	if err := xml.Unmarshal(r.content, &parsed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.content), "A &amp; B.app") || !strings.Contains(string(r.content), "--autostart") {
		t.Fatal(string(r.content))
	}
	r = fileBackend("darwin", filepath.Join(root, "portway"), root, root)
	if s, err := r.status(); err != nil || s.Supported || s.Reason != "app_bundle_required" {
		t.Fatalf("%+v %v", s, err)
	}
	if err := r.set(true); err != errUnsupported {
		t.Fatalf("%v", err)
	}
}

func TestProtectForeignEntries(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreign", true: "symlink"}[symlink], func(t *testing.T) {
			root := t.TempDir()
			r := fileBackend("linux", filepath.Join(root, "portway"), root, root)
			if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
				t.Fatal(err)
			}
			if symlink {
				target := filepath.Join(root, "target")
				if err := os.WriteFile(target, r.content, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, r.path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(r.path, []byte("foreign entry"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{false, true} {
				if err := r.set(enabled); err == nil {
					t.Fatal("modified foreign entry")
				}
			}
			if _, err := os.Lstat(r.path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLinuxExecEscaping(t *testing.T) {
	r := fileBackend("linux", `/opt/A $B%"C\D/portway`, t.TempDir(), t.TempDir())
	if !strings.Contains(string(r.content), `Exec="/opt/A \\$B%%\\"C\\\\D/portway" --autostart`) {
		t.Fatal(string(r.content))
	}
}

func TestLaunchHidden(t *testing.T) {
	if !LaunchHidden([]string{"--autostart"}, true) || LaunchHidden(nil, true) || LaunchHidden([]string{"--autostart"}, false) || LaunchHidden([]string{"--other"}, true) {
		t.Fatal("incorrect launch visibility")
	}
}
