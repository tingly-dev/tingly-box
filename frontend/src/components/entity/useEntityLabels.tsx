import { useMemo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { bareId, entityText, type EntityKind, type EntityLabelProps } from '@tingly/ui';
import { PLATFORM_BRAND_ICONS, platformDisplayName } from '@/constants/platformGuides';
import type { NotifyTarget } from '@/types/bot';

// Adapters from the bot / IM API shapes to the segments of the shared display
// convention `平台 · alias · id` (group chats: `平台群 · alias · id`) (.design/bot-im-alias-display.md §3).

export interface BotRef { uuid?: string; name?: string; platform?: string }
export interface ChatRef { platform: string; external_chat_id: string; alias?: string }
export interface GroupRef { platform: string; external_group_id: string; alias?: string; name?: string }

export type EntityProps = Pick<
    EntityLabelProps,
    'platform' | 'group' | 'alias' | 'id' | 'kind' | 'copyValue' | 'groupLabel' | 'copyLabel' | 'copiedLabel' | 'deletedLabel' | 'unnamedHint' | 'platformIcon'
>;

export interface EntityLabels {
    bot: (bot: BotRef) => EntityProps;
    chat: (chat: ChatRef) => EntityProps;
    group: (group: GroupRef) => EntityProps;
    /** A Notify target (direct chat or group). */
    target: (target: NotifyTarget) => EntityProps;
    /** Plain text for toasts, confirms and aria-labels: `text(entities.chat(c))`. */
    text: (props: EntityProps, opts?: { shorten?: boolean }) => string;
    /** Shorthand for the most common case. */
    botText: (bot: BotRef) => string;
}

export function useEntityLabels(): EntityLabels {
    const { t } = useTranslation();
    return useMemo<EntityLabels>(() => {
        const groupLabel = t('entity.group', { defaultValue: '{{platform}} group', platform: '{{platform}}', interpolation: { escapeValue: false } });
        const common = {
            groupLabel,
            copyLabel: t('entity.copyId', { defaultValue: 'Copy ID' }),
            copiedLabel: t('entity.copied', { defaultValue: 'Copied!' }),
            deletedLabel: t('entity.deleted', { defaultValue: '(deleted)' }),
            unnamedHint: t('entity.noAlias', { defaultValue: 'No alias set' }),
        };
        const platformName = (p?: string) => (p ? platformDisplayName(p, t) : '');
        const icon = (platform: string, size = 16): ReactNode => {
            const Brand = PLATFORM_BRAND_ICONS[platform];
            return Brand ? <Brand size={size} /> : null;
        };
        const ref = (platform: string, externalId: string, opts?: { alias?: string; group?: boolean }): EntityProps => ({
            ...common,
            platform: platformName(platform),
            group: opts?.group,
            alias: opts?.alias,
            id: bareId(platform, externalId),
            copyValue: externalId,
            kind: opts?.group ? 'group' : 'direct',
            platformIcon: icon(platform),
        });
        const text: EntityLabels['text'] = (props, opts) =>
            entityText(
                { platform: props.platform, group: props.group, alias: props.alias, id: props.id },
                { groupLabel, shorten: opts?.shorten ?? true, kind: props.kind as EntityKind | undefined },
            );
        const bot: EntityLabels['bot'] = (b) => ({
            ...common,
            platform: platformName(b.platform),
            alias: b.name,
            id: b.uuid ?? '',
            kind: 'bot',
            platformIcon: icon(b.platform ?? ''),
        });
        const chat: EntityLabels['chat'] = (c) => ref(c.platform, c.external_chat_id, { alias: c.alias });
        const group: EntityLabels['group'] = (g) => ref(g.platform, g.external_group_id, { alias: g.alias || g.name, group: true });
        // A Notify target carries the operator's alias and, for groups, the platform's own name.
        const target: EntityLabels['target'] = (x) => (x.kind === 'group'
            ? group({ platform: x.platform ?? '', external_group_id: x.external_id, alias: x.alias, name: x.name })
            : chat({ platform: x.platform ?? '', external_chat_id: x.external_id, alias: x.alias }));
        return { bot, chat, group, target, text, botText: (b) => text(bot(b)) };
    }, [t]);
}
