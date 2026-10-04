import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPSourceWorkspace from './MCPSourceWorkspace';
import type { MCPRouteSource, MCPSourceConfig } from './types';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, o: { defaultValue: string }) => o.defaultValue }),
}));
vi.mock('./AdvisorSettings', () => ({ default: () => <div>Consultation model settings</div> }));
const save = vi.fn();
const source: MCPSourceConfig = {
    id: 'docs',
    name: 'Docs',
    transport: 'http',
    enabled: true,
    tool_policies: { read: { enabled: true }, other: { enabled: false } },
};
const route: MCPRouteSource = {
    id: 'docs',
    name: 'Docs',
    origin: 'external',
    transport: 'http',
    state: 'connected',
    address: 'https://docs.test/mcp',
    processing: 'standard',
    tools: [
        {
            source_id: 'docs',
            name: 'read',
            normalized_name: 'tingly_box_mcp__docs__read',
            enabled: true,
            implementation: 'mcp',
            usage: { client: true, gateway: true },
            input_schema: {},
        },
    ],
};
const open = (s = source, r = route, enabled = true, usageScope?: 'client' | 'gateway') =>
    render(
        <MCPSourceWorkspace
            source={s}
            usageScope={usageScope}
            route={r}
            enabled={enabled}
            saveSource={save}
            onConnectClient={vi.fn()}
            onDelete={vi.fn()}
            onRefresh={vi.fn()}
        />
    );
beforeEach(() => {
    save.mockReset();
});
describe('source tool usage boundaries', () => {
    it.each(['client', 'gateway'] as const)(
        'exposes only the current %s purpose while preserving the other purpose',
        async (scope) => {
            save.mockResolvedValue(undefined);
            open(source, route, true, scope);
            const row = within(screen.getByRole('group', { name: 'read' }));
            expect(row.queryByRole('checkbox', { name: 'Enabled' })).toBeNull();
            expect(
                row.queryByRole('checkbox', { name: scope === 'client' ? 'Server Tools' : 'Expose through MCP' })
            ).toBeNull();
            fireEvent.click(
                row.getByRole('checkbox', { name: scope === 'client' ? 'Expose through MCP' : 'Server Tools' })
            );
            await waitFor(() =>
                expect(save).toHaveBeenCalledWith({
                    id: 'docs',
                    tool_policies: {
                        other: { enabled: false },
                        read: { enabled: true, usage: { client: scope !== 'client', gateway: scope !== 'gateway' } },
                    },
                })
            );
        }
    );
    it('edits global tool enablement in Tool without overwriting either purpose or unrelated policies', async () => {
        save.mockResolvedValue(undefined);
        open({
            ...source,
            tool_policies: { ...source.tool_policies, read: { enabled: true, usage: { client: true, gateway: true } } },
        });
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.queryByRole('checkbox', { name: 'Expose through MCP' })).toBeNull();
        expect(row.queryByRole('checkbox', { name: 'Server Tools' })).toBeNull();
        fireEvent.click(row.getByRole('checkbox', { name: 'Enabled' }));
        await waitFor(() =>
            expect(save).toHaveBeenCalledWith({
                id: 'docs',
                tool_policies: {
                    other: { enabled: false },
                    read: { enabled: false, usage: { client: true, gateway: true } },
                },
            })
        );
        expect(screen.getByRole('button', { name: 'Connection settings' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument();
    });
    it.each(['client', 'gateway'] as const)(
        'does not expose connection, global enablement or deletion in %s',
        (scope) => {
            open(source, route, true, scope);
            expect(screen.queryByRole('button', { name: 'Connection settings' })).toBeNull();
            expect(screen.queryByRole('button', { name: 'Remove connection' })).toBeNull();
            expect(
                screen.queryByRole('switch', { name: 'Enable shared connection (affects MCP and Server Tool)' })
            ).toBeNull();
            expect(screen.queryByRole('button', { name: 'Test tool' })).toBeNull();
        }
    );
    it('keeps Advisor model settings in Server Tool and blocks a standalone asset test', () => {
        const advisor = { ...source, id: 'advisor', transport: 'advisor' as const };
        const view = open(advisor, { ...route, id: 'advisor' });
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.queryByRole('checkbox', { name: 'Expose through MCP' })).toBeNull();
        expect(row.queryByRole('checkbox', { name: 'Server Tools' })).toBeNull();
        expect(row.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        expect(screen.queryByText('Consultation model settings')).toBeNull();
        view.unmount();
        open(advisor, { ...route, id: 'advisor' }, true, 'gateway');
        expect(screen.getByText('Consultation model settings')).toBeInTheDocument();
        expect(screen.getByRole('checkbox', { name: 'Server Tools' })).not.toBeDisabled();
        expect(screen.queryByRole('button', { name: 'Test tool' })).toBeNull();
    });
    it('keeps usage configuration available while execution is off and reports a rejected change', async () => {
        save.mockRejectedValue(new Error('Permission update failed'));
        open(source, route, false, 'gateway');
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.queryByRole('button', { name: 'Test tool' })).toBeNull();
        fireEvent.click(row.getByRole('checkbox', { name: 'Server Tools' }));
        expect(await screen.findByText('Permission update failed')).toBeInTheDocument();
        expect(row.getByRole('checkbox', { name: 'Server Tools' })).toBeChecked();
    });
});
