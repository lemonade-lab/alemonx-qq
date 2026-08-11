import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // The host proxy owns the browser-facing plugin mount and HMR route. Vite
  // must keep serving its own root path for its startup health check.
  base: './',
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:17390'
      }
    }
  },
  build: {
    outDir: '../web',
    emptyOutDir: true
  }
})
