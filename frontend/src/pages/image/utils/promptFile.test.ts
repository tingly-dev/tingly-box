import { describe, expect, it } from 'vitest';
import { isPromptFile, partitionDroppedFiles, PROMPT_FILE_MAX_BYTES, readPromptFile } from './promptFile';

const file = (name: string, content: string, type = ''): File => new File([content], name, { type });

describe('isPromptFile', () => {
    it('accepts text documents by MIME type or by extension when the type is blank', () => {
        expect(isPromptFile(file('notes.txt', 'x', 'text/plain'))).toBe(true);
        expect(isPromptFile(file('prompt.md', 'x'))).toBe(true);
        expect(isPromptFile(file('spec.yaml', 'x'))).toBe(true);
        expect(isPromptFile(file('template.json', 'x', 'application/json'))).toBe(true);
    });

    it('never mistakes an image for a prompt', () => {
        expect(isPromptFile(file('sheet.png', 'x', 'image/png'))).toBe(false);
        expect(isPromptFile(file('photo.JPG', 'x', ''))).toBe(false);
    });
});

describe('readPromptFile', () => {
    it('returns the text without the trailing newline editors leave', async () => {
        const result = await readPromptFile(file('p.md', 'a sticker sheet\n\n'));
        expect(result).toEqual({ ok: true, text: 'a sticker sheet', name: 'p.md' });
    });

    it('refuses an empty file and an oversized one', async () => {
        expect(await readPromptFile(file('empty.txt', '   \n'))).toMatchObject({ ok: false, reason: 'empty' });
        const big = file('big.txt', 'x'.repeat(PROMPT_FILE_MAX_BYTES + 1));
        expect(await readPromptFile(big)).toMatchObject({ ok: false, reason: 'too-large' });
    });
});

describe('partitionDroppedFiles', () => {
    it('sends the text file to the prompt and the images on', () => {
        const drop = [file('a.png', 'x', 'image/png'), file('prompt.txt', 'x', 'text/plain'), file('b.webp', 'x', 'image/webp')];
        const { prompt, images } = partitionDroppedFiles(drop);
        expect(prompt?.name).toBe('prompt.txt');
        expect(images.map((f) => f.name)).toEqual(['a.png', 'b.webp']);
    });
});
