/**
 * Which kind of IM entity an id names, and whether this bot can use it as a
 * message receiver — the classification behind the bind selector.
 *
 * Candidates come from Remote's access lists (`/bots/:bot/chats` and
 * `/bots/:bot/groups`), merged into `ImChat` by `imHost`. Both lists are built
 * from inbound `Recipient.ID`s, but the id kind is not stored anywhere, so it
 * has to be read from the platform's own conventions. The table below is the
 * explicit mapping; `.research/im-id-types.md` records the evidence per entry
 * (adapter `file:line`, SDK `receive_id_type` list, platform send path).
 *
 * Three outcomes:
 *   - sendable   — a real receiver; offer it in the selector.
 *   - unsendable — known not to be a receiver (a department, an app, …); hide
 *                  it from the selector, keep it in the raw overview.
 *   - unknown    — cannot be classified reliably; show it but keep it
 *                  disabled, with the reason, instead of hiding it silently.
 */
import { bareId } from '@tingly/ui';

export type ImRecordKind = 'direct' | 'group';

export type ImIdKind =
    | 'chat' // a conversation / channel id
    | 'open_id' // Feishu user, app-scoped
    | 'union_id' // Feishu user, developer-scoped
    | 'user_id' // a person id that a platform still accepts as a receiver (Feishu, WeCom, Weixin)
    | 'phone' // WhatsApp wa_id
    | 'email' // Feishu email receiver
    | 'group' // a group id this bot cannot send to
    | 'department' // Feishu department
    | 'app' // Feishu app id
    | 'unknown';

export type ImIdClass = 'sendable' | 'unsendable' | 'unknown';

export interface ImIdVerdict {
    kind: ImIdKind;
    class: ImIdClass;
    /** i18n key for the type label shown in the raw overview. */
    typeKey: string;
    /** Why it is not a safe bind target; absent for sendable. */
    reasonKey?: string;
    /** The platform parameter that addresses this id, where the platform has one. */
    receiveIdType?: string;
}

const TYPE_KEY: Record<ImIdKind, string> = {
    chat: 'im.idType.chat',
    open_id: 'im.idType.openId',
    union_id: 'im.idType.unionId',
    user_id: 'im.idType.userId',
    phone: 'im.idType.phone',
    email: 'im.idType.email',
    group: 'im.idType.group',
    department: 'im.idType.department',
    app: 'im.idType.app',
    unknown: 'im.idType.unknown',
};

const send = (kind: ImIdKind, receiveIdType?: string): ImIdVerdict => ({ kind, class: 'sendable', typeKey: TYPE_KEY[kind], receiveIdType });
const unsendable = (kind: ImIdKind, reasonKey: string): ImIdVerdict => ({ kind, class: 'unsendable', typeKey: TYPE_KEY[kind], reasonKey });
const unknownId = (reasonKey = 'im.idReason.unknown'): ImIdVerdict => ({ kind: 'unknown', class: 'unknown', typeKey: TYPE_KEY.unknown, reasonKey });

const EMPTY = 'im.idReason.empty';

type Rule = (id: string, kind: ImRecordKind) => ImIdVerdict;

// Feishu and Lark: the kind is written into the prefix. The send API's
// `receive_id_type` accepts open_id/union_id/user_id/email/chat_id; an
// `od-` department and a `cli_` app id are not receivers. A prefix-less id is
// the tenant-assigned user_id (the same default the sender uses).
const feishu: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    if (id.startsWith('oc_')) return send('chat', 'chat_id');
    if (id.startsWith('ou_')) return send('open_id', 'open_id');
    if (id.startsWith('on_')) return send('union_id', 'union_id');
    if (id.startsWith('od-')) return unsendable('department', 'im.idReason.department');
    if (id.startsWith('cli_')) return unsendable('app', 'im.idReason.app');
    if (id.includes('@')) return send('email', 'email');
    return send('user_id', 'user_id');
};

