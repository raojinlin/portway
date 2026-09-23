.PHONY: web build desktop desktop-tools desktop-universal desktop-dmg test run

BINARY ?= portway

# Builds the React/antd frontend (web/) into internal/daemon/webui/dist,
# which is what go:embed picks up. Required before `go build` on a clean
# checkout, and any time web/ changes.
web:
	cd web && npm install && npm run build

build: web
	go build -o "$(BINARY)" ./cmd/tunnel

desktop-tools:
	go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

desktop: web
	node scripts/desktop.mjs

desktop-universal: web
	node scripts/desktop.mjs --universal

desktop-dmg: web
	node scripts/desktop.mjs --universal --dmg

test:
	go test ./...

run: web
	go run ./cmd/tunnel daemon
