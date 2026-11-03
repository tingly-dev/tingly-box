import { describe, expect, it, vi } from 'vitest';
import { imHost } from './imHost';

vi.mock('@/components/bot/BotAccessDialog', () => ({ default: () => null }));
vi.mock('@/constants/platformGuides', () => ({ PLATFORM_BRAND_ICONS: {} }));
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
    it('uses aliases and falls back to IDs in chat titles', async () => {
        const chats = await imHost.listChats({ uuid: 'bot', name: 'Bot', platform: 'telegram', online: true });
        expect(chats.map((chat) => chat.title)).toEqual(['Telegram · Alice', 'Telegram · 456', 'Telegram · Team', 'Telegram · 987']);
    });
});
