import {useState} from 'react';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, beforeAll, describe, expect, it, vi} from 'vitest';
import i18n from '@/i18n';
import ProviderFormDialog, {type EnhancedProviderFormData} from './ProviderFormDialog';
import {api} from '../services/api';
import {PROBE_TIMEOUT_MS} from './provider-form-dialog/probe';
vi.mock('@/services/serviceProviders', () => ({useProviderCatalogs: () => []}));
vi.mock('@/services/api', () => ({api: {getConfig: vi.fn().mockResolvedValue({data: {}}), probeProviderLightweight: vi.fn()}}));
vi.mock('./ProviderExportButton', () => ({default: () => null}));
beforeAll(async () => {await i18n.changeLanguage('en');});
afterEach(() => {
    cleanup();
    vi.useRealTimers();
});
function renderLocal(optionalEditableToken = false, noKeyRequired = false, selectedProviderId?: string) {
    const onSubmit = vi.fn();
    function Harness() {
        const [data, setData] = useState<EnhancedProviderFormData>({name: 'Local', apiStyle: 'openai', apiBase: 'http://127.0.0.1:11434/v1', token: '', noKeyRequired, selectedProviderId});
        return <ProviderFormDialog open onClose={() => {}} mode="add" data={data} onSubmit={onSubmit}
            optionalEditableToken={optionalEditableToken} onChange={(field, value) => setData(previous => ({...previous, [field]: value}))}/>;
    }
    render(<Harness/>);
    return onSubmit;
}
// Harness with a token filled in so the connection test can actually run.
function renderProbe() {
    function Harness() {
        const [data, setData] = useState<EnhancedProviderFormData>({name: 'Local', apiStyle: 'openai', apiBase: 'http://127.0.0.1:11434/v1', token: 'local-secret'});
        return <ProviderFormDialog open onClose={() => {}} mode="add" data={data} onSubmit={vi.fn()}
            onChange={(field, value) => setData(previous => ({...previous, [field]: value}))}/>;
    }
    render(<Harness/>);
}
// A probe mock that mirrors fetch's abort semantics: the promise rejects as
// soon as the dialog aborts the signal (hangs forever otherwise).
const mockHangingProbe = () => vi.mocked(api.probeProviderLightweight).mockImplementationOnce(
    (_name, _style, _base, _token, _auth, signal?: AbortSignal) =>
        new Promise((_resolve, reject) => {
            signal?.addEventListener('abort', () => reject(new Error('The operation was aborted')));
        }),
);
describe('self-hosted API keys', () => {
    it.each(['', '   '])('accepts an empty or whitespace key as no authentication: %j', async token => {
        const submit = renderLocal();
        expect(screen.getByLabelText(/API Key/)).not.toBeRequired();
        expect(screen.getByLabelText('HTTP/SOCKS Proxy URL (Optional)')).toBeVisible();
        fireEvent.change(screen.getByLabelText(/API Key/), {target: {value: token}});
        fireEvent.click(screen.getByRole('button', {name: 'Connect'}));
        await waitFor(() => expect(submit).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({token: '', noKeyRequired: true})));
    });
    it('still requires a key for an explicitly selected hosted preset unless no authentication is selected', () => {
        const submit = renderLocal(false, false, 'openai');
        expect(screen.getByLabelText(/API Key/)).toBeRequired();
        fireEvent.click(screen.getByRole('button', {name: 'Connect'}));
        expect(submit).not.toHaveBeenCalled();
        expect(screen.getByText('Enter an API key or select “No API key required”.')).toBeInTheDocument();
    });
    it('enables authentication when a key is entered for an optional-key local preset', async () => {
        const submit = renderLocal(true, true);
        fireEvent.change(screen.getByLabelText(/API Key/), {target: {value: ' local-secret '}});
        fireEvent.click(screen.getByRole('button', {name: 'Connect'}));
        await waitFor(() => expect(submit).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({token: 'local-secret', noKeyRequired: false})));
    });
});
describe('connection test', () => {
    it('reports a successful test', async () => {
        vi.mocked(api.probeProviderLightweight).mockResolvedValueOnce({success: true, data: {
            valid: true, message: 'Connection test completed - 2/2 endpoints accessible',
            options_success: true, options_response_time: 5, models_success: true, models_response_time: 9, models_count: 7,
        }});
        renderProbe();
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(await screen.findByText(/2\/2 endpoints accessible/)).toBeInTheDocument();
        expect(screen.getByText('OPTIONS (5ms)')).toBeInTheDocument();
        expect(screen.getByText('Models (9ms, 7 models)')).toBeInTheDocument();
        expect(api.probeProviderLightweight).toHaveBeenCalledWith('Local', 'openai', 'http://127.0.0.1:11434/v1', 'local-secret', undefined, expect.any(AbortSignal));
    });
    it('reports a failed test with the backend error message', async () => {
        vi.mocked(api.probeProviderLightweight).mockResolvedValueOnce({success: false, error: {message: 'Authentication failed'}});
        renderProbe();
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(await screen.findByText('Authentication failed')).toBeInTheDocument();
    });
    it('stops an in-flight test, returns to idle, and can run a new test successfully', async () => {
        mockHangingProbe();
        renderProbe();
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        // While the test runs, the slot turns into an actionable Stop button.
        const stop = await screen.findByRole('button', {name: 'Stop Test'});
        expect(screen.queryByRole('button', {name: 'Test Connection'})).not.toBeInTheDocument();
        fireEvent.click(stop);
        await waitFor(() => expect(screen.getByRole('button', {name: 'Test Connection'})).toBeInTheDocument());
        expect(screen.getByText('Connection test cancelled')).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: 'Stop Test'})).not.toBeInTheDocument();
        // No loading residue: the dialog can immediately issue a new test that succeeds.
        vi.mocked(api.probeProviderLightweight).mockResolvedValueOnce({success: true, data: {
            valid: true, message: 'Connection test completed - 1/2 endpoints accessible',
            options_success: true, options_response_time: 3,
        }});
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(await screen.findByText(/1\/2 endpoints accessible/)).toBeInTheDocument();
    });
    it('auto-stops with a timeout message when the upstream hangs past the watchdog', async () => {
        vi.useFakeTimers({shouldAdvanceTime: true});
        mockHangingProbe();
        renderProbe();
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(await screen.findByRole('button', {name: 'Stop Test'})).toBeInTheDocument();
        await vi.advanceTimersByTimeAsync(PROBE_TIMEOUT_MS);
        expect(await screen.findByText(/Connection test timed out \(30s\)/)).toBeInTheDocument();
        expect(await screen.findByRole('button', {name: 'Test Connection'})).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: 'Stop Test'})).not.toBeInTheDocument();
    });
});
