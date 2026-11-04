// Display convention for platform-scoped bot / IM entities:
//   平台 · alias · 群 · id
// Any empty or inapplicable segment is dropped together with its separator.
// See .design/bot-im-alias-display.md §3.

export type EntityKind = 'bot' | 'direct' | 'group' | 'actor';

export interface EntityParts {
    /** Platform display name ("Telegram"), never the raw platform id. */
    platform: string;
    /** True for group chats; renders the `群` / `Group` segment. */
    group?: boolean;
    /** The user-facing alias; omitted when empty. */
    alias?: string;
    /** The id the user can use next: external chat id, bot uuid, ... Always full here. */
    id: string;
}

export interface ShortId {
    text: string;
    shortened: boolean;
}

const MAX_EXTERNAL_ID = 20;
const BOT_SUFFIX = '-imbot';
const BOT_TAIL = 6;

/**
 * Width-driven display shortening of an id. The full id is always kept by the
 * caller (copy button / tooltip); this only decides what is drawn inline.
 *
 * Bot ids are `<unixnano>-imbot`: the head digits are nearly identical for bots
 * created close together and the suffix is constant, so only the last digits
 * of the numeric part tell bots apart. External ids keep head and tail.
 */
export function shortId(kind: EntityKind, id: string): ShortId {
    if (kind === 'bot') {
        if (id.endsWith(BOT_SUFFIX)) {
            const core = id.slice(0, -BOT_SUFFIX.length);
            if (core.length > BOT_TAIL + 2) return { text: `…${core.slice(-BOT_TAIL)}`, shortened: true };
        }
        if (id.length <= MAX_EXTERNAL_ID) return { text: id, shortened: false };
    }
    if (id.length <= MAX_EXTERNAL_ID) return { text: id, shortened: false };
    return { text: `${id.slice(0, 6)}…${id.slice(-4)}`, shortened: true };
}

/** External ids may carry a `platform:` prefix; the platform is its own segment, so display the bare id. */
export function bareId(platform: string, id: string): string {
    return platform && id.startsWith(`${platform}:`) ? id.slice(platform.length + 1) : id;
}

export interface EntityTextOptions {
    /** Localised word for the group segment; defaults to "Group". */
    groupLabel?: string;
    /** Shorten the id as the UI does. Logs, IM messages and CLI output keep the full id. */
    shorten?: boolean;
    kind?: EntityKind;
}

/** The plain-text form (toasts, confirms, aria-labels): the same order as the component. */
export function entityText(parts: EntityParts, options: EntityTextOptions = {}): string {
    const { groupLabel = 'Group', shorten = true } = options;
    const kind: EntityKind = options.kind ?? (parts.group ? 'group' : 'direct');
    const id = shorten ? shortId(kind, parts.id).text : parts.id;
    const segments = [
        parts.platform,
        parts.alias?.trim() ?? '',
        parts.group ? groupLabel : '',
        id,
    ];
    return segments.filter((s) => s !== '').join(' · ');
}
