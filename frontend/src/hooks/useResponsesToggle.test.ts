import {act, renderHook, waitFor} from '@testing-library/react';
import {vi} from 'vitest';
import type {Mock} from 'vitest';
import {useResponsesToggle} from './useResponsesToggle';
import {runProbe} from '@/components/probe/runProbe';
import {notify} from '@/utils/notify';
import type {ConfigProvider, ConfigRecord} from '@/components/RoutingGraphTypes';

vi.mock('@/components/probe/runProbe');
vi.mock('@/utils/notify', () => ({
    notify: {success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn()},
}));

const mockRunProbe = runProbe as Mock;

function makeRecord(flags: ConfigRecord['flags'] = {}): ConfigRecord {
    return {
        uuid: 'rule-1', scenario: 'codex', requestModel: 'gpt-5', responseModel: 'gpt-5',
        active: true, providers: [], flags,
    };
}

function makeService(overrides: Partial<ConfigProvider> = {}): ConfigProvider {
    return {uuid: 'svc-1', provider: 'prov-1', model: 'gpt-5', active: true, ...overrides};
}

beforeEach(() => {
    mockRunProbe.mockReset();
    (notify.success as Mock).mockReset();
    (notify.error as Mock).mockReset();
});

describe('useResponsesToggle', () => {
    it('enables the flag after a successful probe', async () => {
        mockRunProbe.mockResolvedValue({success: true});
        const onUpdateRecord = vi.fn();
        const record = makeRecord();
        const service = makeService();

        const {result} = renderHook(() => useResponsesToggle({record, primaryService: service, onUpdateRecord}));
        expect(result.current.enabled).toBe(false);

        await act(async () => {
            await result.current.onToggle();
        });

        expect(mockRunProbe).toHaveBeenCalledWith(expect.objectContaining({
            target_type: 'provider',
            provider_uuid: 'prov-1',
            model: 'gpt-5',
            direct: true,
            endpoint: 'responses',
        }));
        expect(onUpdateRecord).toHaveBeenCalledWith('flags', expect.objectContaining({openaiEndpointOverride: 'responses'}));
        expect(notify.success).toHaveBeenCalled();
    });

    it('does not set the flag when the probe fails', async () => {
        mockRunProbe.mockResolvedValue({success: false, error: {message: 'not supported'}});
        const onUpdateRecord = vi.fn();
        const record = makeRecord();

        const {result} = renderHook(() => useResponsesToggle({record, primaryService: makeService(), onUpdateRecord}));

        await act(async () => {
            await result.current.onToggle();
        });

        expect(onUpdateRecord).not.toHaveBeenCalled();
        expect(notify.error).toHaveBeenCalled();
    });

    it('disabling never probes and reverts to auto immediately', async () => {
        const onUpdateRecord = vi.fn();
        const record = makeRecord({openaiEndpointOverride: 'responses'});

        const {result} = renderHook(() => useResponsesToggle({record, primaryService: makeService(), onUpdateRecord}));
        expect(result.current.enabled).toBe(true);

        await act(async () => {
            await result.current.onToggle();
        });

        expect(mockRunProbe).not.toHaveBeenCalled();
        expect(onUpdateRecord).toHaveBeenCalledWith('flags', expect.objectContaining({openaiEndpointOverride: 'auto'}));
    });

    it('re-validates and reverts when the bound provider/model changes while enabled', async () => {
        mockRunProbe.mockResolvedValue({success: false, error: {message: 'no longer supported'}});
        const onUpdateRecord = vi.fn();
        const record = makeRecord({openaiEndpointOverride: 'responses'});

        const {result, rerender} = renderHook(
            ({service}) => useResponsesToggle({record, primaryService: service, onUpdateRecord}),
            {initialProps: {service: makeService({model: 'gpt-5'})}},
        );

        // No probe on initial mount, even though the flag is already 'responses'.
        expect(mockRunProbe).not.toHaveBeenCalled();

        rerender({service: makeService({model: 'gpt-5-mini'})});

        await waitFor(() => expect(onUpdateRecord).toHaveBeenCalledWith(
            'flags',
            expect.objectContaining({openaiEndpointOverride: 'auto'}),
        ));
        expect(mockRunProbe).toHaveBeenCalledWith(expect.objectContaining({model: 'gpt-5-mini'}));
    });

    it('does not re-validate on model change when the flag is not enabled', async () => {
        const onUpdateRecord = vi.fn();
        const record = makeRecord(); // openaiEndpointOverride unset

        const {rerender} = renderHook(
            ({service}) => useResponsesToggle({record, primaryService: service, onUpdateRecord}),
            {initialProps: {service: makeService({model: 'gpt-5'})}},
        );

        rerender({service: makeService({model: 'gpt-5-mini'})});

        // Give any accidental async work a tick to run, then assert nothing fired.
        await act(async () => {
            await Promise.resolve();
        });
        expect(mockRunProbe).not.toHaveBeenCalled();
        expect(onUpdateRecord).not.toHaveBeenCalled();
    });
});
