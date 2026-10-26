/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/sippy-ng/',
  build: {
    outDir: 'build',
  },
  optimizeDeps: {
    include: [
      '@mui/material',
      '@mui/material/Grid2',
      '@mui/icons-material',
      '@mui/system',
      '@mui/x-data-grid',
      '@mui/x-date-pickers',
      '@emotion/react',
      '@emotion/styled',
    ],
  },
  server: {
    port: 3000,
    host: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.jsx'],
    css: false,
  },
})
