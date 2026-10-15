import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPRoutingPanel from './MCPRoutingPanel';
import type { MCPRouteSource } from './types';
const mocks = vi.hoisted(() => ({ routing: vi.fn(), probe: vi.fn(), edit: vi.fn(), client: vi.fn(), tools: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { getMCPRouting: mocks.routing, probeMCPClient: mocks.probe } }));
vi.mock('./AgentInstallCard', () => ({ default: () => <div>Install instructions</div> }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
const source = (id: string, processing = 'standard'): MCPRouteSource => ({
    id,
    name: id,
    origin: processing === 'advisor' ? 'builtin' : 'external',
    transport: processing === 'advisor' ? 'advisor' : 'http',
    address: 'http://example.test/mcp',
    state: 'connected',
    processing,
    tools: [],
});
const snapshot = {
    success: true,
    enabled: true,
    routing: {
        sources: [],
        clients: [
            {
                id: 'reader',
                name: 'Reader',
                endpoint: '/api/v1/mcp/reader',
                enabled: true,
                legacy: false,
                sources: [source('client-only')],
            },
        ],
        server_tools: [source('server-only'), { ...source('advisor', 'advisor'), state: 'unconfigured' }],
    },
};
const renderPanel = (revision: [] = []) => (
    <MCPRoutingPanel onEditSource={mocks.edit} onClient={mocks.client} onTools={mocks.tools} revision={revision} />
);
beforeEach(() => {
    vi.clearAllMocks();
    mocks.routing.mockResolvedValue(snapshot);
    mocks.probe.mockResolvedValue({ success: true, tools: ['echo'] });
});
describe('effective MCP routing', () => {
    it('separates client grants from server execution and opens the real configuration actions', async () => {
        render(renderPanel());
        const ordinary = await screen.findByRole('region', { name: 'Ordinary tools' });
        const server = screen.getByRole('region', { name: 'Server Tools' });
        expect(within(ordinary).getByRole('button', { name: /^client-only:/ })).toBeInTheDocument();
        expect(within(server).queryByRole('button', { name: /^client-only:/ })).toBeNull();
        expect(within(ordinary).queryByRole('button', { name: /^server-only:/ })).toBeNull();
        fireEvent.click(within(ordinary).getByRole('button', { name: /^Tingly MCP gateway:/ }));
        expect(mocks.client).toHaveBeenCalledWith('reader', true);
        fireEvent.click(within(server).getByRole('button', { name: 'Manage Server Tools' }));
        expect(mocks.tools).toHaveBeenCalledWith('gateway');
        fireEvent.click(screen.getByRole('button', { name: 'Expand special processing' }));
        const special = screen.getByTestId('mcp-special-processing');
        fireEvent.click(within(special).getByRole('button', { name: /^Configure consultation provider:/ }));
        expect(mocks.edit).toHaveBeenCalledWith('advisor');
        fireEvent.click(screen.getByRole('button', { name: 'Check client route' }));
        expect(await screen.findByTestId('mcp-route-probe')).toHaveTextContent('actual gateway endpoint');
        expect(mocks.probe).toHaveBeenCalledWith('reader', {});
    });
    it('prevents an older response from restoring routes after a configuration change', async () => {
        let release: (value: typeof snapshot) => void = () => {};
        mocks.routing.mockReturnValueOnce(
            new Promise((resolve) => {
                release = resolve;
            })
        );
        const view = render(renderPanel());
        await waitFor(() => expect(mocks.routing).toHaveBeenCalledTimes(1));
        mocks.routing.mockResolvedValue({ ...snapshot, routing: { ...snapshot.routing, clients: [] } });
        view.rerender(renderPanel([]));
        expect(await screen.findByText(/No client profiles are configured/)).toBeInTheDocument();
        await act(async () => release(snapshot));
        expect(screen.queryByText('Reader')).toBeNull();
    });
    it('disables execution and shows zero reachable tools when the gateway is disabled', async () => {
        mocks.routing.mockResolvedValue({ ...snapshot, enabled: false });
        render(renderPanel());
        await screen.findByRole('region', { name: 'Ordinary tools' });
        expect(screen.getByRole('button', { name: 'Check client route' })).toBeDisabled();
        expect(screen.getAllByText('0 reachable tools')).toHaveLength(2);
    });
});
