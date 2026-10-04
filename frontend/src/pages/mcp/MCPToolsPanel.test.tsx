import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPToolsPanel from './MCPToolsPanel';
import type { MCPSourceConfig } from './types';

const mocks = vi.hoisted(() => ({ catalog: vi.fn(), call: vi.fn(), save: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { getMCPCatalog: mocks.catalog, callMCPTool: mocks.call } }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
const source: MCPSourceConfig = { id: 'remote', tool_policies: { other: { enabled: false } } };
const tool = {
    source_id: 'remote',
    name: 'echo',
    normalized_name: 'tingly_box_mcp__remote__echo',
    enabled: true,
    usage: { client: true, gateway: true },
    input_schema: { type: 'object', required: ['q'] },
    output_schema: { type: 'object' },
    annotations: { readOnlyHint: true },
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.catalog.mockResolvedValue({
        success: true,
        sources: [
            { source_id: 'remote', state: 'connected', tools: [tool] },
            { source_id: 'failed', state: 'error', error: 'Connection refused', tools: [] },
        ],
    });
    mocks.save.mockResolvedValue(undefined);
});

describe('MCP capability controls', () => {
    it('refreshes a persistent page after shared configuration changes and ignores stale discovery', async () => {
        let finishOldDiscovery!: (response: unknown) => void;
        const stale = new Promise((resolve) => {
            finishOldDiscovery = resolve;
        });
        const view = render(<MCPToolsPanel sources={[source]} enabled scopeOnly saveSource={mocks.save} />);
        expect(await screen.findByText('remote / echo')).toBeInTheDocument();
        expect(screen.queryByRole('checkbox', { name: 'Enabled' })).toBeNull();
        mocks.catalog.mockReturnValueOnce(stale);
        view.rerender(
            <MCPToolsPanel sources={[{ ...source, name: 'Updated' }]} enabled scopeOnly saveSource={mocks.save} />
        );
        await waitFor(() => expect(mocks.catalog).toHaveBeenCalledTimes(2));
        mocks.catalog.mockResolvedValueOnce({
            success: true,
            sources: [
                {
                    source_id: 'remote',
                    state: 'connected',
                    tools: [{ ...tool, usage: { client: false, gateway: true } }],
                },
            ],
        });
        view.rerender(
            <MCPToolsPanel sources={[{ ...source, name: 'Current' }]} enabled scopeOnly saveSource={mocks.save} />
        );
        await waitFor(() => expect(screen.queryByText('Current / echo')).toBeNull());
        await waitFor(() => expect(mocks.catalog).toHaveBeenCalledTimes(3));
        await act(async () => {
            finishOldDiscovery({
                success: true,
                sources: [{ source_id: 'remote', state: 'connected', tools: [tool] }],
            });
            await stale;
        });
        expect(screen.queryByText('Current / echo')).toBeNull();
    });
    it('keeps available tools visible when another server fails and preserves tool policies on edit', async () => {
        render(<MCPToolsPanel sources={[source]} enabled saveSource={mocks.save} />);
        expect(await screen.findByText('remote / echo')).toBeInTheDocument();
        expect(screen.getByText('failed: Connection refused')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('checkbox', { name: 'Use as an ordinary tool' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { usage: { client: false, gateway: true } } },
            })
        );
    });
    it('keeps server-only tools out of the ordinary list and adds them without changing gateway usage', async () => {
        mocks.catalog.mockResolvedValue({
            success: true,
            sources: [
                {
                    source_id: 'remote',
                    state: 'connected',
                    tools: [{ ...tool, usage: { client: false, gateway: true } }],
                },
            ],
        });
        render(<MCPToolsPanel sources={[source]} enabled saveSource={mocks.save} />);
        expect(
            await screen.findByText(
                'No tools are assigned to this section. Enable the corresponding usage in Tool sources.'
            )
        ).toBeInTheDocument();
        expect(screen.queryByText('remote / echo')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Choose ordinary tools' }));
        fireEvent.click(await screen.findByRole('button', { name: 'Add tool' }));
        await waitFor(() =>
            expect(mocks.save).toHaveBeenCalledWith({
                id: 'remote',
                tool_policies: { other: { enabled: false }, echo: { usage: { client: true, gateway: true } } },
            })
        );
    });
    it('requires a model request for Advisor instead of exposing an unusable standalone test', async () => {
        mocks.catalog.mockResolvedValue({
            success: true,
            sources: [
                {
                    source_id: 'advisor',
                    state: 'connected',
                    tools: [
                        {
                            ...tool,
                            source_id: 'advisor',
                            name: 'advisor',
                            implementation: 'virtual',
                            usage: { client: false, gateway: true },
                        },
                    ],
                },
            ],
        });
        render(<MCPToolsPanel sources={[]} enabled usage="gateway" saveSource={mocks.save} />);
        expect(await screen.findByRole('button', { name: 'Test tool' })).toBeDisabled();
        expect(screen.getByText(/Advisor requires model conversation context/)).toBeInTheDocument();
        expect(mocks.call).not.toHaveBeenCalled();
    });
    it('rejects invalid JSON arguments and shows structured tool errors without losing content', async () => {
        mocks.call.mockResolvedValue({
            success: true,
            result: {
                isError: true,
                structuredContent: { retained: true },
                content: [{ type: 'image', data: 'AQID', mimeType: 'image/png' }],
            },
        });
        render(<MCPToolsPanel sources={[source]} enabled saveSource={mocks.save} />);
        fireEvent.click(await screen.findByRole('button', { name: 'Test tool' }));
        const input = screen.getByLabelText('Arguments (JSON)');
        fireEvent.change(input, { target: { value: '[]' } });
        fireEvent.click(screen.getByRole('button', { name: 'Run' }));
        expect(await screen.findAllByText('Arguments must be a JSON object.')).toHaveLength(2);
        expect(mocks.call).not.toHaveBeenCalled();
        fireEvent.change(input, { target: { value: '{"q":"test"}' } });
        fireEvent.click(screen.getByRole('button', { name: 'Run' }));
        const result = await screen.findByTestId('mcp-test-result');
        expect(result).toHaveTextContent('structuredContent');
        expect(result).toHaveTextContent('isError');
        expect(result).toHaveTextContent('AQID');
        expect(mocks.call).toHaveBeenCalledWith({ source_id: 'remote', tool_name: 'echo', arguments: { q: 'test' } });
    });
});
