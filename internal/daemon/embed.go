package daemon

import (
	"embed"
	"io/fs"
	"net/http"
)

// webui/dist is the Vite build output (see web/ at the repo root and
// web/vite.config.ts, which builds straight into this directory). It must
// exist at compile time — run `make web` (or `cd web && npm install && npm
// run build`) before `go build` on a clean checkout.
//
//go:embed webui/dist
var webUIFS embed.FS

func webUIHandler() http.Handler {
	return http.FileServerFS(WebAssets())
}

// WebAssets returns the embedded frontend for HTTP and desktop asset servers.
func WebAssets() fs.FS {
	sub, err := fs.Sub(webUIFS, "webui/dist")
	if err != nil {
		panic(err) // embedded at build time; a failure here is a programming error
	}
	return sub
}
