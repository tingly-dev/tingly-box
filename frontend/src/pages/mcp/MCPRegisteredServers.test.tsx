import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPRegisteredServers from './MCPRegisteredServers';

const mocks = vi.hoisted(() => ({ get: vi.fn(), patch: vi.fn(), notify: vi.fn() }));
vi.mock('@/services/api', () => ({ api: { getMCPConfig: mocks.get, patchMCPSource: mocks.patch } }));
vi.mock('@/hooks/useNotify', () => ({ useNotify: () => ({ error: mocks.notify }) }));
vi.mock('@/contexts/FeatureFlagsContext', () => ({ useFeatureFlags: () => ({ refresh: vi.fn() }) }));
vi.mock('@/components/PageLayout', () => ({ PageLayout: ({ children }: { children: React.ReactNode }) => children }));
vi.mock('./AdvisorSettings', () => ({ default: () => null }));
vi.mock('./MCPRoutingPanel', () => ({ default: () => <div>Routing panel</div> }));
vi.mock('./MCPToolsPanel', () => ({ default: () => <div>Capabilities panel</div> }));
vi.mock('./MCPClientsPanel', () => ({ default: () => <div>Clients panel</div> }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockResolvedValue({
        success: true,
        enabled: false,
        config: {
            sources: [
                {
                    id: 'remote',
                    transport: 'http',
                    endpoint: 'https://example.test/mcp',
                    headers: { Authorization: '${TOKEN}' },
                    env: { TOKEN: 'saved-token' },
                },
            ],
        },
    });
});
describe('MCP center', () => {
    it('allows configuration when execution is disabled and keeps failed edits open for retry', async () => {
        mocks.patch.mockRejectedValue(new Error('Save failed'));
        render(
            <MemoryRouter initialEntries={['/mcp/sources']}>
                <MCPRegisteredServers />
            </MemoryRouter>
        );
        expect(
            await screen.findByText(
                'MCP execution is disabled. You can configure servers and inspect capabilities here.'
            )
        ).toBeInTheDocument();
        fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
        const endpoint = screen.getByLabelText('Endpoint URL');
        fireEvent.change(endpoint, { target: { value: 'https://changed.test/mcp' } });
        fireEvent.click(screen.getByRole('button', { name: 'Save' }));
        await waitFor(() => expect(mocks.notify).toHaveBeenCalledWith('Save failed'));
        expect(endpoint).toHaveValue('https://changed.test/mcp');
        expect(screen.getByLabelText('Server ID')).toBeDisabled();
        expect(mocks.patch.mock.calls[0][1]).toMatchObject({
            id: 'remote',
            endpoint: 'https://changed.test/mcp',
            headers: { Authorization: '${TOKEN}' },
            env: { TOKEN: 'saved-token' },
        });
    });
    it('opens the matching panel from dedicated client and capability routes', async () => {
        const view = render(
            <MemoryRouter initialEntries={['/mcp/tools']}>
                <MCPRegisteredServers />
            </MemoryRouter>
        );
        expect(await screen.findByText('Capabilities panel')).toBeInTheDocument();
        view.unmount();
        render(
            <MemoryRouter initialEntries={['/mcp/clients']}>
                <MCPRegisteredServers />
            </MemoryRouter>
        );
        expect(await screen.findByText('Clients panel')).toBeInTheDocument();
    });
});
