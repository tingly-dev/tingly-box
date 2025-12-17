import react from '@vitejs/plugin-react-swc';
import wails from "@wailsio/runtime/plugins/vite";
import { defineConfig } from 'vite';
import { viteMockServe } from 'vite-plugin-mock';

// Check if we should use mock data
const useMock = process.env.USE_MOCK === 'true'
console.log("use mock", useMock)

// Check if we're building for GUI mode
const isGuiMode = (process.env.VITE_PKG_MODE === 'gui')

// https://vite.dev/config/
export default defineConfig({
    plugins: [
        // Only include wails plugin when building for GUI mode
        ...(isGuiMode ? [wails("./src/bindings")] : []),
        react(),
        viteMockServe({
            mockPath: 'src/mock',
            enable: useMock,
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
