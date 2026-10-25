import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPToolsPanel from './MCPToolsPanel';
import type { MCPRouteSource, MCPSourceConfig } from './types';

const mocks = vi.hoisted(() => ({ save: vi.fn() }));
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
    it('shows one configure action per connection and every tool in the applied snapshot', () => {
        const configure = vi.fn();
        render(
            <MCPToolsPanel
                {...props}
                routes={[{ ...route, tools: [tool, { ...tool, name: 'read', normalized_name: 'read' }] }]}
                onConfigureSource={configure}
            />
        );
        expect(screen.getAllByRole('button', { name: 'Configure this connection' })).toHaveLength(1);
        expect(screen.getByRole('article', { name: 'Remote docs / echo' })).toBeInTheDocument();
        expect(screen.getByRole('article', { name: 'Remote docs / read' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Configure this connection' }));
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
});
