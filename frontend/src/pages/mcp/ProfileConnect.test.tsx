import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ProfileConnect, { registerCommands } from './ProfileConnect';

vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: () => Promise.resolve('http://localhost:12580') }));

describe('ProfileConnect', () => {
    it('builds each client command from the profile endpoint', () => {
        const c = registerCommands('http://h:1', 'prof-1', 'tb-codex');
        expect(c.codex).toBe('codex mcp add tb-codex --url "http://h:1/api/v1/mcp/prof-1" --bearer-token-env-var TINGLY_MCP_TOKEN');
        expect(c.claude).toContain('claude mcp add --transport http tb-codex "http://h:1/api/v1/mcp/prof-1"');
        expect(c.opencode).toContain('"url": "http://h:1/api/v1/mcp/prof-1"');
    });

    it('switches the shown command with the client choice', async () => {
        render(<ProfileConnect profileId="codex-dev" serverName="tb-codex-dev" />);
        await waitFor(() => expect(screen.getByText(/claude mcp add/)).toHaveTextContent('localhost:12580/api/v1/mcp/codex-dev'));
        fireEvent.click(screen.getByRole('button', { name: 'Codex' }));
        expect(screen.getByText(/codex mcp add tb-codex-dev/)).toBeInTheDocument();
    });
});
