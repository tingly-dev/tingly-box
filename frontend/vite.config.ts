import react from '@vitejs/plugin-react-swc';
import wails from "@wailsio/runtime/plugins/vite";
import {defineConfig} from 'vite';
import {viteMockServe} from 'vite-plugin-mock';
import {visualizer} from 'rollup-plugin-visualizer';
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
                    // Optimized chunk splitting strategy
                    manualChunks: (id) => {
                        // Skip node_modules internal
                        if (!id.includes('node_modules/')) {
                            return;
                        }

                        // Core React vendors
                        if (id.includes('node_modules/react/') || id.includes('node_modules/react-dom/')) {
                            return 'react-vendor';
                        }
                        if (id.includes('node_modules/react-router-dom/')) {
                            return 'router-vendor';
                        }

                        // MUI split by sub-package for better caching
                        if (id.includes('node_modules/@mui/material/')) {
                            return 'mui-material';
                        }
                        if (id.includes('node_modules/@mui/icons-material/')) {
                            return 'mui-icons';
                        }
                        if (id.includes('node_modules/@mui/x-date-pickers/')) {
                            return 'mui-pickers';
                        }

                        // Visualization - recharts brings heavy D3 dependencies
                        if (id.includes('node_modules/recharts/') || id.includes('node_modules/d3-')) {
                            return 'charts-vendor';
                        }

                        // Third-party icon libraries
                        if (id.includes('node_modules/@lobehub/icons/')) {
                            return 'lobehub-icons';
                        }
                        if (id.includes('node_modules/devicons-react/')) {
                            return 'devicons';
                        }

                        // i18n
                        if (id.includes('node_modules/i18next/') || id.includes('node_modules/react-i18next/')) {
                            return 'i18n-vendor';
                        }

                        // Markdown processing
                        if (id.includes('node_modules/@ant-design/x-markdown/')) {
                            return 'markdown-vendor';
                        }
                    },
                },
                // Increase parallel file operations limit for faster builds
                maxParallelFileOps: 20,
            },
            chunkSizeWarningLimit: 500,
            // Disable sourcemap in production to reduce memory and output size
            sourcemap: mode !== 'production',
            // Use SWC for minification (via @vitejs/plugin-react-swc)
            // SWC minify is 20-40x faster than terser
            minify: 'swc',
        },
    }
})