import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ProfileConnect, { registerCommands } from './ProfileConnect';

vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: () => Promise.resolve('http://localhost:12580') }));
// The model token must never be what an agent is given for MCP.
vi.mock('@/services/api', () => ({
    api: {
        getToken: () => Promise.resolve({ token: 'tb-model-x' }),
        getMCPToken: () => Promise.resolve({ success: true, data: { token: 'tb-mcp-123' } }),
    },
}));

describe('ProfileConnect', () => {
    it('writes the endpoint and token into each client command', () => {
        const c = registerCommands('http://h:1', 'prof-1', 'tb-codex', 'tok');
        expect(c.claude.command).toBe(
            'claude mcp add --transport http tb-codex "http://h:1/api/v1/mcp/prof-1" --header "Authorization: Bearer tok"'
        );
        expect(c.codex.command).toContain('url = "http://h:1/api/v1/mcp/prof-1"');
        expect(c.codex.command).toContain('Authorization = "Bearer tok"');
        expect(c.opencode.command).toContain('"Authorization": "Bearer tok"');
        expect(Object.values(c).some((x) => x.command.includes('$(') || x.command.includes('env'))).toBe(false);
    });

    it('shows the URL and the whole command with the MCP token, and switches client', async () => {
        render(<ProfileConnect profileId="codex-dev" serverName="tb-codex-dev" />);
        expect(screen.getByText('http://localhost:12580/api/v1/mcp/codex-dev')).toBeInTheDocument();
        await waitFor(() => expect(screen.getByTestId('mcp-connect-command')).toHaveTextContent('Bearer tb-mcp-123'));
        expect(screen.getByTestId('mcp-connect-command')).not.toHaveTextContent('tb-model-x');
        fireEvent.click(screen.getByRole('button', { name: 'Codex' }));
        expect(screen.getByTestId('mcp-connect-command')).toHaveTextContent('[mcp_servers.tb-codex-dev]');
    });
});
