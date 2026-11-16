import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useImageGenRuns } from './useImageGenRuns';

const mocks = vi.hoisted(() => ({
    generate: vi.fn(),
    edit: vi.fn(),
    save: vi.fn(),
}));

vi.mock('@/services/modelApi', () => ({
    getOpenAIClient: async () => ({ images: { generate: mocks.generate, edit: mocks.edit } }),
}));
vi.mock('@/utils/playgroundSession', () => ({
    loadPlaygroundSession: async () => ({ runs: [], imports: [] }),
    savePlaygroundSession: mocks.save,
}));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_key: string, options: { defaultValue: string }) => options.defaultValue }),
}));
vi.mock('./useImageGenRefs', () => ({ readImageSize: async () => null }));
vi.mock('@tingly/vision', () => ({ fetchBlob: async () => new Blob(['image'], { type: 'image/png' }) }));

const request = { prompt: 'a fox', model: 'image-rule', size: '1024x1024', quality: 'auto' as const, count: 1, sources: [] };

describe('image thinking effort transport', () => {
    beforeEach(() => {
        mocks.generate.mockReset().mockResolvedValue({ data: [] });
        mocks.edit.mockReset().mockResolvedValue({ data: [] });
        mocks.save.mockClear();
    });
    afterEach(() => cleanup());

    it('sends auto unchanged and keeps it in the durable run', async () => {
        const { result } = renderHook(() => useImageGenRuns(vi.fn(), 'effort-auto'));
        await act(async () => { await result.current.runGeneration(request); });
        expect(mocks.generate).toHaveBeenCalledWith(expect.objectContaining({ reasoning_effort: 'auto' }), expect.anything());
        expect(result.current.runs[0].reasoningEffort).toBe('auto');
        await waitFor(() => expect(mocks.save).toHaveBeenCalledWith(expect.objectContaining({
            runs: expect.arrayContaining([expect.objectContaining({ profileId: 'effort-auto', reasoningEffort: 'auto' })]),
        })));
    });

    it('preserves the original effort when retrying a failed generation', async () => {
        mocks.generate.mockRejectedValueOnce(new Error('upstream unavailable'));
        const { result } = renderHook(() => useImageGenRuns(vi.fn(), 'effort-retry'));
        await act(async () => { await result.current.runGeneration({ ...request, reasoningEffort: 'high' }); });
        const failed = result.current.runs[0];
        expect(failed.status).toBe('failed');
        await act(async () => { await result.current.handleRetry(failed); });
        expect(mocks.generate).toHaveBeenLastCalledWith(expect.objectContaining({ reasoning_effort: 'high' }), expect.anything());
        expect(result.current.runs).toHaveLength(1);
        expect(result.current.runs[0].reasoningEffort).toBe('high');
    });

    it('carries the same hint with image edits', async () => {
        const { result } = renderHook(() => useImageGenRuns(vi.fn(), 'effort-edit'));
        const file = new File(['image'], 'reference.png', { type: 'image/png' });
        await act(async () => {
            await result.current.runGeneration({ ...request, reasoningEffort: 'medium', sources: [{ file, previewUrl: 'blob:reference' }] });
        });
        expect(mocks.edit).toHaveBeenCalledWith(expect.objectContaining({ image: file, reasoning_effort: 'medium' }), expect.anything());
        expect(result.current.runs[0].reasoningEffort).toBe('medium');
    });

    it('retries older history without a stored effort as auto', async () => {
        const { result } = renderHook(() => useImageGenRuns(vi.fn(), 'effort-legacy'));
        await act(async () => {
            await result.current.handleRetry({ ...request, id: 'old-run', endpoint: 'generations', createdAt: 1, images: [], status: 'failed' });
        });
        expect(mocks.generate).toHaveBeenCalledWith(expect.objectContaining({ reasoning_effort: 'auto' }), expect.anything());
    });
});
