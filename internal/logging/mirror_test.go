package logging

import (
	"bytes"
	"io"
	"testing"
)

type unavailableConsole struct{}

func (unavailableConsole) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestMirrorDoesNotLoseFileLogsWithoutConsole(t *testing.T) {
	var file bytes.Buffer
	logger, err := New(Mirror(unavailableConsole{}, &file), "info", "json")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("desktop started")
	if !bytes.Contains(file.Bytes(), []byte("desktop started")) {
		t.Fatal("console failure suppressed file logging")
	}
}
