import {act, renderHook} from '@testing-library/react';
import {vi} from 'vitest';
import type {Mock} from 'vitest';
import {useSelfUpdate} from './useSelfUpdate';
import {api} from '@/services/api';

vi.mock('@/services/api', () => ({
    api: {applyUpdate: vi.fn(), getVersion: vi.fn()},
}));

const applyUpdate = api.applyUpdate as Mock;
const getVersion = api.getVersion as Mock;

beforeEach(() => {
    applyUpdate.mockReset();
    getVersion.mockReset();
});

describe('useSelfUpdate', () => {
    it('reloads once the restarted server reports the new version', async () => {
        applyUpdate.mockResolvedValue({success: true, data: {version: '0.261001.1', restarting: true, restart_required: false}});
        // Release builds report a "v" prefix and build metadata.
        getVersion.mockResolvedValueOnce('Unknown').mockResolvedValueOnce('v0.260924.1').mockResolvedValue('v0.261001.1+abc');
        const reload = vi.fn();

        const {result} = renderHook(() => useSelfUpdate({pollIntervalMs: 1, timeoutMs: 1000, reload}));
        await act(() => result.current.start());

        expect(reload).toHaveBeenCalledTimes(1);
        expect(getVersion).toHaveBeenCalledTimes(3);
        expect(result.current.state).toEqual({phase: 'restarting', version: '0.261001.1'});
    });

    it('asks for a service restart when the server is supervised', async () => {
        applyUpdate.mockResolvedValue({success: true, data: {version: '0.261001.1', restarting: false, restart_required: true}});
        const reload = vi.fn();

        const {result} = renderHook(() => useSelfUpdate({reload}));
        await act(() => result.current.start());

        expect(result.current.state).toEqual({phase: 'restartRequired', version: '0.261001.1'});
        expect(getVersion).not.toHaveBeenCalled();
        expect(reload).not.toHaveBeenCalled();
    });

    it('reports a refused or failed update', async () => {
        applyUpdate.mockResolvedValue({success: false, error: 'started via npx; run the npx command with the new version'});

        const {result} = renderHook(() => useSelfUpdate());
        await act(() => result.current.start());

        expect(result.current.state).toEqual({phase: 'error', message: 'started via npx; run the npx command with the new version'});
    });

    it('times out when the new version never answers', async () => {
        applyUpdate.mockResolvedValue({success: true, data: {version: '0.261001.1', restarting: true, restart_required: false}});
        getVersion.mockResolvedValue('0.260924.1');
        const reload = vi.fn();

        const {result} = renderHook(() => useSelfUpdate({pollIntervalMs: 1, timeoutMs: 20, reload}));
        await act(() => result.current.start());

        expect(result.current.state).toEqual({phase: 'timedOut', version: '0.261001.1'});
        expect(reload).not.toHaveBeenCalled();
    });
});
