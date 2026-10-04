import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPRoutingPanel from './MCPRoutingPanel';
import type { MCPCatalogTool, MCPRouteSource, MCPRoutingSnapshot } from './types';
const mocks = vi.hoisted(() => ({ edit: vi.fn(), client: vi.fn(), tools: vi.fn(), focus: vi.fn(), refresh: vi.fn() }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
const tool = (name: string, gateway = true): MCPCatalogTool => ({
    source_id: 'docs',
    name,
    normalized_name: `tingly_box_mcp__docs__${name}`,
    enabled: true,
    usage: { client: true, gateway },
    implementation: 'mcp',
});
const source: MCPRouteSource = {
    id: 'docs',
    name: 'Shared docs',
    origin: 'external',
    transport: 'http',
    address: 'http://example.test/mcp',
    state: 'connected',
    processing: 'standard',
    tools: [tool('search'), tool('read', false)],
};
const advisor: MCPRouteSource = {
    ...source,
    id: 'advisor',
    name: 'Advisor',
    processing: 'advisor',
    origin: 'builtin',
    state: 'unconfigured',
    tools: [],
};
const snapshot: MCPRoutingSnapshot = {
    sources: [source, advisor],
    clients: [
        {
            id: 'reader',
            name: 'Reader',
            endpoint: '/api/v1/mcp/reader',
            enabled: true,
            legacy: false,
            sources: [{ ...source, tools: [source.tools[0]] }],
        },
    ],
    server_tools: [{ ...source, tools: [source.tools[0]] }, advisor],
};
const profile = { id: 'reader', name: 'Reader', sources: ['docs'], tools: [source.tools[0].normalized_name] };
const renderPanel = (
    routing: MCPRoutingSnapshot = snapshot,
    extra: Partial<React.ComponentProps<typeof MCPRoutingPanel>> = {}
) => (
    <MCPRoutingPanel
        routing={routing}
        sources={[{ id: 'docs' }]}
        profiles={[profile]}
        enabled
        loading={false}
        onEditSource={mocks.edit}
        onClient={mocks.client}
        onTools={mocks.tools}
        onFocusSource={mocks.focus}
        onRefresh={mocks.refresh}
        {...extra}
    />
);
beforeEach(() => vi.clearAllMocks());
describe('effective MCP usage relationships', () => {
    it('keeps the two paths separate and takes configuration actions to their own surfaces', () => {
        render(renderPanel());
        const ordinary = screen.getByRole('region', { name: 'Ordinary tools' });
        const server = screen.getByRole('region', { name: 'Server Tools' });
        expect(within(ordinary).queryByRole('button', { name: /^Advisor:/ })).toBeNull();
        fireEvent.click(within(ordinary).getByRole('button', { name: /^Tingly MCP gateway:/ }));
        expect(mocks.client).toHaveBeenCalledWith('reader', true);
        fireEvent.click(within(server).getByRole('button', { name: 'Manage Server Tools' }));
        expect(mocks.tools).toHaveBeenCalledWith('gateway');
        fireEvent.click(within(ordinary).getByRole('button', { name: /^Shared docs:/ }));
        expect(mocks.focus).toHaveBeenCalledWith('docs');
        fireEvent.click(screen.getByRole('button', { name: 'Expand special processing' }));
        fireEvent.click(
            within(screen.getByTestId('mcp-special-processing')).getByRole('button', {
                name: /^Configure consultation provider:/,
            })
        );
        expect(mocks.tools).toHaveBeenCalledWith('gateway', 'advisor');
        expect(screen.queryByRole('button', { name: 'Connection instructions' })).toBeNull();
        expect(screen.queryByRole('button', { name: 'Check client route' })).toBeNull();
    });
    it('explains per-tool grants independently from gateway usage and shows shared impact', () => {
        render(renderPanel(snapshot, { focusSource: 'docs' }));
        const focused = screen.getByTestId('mcp-source-relationships');
        expect(within(focused).getAllByText('1 available tools')).toHaveLength(4);
        const read = within(focused).getByRole('article', { name: 'read' });
        expect(within(read).getByText('Tool is not granted to this client')).toBeInTheDocument();
        expect(within(read).getByText('Server Tool usage is off')).toBeInTheDocument();
        expect(within(focused).getByRole('region', { name: 'Who does a change affect?' })).toHaveTextContent(
            'Clients associated by saved grants: Reader'
        );
        fireEvent.click(within(focused).getByRole('button', { name: 'Configure Tool usage' }));
        expect(mocks.tools).toHaveBeenCalledWith('client', 'docs');
        fireEvent.click(within(focused).getByRole('button', { name: 'Configure Server Tool usage' }));
        expect(mocks.tools).toHaveBeenCalledWith('gateway', 'docs');
    });
    it('updates a revoked edge from the shared snapshot without changing gateway availability', () => {
        const view = render(renderPanel(snapshot, { focusSource: 'docs' }));
        const changed = { ...snapshot, clients: [{ ...snapshot.clients[0], sources: [] }] };
        view.rerender(renderPanel(changed, { focusSource: 'docs', profiles: [{ ...profile, sources: [] }] }));
        const search = screen.getByRole('article', { name: 'search' });
        expect(within(search).getByText('Connection is not granted to this client')).toBeInTheDocument();
        expect(within(search).getAllByText('Available')).toHaveLength(1);
        expect(screen.getByRole('region', { name: 'Who does a change affect?' })).toHaveTextContent(
            'Clients associated by saved grants: None'
        );
    });
    it('does not infer a usable edge from wildcard grants when the backend has not confirmed it', () => {
        render(
            renderPanel(
                { ...snapshot, clients: [{ ...snapshot.clients[0], sources: [] }], server_tools: [] },
                { focusSource: 'docs', profiles: [{ ...profile, sources: ['*'], tools: ['*'] }] }
            )
        );
        expect(screen.queryByText('Available')).toBeNull();
        expect(screen.getAllByText('Not confirmed in the effective configuration; refresh to check')).toHaveLength(3);
    });
    it.each([
        [{ enabled: false }, 'MCP execution is off'],
        [{ sources: [{ id: 'docs', enabled: false }] }, 'Shared connection is disabled'],
        [{ profiles: [{ ...profile, tools: [] }] }, 'Tool is not granted to this client'],
    ])('shows blocked paths without positive counts for %j', (extra, message) => {
        const changed = { ...snapshot, clients: [{ ...snapshot.clients[0], sources: [] }] };
        render(renderPanel(changed, { focusSource: 'docs', ...extra }));
        expect(screen.getAllByText(message).length).toBeGreaterThan(0);
        const ordinary = screen.getByRole('region', { name: 'Tool' });
        expect(within(ordinary).queryByText('1 available tools')).toBeNull();
    });
    it('retains a failed saved association while refusing to confirm any tool or gateway count', () => {
        const failed = { ...source, state: 'error', tools: [] };
        render(
            renderPanel(
                { ...snapshot, sources: [failed], server_tools: [{ ...failed, tools: [source.tools[0]] }] },
                { focusSource: 'docs' }
            )
        );
        expect(screen.getByRole('alert')).toHaveTextContent('Connection failed; availability cannot be confirmed');
        expect(screen.getByRole('region', { name: 'Who does a change affect?' })).toHaveTextContent('Reader');
        expect(screen.queryByText('1 available tools')).toBeNull();
        expect(screen.getByText(/No tool catalog is available/)).toBeInTheDocument();
    });
    it('keeps disabled clients in the saved impact review while marking their tools unavailable', () => {
        render(
            renderPanel(
                { ...snapshot, clients: [{ ...snapshot.clients[0], enabled: false, sources: [] }] },
                { focusSource: 'docs' }
            )
        );
        expect(screen.getAllByText('Client is disabled')).toHaveLength(2);
        expect(screen.getByRole('region', { name: 'Who does a change affect?' })).toHaveTextContent('Reader');
    });
    it('distinguishes an allow-list exclusion from a global tool switch and purpose settings', () => {
        const excluded = {
            ...source,
            tools: [
                { ...source.tools[0], enabled: false },
                { ...source.tools[1], enabled: false },
            ],
        };
        render(
            renderPanel(
                { ...snapshot, sources: [excluded] },
                { focusSource: 'docs', sources: [{ id: 'docs', tools: ['read'] }] }
            )
        );
        const search = screen.getByRole('article', { name: 'search' });
        const read = screen.getByRole('article', { name: 'read' });
        expect(within(search).getAllByText('Source allow list does not include this tool')).toHaveLength(2);
        expect(within(read).getAllByText('Disabled by the shared tool policy')).toHaveLength(2);
        expect(screen.queryByText('Available')).toBeNull();
    });
    it('shows the real Advisor provider and model and links its configuration to Server Tool', () => {
        const configured = {
            ...advisor,
            state: 'connected',
            advisor: { provider_uuid: 'local', provider_name: 'Consultation provider', model: 'consult-model' },
            tools: [
                {
                    ...tool('advisor'),
                    source_id: 'advisor',
                    normalized_name: 'tingly_box_mcp__builtin__advisor',
                    implementation: 'virtual',
                    usage: { client: false, gateway: true },
                },
            ],
        };
        render(
            renderPanel({ ...snapshot, sources: [configured], server_tools: [configured] }, { focusSource: 'advisor' })
        );
        expect(screen.getByText('Advisor requires gateway model context')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Configure Tool usage' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: /Consultation provider consult-model/ }));
        expect(mocks.tools).toHaveBeenCalledWith('gateway', 'advisor');
        expect(screen.getByRole('region', { name: 'Who does a change affect?' })).toHaveTextContent('None');
    });
    it('shows a missing bookmarked connection explicitly and keeps refresh available', () => {
        render(renderPanel(snapshot, { focusSource: 'deleted' }));
        expect(screen.getByRole('alert')).toHaveTextContent('This connection no longer exists');
        expect(screen.queryByRole('region', { name: 'Ordinary tools' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Refresh relationships' }));
        expect(mocks.refresh).toHaveBeenCalledTimes(1);
    });
    it('does not present an old snapshot as confirmed availability after refresh fails', () => {
        render(renderPanel(snapshot, { focusSource: 'docs', error: 'Discovery request failed' }));
        expect(screen.getByRole('alert')).toHaveTextContent('Discovery request failed');
        expect(screen.queryByTestId('mcp-source-relationships')).toBeNull();
        expect(screen.queryByText('Available')).toBeNull();
    });
});
