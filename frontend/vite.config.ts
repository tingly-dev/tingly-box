import react from '@vitejs/plugin-react-swc';
import { defineConfig } from 'vite';
import { visualizer } from 'rollup-plugin-visualizer';
import path from 'path';

// Web-only Vite configuration
// For Wails builds, use vite.config.wails.ts instead
export default defineConfig(({ mode }) => {
    // Check if we should use mock data
    const useMock = process.env.USE_MOCK === 'true'
    console.log("use mock", useMock)

    return {
        plugins: [
            react(),
            // Bundle analyzer - generates dist/stats.html for analysis
            visualizer({
                open: false,
                gzipSize: true,
                brotliSize: true,
                filename: 'dist/stats.html',
            }),
        ],
        define: {
            // Make USE_MOCK available to the app
            'import.meta.env.VITE_USE_MOCK': JSON.stringify(useMock ? 'true' : 'false'),
        },
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
                    manualChunks: (id) => {
                        if (id.includes('node_modules')) {
                            // MUI packages
                            if (id.includes('@mui/material') || id.includes('@mui/system') || id.includes('@mui/utils')) {
                                return 'mui-vendor';
                            }
                            if (id.includes('@mui/icons-material')) {
                                return 'mui-icons-vendor';
                            }
                            // Recharts + d3
                            if (id.includes('recharts') || id.includes('d3-') || id.includes('victory-')) {
                                return 'recharts-vendor';
                            }
                        }
                        // App pages chunked by feature area
                        if (id.includes('/pages/remote-control/')) {
                            return 'pages-remote-control';
                        }
                        if (id.includes('/pages/remote-coder/')) {
                            return 'pages-remote-coder';
                        }
                        if (id.includes('/pages/scenario/')) {
                            return 'pages-scenario';
                        }
                        if (id.includes('/pages/prompt/')) {
                            return 'pages-prompt';
                        }
                        if (id.includes('/pages/overview/') || id.includes('/pages/system/')) {
                            return 'pages-misc';
                        }
                        return undefined;
                    },
                },
                maxParallelFileOps: 4,
            },
            chunkSizeWarningLimit: 500,
            // Disable sourcemap in production to reduce memory and output size
            sourcemap: mode !== 'production',
            // Use SWC for minification (via @vitejs/plugin-react-swc)
            minify: 'swc',
        },
    }
})