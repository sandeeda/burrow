import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development, run Burrow on :8080 and `npm run dev`; API calls are proxied.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: false,
  },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
})
