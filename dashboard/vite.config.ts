import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/jobs': 'http://127.0.0.1:8080',
      '/d': 'http://127.0.0.1:8080',
      '/reports': 'http://127.0.0.1:8080',
      '/ws': { target: 'http://127.0.0.1:8080', ws: true },
    },
  },
})
