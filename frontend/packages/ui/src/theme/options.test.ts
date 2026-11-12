import { describe, expect, it } from 'vitest';
import { THEME_MODE_VALUES, readStoredThemeMode } from './options';

describe('readStoredThemeMode', () => {
    it.each(THEME_MODE_VALUES)('keeps the current mode %s', (mode) => {
        expect(readStoredThemeMode(mode)).toBe(mode);
    });

    it('maps the retired sunlit theme to light', () => {
        expect(readStoredThemeMode('sunlit')).toBe('light');
        expect(THEME_MODE_VALUES).not.toContain('sunlit');
    });

    it.each([null, '', 'neon'])('falls back to system for %s', (value) => {
        expect(readStoredThemeMode(value)).toBe('system');
    });
});
