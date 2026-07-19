/// <reference types="vitest" />
import { defineConfig } from 'vite';
import path from 'path';

// Separate Vitest config so the test environment does not pull in the
// production plugins (wails bindings, visualizer, etc.) while still sharing
// the `@/` path alias the source code relies on.
export default defineConfig({
    resolve: {
        alias: {
            '@/bindings': path.resolve(__dirname, './src/bindings-web'),
            '@': path.resolve(__dirname, './src'),
        },
    },
    test: {
        globals: true,
        environment: 'jsdom',
        setupFiles: ['./src/test/setup.ts'],
        include: ['src/**/*.{test,spec}.{ts,tsx}'],
    },
});
