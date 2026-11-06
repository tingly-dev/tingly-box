import {platformDisplayName} from '@/constants/platformGuides';
import {api} from '@/services/api';
import type {BotGroupDetail, DirectChatDetail, NotifyTarget} from '@/types/bot';
import type {TFunction} from 'i18next';

// loadNotifyTargets lists every chat and group a bot has seen as one flat list
// of NotifyTargets — the shape both the Notify work surface and the API guide
// pick a destination from.
export async function loadNotifyTargets(botUUID: string): Promise<NotifyTarget[]> {
    const [chatData, groupData] = await Promise.all([
        api.listBotDirectChats(botUUID),
        api.listBotGroups(botUUID),
    ]);
    const groupDetails: BotGroupDetail[] = await Promise.all(
        (groupData.groups || []).map((group: {id: string}) => api.getBotGroup(botUUID, group.id)),
    );
    const directTargets: NotifyTarget[] = (chatData.chats || []).map((detail: DirectChatDetail) => ({
        id: detail.chat.id,
        kind: 'direct_chat',
        external_id: detail.chat.external_chat_id,
        alias: detail.chat.alias,
        platform: detail.chat.platform,
        is_paired: Boolean(detail.chat.peer_actor_id),
        blocked: detail.chat.blocked,
        can_notify: detail.permissions.some((permission) => permission.capability === 'notify' && permission.action === 'notify.receive' && permission.effect === 'allow'),
        can_reply: detail.permissions.some((permission) => permission.capability === 'notify' && permission.action === 'notify.reply' && permission.effect === 'allow'),
    }));
    const groupTargets: NotifyTarget[] = groupDetails.map((detail) => ({
        id: detail.group.id,
        kind: 'group',
        external_id: detail.group.external_group_id,
        alias: detail.group.alias,
        name: detail.group.name,
        platform: detail.group.platform,
        blocked: detail.group.blocked,
        can_notify: detail.capabilities.notify === 'allow',
        // Group replies are actor-scoped. Keep Confirm on Direct Chats
        // until the UI can select and explain the replying Actor.
        can_reply: false,
    }));
    return [...directTargets, ...groupTargets];
}

export interface NotifyTargetParts {
    platform: string;
    /** Alias, falling back to the platform-provided name; empty when neither is set. */
    name: string;
    kind: string;
    id: string;
}

export function notifyTargetParts(target: NotifyTarget, t: TFunction): NotifyTargetParts {
    return {
        platform: target.platform ? platformDisplayName(target.platform, t) : '',
        name: target.alias || target.name || '',
        kind: target.kind === 'group'
            ? t('notify.target.group', {defaultValue: 'Group'})
            : t('notify.target.direct', {defaultValue: 'Direct'}),
        id: target.external_id,
    };
}

// "Telegram · Release Crew · Group · telegram:-1002468" — the same
// platform · alias · kind · id order everywhere a target is named.
export function notifyTargetLabel(target: NotifyTarget, t: TFunction): string {
    const {platform, name, kind, id} = notifyTargetParts(target, t);
    return [platform, name, kind, id].filter(Boolean).join(' · ');
}
