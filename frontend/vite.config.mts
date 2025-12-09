import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import { viteMockServe } from 'vite-plugin-mock'

// Check if we should use mock data
const useMock = process.env.USE_MOCK === 'true'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    viteMockServe({
      mockPath: 'src/mock',
      enable: useMock,
      supportTs: true,
      logger: true,
    })
  ],
  server: {
    proxy: useMock ? {} : {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
        // Rewrite the path to remove /api prefix if your backend doesn't expect it
        // rewrite: (path) => path.replace(/^\/api/, '')
      }
    },
    port: 3000
  }
})
