import { describe, expect, it } from 'vitest';
import { splitPathTail } from './OutputDirChip';

describe('splitPathTail', () => {
    it('keeps the last folder whole and lets the front give way', () => {
        expect(splitPathTail('/home/demo/.tingly-box/image')).toEqual({ head: '/home/demo/.tingly-box', tail: '/image' });
        expect(splitPathTail('C:\\Users\\demo\\image\\')).toEqual({ head: 'C:\\Users\\demo', tail: '\\image' });
    });

    it('leaves a single-segment path unsplit', () => {
        expect(splitPathTail('/image')).toEqual({ head: '', tail: '/image' });
        expect(splitPathTail('image')).toEqual({ head: '', tail: 'image' });
    });
});
