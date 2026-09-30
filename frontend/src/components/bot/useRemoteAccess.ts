import {api} from '@/services/api';
import type {BotGroupDetail, DirectChatDetail} from '@/types/bot';
import {useCallback, useEffect, useState} from 'react';

const remoteAllowed = (chat: DirectChatDetail, action: string) =>
    chat.permissions.some((permission) =>
        permission.capability === 'remote_control' &&
        permission.action === action &&
        permission.effect === 'allow');

export interface RemoteAccessSummary {
    loading: boolean;
    error: string;
    /** Direct chats that can actually start runs (access + start allowed, not blocked). */
    controllers: DirectChatDetail[];
    /** Groups with Remote Control allowed AND at least one authorized actor. */
    controllingGroups: BotGroupDetail[];
    /** Paired (or observed) direct chats that don't have Remote Control yet. */
    pendingChats: DirectChatDetail[];
    reload: () => Promise<void>;
}

// useRemoteAccess answers "who can control this bot right now?" for the
// Remote page. A group only counts once someone in it is authorized — Remote
// Control allowed on a group with zero actors still lets nobody in, and
// counting it would hide exactly the "nobody can use this" state the page
// has to surface.
export function useRemoteAccess(botUuid?: string): RemoteAccessSummary {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [chats, setChats] = useState<DirectChatDetail[]>([]);
    const [groups, setGroups] = useState<BotGroupDetail[]>([]);

    const reload = useCallback(async () => {
        if (!botUuid) return;
        setLoading(true);
        setError('');
        try {
            const [chatData, groupData] = await Promise.all([
                api.listBotDirectChats(botUuid),
                api.listBotGroups(botUuid),
            ]);
            const details = await Promise.all((groupData.groups || []).map((group) =>
                api.getBotGroup(botUuid, group.id)));
            setChats(chatData.chats || []);
            setGroups(details);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [botUuid]);

    useEffect(() => { void reload(); }, [reload]);

    const open = chats.filter((detail) => !detail.chat.blocked);
    return {
        loading,
        error,
        controllers: open.filter((detail) => remoteAllowed(detail, 'access') && remoteAllowed(detail, 'remote_control.start')),
        controllingGroups: groups.filter((detail) =>
            !detail.group.blocked && detail.capabilities.remote_control === 'allow' && detail.actors.length > 0),
        pendingChats: open.filter((detail) => !remoteAllowed(detail, 'access')),
        reload,
    };
}
