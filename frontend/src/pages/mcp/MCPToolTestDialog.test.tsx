import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPToolTestDialog from './MCPToolTestDialog';
const call = vi.hoisted(() => vi.fn());
vi.mock('@/services/api', () => ({ api: { callMCPTool: call } }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, o: { defaultValue: string }) => o.defaultValue }),
}));
const tool = {
    source_id: 'docs',
    name: 'read',
    normalized_name: 'docs-read',
    enabled: true,
    implementation: 'mcp',
    usage: { client: false, gateway: false },
    input_schema: { type: 'object' },
};
beforeEach(() => call.mockReset());
describe('shared administrative tool test', () => {
    it('rejects array arguments and retains structured upstream error content', async () => {
        call.mockResolvedValue({
            success: true,
            result: {
                isError: true,
                structuredContent: { retained: true },
                content: [{ type: 'image', data: 'AQID', mimeType: 'image/png' }],
            },
        });
        render(<MCPToolTestDialog tool={tool} enabled onClose={vi.fn()} />);
        const input = screen.getByLabelText('Arguments (JSON)');
        fireEvent.change(input, { target: { value: '[]' } });
        fireEvent.click(screen.getByRole('button', { name: 'Run test' }));
        expect(await screen.findByText('Arguments must be a JSON object.')).toBeInTheDocument();
        expect(call).not.toHaveBeenCalled();
        fireEvent.change(input, { target: { value: '{"note_id":"review"}' } });
        fireEvent.click(screen.getByRole('button', { name: 'Run test' }));
        const result = await screen.findByTestId('mcp-tool-test-result');
        expect(result).toHaveTextContent('AQID');
        expect(result).toHaveTextContent('isError');
        expect(result).toHaveTextContent('structuredContent');
        expect(call).toHaveBeenCalledWith({ source_id: 'docs', tool_name: 'read', arguments: { note_id: 'review' } });
    });
    it('resets old results on retry and displays a rejected API response', async () => {
        call.mockResolvedValueOnce({ success: true, result: { content: 'old-content' } }).mockResolvedValueOnce({
            success: false,
            error: 'Rejected by gateway',
        });
        render(<MCPToolTestDialog tool={tool} enabled onClose={vi.fn()} />);
        fireEvent.click(screen.getByRole('button', { name: 'Run test' }));
        expect(await screen.findByTestId('mcp-tool-test-result')).toHaveTextContent('old-content');
        await waitFor(() => expect(screen.getByRole('button', { name: 'Run test' })).toBeEnabled());
        fireEvent.click(screen.getByRole('button', { name: 'Run test' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Rejected by gateway');
        expect(screen.getByTestId('mcp-tool-test-result')).not.toHaveTextContent('old-content');
    });
    it('blocks execution when shared runtime or tool enablement is off', () => {
        const view = render(<MCPToolTestDialog tool={tool} enabled={false} onClose={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Run test' })).toBeDisabled();
        view.rerender(<MCPToolTestDialog tool={{ ...tool, enabled: false }} enabled onClose={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Run test' })).toBeDisabled();
        expect(call).not.toHaveBeenCalled();
    });
});