// Slack: chat.postMessage addresses a channel (C/G/D). A user id is not a
// conversation the bot can post to directly, but Slack is not fully certain
// here, so it stays visible and disabled rather than hidden.
const slack: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    switch (id[0]) {
        case 'C':
        case 'D':
        case 'G':
            return send('chat');
        case 'U':
        case 'W':
        case 'B':
            return unknownId('im.idReason.userNotChat');
        default:
            return unknownId();
    }
};

// Telegram: a numeric chat id is the receiver; @name is a public channel name.
const telegram: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    if (/^-?\d+$/.test(id) || id.startsWith('@')) return send('chat');
    return unknownId('im.idReason.notConversation');
};

// Discord: channel ids are snowflakes. User and guild ids share the shape, so
// the record kind (it came from a received message) is what makes it a channel.
const discord: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    return /^\d+$/.test(id) ? send('chat') : unknownId('im.idReason.notConversation');
};

// DingTalk: the bot replies into a conversation (`cid…`) through its session
// webhook; a bare staff id is a person, not a conversation.
const dingtalk: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    if (id.startsWith('cid') || id.startsWith('$:')) return send('chat');
    return unknownId('im.idReason.notConversation');
};

// WeCom: groups use a chatid, a direct chat uses the external user id.
const wecom: Rule = (id, kind) => (id ? send(kind === 'group' ? 'chat' : 'user_id') : unknownId(EMPTY));

// Weixin: a person (`wxid_…`) or a chatroom (`…@chatroom`); both are reply targets.
const weixin: Rule = (id, kind) => (id ? send(kind === 'group' || id.endsWith('@chatroom') ? 'chat' : 'user_id') : unknownId(EMPTY));

// WhatsApp: the send path posts a phone_number, so a wa_id/phone is the
// receiver; a group JID (`@g.us`) cannot be addressed by it.
const whatsapp: Rule = (id) => {
    if (!id) return unknownId(EMPTY);
    if (id.endsWith('@g.us')) return unsendable('group', 'im.idReason.whatsappGroup');
    if (/^\d+$/.test(id) || id.endsWith('@s.whatsapp.net')) return send('phone');
    return unknownId('im.idReason.notConversation');
};

const tingly: Rule = (id) => (id ? send('chat') : unknownId(EMPTY));

const RULES: Record<string, Rule> = {
    feishu,
    lark: feishu,
    slack,
    telegram,
    discord,
    dingtalk,
    wecom,
    weixin,
    whatsapp,
    tingly,
};

/** Strip a `platform:` prefix a stored id may carry (the same shape `bareId` hides). */
function stripPrefix(platform: string, raw: string): string {
    const id = raw.trim();
    for (const p of andLarkAlias(platform)) {
        if (p && id.startsWith(`${p}:`)) return id.slice(p.length + 1).trim();
    }
    return bareId(platform, id);
}

function andLarkAlias(platform: string): string[] {
    const p = platform.toLowerCase();
    return p === 'lark' ? ['lark', 'feishu'] : p === 'feishu' ? ['feishu', 'lark'] : [p];
}

/**
 * Classify one chat/group id for the bind selector and the raw overview.
 * `kind` is the access record's kind (which of Remote's two lists it came from).
 */
export function classifyImId(platform: string, kind: ImRecordKind, rawId: string): ImIdVerdict {
    const p = (platform || '').toLowerCase();
    const id = stripPrefix(p, rawId || '');
    const rule = RULES[p];
    if (!rule) return unknownId('im.idReason.unknownPlatform');
    return rule(id, kind);
}

/** The selector offers sendable and unknown rows; known non-receivers are hidden. */
export const isOfferedForBinding = (v: ImIdVerdict): boolean => v.class !== 'unsendable';

/** Unknown rows stay visible but cannot be picked. */
export const isBindable = (v: ImIdVerdict): boolean => v.class === 'sendable';
