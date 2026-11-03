import { useMemo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { bareId, entityText, type EntityKind, type EntityLabelProps } from '@tingly/ui';
import { PLATFORM_BRAND_ICONS, platformDisplayName } from '@/constants/platformGuides';

// Adapters from the bot / IM API shapes to the segments of the shared display
// convention `平台 · 群 · alias · id` (.design/bot-im-alias-display.md §3).

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
    /** A chat/group reference that may no longer exist (binding, route target): same segments, flagged by the caller. */
    ref: (platform: string, externalId: string, opts?: { alias?: string; group?: boolean }) => EntityProps;
    /** Plain text for toasts, confirms and aria-labels. */
    text: (props: EntityProps, opts?: { shorten?: boolean }) => string;
    botText: (bot: BotRef) => string;
    chatText: (chat: ChatRef) => string;
    groupText: (group: GroupRef) => string;
    icon: (platform: string, size?: number) => ReactNode;
}

export function useEntityLabels(): EntityLabels {
    const { t } = useTranslation();
    return useMemo<EntityLabels>(() => {
        const groupLabel = t('entity.group', { defaultValue: 'Group' });
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
        const ref: EntityLabels['ref'] = (platform, externalId, opts) => ({
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
            id: b.uuid ?? "",
            kind: 'bot',
            platformIcon: icon(b.platform ?? ''),
        });
        const chat: EntityLabels['chat'] = (c) => ref(c.platform, c.external_chat_id, { alias: c.alias });
        const group: EntityLabels['group'] = (g) => ref(g.platform, g.external_group_id, { alias: g.alias || g.name, group: true });
        return {
            bot, chat, group, ref, text, icon,
            botText: (b) => text(bot(b)),
            chatText: (c) => text(chat(c)),
            groupText: (g) => text(group(g)),
        };
    }, [t]);
}
