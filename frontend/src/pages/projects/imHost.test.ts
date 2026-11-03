import { describe, expect, it, vi } from 'vitest';
import { imHost } from './imHost';

vi.mock('@/components/bot/BotAccessDialog', () => ({ default: () => null }));
vi.mock('@/constants/platformGuides', () => ({ PLATFORM_BRAND_ICONS: {}, platformDisplayName: (id: string) => id.charAt(0).toUpperCase() + id.slice(1) }));
vi.mock('@/i18n', () => ({ default: { t: (_key: string, options: { defaultValue: string }) => options.defaultValue } }));
vi.mock('@/services/botApi', () => ({
    listBotDirectChats: async () => ({ chats: [
        { chat: { platform: 'telegram', external_chat_id: 'telegram:123', alias: 'Alice', peer_actor_id: 'actor' } },
        { chat: { platform: 'telegram', external_chat_id: 'telegram:456', alias: '  ', peer_actor_id: 'actor' } },
    ] }),
    listBotGroups: async () => ({ groups: [
        { platform: 'telegram', external_group_id: 'telegram:789', alias: 'Team', name: 'Platform name' },
        { platform: 'telegram', external_group_id: 'telegram:987', name: 'Platform name' },
    ] }),
}));

describe('Projects IM host', () => {
    it('titles chats as 平台 · 群 · alias · id, with the group name as default alias and the id when unnamed', async () => {
        const chats = await imHost.listChats({ uuid: 'bot', name: 'Bot', platform: 'telegram', online: true });
        expect(chats.map((chat) => chat.title)).toEqual([
            'Telegram · Alice · 123',
            'Telegram · 456',
            'Telegram · Group · Team · 789',
            'Telegram · Group · Platform name · 987',
        ]);
        expect(chats.map((chat) => chat.alias)).toEqual(['Alice', undefined, 'Team', 'Platform name']);
    });

    it('answers chat lookups for bindings and message sources after a listing', async () => {
        await imHost.listChats({ uuid: 'bot', name: 'Bot', platform: 'telegram', online: true });
        expect(imHost.chatInfo?.('telegram', 'telegram:789')).toEqual({ alias: 'Team', displayId: '789', group: true });
        expect(imHost.chatInfo?.('telegram', 'telegram:456')).toEqual({ alias: undefined, displayId: '456', group: false });
    });
});
