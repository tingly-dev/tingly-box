import { beforeEach, describe, expect, it, vi } from 'vitest';
import { imHost, refreshChatIndex } from './imHost';

const m = vi.hoisted(() => ({ groupsFail: false, aliasOf123: 'Alice', bots: [{ uuid: 'bot', name: 'Bot', platform: 'telegram', enabled: true }] as { uuid: string; name: string; platform: string; enabled: boolean }[] }));

vi.mock('@/components/bot/BotAccessDialog', () => ({ default: () => null }));
vi.mock('@/constants/platformGuides', () => ({ PLATFORM_BRAND_ICONS: {}, platformDisplayName: (id: string) => id.charAt(0).toUpperCase() + id.slice(1) }));
vi.mock('@/i18n', () => ({ default: { t: (_key: string, options: { defaultValue: string }) => options.defaultValue } }));
vi.mock('@/services/imbotApi', () => ({ imbotApi: { getImBotSettingsList: async () => ({ success: true, settings: m.bots }) } }));
vi.mock('@/services/botApi', () => ({
    listBotDirectChats: async () => ({ chats: [
        { chat: { platform: 'telegram', external_chat_id: 'telegram:123', alias: m.aliasOf123, peer_actor_id: 'actor' } },
        { chat: { platform: 'telegram', external_chat_id: 'telegram:456', alias: '  ', peer_actor_id: 'actor' } },
    ] }),
    listBotGroups: async () => {
        if (m.groupsFail) throw new Error('boom');
        return { groups: [
            { platform: 'telegram', external_group_id: 'telegram:789', alias: 'Team', name: 'Platform name' },
            { platform: 'telegram', external_group_id: 'telegram:987', name: 'Platform name' },
        ] };
    },
}));

const bot = { uuid: 'bot', name: 'Bot', platform: 'telegram', online: true };

describe('Projects IM host', () => {
    beforeEach(() => { m.groupsFail = false; });

    it('titles chats as 平台 · alias · 群 · id, with the group name as default alias and the id when unnamed', async () => {
        const chats = await imHost.listChats(bot);
        expect(chats.map((chat) => chat.title)).toEqual([
            'Telegram · Alice · 123',
            'Telegram · 456',
            'Telegram · Team · Group · 789',
            'Telegram · Platform name · Group · 987',
        ]);
        expect(chats.map((chat) => chat.alias)).toEqual(['Alice', undefined, 'Team', 'Platform name']);
    });

    it('answers chat lookups for bindings and message sources after a listing', async () => {
        await imHost.listChats(bot);
        expect(imHost.chatInfo?.('telegram', 'telegram:789')).toEqual({ alias: 'Team', displayId: '789', group: true });
        expect(imHost.chatInfo?.('telegram', 'telegram:456')).toEqual({ alias: undefined, displayId: '456', group: false });
    });

    it('calls a chat deleted only after two complete listings both lack it', async () => {
        await refreshChatIndex();
        // First miss: a chat that appeared since the listing must not be called deleted.
        expect(imHost.chatInfo?.('telegram', 'telegram:gone')).toBeUndefined();
        await vi.waitFor(() => expect(imHost.chatInfo?.('telegram', 'telegram:gone')).toEqual({ displayId: 'gone', group: false, deleted: true }));
        expect(imHost.chatInfo?.('telegram', 'telegram:123', 'bot')?.deleted).toBeUndefined();
        expect(imHost.chatInfo?.('telegram', 'telegram:123', 'deleted-bot')?.deleted).toBe(true);
    });

    it('shows an unnamed bot by its short id, not just the platform', async () => {
        m.bots = [{ uuid: '1730600044998877665-imbot', name: '', platform: 'telegram', enabled: true }];
        const bots = await imHost.listBots();
        expect(bots[0].name).toBe('…877665');
        m.bots = [{ uuid: 'bot', name: 'Bot', platform: 'telegram', enabled: true }];
    });

    it('a listing that no longer has an alias replaces the old one', async () => {
        await imHost.listChats(bot);
        expect(imHost.chatInfo?.('telegram', 'telegram:123')?.alias).toBe('Alice');
        m.aliasOf123 = '';
        await imHost.listChats(bot);
        expect(imHost.chatInfo?.('telegram', 'telegram:123')?.alias).toBeUndefined();
        m.aliasOf123 = 'Alice';
    });

    it('does not guess "deleted" when a listing was incomplete', async () => {
        m.groupsFail = true;
        await refreshChatIndex();
        expect(imHost.chatInfo?.('telegram', 'telegram:gone')).toBeUndefined();
    });
});
