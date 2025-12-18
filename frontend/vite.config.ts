import react from '@vitejs/plugin-react-swc';
import wails from "@wailsio/runtime/plugins/vite";
import {defineConfig} from 'vite';
import {viteMockServe} from 'vite-plugin-mock';

// Check if we should use mock data
const useMock = process.env.USE_MOCK === 'true'
console.log("use mock", useMock)

// Check if we're building for GUI mode
const useGUI = process.env.USE_GUI === 'true'
console.log("use gui", useGUI)

// https://vite.dev/config/
export default defineConfig({
    plugins: [
        react(),
        // Only include wails plugin when building for GUI mode
        ...(useGUI ? [wails("./src/bindings")] : []),
        ...(useMock ? [viteMockServe({
            mockPath: 'src/mock',
            enable: useMock,
            logger: true,
        })] : []),
    ],
    resolve: {
        alias: {
            // Provide fallback for bindings in non-GUI builds
            '@/bindings': useGUI ?
                '/src/bindings-wails' :
                '/src/bindings-web'
        }
    },
    server: {
        proxy: useMock ? {} : {
            '/api': {
                target: 'http://localhost:12580',
                changeOrigin: true,
                secure: false,
                // Rewrite the path to remove /api prefix if your backend doesn't expect it
                // rewrite: (path) => path.replace(/^\/api/, '')
            }
        },
        port: 3000
    }
})
