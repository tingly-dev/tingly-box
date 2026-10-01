import { describe, expect, it } from 'vitest';
import { deriveLabel } from './promptLabel';

describe('deriveLabel', () => {
    it('takes the opening clause', () => {
        expect(deriveLabel('团子趴在窗台上晒太阳，眯着眼睛。')).toBe('团子趴在窗台上晒太阳');
    });

    it('cuts a long clause with an ellipsis', () => {
        expect(deriveLabel('林夏坐在雨夜便利店门口的台阶上，手里捧着热咖啡')).toBe('林夏坐在雨夜便利店门…');
    });

    it('is empty for an empty prompt, so the caller can fall back', () => {
        expect(deriveLabel('   ')).toBe('');
    });
});
