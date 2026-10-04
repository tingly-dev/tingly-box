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
                row.queryByRole('checkbox', { name: scope === 'client' ? 'Server Tools' : 'Ordinary tools' })
            ).toBeNull();
            fireEvent.click(
                row.getByRole('checkbox', { name: scope === 'client' ? 'Ordinary tools' : 'Server Tools' })
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
    it('revokes one usage without overwriting the other usage or unrelated tool policies', async () => {
        save.mockResolvedValue(undefined);
        open();
        fireEvent.click(
            within(screen.getByRole('group', { name: 'read' })).getByRole('checkbox', { name: 'Ordinary tools' })
        );
        await waitFor(() =>
            expect(save).toHaveBeenCalledWith({
                id: 'docs',
                tool_policies: {
                    other: { enabled: false },
                    read: { enabled: true, usage: { client: false, gateway: true } },
                },
            })
        );
    });
    it('requires model context for Advisor and keeps it out of ordinary tools and standalone tests', () => {
        open({ ...source, id: 'advisor', transport: 'advisor' }, { ...route, id: 'advisor' });
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.getByRole('checkbox', { name: 'Ordinary tools' })).toBeDisabled();
        expect(row.getByRole('checkbox', { name: 'Ordinary tools' })).not.toBeChecked();
        expect(row.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        expect(row.getByRole('checkbox', { name: 'Server Tools' })).not.toBeDisabled();
    });
    it('keeps usage configuration available while execution is off and reports a rejected change', async () => {
        save.mockRejectedValue(new Error('Permission update failed'));
        open(source, route, false);
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        fireEvent.click(row.getByRole('checkbox', { name: 'Server Tools' }));
        expect(await screen.findByText('Permission update failed')).toBeInTheDocument();
        expect(row.getByRole('checkbox', { name: 'Server Tools' })).toBeChecked();
    });
});
