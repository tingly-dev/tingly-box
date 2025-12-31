import react from '@vitejs/plugin-react-swc';
import wails from "@wailsio/runtime/plugins/vite";
import {defineConfig} from 'vite';
import {viteMockServe} from 'vite-plugin-mock';
import path from 'path';

// https://vite.dev/config/
export default defineConfig(({mode}) => {
    // Check if we should use mock data
    const useMock = process.env.USE_MOCK === 'true'
    console.log("use mock", useMock)

    const isWails = mode === 'development-wails' || mode === 'production-wails'
    console.log("is wails mode", isWails)

    return {
        plugins: [
            react(),
            // Only include wails plugin when building for GUI mode
            ...(isWails ? [wails("./src/bindings")] : []),
            ...(useMock ? [viteMockServe({
                mockPath: 'src/mock',
                enable: useMock,
                logger: true,
            })] : []),
        ],
        resolve: {
            alias: {
                // Provide fallback for bindings in non-GUI builds
                '@/bindings': isWails ?
                    '/src/bindings-wails' :
                    '/src/bindings-web',
                '@': path.resolve(__dirname, './src'),
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
    }
})
