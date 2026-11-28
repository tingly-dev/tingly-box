import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import type { MCPCatalogTool } from '../../features/mcp/types';
import WebtoolsSettings from './WebtoolsSettings';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, o: { defaultValue: string }) => o.defaultValue }),
}));
vi.mock('@/services/api', () => ({ api: { getProviders: vi.fn() } }));
const provider: Provider = { uuid: 'account-a', name: 'DS account A', enabled: true, api_base: 'https://api.deepseek.com/v1', api_style: 'openai', auth_type: 'api_key', token: 'masked-key', no_key_required: false };
const providerB = { ...provider, uuid: 'account-b', name: 'DS account B' };
const save = vi.fn();
const defaults = { WEB_SEARCH_BACKEND: 'serper', WEB_FETCH_BACKEND: 'jina' };
const choose = (label: string, option: string, region?: HTMLElement) => {
    fireEvent.mouseDown((region ? within(region) : screen).getByRole('combobox', { name: label }));
    fireEvent.click(screen.getByRole('option', { name: option }));
};
const submit = () => fireEvent.click(screen.getByRole('button', { name: 'Save web tool settings' }));
beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getProviders).mockResolvedValue({ success: true, data: [provider, providerB] });
    save.mockResolvedValue(undefined);
});

describe('Server web tool configuration', () => {
    it('saves explicit sources and credentials without enabling tools', async () => {
        render(<WebtoolsSettings source={{ id: 'server-webtools', enabled: false, env: { OTHER: 'kept' } }} onSave={save} />);
        fireEvent.change(screen.getByLabelText('Serper API key'), { target: { value: ' configured-key ' } });
        submit();
        await waitFor(() => expect(save).toHaveBeenCalledWith({
            id: 'server-webtools', env: { ...defaults, OTHER: 'kept', SERPER_API_KEY: 'configured-key' },
        }));
        expect(screen.queryByRole('switch')).toBeNull();
        expect(screen.getByText(/This source is off/)).toBeInTheDocument();
    });

    it('removes a saved Serper key without changing Fetch or unrelated environment', async () => {
        render(<WebtoolsSettings source={{ id: 'server-webtools', env: { SERPER_API_KEY: 'old', OTHER: 'kept', WEB_FETCH_BACKEND: 'deepseek', WEB_FETCH_PROVIDER_UUID: 'account-b' } }} onSave={save} />);
        fireEvent.change(screen.getByLabelText('Serper API key'), { target: { value: '' } });
        await waitFor(() => expect(screen.getByRole('button', { name: 'Save web tool settings' })).toBeEnabled());
        submit();
        await waitFor(() => expect(save).toHaveBeenCalledWith({ id: 'server-webtools', env: { ...defaults, OTHER: 'kept', WEB_FETCH_BACKEND: 'deepseek', WEB_FETCH_PROVIDER_UUID: 'account-b' } }));
    });

    it('offers only Search DeepSeek/Serper and Fetch DeepSeek/Jina, with no Auto', async () => {
        render(<WebtoolsSettings source={{ id: 'server-webtools' }} onSave={save} />);
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Search source' }));
        expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['DeepSeek', 'Serper']);
        fireEvent.click(screen.getByRole('option', { name: 'Serper' }));
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Fetch source' }));
        expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['DeepSeek', 'Jina Reader']);
        fireEvent.click(screen.getByRole('option', { name: 'Jina Reader' }));
        expect(screen.getAllByText(/No configuration required/).length).toBeGreaterThan(0);
        submit();
        await waitFor(() => expect(save).toHaveBeenCalled());
    });

    it('saves DeepSeek Search + Jina Fetch, preserving the hidden Serper key', async () => {
        render(<WebtoolsSettings source={{ id: 'server-webtools', env: { SERPER_API_KEY: 'keep', OTHER: 'value' } }} onSave={save} />);
        choose('Search source', 'DeepSeek');
        expect(screen.queryByLabelText('Serper API key')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Save web tool settings' })).toBeDisabled();
        await waitFor(() => expect(screen.getByRole('combobox', { name: 'DeepSeek credential' })).not.toHaveAttribute('aria-disabled', 'true'));
        choose('DeepSeek credential', 'DS account A · account-');
        submit();
        await waitFor(() => expect(save).toHaveBeenCalledWith({ id: 'server-webtools', env: {
            OTHER: 'value', SERPER_API_KEY: 'keep', WEB_SEARCH_BACKEND: 'deepseek', WEB_FETCH_BACKEND: 'jina', WEB_SEARCH_PROVIDER_UUID: 'account-a',
        } }));
        expect(JSON.stringify(save.mock.calls)).not.toContain('masked-key');
    });

    it('selects different credentials for Search and Fetch and reloads saved selections', async () => {
        const env = { WEB_SEARCH_BACKEND: 'deepseek', WEB_FETCH_BACKEND: 'deepseek', WEB_SEARCH_PROVIDER_UUID: 'account-a', WEB_FETCH_PROVIDER_UUID: 'account-b' };
        render(<WebtoolsSettings source={{ id: 'server-webtools', env }} onSave={save} />);
        await waitFor(() => expect(screen.getByRole('button', { name: 'Save web tool settings' })).toBeEnabled());
        expect(within(screen.getByRole('region', { name: 'Search' })).getByRole('combobox', { name: 'DeepSeek credential' })).toHaveTextContent('DS account A');
        expect(within(screen.getByRole('region', { name: 'Fetch' })).getByRole('combobox', { name: 'DeepSeek credential' })).toHaveTextContent('DS account B');
        choose('DeepSeek credential', 'DS account B · account-', screen.getByRole('region', { name: 'Search' }));
        submit();
        await waitFor(() => expect(save).toHaveBeenCalledWith({ id: 'server-webtools', env: { ...env, WEB_SEARCH_PROVIDER_UUID: 'account-b' } }));
    });

    it('excludes disabled, third-party, OAuth and keyless credentials, retaining invalid saved references', async () => {
        vi.mocked(api.getProviders).mockResolvedValue({ success: true, data: [provider,
            { ...provider, uuid: 'disabled', enabled: false },
            { ...provider, uuid: 'third-party', api_base: 'https://api.deepseek.com.evil.example' },
            { ...provider, uuid: 'oauth', auth_type: 'oauth' },
            { ...provider, uuid: 'empty-key', token: '' },
        ] });
        render(<WebtoolsSettings source={{ id: 'server-webtools', env: { WEB_SEARCH_BACKEND: 'deepseek', WEB_SEARCH_PROVIDER_UUID: 'disabled' } }} onSave={save} />);
        await waitFor(() => expect(screen.getByRole('combobox', { name: 'DeepSeek credential' })).not.toHaveAttribute('aria-disabled', 'true'));
        expect(screen.getByRole('button', { name: 'Save web tool settings' })).toBeDisabled();
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'DeepSeek credential' }));
        expect(screen.getAllByRole('option').filter((option) => option.getAttribute('aria-disabled') !== 'true')).toHaveLength(1);
        expect(screen.getByRole('option', { name: 'disabled · Unavailable' })).toHaveAttribute('aria-disabled', 'true');
        fireEvent.click(screen.getByRole('option', { name: 'DS account A · account-' }));
    });

    it('shows saved readiness separately from unsaved source choices', async () => {
        const tools = [{ name: 'mcp_web_search', backend: 'serper', ready: false }, { name: 'mcp_web_fetch', backend: 'jina', ready: true }] as MCPCatalogTool[];
        render(<WebtoolsSettings source={{ id: 'server-webtools' }} tools={tools} onSave={save} />);
        choose('Search source', 'DeepSeek');
        expect(screen.getByText(/Unsaved changes/)).toBeInTheDocument();
        expect(screen.getByText('Actual source (saved): Serper')).toBeInTheDocument();
        expect(screen.getByText('Not ready')).toBeInTheDocument();
        expect(screen.getByText('Configuration ready')).toBeInTheDocument();
        await waitFor(() => expect(screen.getByRole('combobox', { name: 'DeepSeek credential' })).not.toHaveAttribute('aria-disabled', 'true'));
    });

    it('shows unknown status instead of inventing readiness; preserves draft on save failure', async () => {
        save.mockRejectedValue(new Error('Save failed'));
        render(<WebtoolsSettings source={{ id: 'server-webtools' }} onSave={save} />);
        expect(screen.getAllByText('Actual source (saved): —')).toHaveLength(2);
        fireEvent.change(screen.getByLabelText('Serper API key'), { target: { value: 'draft' } });
        submit();
        expect(await screen.findByText('Save failed')).toBeInTheDocument();
        expect(screen.getByLabelText('Serper API key')).toHaveValue('draft');
    });

    it('can retry credential loading without changing the selected sources', async () => {
        vi.mocked(api.getProviders).mockRejectedValueOnce(new Error('Offline'));
        const log = vi.spyOn(console, 'error').mockImplementation(() => {});
        render(<WebtoolsSettings source={{ id: 'server-webtools', env: { WEB_SEARCH_BACKEND: 'deepseek' } }} onSave={save} />);
        expect(await screen.findByText('Could not load credentials. Retry below.')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Refresh credentials' }));
        await waitFor(() => expect(screen.queryByText('Could not load credentials. Retry below.')).not.toBeInTheDocument());
        expect(screen.getByRole('combobox', { name: 'Search source' })).toHaveTextContent('DeepSeek');
        log.mockRestore();
    });
});
