import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// The Go server (`task dev:backend`) listens on 12590; the dev server proxies
// the API to it. Production builds are embedded into the Go binary.
export default defineConfig({
    plugins: [react()],
    server: {
        proxy: {
            '/api/': { target: 'http://localhost:12590', changeOrigin: true },
        },
    },
    build: {
        rollupOptions: {
            output: {
                // Same policy as tingly-box: only MUI is eager enough to deserve
                // a named vendor chunk; route-only deps stay in lazy chunks.
                manualChunks: (id) => {
                    if (id.includes('node_modules') && (id.includes('@mui/material') || id.includes('@mui/system') || id.includes('@mui/utils'))) {
                        return 'mui-vendor';
                    }
                    return undefined;
                },
            },
        },
        chunkSizeWarningLimit: 600,
    },
    test: {
        environment: 'jsdom',
        globals: true,
        setupFiles: ['./src/test/setup.ts'],
    },
});
