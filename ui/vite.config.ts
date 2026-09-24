import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: '../internal/server/dist', emptyOutDir: true },
  server: { proxy: { '/api': { target: 'http://127.0.0.1:7788', ws: true } } },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'], globals: true, exclude: ['e2e/**', 'node_modules/**'] },
} as never)
