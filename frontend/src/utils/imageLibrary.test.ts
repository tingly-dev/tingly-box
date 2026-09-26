import { describe, expect, it } from 'vitest';
import {
    appendPromptPiece,
    collectTags,
    findPromptByText,
    normalizeTags,
    promptLabel,
    suggestPromptPieces,
    type LibraryPrompt,
} from './imageLibrary';

const piece = (overrides: Partial<LibraryPrompt>): LibraryPrompt => ({
    id: 'x',
    kind: 'prompt',
    title: '',
    text: '',
    tags: [],
    createdAt: 0,
    updatedAt: 0,
    ...overrides,
});

describe('suggestPromptPieces', () => {
    it('cuts a comma list into terms and longer clauses into phrases', () => {
        expect(suggestPromptPieces('a red fox sleeping in the snow under a pine tree, cinematic lighting, 35mm')).toEqual([
            { text: 'a red fox sleeping in the snow under a pine tree', kind: 'phrase' },
            { text: 'cinematic lighting', kind: 'term' },
            { text: '35mm', kind: 'term' },
        ]);
    });

    it('splits sentences but not decimals, and keeps "3D"', () => {
        expect(suggestPromptPieces('3D render at 1.5x scale. Soft light.').map((p) => p.text)).toEqual([
            '3D render at 1.5x scale.',
            'Soft light.',
        ]);
    });

    it('handles CJK punctuation and drops list markers and duplicates', () => {
        expect(suggestPromptPieces('- 赛博朋克，霓虹灯、霓虹灯\n1. 雨后的街道上倒映着城市的灯光。')).toEqual([
            { text: '赛博朋克', kind: 'term' },
            { text: '霓虹灯', kind: 'term' },
            { text: '雨后的街道上倒映着城市的灯光。', kind: 'phrase' },
        ]);
    });
});

describe('appendPromptPiece', () => {
    it('joins by what the prompt ends with', () => {
        expect(appendPromptPiece('', 'rim light')).toBe('rim light');
        expect(appendPromptPiece('a fox', 'rim light')).toBe('a fox, rim light');
        expect(appendPromptPiece('a fox,', 'rim light')).toBe('a fox, rim light');
        expect(appendPromptPiece('A fox.  ', 'Rim light.')).toBe('A fox. Rim light.');
    });
});

describe('library helpers', () => {
    it('normalizes and counts tags', () => {
        expect(normalizeTags([' Style', 'style', '', 'Light '])).toEqual(['style', 'light']);
        expect(collectTags([piece({ tags: ['b', 'a'] }), piece({ tags: ['a'] })])).toEqual(['a', 'b']);
    });

    it('labels by title, else first line', () => {
        expect(promptLabel({ title: '', text: '  line one\nline two' })).toBe('line one');
        expect(promptLabel({ title: 'Fox', text: 'x' })).toBe('Fox');
    });

    it('finds an identical prompt ignoring surrounding space', () => {
        expect(findPromptByText([piece({ id: 'a', text: 'fox' })], ' fox ')?.id).toBe('a');
    });
});
