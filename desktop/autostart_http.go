package main

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"ssh-tunnel-manager/desktop/autostart"
)

func autostartHandler(get func() (autostart.Status, error), set func(bool) (autostart.Status, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fail := func(code int, message string) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		}
		var result autostart.Status
		var err error
		switch r.Method {
		case http.MethodGet:
			result, err = get()
		case http.MethodPut:
			media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if media != "application/json" {
				fail(http.StatusUnsupportedMediaType, "expected application/json")
				return
			}
			var body struct {
				Enabled *bool `json:"enabled"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&body) != nil || body.Enabled == nil {
				fail(http.StatusBadRequest, "expected an enabled boolean")
				return
			}
			if decoder.Decode(new(any)) != io.EOF {
				fail(http.StatusBadRequest, "unexpected trailing JSON")
				return
			}
			result, err = set(*body.Enabled)
		default:
			w.Header().Set("Allow", "GET, PUT")
			fail(http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err != nil {
			fail(http.StatusInternalServerError, err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}
