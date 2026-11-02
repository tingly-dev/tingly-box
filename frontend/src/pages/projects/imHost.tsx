// What Tingly Box tells @tingly/projects about IM (TEP #2143, .design/projects-im.md):
// its bots and the chats each has seen, read from the Remote APIs that already
// exist (imbot-settings, bots/:bot/chats, bots/:bot/groups). Bindings are
// Projects' own and do not pass through here.
import { useState } from 'react';
import type { ImBot, ImChat, ImHost } from '@tingly/projects';
import BotAccessDialog from '@/components/bot/BotAccessDialog';
import { PLATFORM_BRAND_ICONS } from '@/constants/platformGuides';
import i18n from '@/i18n';
import { listBotDirectChats, listBotGroups } from '@/services/botApi';
import { imbotApi } from '@/services/imbotApi';
import type { BotSettings } from '@/types/bot';

// The full settings of the bots last listed, for the access dialog (the
// package only knows an ImBot).
const knownBots = new Map<string, BotSettings>();

const PLATFORM_NAMES: Record<string, string> = {
    telegram: 'Telegram', feishu: 'Feishu', lark: 'Lark', dingtalk: 'DingTalk', weixin: 'Weixin', wecom: 'WeCom', qq: 'QQ',
};

const platformLabel = (platform: string) =>
    i18n.t(`layout.platforms.${platform}`, { defaultValue: PLATFORM_NAMES[platform] ?? platform });

// External chat ids may include the platform prefix.
const bareId = (platform: string, id: string) => (id.startsWith(`${platform}:`) ? id.slice(platform.length + 1) : id);

async function listBots(): Promise<ImBot[]> {
    const res = await imbotApi.getImBotSettingsList();
    if (!res?.success || !Array.isArray(res.settings)) throw new Error(res?.error || 'failed to list bots');
    knownBots.clear();
    return (res.settings as BotSettings[]).filter((b) => !!b.uuid).map((b) => {
        knownBots.set(b.uuid!, b);
        return { uuid: b.uuid!, name: b.name || platformLabel(b.platform), platform: b.platform, online: b.enabled !== false };
    });
}

async function listChats(bot: ImBot): Promise<ImChat[]> {
    // Private chats and groups are separate lists; either failing leaves the other.
    const [direct, groups] = await Promise.allSettled([listBotDirectChats(bot.uuid), listBotGroups(bot.uuid)]);
    const chats: ImChat[] = [];
    if (direct.status === 'fulfilled') {
        for (const { chat } of direct.value.chats ?? []) {
            chats.push({
                botUuid: bot.uuid,
                platform: chat.platform,
                chatId: chat.external_chat_id,
                title: `${platformLabel(chat.platform)} · ${chat.alias?.trim() || bareId(chat.platform, chat.external_chat_id)}`,
                kind: 'direct',
                state: chat.blocked ? 'blocked' : chat.peer_actor_id ? 'ok' : 'unpaired',
            });
        }
    }
    if (groups.status === 'fulfilled') {
        for (const g of groups.value.groups ?? []) {
            chats.push({
                botUuid: bot.uuid,
                platform: g.platform,
                chatId: g.external_group_id,
                title: `${platformLabel(g.platform)} · ${g.alias?.trim() || bareId(g.platform, g.external_group_id)}`,
                kind: 'group',
                state: g.blocked ? 'blocked' : 'ok',
            });
        }
    }
    return chats;
}

function AccessDialog({ bot, onClose }: { bot: ImBot; onClose: () => void }) {
    // Settings may have been listed again since; the dialog wants the full record.
    const [settings] = useState(() => knownBots.get(bot.uuid) ?? null);
    return <BotAccessDialog open bot={settings} scope="remote_control" onClose={onClose} onChanged={() => {}} />;
}

export const imHost: ImHost = {
    listBots,
    listChats,
    platformLabel,
    platformIcon: (platform, size) => {
        const Icon = PLATFORM_BRAND_ICONS[platform];
        return Icon ? <Icon size={size} /> : null;
    },
    accessDialog: ({ bot, onClose }) => <AccessDialog bot={bot} onClose={onClose} />,
};
