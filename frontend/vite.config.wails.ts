import react from '@vitejs/plugin-react';
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
            // Bundle analyzer - only in analyze mode
            mode === 'analyze' && visualizer({
                open: true,
                gzipSize: true,
                brotliSize: true,
                filename: 'dist/stats.html',
            }),
        ].filter(Boolean),
        resolve: {
            // stylis must resolve to a single instance. @emotion/cache imports it
            // internally and the RTL cache (see i18n/DirectionProvider.tsx) imports
            // its `prefixer`; if the dev dep-optimizer pre-bundles those into two
            // separate chunks, emotion's serializer hands elements built by one
            // copy to middleware from the other and every page throws
            // "Cannot read properties of undefined (reading 'push')".
            dedupe: ['stylis', '@emotion/react', '@emotion/cache'],
            alias: {
                // Wails mode: use real bindings
                '@/bindings': '/src/bindings-wails',
                '@': path.resolve(__dirname, './src'),
            }
        },
        // Memory optimization for build process.
        // @mui/icons-material is NOT listed: the app only ever imports it as a
        // type (see components/icons/tablerMui.tsx), so it's a devDependency
        // with no runtime module to pre-bundle.
        optimizeDeps: {
            include: [
                'react',
                'react-dom',
                '@mui/material',
                // Optimized together so they share one pre-bundled stylis chunk
                // — see the resolve.dedupe comment above.
                '@emotion/react',
                '@emotion/cache',
                'stylis',
                'stylis-plugin-rtl',
            ],
        },
        build: {
            rollupOptions: {
                output: {
                    // Chunk splitting strategy — kept in sync with vite.config.ts (see
                    // that file's comment for the full rationale). Routes are lazy-loaded
                    // (see App.tsx), so only MUI — imported eagerly by Layout/App — is
                    // forced into its own vendor chunk. recharts/d3 deliberately have NO
                    // manual rule: they're only reached through lazy pages (Dashboard,
                    // UserUsage), and forcing them into a named vendor chunk makes the
                    // bundler preload it on every page load instead of only when one of
                    // those pages is actually visited.
                    manualChunks: (id) => {
                        if (!id.includes('node_modules')) {
                            return;
                        }

                        // MUI packages - group together for better caching
                        if (id.includes('@mui/material') || id.includes('@mui/system') || id.includes('@mui/utils')) {
                            return 'mui-vendor';
                        }
                        // Let Rollup handle remaining node_modules automatically
                        return undefined;
                    },
                },
                maxParallelFileOps: 4,
            },
            chunkSizeWarningLimit: 500,
            sourcemap: mode !== 'production',
            // 'swc' is not a value Vite/Rolldown recognizes for build.minify
            // (valid: true | false | 'oxc' | 'terser' | 'esbuild') — it was
            // silently falling through to unminified output. 'oxc' is
            // Rolldown-Vite's native minifier (same as the `true` default).
            minify: 'oxc',
        },
    }
})