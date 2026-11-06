// What Tingly Box tells @tingly/projects about IM (TEP #2143, .design/projects-im.md):
// its bots and the chats each has seen, read from the Remote APIs that already
// exist (imbot-settings, bots/:bot/chats, bots/:bot/groups). Bindings are
// Projects' own and do not pass through here.
import { useState } from 'react';
import type { ImBot, ImChat, ImChatInfo, ImHost } from '@tingly/projects';
import { aliasOrName, bareId, entityText, shortId } from '@tingly/ui';
import BotAccessDialog from '@/components/bot/BotAccessDialog';
import { PLATFORM_BRAND_ICONS, platformDisplayName } from '@/constants/platformGuides';
import i18n from '@/i18n';
import { listBotDirectChats, listBotGroups } from '@/services/botApi';
import { imbotApi } from '@/services/imbotApi';
import type { BotSettings } from '@/types/bot';

// The full settings of the bots last listed, for the access dialog (the
// package only knows an ImBot).
const knownBots = new Map<string, BotSettings>();

// platformDisplayName rebuilds every platform guide per call: look each name up once per language.
const platformNames = new Map<string, string>();
const platformLabel = (platform: string) => {
    const key = `${i18n.language}:${platform}`;
    let name = platformNames.get(key);
    if (name === undefined) {
        name = platformDisplayName(platform, i18n.t.bind(i18n));
        platformNames.set(key, name);
    }
    return name;
};

// What the host knows about each chat it has listed, so a binding or a message
// that only holds a chat id can still be shown as `平台 · alias · id` (group chats: `平台群 · alias · id`).
// Every bot's latest listing is kept; the lookup index is rebuilt from them, so
// a cleared alias or a vanished chat is not kept alive by an older listing.
const chatKeyOf = (platform: string, chatId: string) => `${platform}\u0000${chatId}`;
const listeners = new Set<() => void>();
let infoVersion = 0;
let perBot = new Map<string, ImChat[]>();
const chatIndex = new Map<string, ImChatInfo>();
// Set only by a full refresh that listed every bot and all of its chats: the
// one state in which a missing chat may be called deleted.
let indexComplete = false;
let indexedBots = new Set<string>();
let indexedAt = 0; // when the last successful full refresh finished
let requestedAt = 0; // throttle for the background refresh
let indexing = false;
let refreshAgain = false;
// chats first seen missing from a complete listing, and which refresh saw it.
const firstMissing = new Map<string, number>();
const STALE_MS = 60_000;

const infoOf = (c: ImChat): ImChatInfo => ({ alias: c.alias, displayId: bareId(c.platform, c.chatId), group: c.kind === 'group' });

const publish = () => {
    infoVersion += 1;
    listeners.forEach((fn) => fn());
};

/** Rebuilds the lookup from every bot's latest listing. Two bots can see one chat: the entry with a name wins. */
const rebuild = () => {
    chatIndex.clear();
    for (const chats of perBot.values()) {
        for (const c of chats) {
            const key = chatKeyOf(c.platform, c.chatId);
            const info = infoOf(c);
            if (!chatIndex.has(key) || (info.alias && !chatIndex.get(key)?.alias)) chatIndex.set(key, info);
        }
    }
    chatIndex.forEach((_, key) => firstMissing.delete(key));
};

/** Records one bot's chat listing (replacing what was known about that bot's chats). */
const remember = (botUuid: string, chats: ImChat[]) => {
    perBot.set(botUuid, chats);
    rebuild();
    publish();
};

/** Re-reads every bot's chats and replaces the index (after a rename, or when it has gone stale). */
export async function refreshChatIndex(): Promise<void> {
    if (indexing) {
        refreshAgain = true; // a rename during a refresh must still be read
        return;
    }
    indexing = true;
    try {
        const bots = await listBots();
        const results = await Promise.all(bots.map((b) => fetchChats(b).then((r) => r, () => ({ chats: [] as ImChat[], complete: false }))));
        perBot = new Map(bots.map((b, i) => [b.uuid, results[i].chats]));
        indexedBots = new Set(bots.map((b) => b.uuid));
        indexComplete = results.every((r) => r.complete);
        rebuild();
        indexedAt = Date.now();
        publish();
    } catch {
        // keep the last index; it is no longer known to be complete
        indexComplete = false;
    } finally {
        indexing = false;
        if (refreshAgain) {
            refreshAgain = false;
            void refreshChatIndex();
        }
    }
}

