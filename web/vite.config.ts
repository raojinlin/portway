import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Built assets are embedded straight into the Go binary via go:embed, so
// the build output goes directly into internal/daemon/webui/dist — no
// separate copy step. base is relative so the page works regardless of
// what path the daemon happens to serve it from.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/daemon/webui/dist',
    emptyOutDir: true,
  },
})
