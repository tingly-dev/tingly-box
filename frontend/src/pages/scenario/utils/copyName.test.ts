import { describe, expect, it } from 'vitest';
import { suggestProfileCopyName, suggestTeamCopyName } from './copyName';

describe('suggestProfileCopyName', () => {
    it('appends -copy', () => {
        expect(suggestProfileCopyName('work', ['work'])).toBe('work-copy');
    });

    it('skips taken names', () => {
        expect(suggestProfileCopyName('work', ['work', 'work-copy', 'work-copy-2'])).toBe('work-copy-3');
    });

    it('turns characters a profile name cannot hold into dashes', () => {
        expect(suggestProfileCopyName('my work!', [])).toBe('my-work-copy');
        expect(suggestProfileCopyName('!!', [])).toBe('profile-copy');
    });
});

describe('suggestTeamCopyName', () => {
    it('appends " copy" and skips taken names', () => {
        expect(suggestTeamCopyName('Research', ['Research'])).toBe('Research copy');
        expect(suggestTeamCopyName('Research', ['Research copy', 'Research copy 2'])).toBe('Research copy 3');
    });

    it('keeps the name within the 128-byte limit', () => {
        const name = suggestTeamCopyName('é'.repeat(100), []);
        expect(new TextEncoder().encode(name).length).toBeLessThanOrEqual(128);
        expect(name.endsWith(' copy')).toBe(true);
    });
});
