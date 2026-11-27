import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useProviderQuota } from './useProviderQuota';

const { fetchUIAPI } = vi.hoisted(() => ({ fetchUIAPI: vi.fn() }));
vi.mock('@/services/api', () => ({ fetchUIAPI }));

describe('useProviderQuota automatic loading', () => {
    beforeEach(() => {
        fetchUIAPI.mockReset();
        fetchUIAPI.mockResolvedValue({ data: {} });
    });

    it('fetches a new provider when membership changes without a change in count', async () => {
        const { rerender } = renderHook(({ providers }) => useProviderQuota(providers), {
            initialProps: { providers: [{ uuid: 'a', name: 'A' }, { uuid: 'b', name: 'B' }] },
        });
        await waitFor(() => expect(fetchUIAPI).toHaveBeenCalledTimes(1));

        rerender({ providers: [{ uuid: 'b', name: 'B renamed' }, { uuid: 'a', name: 'A' }] });
        expect(fetchUIAPI).toHaveBeenCalledTimes(1);

        rerender({ providers: [{ uuid: 'a', name: 'A' }, { uuid: 'c', name: 'C' }] });
        await waitFor(() => expect(fetchUIAPI).toHaveBeenCalledTimes(2));
        expect(fetchUIAPI).toHaveBeenLastCalledWith('/provider-quota/batch', {
            method: 'POST',
            body: JSON.stringify({ provider_uuids: ['a', 'c'] }),
        });
    });
});
