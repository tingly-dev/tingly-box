import { describe, it, expect, vi } from 'vitest';
import { reviveProfile } from './imageProfileStore';
import type { ImageProfile } from './imageProfileTypes';

const profile = (previewUrls: string[]): ImageProfile => ({
    id: 'p1',
    name: 'p',
    refs: previewUrls.map((previewUrl) => ({ file: new File(['x'], 'r.png'), previewUrl, source: 'upload' as const })),
    prompts: [],
    activePromptId: '',
    model: '',
    size: '1024x1024',
    quality: 'auto',
    count: 1,
    updatedAt: 1,
});

describe('reviveProfile', () => {
    it('rebuilds preview URLs in place, so the store has nothing new to write', () => {
        const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock/new');
        const saved = profile(['blob:mock/stale']);
        const revived = reviveProfile(saved);
        expect(revived).toBe(saved);
        expect(saved.refs[0].previewUrl).toBe('blob:mock/new');
        create.mockRestore();
    });

    it('drops the base64 copy older builds saved, as a new object that gets saved once', () => {
        const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock/new');
        const saved = profile(['data:image/png;base64,aW1n']);
        const revived = reviveProfile(saved);
        expect(revived).not.toBe(saved);
        expect(revived.refs[0].previewUrl).toBe('blob:mock/new');
        expect(saved.refs[0].previewUrl).toBe('data:image/png;base64,aW1n');
        create.mockRestore();
    });
});