const chatInfo = (platform: string, chatId: string, botUuid?: string): ImChatInfo | undefined => {
    if (!indexing && Date.now() - requestedAt > STALE_MS) {
        requestedAt = Date.now();
        void refreshChatIndex();
    }
    const key = chatKeyOf(platform, chatId);
    const info = chatIndex.get(key);
    const botGone = Boolean(botUuid) && indexComplete && !indexedBots.has(botUuid!);
    if (info) return botGone ? { ...info, deleted: true } : info;
    if (!indexComplete) return undefined;
    // Missing from a complete listing. A chat that appeared since that listing
    // must not be called deleted, so confirm with a later complete listing.
    const first = firstMissing.get(key);
    if (first === undefined) {
        firstMissing.set(key, indexedAt);
        void refreshChatIndex();
        return undefined;
    }
    return indexedAt > first ? { displayId: bareId(platform, chatId), group: false, deleted: true } : undefined;
};

async function listBots(): Promise<ImBot[]> {
    const res = await imbotApi.getImBotSettingsList();
    if (!res?.success || !Array.isArray(res.settings)) throw new Error(res?.error || 'failed to list bots');
    knownBots.clear();
    const bots = (res.settings as BotSettings[]).filter((b) => !!b.uuid).map((b) => {
        knownBots.set(b.uuid!, b);
        // An unnamed bot is shown by its short id (the platform is printed next to the name).
        return { uuid: b.uuid!, name: b.name?.trim() || shortId('bot', b.uuid!).text, platform: b.platform, online: b.enabled !== false };
    });
    // A bot the index has not seen: what it says about deleted bots is out of date.
    if (bots.some((b) => !indexedBots.has(b.uuid))) indexComplete = false;
    return bots;
}

async function fetchChats(bot: ImBot): Promise<{ chats: ImChat[]; complete: boolean }> {
    // Private chats and groups are separate lists; either failing leaves the other.
    const [direct, groups] = await Promise.allSettled([listBotDirectChats(bot.uuid), listBotGroups(bot.uuid)]);
    const chats: ImChat[] = [];
    if (direct.status === 'fulfilled') {
        for (const { chat } of direct.value.chats ?? []) {
            const shown = aliasOrName(chat.alias, chat.name);
            chats.push({
                botUuid: bot.uuid,
                platform: chat.platform,
                chatId: chat.external_chat_id,
                alias: shown || undefined,
                title: entityText({ platform: platformLabel(chat.platform), alias: shown, id: bareId(chat.platform, chat.external_chat_id) }),
                kind: 'direct',
                state: chat.blocked ? 'blocked' : chat.peer_actor_id ? 'ok' : 'unpaired',
            });
        }
    }
    if (groups.status === 'fulfilled') {
        const groupLabel = i18n.t('entity.group', { defaultValue: '{{platform}} group', platform: '{{platform}}', interpolation: { escapeValue: false } });
        for (const g of groups.value.groups ?? []) {
            const alias = aliasOrName(g.alias, g.name) || undefined;
            chats.push({
                botUuid: bot.uuid,
                platform: g.platform,
                chatId: g.external_group_id,
                alias,
                title: entityText({ platform: platformLabel(g.platform), group: true, alias, id: bareId(g.platform, g.external_group_id) }, { groupLabel }),
                kind: 'group',
                state: g.blocked ? 'blocked' : 'ok',
            });
        }
    }
    return { chats, complete: direct.status === 'fulfilled' && groups.status === 'fulfilled' };
}

async function listChats(bot: ImBot): Promise<ImChat[]> {
    const { chats } = await fetchChats(bot);
    remember(bot.uuid, chats);
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
