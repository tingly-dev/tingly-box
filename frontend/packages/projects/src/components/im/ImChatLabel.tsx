import { useSyncExternalStore } from 'react';
import { EntityLabel, bareId, entityText } from '@tingly/ui';
import { useTranslation } from 'react-i18next';
import { useProjectsHost } from '../../host/ProjectsHost';
import { PlatformMark, usePlatformLabel } from './PlatformMark';

interface ChatRefProps {
    platform: string;
    /** The platform's chat id as the host stores it (may carry a `platform:` prefix). */
    chatId: string;
    /** Set when the caller knows the chat's kind (a chat list); otherwise the host is asked. */
    kind?: 'direct' | 'group';
    /** A name the caller already has: a chat list's alias, or a binding's saved `chat_title`. */
    alias?: string;
    /** The bot a binding points at, so a deleted bot is noticed too. */
    botUuid?: string;
}

/** Segments of `平台 · alias · id` (group chats: `平台群 · alias · id`) for one chat, resolved through the host when it knows the chat. */
const noSubscribe = () => () => {};
const noVersion = () => 0;

function useChatSegments() {
    const { im } = useProjectsHost();
    // The host learns its chats lazily; re-render when it does.
    useSyncExternalStore(im?.subscribeChatInfo ?? noSubscribe, im?.chatInfoVersion ?? noVersion);
    const { t } = useTranslation('projects');
    const platformLabel = usePlatformLabel();
    const groupLabel = t('im.group', { platform: '{{platform}}', interpolation: { escapeValue: false } });
    return ({ platform, chatId, kind, alias, botUuid }: ChatRefProps) => {
        const info = im?.chatInfo?.(platform, chatId, botUuid);
        const id = info?.displayId ?? bareId(platform, chatId);
        // The host's answer is authoritative while it knows the chat (a cleared
        // alias must not come back from a stale caller copy); the caller's alias
        // is the fallback for a chat the host no longer lists.
        const named = (info && !info.deleted ? info.alias : alias)?.trim() ?? '';
        return {
            deleted: info?.deleted ?? false,
            platform: platformLabel(platform),
            group: kind ? kind === 'group' : info?.group ?? false,
            // A saved title that is just the id adds nothing next to the id itself.
            alias: named === id || named === chatId ? '' : named,
            id,
            groupLabel,
        };
    };
}

/** `平台 · alias · id` (group chats: `平台群 · alias · id`) as plain text, for toasts and aria-labels. */
export function useImChatText(): (chat: ChatRefProps) => string {
    const { t } = useTranslation('projects');
    const segments = useChatSegments();
    return (chat) => {
        const { groupLabel, deleted, ...parts } = segments(chat);
        const text = entityText(parts, { groupLabel });
        return deleted ? `${text} ${t('im.deleted')}` : text;
    };
}

/**
 * One chat in the shared display convention. The platform shows as its logo by
 * default (the surrounding bot / tag already says which platform); pass
 * `platformAs="text"` where nothing else does.
 */
export function ImChatLabel({ platformAs = 'icon', copy = false, compact = false, ...chat }: ChatRefProps & { platformAs?: 'icon' | 'text'; copy?: boolean; compact?: boolean }) {
    const { t } = useTranslation('projects');
    const segments = useChatSegments();
    const { groupLabel, deleted, ...parts } = segments(chat);
    return (
        <EntityLabel
            {...parts}
            resolved={!deleted}
            deletedLabel={t('im.deleted')}
            groupLabel={groupLabel}
            platformIcon={<PlatformMark platform={chat.platform} size={14} colored />}
            platformAs={platformAs}
            copy={copy}
            compact={compact}
            copyValue={chat.chatId}
            copyLabel={t('common.copy')}
            copiedLabel={t('common.copied')}
        />
    );
}
