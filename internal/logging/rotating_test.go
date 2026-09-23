package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestRotatingFileRetentionAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "test.log")
	w, err := OpenRotating(path, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"one\n", "two\n", "six\n", "ten\n"} {
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for suffix, want := range map[string]string{"": "ten\n", ".1": "six\n", ".2": "two\n"} {
		got, err := os.ReadFile(path + suffix)
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", suffix, got, err)
		}
		info, _ := os.Stat(path + suffix)
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions: %v", info.Mode())
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("too many archives")
	}
	if _, err := w.Write([]byte("closed")); err == nil {
		t.Fatal("write after close succeeded")
	}
	w, err = OpenRotating(path, 20, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Write([]byte("append\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "ten\nappend\n" {
		t.Fatalf("restart truncated file: %q", got)
	}
}

func TestRotatingConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	w, err := OpenRotating(path, 1024*1024, 2)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 100; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, err := w.Write([]byte(fmt.Sprintf("%03d\n", n))); err != nil {
				t.Error(err)
			}
		}(n)
	}
	wg.Wait()
	w.Close()
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 400 {
		t.Fatalf("lost writes: %d %v", len(data), err)
	}
}
