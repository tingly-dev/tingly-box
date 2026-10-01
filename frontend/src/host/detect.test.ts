import { describe, expect, it } from 'vitest';
import { isDesktopShell } from './detect';

describe('isDesktopShell', () => {
    it.each([
        ['wails:', 'localhost', true], // macOS / Linux asset server
        ['http:', 'wails.localhost', true], // Windows (WebView2)
        ['http:', 'localhost', false], // tb open, vite dev
        ['http:', '127.0.0.1', false],
        ['https:', 'box.example.com', false], // team deployment
    ])('%s//%s → %s', (protocol, hostname, expected) => {
        expect(isDesktopShell({ protocol, hostname })).toBe(expected);
    });

    it('is false for the test page itself', () => {
        expect(isDesktopShell()).toBe(false);
    });
});
