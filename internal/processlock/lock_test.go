package processlock

import (
	"path/filepath"
	"testing"
)

func TestExclusiveLockAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := Acquire(path); err == nil {
		second.Close()
		t.Fatal("duplicate owner acquired lock")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("stale lock file prevented reopening: %v", err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal("close not idempotent:", err)
	}
	if third, err := Acquire(path); err == nil {
		third.Close()
		t.Fatal("second close released another owner's lock")
	}
}
