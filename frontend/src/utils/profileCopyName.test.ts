import { describe, expect, it } from 'vitest';
import { suggestProfileCopyName } from './profileCopyName';

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
