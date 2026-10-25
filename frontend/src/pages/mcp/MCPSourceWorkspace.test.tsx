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
            usage: { client: true, gateway: false },
            input_schema: {},
        },
    ],
};
const open = (s = source, r = route, enabled = true) =>
    render(
        <MCPSourceWorkspace
            source={s}
            route={r}
            enabled={enabled}
            saveSource={save}
            onDelete={vi.fn()}
            onRefresh={vi.fn()}
        />
    );
beforeEach(() => {
    save.mockReset();
});
describe('connection workspace', () => {
    it('switches one tool globally without overwriting unrelated policies', async () => {
        save.mockResolvedValue(undefined);
        open({
            ...source,
            tool_policies: { ...source.tool_policies, read: { enabled: true, usage: { client: true, gateway: true } } },
        });
        const row = within(screen.getByRole('group', { name: 'read' }));
        expect(row.queryByRole('checkbox')).toBeNull();
        fireEvent.click(row.getByRole('switch', { name: 'On' }));
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
    it('shows Advisor model settings with the tool switch and blocks a standalone test', () => {
        const advisor = { ...source, id: 'advisor', transport: 'advisor' as const };
        open(advisor, { ...route, id: 'advisor' });
        expect(screen.getByText('Consultation model settings')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Test tool' })).toBeDisabled();
        expect(screen.queryByRole('button', { name: 'Remove connection' })).toBeNull();
    });
    it('reports a rejected change and keeps the previous state', async () => {
        save.mockRejectedValue(new Error('Update failed'));
        open();
        const row = within(screen.getByRole('group', { name: 'read' }));
        fireEvent.click(row.getByRole('switch', { name: 'On' }));
        expect(await screen.findByText('Update failed')).toBeInTheDocument();
        expect(row.getByRole('switch', { name: 'On' })).toBeChecked();
    });
});
