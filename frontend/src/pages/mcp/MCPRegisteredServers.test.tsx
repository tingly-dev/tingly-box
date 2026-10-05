import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPRegisteredServers from './MCPRegisteredServers';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    routing: vi.fn(),
    patch: vi.fn(),
    create: vi.fn(),
    saveClient: vi.fn(),
    notify: vi.fn(),
}));
vi.mock('@/services/api', () => ({
    api: {
        getMCPConfig: mocks.get,
        getMCPRouting: mocks.routing,
        patchMCPSource: mocks.patch,
        createMCPSource: mocks.create,
        saveMCPClientProfile: mocks.saveClient,
    },
}));
vi.mock('@/hooks/useNotify', () => ({ useNotify: () => ({ error: mocks.notify }) }));
vi.mock('@/contexts/FeatureFlagsContext', () => ({ useFeatureFlags: () => ({ refresh: vi.fn() }) }));
vi.mock('@/components/PageLayout', () => ({ PageLayout: ({ children }: { children: React.ReactNode }) => children }));
vi.mock('./AdvisorSettings', () => ({ default: () => null }));
vi.mock('./AgentInstallCard', () => ({
    default: ({ clientId }: { clientId: string }) => <div>Setup command for {clientId}</div>,
}));
vi.mock('./MCPRoutingPanel', () => ({
    default: ({ focusSource }: { focusSource?: string }) => <div>Routing panel {focusSource}</div>,
}));
vi.mock('./MCPToolsPanel', () => ({
    default: ({
        mode,
        routes,
        onConfigureSource,
    }: {
        mode: string;
        routes: { id: string; state: string; tools: unknown[] }[];
        onConfigureSource: (id: string) => void;
    }) => (
        <div>
            Capabilities panel: {mode === 'asset' ? 'assets' : mode}
            <div data-testid="catalog-snapshot">{routes.map((r) => `${r.state}:${r.tools.length}`).join(',')}</div>
            {mode === 'asset' && routes.some((r) => r.id === 'remote') && (
                <button onClick={() => onConfigureSource('remote')}>Configure tools</button>
            )}
        </div>
    ),
}));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
const source = {
    id: 'remote',
    name: 'Remote docs',
    transport: 'http',
    endpoint: 'https://example.test/mcp',
    headers: { Authorization: '${TOKEN}' },
    env: { TOKEN: 'saved-token' },
    usage: { client: true, gateway: true },
    tool_policies: { echo: { enabled: true } },
};
const tool = {
    source_id: 'remote',
    name: 'echo',
    normalized_name: 'tingly_box_mcp__remote__echo',
    enabled: true,
    usage: { client: true, gateway: true },
    input_schema: { type: 'object' },
    output_schema: { type: 'object' },
    implementation: 'mcp',
};
const routeSource = {
    id: 'remote',
    name: 'Remote docs',
    origin: 'external',
    transport: 'http',
    address: 'https://example.test/mcp',
    state: 'connected',
    processing: 'standard',
    tools: [tool],
};
const config = {
    sources: [source],
    client_profiles_configured: true,
    client_profiles: [{ id: 'reader', name: 'Codex work', enabled: true, sources: ['remote'], tools: ['*'] }],
};
beforeEach(() => {
    HTMLElement.prototype.scrollIntoView = vi.fn();
    vi.clearAllMocks();
    mocks.get.mockResolvedValue({ success: true, enabled: true, config });
    mocks.routing.mockResolvedValue({
        success: true,
        enabled: true,
        routing: {
            sources: [routeSource],
            clients: [
                {
                    id: 'reader',
                    name: 'Codex work',
                    endpoint: '/api/v1/mcp/reader',
                    enabled: true,
                    legacy: false,
                    sources: [routeSource],
                },
            ],
            server_tools: [routeSource],
        },
    });
});
const open = (url = '/mcp') =>
    render(
        <MemoryRouter initialEntries={[url]}>
            <MCPRegisteredServers />
        </MemoryRouter>
    );
