import { describe, it, expect, vi } from 'vitest';
import { reviveAsset } from './assetStore';
import { base64DataUrlToBlob } from '../components/imageGenSession';
import type { ImageAsset } from './assetTypes';

const asset = (over: Partial<ImageAsset>): ImageAsset => ({ id: 'a', src: '', name: 'a.png', createdAt: 1, ...over });

describe('base64DataUrlToBlob', () => {
    it('decodes a base64 data URL with its mime type', () => {
        const blob = base64DataUrlToBlob('data:image/png;base64,aW1n');
        expect(blob?.size).toBe(3);
        expect(blob?.type).toBe('image/png');
    });

    it('leaves URL-encoded data URLs and links alone', () => {
        expect(base64DataUrlToBlob('data:image/svg+xml;charset=utf-8,%3Csvg%2F%3E')).toBeNull();
        expect(base64DataUrlToBlob('https://cdn.example/a.png')).toBeNull();
    });
});

describe('reviveAsset', () => {
    it('rebuilds src from the kept blob in place', () => {
        const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock/new');
        const saved = asset({ src: 'blob:mock/stale', blob: new Blob(['x']) });
        expect(reviveAsset(saved)).toBe(saved);
        expect(saved.src).toBe('blob:mock/new');
        create.mockRestore();
    });

    it('migrates a base64 asset from an older build into a blob, as a new object', () => {
        const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock/new');
        const saved = asset({ src: 'data:image/png;base64,aW1n' });
        const revived = reviveAsset(saved);
        expect(revived).not.toBe(saved);
        expect(revived.blob?.size).toBe(3);
        expect(revived.src).toBe('blob:mock/new');
        create.mockRestore();
    });

    it('leaves a link or an unencodable data URL as it is', () => {
        const link = asset({ src: 'https://cdn.example/a.png' });
        expect(reviveAsset(link)).toBe(link);
    });
});
