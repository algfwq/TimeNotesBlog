import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // Absolute base keeps nested SPA routes (/note/:id, /admin/:token/*) from resolving assets as relative paths.
  base: '/',
  build: {
    outDir: '../web',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5190,
  },
  worker: {
    format: 'es',
  },
})