describe('MCP secondary layouts', () => {
    it('keeps client access and publication in MCP, with the relationship graph expandable', async () => {
        open();
        expect(await screen.findByText('Codex work')).toBeInTheDocument();
        expect(screen.getByText('Capabilities panel: client')).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'Connected tools' })).toBeNull();
        expect(screen.queryByRole('tab')).toBeNull();
        expect(screen.queryByText(/Routing panel/)).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: /Usage relationships/ }));
        expect(await screen.findByText(/Routing panel/)).toBeInTheDocument();
    });
    it('reopens a focused usage relationship bookmark without opening the source editor', async () => {
        open('/mcp?relationship=remote');
        expect(await screen.findByText('Routing panel remote')).toBeInTheDocument();
        expect(screen.queryByRole('dialog')).toBeNull();
    });
    it('moves shared connections and all tool assets into Tool and leaves client access in MCP', async () => {
        open();
        await screen.findByText('Codex work');
        fireEvent.click(screen.getByRole('button', { name: 'Manage tool connections' }));
        expect(await screen.findByRole('heading', { name: 'Tool', level: 1 })).toBeInTheDocument();
        expect(screen.getByText('Capabilities panel: assets')).toBeInTheDocument();
        expect(screen.getByRole('region', { name: 'Connected tools' })).toBeInTheDocument();
        expect(screen.getAllByRole('button', { name: 'Connect tools' })).toHaveLength(1);
        expect(screen.queryByRole('button', { name: 'Access & setup' })).toBeNull();
        expect(screen.queryByText('Codex work')).toBeNull();
        expect(screen.queryByRole('button', { name: /Usage relationships/ })).toBeNull();
    });
    it('routes new connection onboarding from Server Tool to Tool', async () => {
        open('/mcp/server-tools');
        await screen.findByText('Capabilities panel: gateway');
        expect(screen.queryByRole('button', { name: 'Connect tools' })).toBeNull();
        expect(screen.queryByRole('button', { name: 'Access & setup' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Manage tool connections' }));
        expect(await screen.findByRole('heading', { name: 'Tool', level: 1 })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Connect tools' })).toBeInTheDocument();
    });
    it('keeps failed connection edits open and preserves credentials without restoring stale policies', async () => {
        mocks.patch.mockRejectedValue(new Error('Save failed'));
        open('/mcp/tools');
        fireEvent.click(await screen.findByRole('button', { name: 'Configure tools' }));
        fireEvent.click(screen.getByRole('button', { name: 'Connection settings' }));
        const endpoint = screen.getByLabelText('Endpoint URL');
        fireEvent.change(endpoint, { target: { value: 'https://changed.test/mcp' } });
        fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));
        expect(await screen.findByText('Save failed')).toBeInTheDocument();
        expect(endpoint).toHaveValue('https://changed.test/mcp');
        expect(screen.getByLabelText('Server ID')).toBeDisabled();
        expect(mocks.patch.mock.calls[0][1]).toMatchObject({
            id: 'remote',
            endpoint: 'https://changed.test/mcp',
            headers: { Authorization: '${TOKEN}' },
            env: { TOKEN: 'saved-token' },
        });
        expect(mocks.patch.mock.calls[0][1]).not.toHaveProperty('tool_policies');
        expect(mocks.patch.mock.calls[0][1]).not.toHaveProperty('usage');
    });
    it('connects from a name and URL, generates a stable ID, and opens an unpublished tool asset', async () => {
        mocks.create.mockImplementation(async (s) => ({
            success: true,
            enabled: false,
            config: { ...config, sources: [...config.sources, s] },
        }));
        open('/mcp/tools');
        fireEvent.click(await screen.findByRole('button', { name: 'Connect tools' }));
        const dialog = screen.getByRole('dialog');
        expect(within(dialog).queryByRole('checkbox', { name: 'MCP clients' })).toBeNull();
        fireEvent.change(within(dialog).getByLabelText(/Connection name/), { target: { value: 'Team docs' } });
        fireEvent.change(within(dialog).getByLabelText('Endpoint URL'), { target: { value: 'https://team.test/mcp' } });
        fireEvent.click(within(dialog).getByRole('button', { name: 'Connect and inspect tools' }));
        await waitFor(() =>
            expect(mocks.create).toHaveBeenCalledWith(
                expect.objectContaining({
                    id: 'team-docs',
                    name: 'Team docs',
                    endpoint: 'https://team.test/mcp',
                    usage: { client: false, gateway: false },
                })
            )
        );
        expect(await screen.findByRole('heading', { name: 'Team docs' })).toBeInTheDocument();
        expect(within(screen.getByRole('dialog')).getByText('Tool catalog')).toBeInTheDocument();
        expect(screen.queryByRole('checkbox', { name: 'Expose through MCP' })).toBeNull();
        expect(screen.queryByRole('checkbox', { name: 'Server Tools' })).toBeNull();
    });
    it('keeps client grants and setup together and saves an explicit empty grant without exposing other tools', async () => {
        mocks.saveClient.mockImplementation(async (profile) => ({
            success: true,
            enabled: false,
            config: { ...config, client_profiles: [profile] },
        }));
        open('/mcp');
        await screen.findByText('Codex work');
        fireEvent.click(await screen.findByRole('button', { name: 'Access & setup' }));
        expect(await screen.findByText('Setup command for reader')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /Which tools can this client use/ }));
        fireEvent.click(screen.getByRole('checkbox', { name: /Remote docs/ }));
        fireEvent.click(screen.getByRole('button', { name: 'Save access' }));
        await waitFor(() =>
            expect(mocks.saveClient).toHaveBeenCalledWith(expect.objectContaining({ id: 'reader', sources: [] }))
        );
    });
    it.each([
        ['/mcp/tools', 'assets'],
        ['/mcp/server-tools', 'gateway'],
    ])('renders the corresponding tool page without a modal for %s', async (url, usage) => {
        open(url);
        expect(await screen.findByText(`Capabilities panel: ${usage}`)).toBeInTheDocument();
        expect(screen.queryByRole('dialog')).toBeNull();
        expect(screen.queryByRole('tab')).toBeNull();
    });
    it('opens the client panel from an existing install bookmark', async () => {
        open('/mcp/clients?install=reader');
        expect(await screen.findByText('Setup command for reader')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close workspace panel' }));
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
        expect(screen.queryByRole('heading', { name: 'Choose client' })).toBeNull();
        expect(screen.getByRole('heading', { name: 'MCP', level: 1 })).toBeInTheDocument();
    });
    it.each(['/mcp/tools?install=reader', '/mcp/server-tools?profile=reader'])(
        'keeps client bookmarks in MCP: %s',
        async (url) => {
            open(url);
            expect(await screen.findByText('Setup command for reader')).toBeInTheDocument();
            fireEvent.click(screen.getByRole('button', { name: 'Close workspace panel' }));
            await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
            expect(screen.getByRole('heading', { name: 'MCP', level: 1 })).toBeInTheDocument();
        }
    );
    it.each(['/mcp?source=remote', '/mcp/sources?source=remote'])(
        'keeps connection bookmarks in Tool: %s',
        async (url) => {
            open(url);
            expect(await screen.findByRole('dialog')).toBeInTheDocument();
            expect(screen.getByRole('button', { name: 'Connection settings' })).toBeInTheDocument();
            fireEvent.click(screen.getByRole('button', { name: 'Close workspace panel' }));
            await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
            expect(screen.getByRole('heading', { name: 'Tool', level: 1 })).toBeInTheDocument();
        }
    );
    it('opens a publication bookmark without shared asset controls', async () => {
        open('/mcp?publish=remote');
        expect(await screen.findByRole('checkbox', { name: 'Expose through MCP' })).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Connection settings' })).toBeNull();
        expect(screen.queryByRole('button', { name: 'Remove connection' })).toBeNull();
        expect(screen.queryByRole('checkbox', { name: 'Server Tools' })).toBeNull();
    });
    it('reports zero available tools when MCP execution is off without discarding saved grants', async () => {
        const snapshot = await mocks.routing();
        mocks.routing.mockResolvedValue({ ...snapshot, enabled: false });
        open();
        expect(await screen.findByText('0 tools available through MCP · Remote docs')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Access & setup' })).toBeInTheDocument();
        expect(mocks.saveClient).not.toHaveBeenCalled();
    });
    it('ignores an old discovery response after a shared source change, keeping catalog and graph on the newer snapshot', async () => {
        let completeOld!: (value: unknown) => void;
        const oldResponse = new Promise((resolve) => {
            completeOld = resolve;
        });
        mocks.routing
            .mockReturnValueOnce(oldResponse)
            .mockResolvedValueOnce({
                success: true,
                enabled: true,
                routing: { sources: [{ ...routeSource, state: 'disabled', tools: [] }], clients: [], server_tools: [] },
            });
        mocks.patch.mockResolvedValue({
            success: true,
            enabled: true,
            config: { ...config, sources: [{ ...source, enabled: false }] },
        });
        open('/mcp/tools?source=remote');
        fireEvent.click(
            await screen.findByRole('switch', { name: 'Enable shared connection (affects MCP and Server Tool)' })
        );
        await waitFor(() => expect(screen.getByTestId('catalog-snapshot')).toHaveTextContent('disabled:0'));
        await act(async () => {
            completeOld({
                success: true,
                enabled: true,
                routing: { sources: [routeSource], clients: [], server_tools: [routeSource] },
            });
            await oldResponse;
        });
        await waitFor(() => expect(screen.getByTestId('catalog-snapshot')).toHaveTextContent('disabled:0'));
        expect(mocks.patch).toHaveBeenCalledWith('remote', { id: 'remote', enabled: false });
    });
});
