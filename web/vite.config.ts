import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Build for a local static bundle served by the Go server.
// base './' so the bundle works when served from any subpath via Go's
// http.FileServer and when opened directly in OBS Browser Source.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Keep the bundle light for the OBS Browser Source overlay.
    target: 'es2020',
    minify: 'esbuild',
  },
  server: {
    port: 5173,
    // In dev, proxy API + WS to the Go backend.
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/ws': { target: 'ws://127.0.0.1:8080', ws: true },
    },
  },
})
