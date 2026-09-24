package main

import (
	"errors"
	"net/http/httptest"
	"ssh-tunnel-manager/desktop/autostart"
	"strings"
	"testing"
)

func TestAutostartHTTP(t *testing.T) {
	for _, tc := range []struct {
		method, body, content string
		code, writes          int
	}{
		{"GET", "", "", 200, 0},
		{"PUT", `{"enabled":true}`, "application/json", 200, 1},
		{"PUT", `{"enabled":false}`, "application/json", 200, 1},
		{"PUT", `{}`, "application/json", 400, 0},
		{"PUT", `{"enabled":null}`, "application/json", 400, 0},
		{"PUT", `{"enabled":"true"}`, "application/json", 400, 0},
		{"PUT", `{"enabled":true,"extra":1}`, "application/json", 400, 0},
		{"PUT", `{"enabled":true} {}`, "application/json", 400, 0},
		{"PUT", `{"enabled":true}`, "text/plain", 415, 0},
		{"DELETE", "", "", 405, 0},
	} {
		t.Run(tc.method+tc.body+tc.content, func(t *testing.T) {
			writes := 0
			h := autostartHandler(func() (autostart.Status, error) { return autostart.Status{Supported: true}, nil }, func(enabled bool) (autostart.Status, error) {
				writes++
				return autostart.Status{Supported: true, Enabled: enabled}, nil
			})
			r := httptest.NewRequest(tc.method, "/api/desktop/autostart", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code || writes != tc.writes {
				t.Fatalf("status=%d writes=%d body=%s", w.Code, writes, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache enabled")
			}
		})
	}
	h := autostartHandler(func() (autostart.Status, error) { return autostart.Status{}, errors.New("denied") }, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/desktop/autostart", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "denied") {
		t.Fatal(w)
	}
}
