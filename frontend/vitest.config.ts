/// <reference types="vitest" />
import { defineConfig } from 'vite';
import path from 'path';

// Separate Vitest config so the test environment does not pull in the
// production plugins (wails bindings, visualizer, etc.) while still sharing
// the `@/` path alias the source code relies on.
export default defineConfig({
    resolve: {
        alias: {
            '@': path.resolve(__dirname, './src'),
        },
    },
    // routes.contract.test.tsx reads the Wails tray's route constants from
    // ../gui/wails3 (via ?raw); Vite refuses files outside the root otherwise.
    server: {
        fs: {
            allow: ['..'],
        },
    },
    test: {
        globals: true,
        environment: 'jsdom',
        setupFiles: ['./src/test/setup.ts'],
        include: ['src/**/*.{test,spec}.{ts,tsx}', 'packages/*/src/**/*.{test,spec}.ts'],
    },
});
