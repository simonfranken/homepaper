import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// `npm run dev` serves the UI on :5173 and proxies the API to the Go server
// on :8080. `npm run build` writes into ../dist, which the Go binary embeds.
export default defineConfig({
  plugins: [vue()],
  build: { outDir: '../dist', emptyOutDir: true },
  server: {
    proxy: {
      '/api/events': { target: 'ws://localhost:8080', ws: true },
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
})
