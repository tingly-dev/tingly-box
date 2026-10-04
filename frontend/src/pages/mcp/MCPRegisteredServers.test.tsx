import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
vi.mock('./MCPRoutingPanel', () => ({ default: () => <div>Routing panel</div> }));
vi.mock('./MCPToolsPanel', () => ({
    default: ({ usage }: { usage: string }) => <div>Capabilities panel: {usage}</div>,
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
    vi.clearAllMocks();
    mocks.get.mockResolvedValue({ success: true, enabled: false, config });
    mocks.routing.mockResolvedValue({
        success: true,
        enabled: false,
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
    it('shows sources and actual destinations together, with advanced routing hidden until requested', async () => {
        open();
        expect(await screen.findByText('Codex work · Gateway model')).toBeInTheDocument();
        expect(screen.queryByRole('tab')).toBeNull();
        expect(screen.queryByText('Routing panel')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: /View call relationships/ }));
        expect(await screen.findByText('Routing panel')).toBeInTheDocument();
    });
    it('uses the overview for shared connections and navigates to separate Tool and Server Tool pages', async () => {
        open();
        await screen.findByText('Codex work · Gateway model');
        expect(screen.queryByRole('button', { name: 'Access & setup' })).toBeNull();
        expect(screen.queryByText(/Capabilities panel/)).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Open Tool' }));
        expect(await screen.findByRole('heading', { name: 'Tool', level: 1 })).toBeInTheDocument();
        expect(screen.getByText('Capabilities panel: client')).toBeInTheDocument();
        expect(screen.queryByText('Capabilities panel: gateway')).toBeNull();
        expect(screen.getByRole('button', { name: 'Access & setup' })).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'Connected tools' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Manage connections' }));
        fireEvent.click(await screen.findByRole('button', { name: 'Open Server Tool' }));
        expect(await screen.findByRole('heading', { name: 'Server Tool', level: 1 })).toBeInTheDocument();
        expect(screen.getByText('Capabilities panel: gateway')).toBeInTheDocument();
        expect(screen.queryByText('Capabilities panel: client')).toBeNull();
        expect(screen.queryByRole('button', { name: 'Access & setup' })).toBeNull();
        expect(screen.queryByRole('region', { name: 'Connected tools' })).toBeNull();
    });
    it('connects directly from Server Tool without granting ordinary tool usage', async () => {
        mocks.create.mockImplementation(async (s) => ({
            success: true,
            enabled: false,
            config: { ...config, sources: [...config.sources, s] },
        }));
        open('/mcp/server-tools');
        await screen.findByText('Capabilities panel: gateway');
        fireEvent.click(screen.getByRole('button', { name: 'Connect tools' }));
        const dialog = screen.getByRole('dialog');
        fireEvent.change(within(dialog).getByLabelText(/Connection name/), { target: { value: 'Server notes' } });
        fireEvent.change(within(dialog).getByLabelText('Endpoint URL'), {
            target: { value: 'https://notes.test/mcp' },
        });
        fireEvent.click(within(dialog).getByRole('button', { name: 'Connect and choose tools' }));
        await waitFor(() =>
            expect(mocks.create).toHaveBeenCalledWith(
                expect.objectContaining({ id: 'server-notes', usage: { client: false, gateway: true } })
            )
        );
        expect(await screen.findByRole('heading', { name: 'Server notes' })).toBeInTheDocument();
        expect(screen.queryByRole('checkbox', { name: 'Ordinary tools' })).toBeNull();
        expect(screen.queryByRole('button', { name: 'Choose a client to use these tools' })).toBeNull();
    });
    it('keeps failed connection edits open and preserves credentials without restoring stale policies', async () => {
        mocks.patch.mockRejectedValue(new Error('Save failed'));
        open();
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
    it('connects from a name and URL, generates a stable ID, and opens the tool assignment panel', async () => {
        mocks.create.mockImplementation(async (s) => ({
            success: true,
            enabled: false,
            config: { ...config, sources: [...config.sources, s] },
        }));
        open();
        fireEvent.click(await screen.findByRole('button', { name: 'Connect tools' }));
        const dialog = screen.getByRole('dialog');
        expect(within(dialog).queryByRole('checkbox', { name: 'MCP clients' })).toBeNull();
        fireEvent.change(within(dialog).getByLabelText(/Connection name/), { target: { value: 'Team docs' } });
        fireEvent.change(within(dialog).getByLabelText('Endpoint URL'), { target: { value: 'https://team.test/mcp' } });
        fireEvent.click(within(dialog).getByRole('button', { name: 'Connect and choose tools' }));
        await waitFor(() =>
            expect(mocks.create).toHaveBeenCalledWith(
                expect.objectContaining({
                    id: 'team-docs',
                    name: 'Team docs',
                    endpoint: 'https://team.test/mcp',
                    usage: { client: true, gateway: false },
                })
            )
        );
        expect(await screen.findByRole('heading', { name: 'Team docs' })).toBeInTheDocument();
        expect(screen.getByText('How should these tools be used?')).toBeInTheDocument();
    });
    it('keeps client grants and setup together and saves an explicit empty grant without exposing other tools', async () => {
        mocks.saveClient.mockImplementation(async (profile) => ({
            success: true,
            enabled: false,
            config: { ...config, client_profiles: [profile] },
        }));
        open('/mcp/tools');
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
        ['/mcp/tools', 'client'],
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
        expect(screen.getByRole('heading', { name: 'Tool', level: 1 })).toBeInTheDocument();
    });
});
