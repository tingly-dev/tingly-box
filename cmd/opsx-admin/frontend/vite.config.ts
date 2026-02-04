import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import path from 'path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 9246,
    proxy: {
      '/admin': {
        target: 'http://localhost:18080',
        changeOrigin: true,
      },
      '/health': {
        target: 'http://localhost:18080',
        changeOrigin: true,
      },
    },
  },
});
