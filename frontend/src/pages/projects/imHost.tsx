// What Tingly Box tells @tingly/projects about IM (TEP #2143, .design/projects-im.md):
// its bots and the chats each has seen, read from the Remote APIs that already
// exist (imbot-settings, bots/:bot/chats, bots/:bot/groups). Bindings are
// Projects' own and do not pass through here.
import { useState } from 'react';
import type { ImBot, ImChat, ImChatInfo, ImHost } from '@tingly/projects';
import { bareId, entityText } from '@tingly/ui';
import BotAccessDialog from '@/components/bot/BotAccessDialog';
import { PLATFORM_BRAND_ICONS, platformDisplayName } from '@/constants/platformGuides';
import i18n from '@/i18n';
import { listBotDirectChats, listBotGroups } from '@/services/botApi';
import { imbotApi } from '@/services/imbotApi';
import type { BotSettings } from '@/types/bot';

// The full settings of the bots last listed, for the access dialog (the
// package only knows an ImBot).
const knownBots = new Map<string, BotSettings>();

const platformLabel = (platform: string) => platformDisplayName(platform, i18n.t.bind(i18n));

// What the host knows about each chat it has listed, so a binding or a message
// that only holds a chat id can still be shown as `平台 · 群 · alias · id`.
// Filled by every chat listing and, lazily, on the first lookup.
const chatIndex = new Map<string, ImChatInfo>();
const chatKeyOf = (platform: string, chatId: string) => `${platform}\u0000${chatId}`;
const listeners = new Set<() => void>();
let infoVersion = 0;
let indexedAt = 0;
let indexing = false;
const STALE_MS = 60_000;

const infoOf = (c: ImChat): ImChatInfo => ({ alias: c.alias, displayId: bareId(c.platform, c.chatId), group: c.kind === 'group' });

const publish = () => {
    infoVersion += 1;
    listeners.forEach((fn) => fn());
};

// Two bots can see the same chat: keep the entry that carries a name.
const merge = (into: Map<string, ImChatInfo>, chats: ImChat[]) => {
    for (const c of chats) {
        const key = chatKeyOf(c.platform, c.chatId);
        const info = infoOf(c);
        if (!into.has(key) || info.alias || !into.get(key)?.alias) into.set(key, info);
    }
};

/** Records one bot's chat listing (replacing what was known about those chats). */
const remember = (chats: ImChat[]) => {
    merge(chatIndex, chats);
    publish();
};

/** Re-reads every bot's chats and replaces the index (after a rename, or when it has gone stale). */
export async function refreshChatIndex(): Promise<void> {
    if (indexing) return;
    indexing = true;
    try {
        const bots = await listBots();
        const lists = await Promise.all(bots.map((b) => fetchChats(b).catch(() => [] as ImChat[])));
        const next = new Map<string, ImChatInfo>();
        merge(next, lists.flat());
        chatIndex.clear();
        next.forEach((v, k) => chatIndex.set(k, v));
        publish();
    } catch {
        // keep the last index
    } finally {
        indexedAt = Date.now();
        indexing = false;
    }
}

const chatInfo = (platform: string, chatId: string): ImChatInfo | undefined => {
    if (!indexing && Date.now() - indexedAt > STALE_MS) {
        indexedAt = Date.now();
        void refreshChatIndex();
    }
    return chatIndex.get(chatKeyOf(platform, chatId));
};

async function listBots(): Promise<ImBot[]> {
    const res = await imbotApi.getImBotSettingsList();
    if (!res?.success || !Array.isArray(res.settings)) throw new Error(res?.error || 'failed to list bots');
    knownBots.clear();
    return (res.settings as BotSettings[]).filter((b) => !!b.uuid).map((b) => {
        knownBots.set(b.uuid!, b);
        return { uuid: b.uuid!, name: b.name || platformLabel(b.platform), platform: b.platform, online: b.enabled !== false };
    });
}

async function fetchChats(bot: ImBot): Promise<ImChat[]> {
    // Private chats and groups are separate lists; either failing leaves the other.
    const [direct, groups] = await Promise.allSettled([listBotDirectChats(bot.uuid), listBotGroups(bot.uuid)]);
    const chats: ImChat[] = [];
    if (direct.status === 'fulfilled') {
        for (const { chat } of direct.value.chats ?? []) {
            chats.push({
                botUuid: bot.uuid,
                platform: chat.platform,
                chatId: chat.external_chat_id,
                alias: chat.alias?.trim() || undefined,
                title: entityText({ platform: platformLabel(chat.platform), alias: chat.alias, id: bareId(chat.platform, chat.external_chat_id) }),
                kind: 'direct',
                state: chat.blocked ? 'blocked' : chat.peer_actor_id ? 'ok' : 'unpaired',
            });
        }
    }
    if (groups.status === 'fulfilled') {
        for (const g of groups.value.groups ?? []) {
            const alias = g.alias?.trim() || g.name?.trim() || undefined;
            chats.push({
                botUuid: bot.uuid,
                platform: g.platform,
                chatId: g.external_group_id,
                alias,
                title: entityText({ platform: platformLabel(g.platform), group: true, alias, id: bareId(g.platform, g.external_group_id) }, { groupLabel: i18n.t('entity.group', { defaultValue: 'Group' }) }),
                kind: 'group',
                state: g.blocked ? 'blocked' : 'ok',
            });
        }
    }
    return chats;
}

async function listChats(bot: ImBot): Promise<ImChat[]> {
    const chats = await fetchChats(bot);
    remember(chats);
    return chats;
}

function AccessDialog({ bot, onClose }: { bot: ImBot; onClose: () => void }) {
    // Settings may have been listed again since; the dialog wants the full record.
    const [settings] = useState(() => knownBots.get(bot.uuid) ?? null);
    return <BotAccessDialog open bot={settings} scope="remote_control" onClose={onClose} onChanged={() => void refreshChatIndex()} />;
}

export const imHost: ImHost = {
    listBots,
    listChats,
    platformLabel,
    chatInfo,
    subscribeChatInfo: (onChange) => {
        listeners.add(onChange);
        return () => { listeners.delete(onChange); };
    },
    chatInfoVersion: () => infoVersion,
    platformIcon: (platform, size, colored) => {
        const Icon = PLATFORM_BRAND_ICONS[platform];
        return Icon ? <Icon size={size} grayscale={!colored} /> : null;
    },
    accessDialog: ({ bot, onClose }) => <AccessDialog bot={bot} onClose={onClose} />,
};
