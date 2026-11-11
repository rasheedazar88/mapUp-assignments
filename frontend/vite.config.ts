/// <reference types="vitest/config" />
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 5173 },
  build: {
    // Big, rarely-changing libraries in their own long-cacheable chunks.
    rolldownOptions: {
      output: {
        advancedChunks: {
          groups: [
            { name: 'antd', test: /node_modules[\\/](antd|@ant-design|rc-|@rc-component)/ },
            { name: 'leaflet', test: /node_modules[\\/](leaflet|react-leaflet|@react-leaflet|@geoman-io)/ },
            { name: 'react', test: /node_modules[\\/](react|react-dom|react-router|scheduler)[\\/]/ },
          ],
        },
      },
    },
    chunkSizeWarningLimit: 1400, // antd alone is ~1.26 MB minified (~400 kB gzip)
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
