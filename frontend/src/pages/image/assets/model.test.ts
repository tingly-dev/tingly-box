import { describe, expect, it } from 'vitest';
import {
    appendPiece,
    collectTags,
    findPromptByText,
    normalizeTags,
    pieceLabel,
    suggestPieces,
    type PromptPiece,
} from './model';

const piece = (overrides: Partial<PromptPiece>): PromptPiece => ({
    id: 'x',
    kind: 'prompt',
    title: '',
    text: '',
    tags: [],
    createdAt: 0,
    updatedAt: 0,
    ...overrides,
});

describe('suggestPieces', () => {
    it('cuts a comma list into terms and longer clauses into phrases', () => {
        expect(suggestPieces('a red fox sleeping in the snow under a pine tree, cinematic lighting, 35mm')).toEqual([
            { text: 'a red fox sleeping in the snow under a pine tree', kind: 'phrase' },
            { text: 'cinematic lighting', kind: 'term' },
            { text: '35mm', kind: 'term' },
        ]);
    });

    it('splits sentences but not decimals, and keeps "3D"', () => {
        expect(suggestPieces('3D render at 1.5x scale. Soft light.').map((p) => p.text)).toEqual([
            '3D render at 1.5x scale.',
            'Soft light',
        ]);
    });

    it('treats a short fragment that ends a sentence as a term, without its full stop', () => {
        expect(suggestPieces('广角镜头，电影感光影。Soft light. A lone figure walks through the fog.')).toEqual([
            { text: '广角镜头', kind: 'term' },
            { text: '电影感光影', kind: 'term' },
            { text: 'Soft light', kind: 'term' },
            { text: 'A lone figure walks through the fog.', kind: 'phrase' },
        ]);
    });

    it('handles CJK punctuation and drops list markers and duplicates', () => {
        expect(suggestPieces('- 赛博朋克，霓虹灯、霓虹灯\n1. 雨后的街道上倒映着城市的灯光。')).toEqual([
            { text: '赛博朋克', kind: 'term' },
            { text: '霓虹灯', kind: 'term' },
            { text: '雨后的街道上倒映着城市的灯光。', kind: 'phrase' },
        ]);
    });
});

describe('appendPiece', () => {
    it('joins by what the prompt ends with', () => {
        expect(appendPiece('', 'rim light')).toBe('rim light');
        expect(appendPiece('a fox', 'rim light')).toBe('a fox, rim light');
        expect(appendPiece('a fox,', 'rim light')).toBe('a fox, rim light');
        expect(appendPiece('A fox.  ', 'Rim light.')).toBe('A fox. Rim light.');
    });
});

describe('asset helpers', () => {
    it('normalizes and counts tags', () => {
        expect(normalizeTags([' Style', 'style', '', 'Light '])).toEqual(['style', 'light']);
        expect(collectTags([piece({ tags: ['b', 'a'] }), piece({ tags: ['a'] })])).toEqual(['a', 'b']);
    });

    it('labels by title, else first line', () => {
        expect(pieceLabel({ title: '', text: '  line one\nline two' })).toBe('line one');
        expect(pieceLabel({ title: 'Fox', text: 'x' })).toBe('Fox');
    });

    it('finds an identical whole prompt ignoring surrounding space, not a piece', () => {
        expect(findPromptByText([piece({ id: 'a', text: 'fox' })], ' fox ')?.id).toBe('a');
        expect(findPromptByText([piece({ id: 'b', kind: 'term', text: 'fox' })], 'fox')).toBeUndefined();
    });
});
