import { describe, expect, it } from 'vitest';
import { slugify } from './download';

describe('slugify', () => {
    it('builds a filename-safe stem', () => {
        expect(slugify('A sleepy orange cat!')).toBe('a-sleepy-orange-cat');
    });

    it('keeps CJK subjects instead of emptying the name', () => {
        expect(slugify('犯困的橘猫')).toBe('犯困的橘猫');
    });

    it('truncates without leaving a trailing separator', () => {
        expect(slugify('a very long prompt that runs past the limit', 12)).toBe('a-very-long');
    });

    it('falls back when nothing usable survives', () => {
        expect(slugify('!!! ???')).toBe('image');
    });
});
