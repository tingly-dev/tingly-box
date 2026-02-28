import react from '@vitejs/plugin-react-swc';
import {defineConfig} from 'vite';
import {viteMockServe} from 'vite-plugin-mock';
import {visualizer} from 'rollup-plugin-visualizer';
import path from 'path';

// Web-only Vite configuration
// For Wails builds, use vite.config.wails.ts instead
export default defineConfig(({mode}) => {
    // Check if we should use mock data
    const useMock = process.env.USE_MOCK === 'true'
    console.log("use mock", useMock)

    return {
        plugins: [
            react(),
            ...(useMock ? [viteMockServe({
                mockPath: 'src/mock',
                enable: useMock,
                logger: true,
            })] : []),
            // Bundle analyzer - generates dist/stats.html for analysis
            visualizer({
                open: false,
                gzipSize: true,
                brotliSize: true,
                filename: 'dist/stats.html',
            }),
        ],
        resolve: {
            alias: {
                // Web mode: always use mock bindings
                '@/bindings': '/src/bindings-web',
                '@': path.resolve(__dirname, './src'),
            }
        },
        server: {
            proxy: useMock ? {} : {
                '/api': {
                    target: 'http://localhost:12580',
                    changeOrigin: true,
                    secure: false,
                }
            },
            port: 3000
        },
        // Memory optimization for build process
        optimizeDeps: {
            // Pre-bundle large dependencies to reduce build memory
            include: [
                'react',
                'react-dom',
                '@mui/material',
                '@mui/icons-material',
            ],
        },
        build: {
            rollupOptions: {
                output: {
                    manualChunks: {
                        'mui-vendor': ['@mui/material'],
                        'mui-icons-vendor': ['@mui/icons-material'],
                        'react-vendor': ['react', 'react-dom', 'react-router-dom'],
                        'recharts-vendor': ['recharts'],
                        // 'antd-vendor': ['@ant-design/x-markdown'],
                        // 'lobe-vendor': ['@lobehub/icons'],
                    }
                }
            },
            chunkSizeWarningLimit: 500,
            // Disable sourcemap in production to reduce memory and output size
            sourcemap: mode !== 'production',
            // Use SWC for minification (via @vitejs/plugin-react-swc)
            minify: 'swc',
        },
    }
})