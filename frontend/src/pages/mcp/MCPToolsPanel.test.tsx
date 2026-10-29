import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPToolsPanel from './MCPToolsPanel';
import type { MCPRouteSource, MCPSourceConfig } from './types';

const mocks = vi.hoisted(() => ({ save: vi.fn() }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (_: string, options: Record<string, unknown> & { defaultValue: string }) =>
            options.defaultValue.replace(/\{\{(\w+)\}\}/g, (_m, k) => String(options[k])),
    }),
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
    usage: { client: true, gateway: false },
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
const props = {
    sources: [source],
    routes: [route],
    loading: false,
    enabled: true,
    saveSource: mocks.save,
    onConfigureSource: vi.fn(),
};
beforeEach(() => {
    vi.clearAllMocks();
    mocks.save.mockResolvedValue(undefined);
});

describe('tool list', () => {
    it('uses the shared card chrome and heading hierarchy for each source', () => {
        render(<MCPToolsPanel {...props} />);
        const connection = screen.getByRole('article', { name: 'Remote docs' });
        expect(connection.classList.contains('MuiCard-root')).toBe(true);
        expect(within(connection).getByRole('heading', { level: 2, name: 'Remote docs' })).toBeInTheDocument();
        expect(within(connection).getByText('Connected')).toBeInTheDocument();
        expect(within(connection).getByRole('button', { name: 'Settings' })).toBeEnabled();
    });
    it('retains loading, off, empty-discovery and save-error states', async () => {
        const view = render(<MCPToolsPanel {...props} loading />);
        expect(screen.getByRole('progressbar')).toBeInTheDocument();
        expect(screen.getByText('Checking…')).toBeInTheDocument();
        expect(screen.getByRole('switch', { name: 'On' })).toBeDisabled();
        view.rerender(<MCPToolsPanel {...props} sources={[{ ...source, enabled: false }]} />);
        expect(screen.getByText('Off')).toBeInTheDocument();
        view.rerender(<MCPToolsPanel {...props} routes={[{ ...route, tools: [] }]} />);
        expect(screen.getByText('The connection returned no tools.')).toBeInTheDocument();
        mocks.save.mockRejectedValueOnce(new Error('Could not save tool'));
        view.rerender(<MCPToolsPanel {...props} />);
        fireEvent.click(screen.getByRole('switch', { name: 'On' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Could not save tool');
        await waitFor(() => expect(screen.getByRole('switch', { name: 'On' })).toBeEnabled());
    });
    it('shows one configure action per connection and every tool in the applied snapshot', () => {
        const configure = vi.fn();
        render(
            <MCPToolsPanel
                {...props}
                routes={[{ ...route, tools: [tool, { ...tool, name: 'read', normalized_name: 'read' }] }]}
                onConfigureSource={configure}
            />
        );
        expect(screen.getAllByRole('button', { name: 'Settings' })).toHaveLength(1);
        expect(screen.getByRole('article', { name: 'Remote docs / echo' })).toBeInTheDocument();
        expect(screen.getByRole('article', { name: 'Remote docs / read' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Settings' }));
        expect(configure).toHaveBeenCalledWith('remote');
    });
    it('keeps a failed connection visible and switches one tool without touching other policies', async () => {
        render(
            <MCPToolsPanel
                {...props}
                sources={[source, { id: 'failed' }]}
                routes={[
                    route,
                    { ...route, id: 'failed', name: 'failed', state: 'error', error: 'Connection refused', tools: [] },
                ]}
            />
        );
        expect(screen.getByText('Connection refused')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Fix connection' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('switch', { name: 'On' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { enabled: false } },
            })
        );
    });
    it('has no per-purpose usage controls', () => {
        render(<MCPToolsPanel {...props} />);
        expect(screen.queryByRole('checkbox')).toBeNull();
        expect(screen.getAllByRole('switch')).toHaveLength(1);
    });
    it('lists Advisor with its model action and blocks a standalone test', () => {
        const advisor = { ...source, id: 'advisor', transport: 'advisor' as const };
        const advisorRoute = {
            ...route,
            id: 'advisor',
            tools: [{ ...tool, source_id: 'advisor', implementation: 'virtual', usage: { client: false, gateway: true } }],
        };
        render(<MCPToolsPanel {...props} sources={[advisor]} routes={[advisorRoute]} />);
        expect(screen.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        expect(screen.getByRole('button', { name: 'Configure Advisor model' })).toBeInTheDocument();
        expect(screen.getByText('Server Tool')).toBeInTheDocument();
    });

    it('expands a tool to plain JSON schemas and folds long lists', () => {
        const many = Array.from({ length: 7 }, (_, i) => ({
            ...tool,
            name: `tool_${i}`,
            normalized_name: `tool_${i}`,
            output_schema: { type: 'object' },
        }));
        render(<MCPToolsPanel {...props} routes={[{ ...route, tools: many }]} />);
        expect(screen.getByText('7 of 7 tools on')).toBeInTheDocument();
        expect(screen.queryByText('tool_6')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Show all 7 tools' }));
        expect(screen.getByText('tool_6')).toBeInTheDocument();
        fireEvent.click(screen.getByText('tool_0'));
        expect(screen.getByText('Parameters')).toBeInTheDocument();
        expect(screen.getByText('Output schema')).toBeInTheDocument();
        expect(screen.getAllByText(/"type": "object"/).length).toBeGreaterThan(0);
    });
    it('turns every tool of a connection on or off in one save', async () => {
        const two = [tool, { ...tool, name: 'read', normalized_name: 'read' }];
        render(<MCPToolsPanel {...props} routes={[{ ...route, tools: two }]} />);
        fireEvent.click(screen.getByRole('button', { name: 'All' }));
        fireEvent.click(screen.getByRole('menuitem', { name: 'Turn off' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { echo: { enabled: false }, read: { enabled: false }, other: undefined },
            })
        );
    });
});
