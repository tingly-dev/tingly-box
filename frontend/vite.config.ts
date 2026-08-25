import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';
import { visualizer } from 'rollup-plugin-visualizer';
import path from 'path';

// Web-only Vite configuration
// For Wails builds, use vite.config.wails.ts instead
export default defineConfig(({ mode }) => {
    // Check if we should use mock data. Read from the mode's .env file
    // (`.env.mock` sets USE_MOCK=true) via loadEnv so it works on any shell —
    // a `USE_MOCK=true vite ...` prefix is POSIX-only and breaks on Windows.
    // A real `USE_MOCK` env var still overrides it (loadEnv merges process.env).
    const env = loadEnv(mode, process.cwd(), '');
    const useMock = env.USE_MOCK === 'true';
    console.log("use mock", useMock)

    return {
        plugins: [
            react(),
            // Bundle analyzer - only in analyze mode for development
            // Run with: pnpm build -- --mode analyze
            mode === 'analyze' && visualizer({
                open: true,
                gzipSize: true,
                brotliSize: true,
                filename: 'dist/stats.html',
            }),
        ].filter(Boolean),
        define: {
            // Make USE_MOCK available to the app
            'import.meta.env.VITE_USE_MOCK': JSON.stringify(useMock ? 'true' : 'false'),
        },
        resolve: {
            // stylis must resolve to a single instance. @emotion/cache imports it
            // internally and the RTL cache (see i18n/DirectionProvider.tsx) imports
            // its `prefixer`; if the dev dep-optimizer pre-bundles those into two
            // separate chunks, emotion's serializer hands elements built by one
            // copy to middleware from the other and every page throws
            // "Cannot read properties of undefined (reading 'push')".
            dedupe: ['stylis', '@emotion/react', '@emotion/cache'],
            alias: {
                // Web mode: always use mock bindings
                '@/bindings': '/src/bindings-web',
                '@': path.resolve(__dirname, './src'),
            }
        },
        server: {
            proxy: useMock ? {} : {
                '/api/': {
                    target: 'http://localhost:12580',
                    changeOrigin: true,
                    secure: false,
                },
                '/tingly/': {
                    target: 'http://localhost:12580',
                    changeOrigin: true,
                    secure: false,
                }
            },
            port: 3000
        },
        // Memory optimization for build process
        optimizeDeps: {
            // Pre-bundle large dependencies to reduce build memory.
            // @mui/icons-material is NOT listed: the app only ever imports it as
            // a type (see components/icons/tablerMui.tsx), so it's a devDependency
            // with no runtime module to pre-bundle.
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
                    // Routes are lazy-loaded (see App.tsx), so Rollup already splits each
                    // page into its own chunk at the import() boundary, and shares code
                    // between pages automatically where they overlap.
                    //
                    // Only MUI is forced into its own vendor chunk here: it's imported
                    // eagerly by Layout/App (not just lazy pages), so it belongs in the
                    // always-loaded set and benefits from a stable, cacheable chunk name.
                    // (@mui/icons-material is deliberately NOT grouped here — it's only
                    // ever imported as a type, so no runtime module from it ever reaches
                    // the bundle; see the optimizeDeps comment above.)
                    //
                    // recharts/d3 deliberately have NO manual rule. They're only used by
                    // two lazy pages (Dashboard, UserUsage) and are never imported eagerly
                    // — but forcing them into a named "recharts-vendor" chunk previously
                    // made the bundler treat that chunk as always-needed and preload it
                    // from index.html on every page load (~850KB, unused outside those two
                    // routes). Leaving them unnamed lets Rollup fold them into a
                    // dynamic-import-only shared chunk that's fetched solely when one of
                    // those two pages is actually visited. Verify with `pnpm build` +
                    // check dist/index.html's <link rel="modulepreload"> list before
                    // re-adding a manual grouping for either of these.
                    manualChunks: (id) => {
                        if (id.includes('node_modules')) {
                            // MUI packages
                            if (id.includes('@mui/material') || id.includes('@mui/system') || id.includes('@mui/utils')) {
                                return 'mui-vendor';
                            }
                        }
                        return undefined;
                    },
                },
                maxParallelFileOps: 4,
            },
            chunkSizeWarningLimit: 500,
            // Disable sourcemap in production to reduce memory and output size
            sourcemap: mode !== 'production',
            // 'swc' is not a value Vite/Rolldown recognizes for build.minify
            // (valid: true | false | 'oxc' | 'terser' | 'esbuild') — it was
            // silently falling through to unminified output. 'oxc' is
            // Rolldown-Vite's native minifier (same as the `true` default).
            minify: 'oxc',
        },
    }
})