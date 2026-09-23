package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestLevelsAndFormats(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		for _, level := range []string{"debug", "info", "warn", "error"} {
			t.Run(format+"/"+level, func(t *testing.T) {
				var out bytes.Buffer
				logger, err := New(&out, level, format)
				if err != nil {
					t.Fatal(err)
				}
				logger.Debug("debug-event")
				logger.Info("info-event", "tunnel", "test")
				logger.Warn("warn-event")
				logger.Error("error-event")
				if strings.Contains(out.String(), "debug-event") != (level == "debug") {
					t.Fatal("debug filtering failed")
				}
				if strings.Contains(out.String(), "info-event") != (level == "debug" || level == "info") {
					t.Fatal("info filtering failed")
				}
				if !strings.Contains(out.String(), "error-event") {
					t.Fatal("error log missing")
				}
				if format == "json" {
					for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
						var record map[string]any
						if err := json.Unmarshal([]byte(line), &record); err != nil || record["time"] == nil || record["level"] == nil {
							t.Fatalf("invalid JSON log: %s", line)
						}
					}
				}
				ctx := WithLogger(context.Background(), logger)
				if FromContext(ctx) != logger {
					t.Fatal("context logger lost")
				}
			})
		}
	}
	for _, option := range [][2]string{{"trace", "text"}, {"info", "xml"}} {
		if _, err := New(&bytes.Buffer{}, option[0], option[1]); err == nil {
			t.Fatal("invalid log options accepted")
		}
	}
}

type sensitiveError struct{}

func (sensitiveError) Error() string        { return "proxy stderr contains unknown-secret" }
func (sensitiveError) SafeLogError() string { return "proxy handshake failed (stderr omitted)" }

func TestSafeError(t *testing.T) {
	message := "password=super-secret\n-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-material\n-----END OPENSSH PRIVATE KEY-----"
	got := SafeError(errors.New(message), "super-secret")
	for _, secret := range []string{"super-secret", "private-material", "BEGIN OPENSSH"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked: %s", got)
		}
	}
	got = SafeError(fmt.Errorf("outer: %w", sensitiveError{}))
	if strings.Contains(got, "unknown-secret") || !strings.Contains(got, "stderr omitted") {
		t.Fatalf("unsafe error: %s", got)
	}
	if SafeError(nil) != "" {
		t.Fatal("nil error not empty")
	}
}
