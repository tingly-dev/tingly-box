import {api} from '@/services/api';
import type {BotGroupDetail, DirectChatDetail} from '@/types/bot';
import {chatCanControl, chatPermissionAllowed, groupCanControl} from '@/types/bot';
import {useCallback, useEffect, useState} from 'react';

export interface RemoteAccess {
    loading: boolean;
    error: string;
    /** Every Direct Chat and Group the bot has seen — the Remote access dialog renders these. */
    chats: DirectChatDetail[];
    groups: BotGroupDetail[];
    /** Direct chats that can drive the bot now (see chatCanControl). */
    controllers: DirectChatDetail[];
    /** Groups where someone can drive the bot now (see groupCanControl). */
    controllingGroups: BotGroupDetail[];
    /** Unblocked direct chats that reached the bot but have no Remote Control access. */
    pendingChats: DirectChatDetail[];
    reload: () => Promise<void>;
}

// useRemoteAccess answers "who can control this bot right now?" for the
// Remote page, and is also the data source of the Remote-scoped access
// dialog, so the card and the dialog read one copy. Pass enabled=false to
// skip fetching (an off card shows none of this until it is expanded).
export function useRemoteAccess(botUuid: string | undefined, enabled = true): RemoteAccess {
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

    useEffect(() => { if (enabled) void reload(); }, [enabled, reload]);

    return {
        loading,
        error,
        chats,
        groups,
        controllers: chats.filter(chatCanControl),
        controllingGroups: groups.filter(groupCanControl),
        pendingChats: chats.filter((detail) =>
            !detail.chat.blocked && !chatPermissionAllowed(detail, 'remote_control', 'access')),
        reload,
    };
}
