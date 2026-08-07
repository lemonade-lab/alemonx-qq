import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // The plugin web UI is served by alx under /api/v1/setup/plugins/web/<id>/.
  // Relative base keeps Vite's asset URLs inside that mount.
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
