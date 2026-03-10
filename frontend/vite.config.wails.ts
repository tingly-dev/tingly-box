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
                    // Optimized chunk splitting strategy - aligned with vite.config.ts
                    manualChunks: (id) => {
                        if (!id.includes('node_modules')) {
                            return;
                        }

                        // MUI packages - group together for better caching
                        if (id.includes('@mui/material') || id.includes('@mui/system') || id.includes('@mui/utils')) {
                            return 'mui-vendor';
                        }
                        if (id.includes('@mui/icons-material')) {
                            return 'mui-icons-vendor';
                        }
                        // Charts/visualization - depends on react and d3
                        if (id.includes('recharts') || id.includes('d3-') || id.includes('victory-')) {
                            return 'recharts-vendor';
                        }
                        // Let Rollup handle remaining node_modules automatically
                        return undefined;
                    },
                },
                maxParallelFileOps: 4,
            },
            chunkSizeWarningLimit: 500,
            sourcemap: mode !== 'production',
            minify: 'swc',
        },
    }
})