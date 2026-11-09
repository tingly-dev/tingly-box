import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, beforeAll, beforeEach, describe, expect, it, vi} from 'vitest';
import i18n from '@/i18n';
import {api} from '@/services/api';
import {useProviderEditDialog} from './useProviderEditDialog';
vi.mock('@/services/serviceProviders', () => ({useProviderCatalogs: () => []}));
vi.mock('@/components/ProviderExportButton', () => ({default: () => null}));
vi.mock('@/services/api', () => ({api: {
    getConfig: vi.fn().mockResolvedValue({data: {}}), getProvider: vi.fn(), updateProvider: vi.fn(), probeProviderLightweight: vi.fn(),
}}));
const provider = {uuid: 'p1', name: 'My provider', api_style: 'openai', api_base: 'https://example.com/v1',
    api_base_openai: 'https://example.com/v1', api_base_anthropic: 'https://example.com/anthropic',
    token: 'stored-secret', enabled: true, auth_type: 'api_key', no_key_required: false};
function Harness() {
    const {editProvider, providerEditDialogs} = useProviderEditDialog();
    return <><button onClick={() => void editProvider('p1')}>Edit</button>{providerEditDialogs}</>;
}
async function open() {render(<Harness/>); fireEvent.click(screen.getByRole('button', {name: 'Edit'})); await screen.findByRole('dialog');}
beforeAll(async () => {await i18n.changeLanguage('en');});
beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getProvider).mockResolvedValue({success: true, data: provider});
    vi.mocked(api.updateProvider).mockResolvedValue({success: true});
});
afterEach(() => {cleanup(); vi.unstubAllGlobals();});
describe('provider edit', () => {
    it('pre-fills the saved key, reveals it via the eye toggle and no-ops on unchanged save', async () => {
        await open();
        expect(screen.getByRole('dialog', {name: 'Edit AI Config'})).toBeInTheDocument();
        expect(screen.getByText('Authentication')).toBeInTheDocument();
        expect(screen.getByLabelText('HTTP/SOCKS Proxy URL (Optional)')).toBeVisible();
        expect(screen.getByRole('switch', {name: 'Enabled'})).toBeChecked();
        expect(screen.getByLabelText('Name')).toHaveValue('My provider');
        const keyField = screen.getByLabelText('API Key');
        expect(keyField).toHaveValue('stored-secret');
        expect(keyField).toHaveAttribute('type', 'password');
        fireEvent.click(screen.getByRole('button', {name: 'Show key'}));
        expect(keyField).toHaveAttribute('type', 'text');
        fireEvent.click(screen.getByRole('button', {name: 'Hide key'}));
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({token: 'stored-secret'})));
    });
    it('validates every enabled URL and prevents protocol toggles from URL clicks', async () => {
        await open();
        fireEvent.click(screen.getByLabelText(/OpenAI Base URL/));
        expect(screen.getByRole('checkbox', {name: /OpenAI Compatible/})).toBeChecked();
        fireEvent.change(screen.getByLabelText(/Anthropic Base URL/), {target: {value: 'ftp://invalid.example'}});
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        expect(api.updateProvider).not.toHaveBeenCalled();
        expect(screen.getAllByText(/Enable at least one protocol/).length).toBeGreaterThan(0);
    });
    it('uses the enabled endpoint rather than an inactive draft URL', async () => {
        await open();
        fireEvent.click(screen.getByRole('checkbox', {name: /OpenAI Compatible/}));
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({api_base: provider.api_base_anthropic, api_base_openai: '', api_style: 'anthropic'})));
    });
    it('retains replacement credentials and shows an update error inline', async () => {
        vi.mocked(api.updateProvider).mockResolvedValue({success: false, error: 'Offline'});
        await open();
        fireEvent.change(screen.getByLabelText('API Key'), {target: {value: 'new-secret'}});
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        expect(await screen.findByText('Failed to update provider: Offline')).toBeInTheDocument();
        expect(screen.getByLabelText('API Key')).toHaveValue('new-secret');
    });
    it('tests with the stored key while keeping it out of the replacement input', async () => {
        vi.mocked(api.probeProviderLightweight).mockResolvedValue({success: false, error: {message: 'Authentication failed'}});
        await open();
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(await screen.findAllByText(/Authentication failed/)).not.toHaveLength(0);
        expect(api.probeProviderLightweight).toHaveBeenCalledWith('My provider', 'openai', provider.api_base, 'stored-secret', 'api_key');
    });
    it('explicitly selects no authentication without resending the stored key', async () => {
        await open();
        expect(screen.getByLabelText('HTTP/SOCKS Proxy URL (Optional)')).toBeVisible();
        fireEvent.click(screen.getByRole('checkbox', {name: 'No API key required'}));
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({no_key_required: true, token: undefined})));
    });
    it('initialises keyless providers correctly', async () => {
        vi.mocked(api.getProvider).mockResolvedValue({success: true, data: {...provider, no_key_required: true, token: ''}});
        await open();
        expect(screen.getByRole('checkbox', {name: 'No API key required'})).toBeChecked();
        expect(screen.getByLabelText('API Key')).not.toHaveAttribute('placeholder', '****');
        fireEvent.click(screen.getByRole('button', {name: 'Test Connection'}));
        expect(screen.getByText('An API key is needed for this test. You can save without testing.')).toBeInTheDocument();
    });
    it('exposes OAuth status while collapsing managed credentials', async () => {
        vi.mocked(api.getProvider).mockResolvedValue({success: true, data: {...provider, auth_type: 'oauth', oauth_detail: {issuer: 'codex', access_token: 'short'}}});
        await open();
        expect(screen.getByRole('dialog', {name: 'Edit AI Config'})).toBeInTheDocument();
        expect(screen.getByText('Authentication')).toBeInTheDocument();
        expect(screen.getByLabelText('HTTP/SOCKS Proxy URL (Optional)')).toBeVisible();
        expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument();
        expect(screen.getByRole('switch', {name: 'Enabled'})).toBeChecked();
        expect(screen.getByRole('button', {name: /OAuth · codex/})).toHaveAttribute('aria-expanded', 'false');
        fireEvent.change(screen.getByLabelText(/OpenAI Base URL/), {target: {value: 'https://new.example/v1'}});
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({api_base: 'https://new.example/v1'})));
        const payload = vi.mocked(api.updateProvider).mock.calls[0][1];
        for (const field of ['api_style', 'token', 'credential', 'oauth_detail']) expect(payload).not.toHaveProperty(field);
    });
    it('preserves an Anthropic primary style on a dual OAuth provider', async () => {
        vi.mocked(api.getProvider).mockResolvedValue({success: true, data: {...provider, auth_type: 'oauth', api_style: 'anthropic', api_base: provider.api_base_anthropic}});
        await open();
        expect(screen.queryByRole('radio')).not.toBeInTheDocument();
        fireEvent.change(screen.getByLabelText(/Anthropic Base URL/), {target: {value: 'https://new.example/anthropic'}});
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({
            api_base: 'https://new.example/anthropic', api_base_openai: provider.api_base_openai, api_base_anthropic: 'https://new.example/anthropic',
        })));
        expect(vi.mocked(api.updateProvider).mock.calls[0][1]).not.toHaveProperty('api_style');
    });
    it.each(['openai', 'anthropic', 'google'])('locks %s OAuth style while allowing name, status and proxy edits', async apiStyle => {
        vi.mocked(api.getProvider).mockResolvedValue({success: true, data: {...provider, auth_type: 'oauth', api_style: apiStyle, api_base_openai: '', api_base_anthropic: ''}});
        await open();
        expect(screen.queryByRole('radio')).not.toBeInTheDocument();
        expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
        expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
        expect(screen.getByText('API style is managed by OAuth and cannot be changed here.')).toBeInTheDocument();
        expect(screen.getByText(apiStyle === 'google' ? 'Google Compatible' : apiStyle === 'anthropic' ? 'Anthropic Compatible' : 'OpenAI Compatible')).toBeInTheDocument();
        fireEvent.change(screen.getByLabelText('Name'), {target: {value: 'Renamed OAuth'}});
        fireEvent.click(screen.getByRole('switch', {name: 'Enabled'}));
        fireEvent.change(screen.getByLabelText('HTTP/SOCKS Proxy URL (Optional)'), {target: {value: 'http://localhost:7890'}});
        fireEvent.click(screen.getByRole('button', {name: 'Save Changes'}));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p1', expect.objectContaining({name: 'Renamed OAuth', enabled: false, proxy_url: 'http://localhost:7890'})));
        const payload = vi.mocked(api.updateProvider).mock.calls[0][1];
        for (const field of ['api_style', 'token', 'credential', 'oauth_detail', 'api_base_openai', 'api_base_anthropic']) {
            expect(payload).not.toHaveProperty(field);
        }
    });
    it('keeps OAuth credentials read-only and masks them again when reopening', async () => {
        vi.mocked(api.getProvider).mockResolvedValue({success: true, data: {...provider, auth_type: 'oauth', oauth_detail: {issuer: 'codex', access_token: 'secret-token'}}});
        await open();
        fireEvent.click(screen.getByRole('button', {name: /OAuth · codex/}));
        vi.stubGlobal('isSecureContext', true);
        const clipboardWrite = vi.fn().mockResolvedValue(undefined);
        Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: clipboardWrite}});
        const token = screen.getByLabelText('Access token');
        expect(token).toHaveAttribute('readonly');
        expect(token).toHaveAttribute('type', 'password');
        fireEvent.click(screen.getByRole('button', {name: 'Show key'}));
        expect(token).toHaveAttribute('type', 'text');
        const copy = screen.getByRole('button', {name: 'Copy'});
        expect(copy).toBeEnabled();
        fireEvent.click(copy);
        await waitFor(() => expect(clipboardWrite).toHaveBeenCalledWith('secret-token'));
        fireEvent.click(screen.getByRole('button', {name: 'Cancel'}));
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', {name: 'Edit'}));
        await screen.findByRole('dialog');
        fireEvent.click(screen.getByRole('button', {name: /OAuth · codex/}));
        expect(screen.getByLabelText('Access token')).toHaveAttribute('type', 'password');
    });

});
