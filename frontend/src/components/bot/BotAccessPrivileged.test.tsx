import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import BotAccessDialog from './BotAccessDialog';
import type { BotGroupDetail, BotSettings, DirectChatDetail } from '@/types/bot';
import type { RemoteAccess } from './useRemoteAccess';

const api = vi.hoisted(() => ({
    setBotDirectChatPermission: vi.fn().mockResolvedValue({}),
    setBotDirectChatPermissions: vi.fn().mockResolvedValue({}),
    setBotGroupActorPermission: vi.fn().mockResolvedValue({}),
}));
vi.mock('@/services/api', () => ({ api }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (_: string, options?: { defaultValue?: string }) => options?.defaultValue ?? _ }) }));
vi.mock('./PairingCodePanel', () => ({ default: () => null }));

const row = (action: string, effect: 'allow' | 'deny') => ({ capability: 'remote_control' as const, action, effect });
const controlRows = [row('access', 'allow'), row('remote_control.start', 'allow'), row('remote_control.approve', 'allow')];
const PRIV = 'remote_control.privileged';

const renderAccess = (chats: DirectChatDetail[], groups: BotGroupDetail[] = []) => {
    const access: RemoteAccess = { loaded: true, loading: false, error: '', chats, groups, controllers: [], controllingGroups: [], pendingChats: [], reload: vi.fn().mockResolvedValue(undefined) };
    render(<BotAccessDialog inline open scope="remote_control" bot={{ uuid: 'bot', platform: 'telegram' } as BotSettings} onClose={vi.fn()} remoteAccess={access} />);
};
const chatWith = (permissions: DirectChatDetail['permissions']): DirectChatDetail => ({
    chat: { id: 'c1', bot_uuid: 'bot', platform: 'telegram', external_chat_id: '123', alias: 'Alice', peer_actor_id: 'a1', blocked: false },
    permissions,
});
const privilegedSwitch = () => screen.getByRole('switch', { name: /Any folder and files/ });

describe('Any folder and files (privileged)', () => {
    beforeEach(() => vi.clearAllMocks());

    it('toggles one explicit row for a chat that can control', async () => {
        renderAccess([chatWith([...controlRows, row(PRIV, 'deny')])]);
        expect(privilegedSwitch()).not.toBeChecked();
        fireEvent.click(privilegedSwitch());
        await waitFor(() => expect(api.setBotDirectChatPermission).toHaveBeenCalledWith('bot', 'c1', 'remote_control', PRIV, 'allow'));
    });

    it('is unavailable until the chat can control the bot', () => {
        renderAccess([chatWith([row(PRIV, 'deny')])]);
        expect(privilegedSwitch()).toBeDisabled();
    });

    it('is kept when control is given again', async () => {
        renderAccess([chatWith([row('access', 'allow'), row('remote_control.start', 'allow'), row(PRIV, 'allow')])]);
        fireEvent.click(screen.getByRole('button', { name: 'Can control' }));
        await waitFor(() => expect(api.setBotDirectChatPermissions).toHaveBeenCalledTimes(1));
        expect(api.setBotDirectChatPermissions.mock.calls[0][2]).toContainEqual(row(PRIV, 'allow'));
    });

    it('is taken away with control', async () => {
        renderAccess([chatWith([...controlRows, row(PRIV, 'allow')])]);
        fireEvent.click(screen.getByRole('button', { name: 'No access' }));
        await waitFor(() => expect(api.setBotDirectChatPermissions).toHaveBeenCalledTimes(1));
        expect(api.setBotDirectChatPermissions.mock.calls[0][2]).toContainEqual(row(PRIV, 'deny'));
    });

    it('toggles one explicit row for a group member', async () => {
        const group: BotGroupDetail = {
            group: { id: 'g1', bot_uuid: 'bot', platform: 'telegram', external_group_id: '-100', alias: 'Eng', blocked: false },
            capabilities: { remote_control: 'allow' },
            actors: [{ actor: { id: 'act1', external_actor_id: 'bob', display_name: 'Bob' }, permissions: [row('remote_control.start', 'allow'), row('remote_control.approve', 'allow'), row(PRIV, 'allow')] }],
        };
        renderAccess([], [group]);
        expect(privilegedSwitch()).toBeChecked();
        fireEvent.click(privilegedSwitch());
        await waitFor(() => expect(api.setBotGroupActorPermission).toHaveBeenCalledWith('bot', 'g1', 'act1', 'remote_control', PRIV, 'deny'));
    });
});
