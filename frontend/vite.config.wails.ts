import react from '@vitejs/plugin-react-swc';
import wails from "@wailsio/runtime/plugins/vite";
import {defineConfig} from 'vite';
import {visualizer} from 'rollup-plugin-visualizer';
import path from 'path';

// Wails-specific Vite configuration
// This config extends the base configuration with Wails-specific plugins
export default defineConfig(({mode}) => {
    return {
        plugins: [
            react(),
            // Wails plugin for binding generation
            wails("./src/bindings"),
            // Bundle analyzer
            visualizer({
                open: false,
                gzipSize: true,
                brotliSize: true,
                filename: 'dist/stats.html',
            }),
        ],
        resolve: {
            alias: {
                // Wails mode: use real bindings
                '@/bindings': '/src/bindings-wails',
                '@': path.resolve(__dirname, './src'),
            }
        },
        // Memory optimization for build process
        optimizeDeps: {
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

                        // MUI split by sub-package
                        if (id.includes('node_modules/@mui/material/')) {
                            return 'mui-material';
                        }
                        if (id.includes('node_modules/@mui/icons-material/')) {
                            return 'mui-icons';
                        }
                        if (id.includes('node_modules/@mui/x-date-pickers/')) {
                            return 'mui-pickers';
                        }

                        // Visualization
                        if (id.includes('node_modules/recharts/') || id.includes('node_modules/d3-')) {
                            return 'charts-vendor';
                        }

                        // Icon libraries
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

                        // Markdown
                        if (id.includes('node_modules/@ant-design/x-markdown/')) {
                            return 'markdown-vendor';
                        }
                    },
                },
                maxParallelFileOps: 20,
            },
            chunkSizeWarningLimit: 500,
            sourcemap: mode !== 'production',
            minify: 'swc',
        },
    }
})