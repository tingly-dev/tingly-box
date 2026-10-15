import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPToolsPanel from './MCPToolsPanel';
import type { MCPRouteSource, MCPSourceConfig } from './types';

const mocks = vi.hoisted(() => ({ catalog: vi.fn(), save: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { getMCPCatalog: mocks.catalog } }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
const source: MCPSourceConfig = {
    id: 'remote',
    name: 'Remote docs',
    transport: 'http',
    tool_policies: { other: { enabled: false } },
};
const tool = {
    source_id: 'remote',
    name: 'echo',
    normalized_name: 'tingly_box_mcp__remote__echo',
    enabled: true,
    implementation: 'mcp',
    usage: { client: true, gateway: true },
    input_schema: { type: 'object' },
};
const route: MCPRouteSource = {
    id: 'remote',
    name: 'Remote docs',
    origin: 'external',
    transport: 'http',
    state: 'connected',
    processing: 'standard',
    address: 'https://docs.test/mcp',
    tools: [tool],
};
const props = { sources: [source], routes: [route], loading: false, enabled: true, saveSource: mocks.save };
beforeEach(() => {
    vi.clearAllMocks();
    mocks.save.mockResolvedValue(undefined);
});

describe('MCP grouped catalog', () => {
    it('shows connection actions once for multiple tools and uses the applied workspace snapshot', () => {
        const configure = vi.fn(),
            relationships = vi.fn();
        render(
            <MCPToolsPanel
                {...props}
                routes={[{ ...route, tools: [tool, { ...tool, name: 'read', normalized_name: 'read' }] }]}
                mode="client"
                onConfigureSource={configure}
                onRelationships={relationships}
            />
        );
        expect(screen.getAllByRole('button', { name: 'Configure MCP publication' })).toHaveLength(1);
        expect(screen.getAllByRole('button', { name: 'View usage relationships' })).toHaveLength(1);
        expect(screen.getByRole('article', { name: 'Remote docs / echo' })).toBeInTheDocument();
        expect(screen.getByRole('article', { name: 'Remote docs / read' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Configure MCP publication' }));
        expect(configure).toHaveBeenCalledWith('remote');
        expect(mocks.catalog).not.toHaveBeenCalled();
    });
    it('immediately removes stale cards when the controlled route snapshot changes or becomes unknown', () => {
        const view = render(<MCPToolsPanel {...props} mode="client" />);
        expect(screen.getByRole('article', { name: 'Remote docs / echo' })).toBeInTheDocument();
        view.rerender(
            <MCPToolsPanel
                {...props}
                mode="client"
                routes={[{ ...route, tools: [{ ...tool, usage: { client: false, gateway: true } }] }]}
            />
        );
        expect(screen.queryByRole('article', { name: 'Remote docs / echo' })).toBeNull();
        view.rerender(<MCPToolsPanel {...props} mode="client" routes={[]} loading />);
        expect(screen.queryByRole('checkbox')).toBeNull();
        expect(mocks.catalog).not.toHaveBeenCalled();
    });
    it('keeps a failed source visible beside healthy tools without overwriting unrelated policies', async () => {
        render(
            <MCPToolsPanel
                {...props}
                mode="client"
                sources={[source, { id: 'failed', usage: { client: true, gateway: false } }]}
                routes={[
                    route,
                    { ...route, id: 'failed', name: 'failed', state: 'error', error: 'Connection refused', tools: [] },
                ]}
            />
        );
        expect(screen.getByText('Connection refused')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('checkbox', { name: 'Expose through MCP' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { usage: { client: false, gateway: true } } },
            })
        );
    });
    it('publishes a server-only candidate without changing execution eligibility', async () => {
        render(
            <MCPToolsPanel
                {...props}
                mode="client"
                routes={[{ ...route, tools: [{ ...tool, usage: { client: false, gateway: true } }] }]}
            />
        );
        expect(screen.queryByRole('article', { name: 'Remote docs / echo' })).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Choose tools to publish' }));
        fireEvent.click(screen.getByRole('button', { name: 'Add tool' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { usage: { client: true, gateway: true } } },
            })
        );
    });
    it('shows unpublished assets and changes global enablement without changing usage', async () => {
        render(
            <MCPToolsPanel
                {...props}
                mode="asset"
                routes={[{ ...route, tools: [{ ...tool, usage: { client: false, gateway: false } }] }]}
            />
        );
        expect(screen.getByRole('article', { name: 'Remote docs / echo' })).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Choose tools to publish' })).toBeNull();
        fireEvent.click(screen.getByRole('checkbox', { name: 'Enabled' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { enabled: false } },
            })
        );
    });
    it.each(['client', 'gateway'] as const)(
        'exposes only the %s purpose with no global switch or standalone test',
        (mode) => {
            render(<MCPToolsPanel {...props} mode={mode} />);
            expect(screen.queryByRole('checkbox', { name: 'Enabled' })).toBeNull();
            expect(screen.queryByRole('button', { name: 'Test tool' })).toBeNull();
            expect(screen.getAllByRole('checkbox')).toHaveLength(1);
        }
    );
    it('keeps Advisor out of MCP candidates and blocks a standalone asset test', () => {
        const advisor = { ...source, id: 'advisor', transport: 'advisor' as const };
        const advisorRoute = {
            ...route,
            id: 'advisor',
            tools: [
                { ...tool, source_id: 'advisor', implementation: 'virtual', usage: { client: false, gateway: true } },
            ],
        };
        const view = render(<MCPToolsPanel {...props} mode="asset" sources={[advisor]} routes={[advisorRoute]} />);
        expect(screen.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        view.rerender(<MCPToolsPanel {...props} mode="client" sources={[advisor]} routes={[advisorRoute]} />);
        fireEvent.click(screen.getByRole('button', { name: 'Choose tools to publish' }));
        expect(within(screen.getByRole('dialog')).queryByRole('button', { name: 'Add tool' })).toBeNull();
    });
});
