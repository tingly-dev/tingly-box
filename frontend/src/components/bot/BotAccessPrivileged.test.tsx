import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import BotAccessDialog from './BotAccessDialog';
import type { BotGroupDetail, BotSettings, DirectChatDetail } from '@/types/bot';
import type { RemoteAccess } from './useRemoteAccess';

const api = vi.hoisted(() => ({
    setBotDirectChatPermission: vi.fn().mockResolvedValue({}),
    setBotDirectChatPermissions: vi.fn().mockResolvedValue({}),
    setBotDirectChatBlocked: vi.fn().mockResolvedValue({}),
    setBotGroupActorPermission: vi.fn().mockResolvedValue({}),
}));
vi.mock('@/services/api', () => ({ api }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (_: string, options?: { defaultValue?: string }) => options?.defaultValue ?? _ }) }));
vi.mock('./PairingCodePanel', () => ({ default: () => null }));

const row = (action: string, effect: 'allow' | 'deny') => ({ capability: 'remote_control' as const, action, effect });
const controlRows = [row('access', 'allow'), row('remote_control.start', 'allow'), row('remote_control.approve', 'allow')];

const chatWith = (permissions: DirectChatDetail['permissions'], over: Partial<DirectChatDetail['chat']> = {}): DirectChatDetail => ({
    chat: { id: 'c1', bot_uuid: 'bot', platform: 'telegram', external_chat_id: '123', alias: 'Alice', peer_actor_id: 'a1', blocked: false, ...over },
    permissions,
});

const renderChat = (chat: DirectChatDetail) => {
    const reload = vi.fn().mockResolvedValue(undefined);
    const access: RemoteAccess = { loaded: true, loading: false, error: '', chats: [chat], groups: [], controllers: [], controllingGroups: [], pendingChats: [], reload };
    render(<BotAccessDialog inline open scope="remote_control" bot={{ uuid: 'bot', platform: 'telegram' } as BotSettings} onClose={vi.fn()} remoteAccess={access} />);
};

const privilegedSwitch = () => screen.getByRole('switch', { name: /Any folder and files/ });

describe('Any folder and files (privileged) for a direct chat', () => {
    beforeEach(() => vi.clearAllMocks());

    it('is off for a chat that can control, and turns on with one explicit row', async () => {
        renderChat(chatWith([...controlRows, row('remote_control.privileged', 'deny')]));
        expect(privilegedSwitch()).not.toBeChecked();
        fireEvent.click(privilegedSwitch());
        await waitFor(() => expect(api.setBotDirectChatPermission).toHaveBeenCalledWith('bot', 'c1', 'remote_control', 'remote_control.privileged', 'allow'));
        expect(api.setBotDirectChatPermissions).not.toHaveBeenCalled();
    });

    it('is on when granted, and turns off with an explicit deny', async () => {
        renderChat(chatWith([...controlRows, row('remote_control.privileged', 'allow')]));
        expect(privilegedSwitch()).toBeChecked();
        fireEvent.click(privilegedSwitch());
        await waitFor(() => expect(api.setBotDirectChatPermission).toHaveBeenCalledWith('bot', 'c1', 'remote_control', 'remote_control.privileged', 'deny'));
    });

    it('cannot be turned on for a chat that cannot control the bot', () => {
        renderChat(chatWith([row('remote_control.privileged', 'deny')]));
        expect(privilegedSwitch()).toBeDisabled();
    });

    it('is kept when the chat is given control again, and dropped when control is removed', async () => {
        // A repaired chat (start allowed, approve missing) that already has the grant.
        renderChat(chatWith([row('access', 'allow'), row('remote_control.start', 'allow'), row('remote_control.privileged', 'allow')]));
        fireEvent.click(screen.getByRole('button', { name: 'Can control' }));
        await waitFor(() => expect(api.setBotDirectChatPermissions).toHaveBeenCalledTimes(1));
        expect(api.setBotDirectChatPermissions.mock.calls[0][2]).toEqual([
            row('access', 'allow'), row('remote_control.start', 'allow'), row('remote_control.approve', 'allow'), row('remote_control.privileged', 'allow'),
        ]);
    });

    it('giving control to a chat that never had the grant leaves it denied', async () => {
        renderChat(chatWith([row('remote_control.privileged', 'deny')]));
        fireEvent.click(screen.getByRole('button', { name: 'Can control' }));
        await waitFor(() => expect(api.setBotDirectChatPermissions).toHaveBeenCalledTimes(1));
        expect(api.setBotDirectChatPermissions.mock.calls[0][2]).toContainEqual(row('remote_control.privileged', 'deny'));
    });

    it('removing access also removes the grant', async () => {
        renderChat(chatWith([...controlRows, row('remote_control.privileged', 'allow')]));
        fireEvent.click(screen.getByRole('button', { name: 'No access' }));
        await waitFor(() => expect(api.setBotDirectChatPermissions).toHaveBeenCalledTimes(1));
        expect(api.setBotDirectChatPermissions.mock.calls[0][2]).toEqual([
            row('access', 'deny'), row('remote_control.start', 'deny'), row('remote_control.approve', 'deny'), row('remote_control.privileged', 'deny'),
        ]);
    });
});

describe('Any folder and files (privileged) for a group member', () => {
    beforeEach(() => vi.clearAllMocks());

    const group = (permissions: BotGroupDetail['actors'][number]['permissions']): BotGroupDetail => ({
        group: { id: 'g1', bot_uuid: 'bot', platform: 'telegram', external_group_id: '-100', alias: 'Eng', blocked: false },
        capabilities: { remote_control: 'allow' },
        actors: [{ actor: { id: 'act1', external_actor_id: 'bob', display_name: 'Bob' }, permissions }],
    });
    const renderGroup = (detail: BotGroupDetail) => {
        const access: RemoteAccess = { loaded: true, loading: false, error: '', chats: [], groups: [detail], controllers: [], controllingGroups: [], pendingChats: [], reload: vi.fn().mockResolvedValue(undefined) };
        render(<BotAccessDialog inline open scope="remote_control" bot={{ uuid: 'bot', platform: 'telegram' } as BotSettings} onClose={vi.fn()} remoteAccess={access} />);
    };

    it('turns on for one member with one explicit row', async () => {
        renderGroup(group([row('remote_control.start', 'allow'), row('remote_control.approve', 'allow'), row('remote_control.privileged', 'deny')]));
        const toggle = screen.getByRole('switch', { name: /Bob.*Any folder and files|Any folder and files.*Bob/ });
        expect(toggle).not.toBeChecked();
        fireEvent.click(toggle);
        await waitFor(() => expect(api.setBotGroupActorPermission).toHaveBeenCalledWith('bot', 'g1', 'act1', 'remote_control', 'remote_control.privileged', 'allow'));
    });

    it('turns off with an explicit deny', async () => {
        renderGroup(group([row('remote_control.start', 'allow'), row('remote_control.approve', 'allow'), row('remote_control.privileged', 'allow')]));
        const toggle = screen.getByRole('switch', { name: /Any folder and files/ });
        expect(toggle).toBeChecked();
        fireEvent.click(toggle);
        await waitFor(() => expect(api.setBotGroupActorPermission).toHaveBeenCalledWith('bot', 'g1', 'act1', 'remote_control', 'remote_control.privileged', 'deny'));
    });
});
