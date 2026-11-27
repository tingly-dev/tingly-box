import { useCallback, useEffect, useState } from 'react';
import { api, unwrap, type Thread } from '../client';

export interface ChatList {
    chats: Thread[];
    /** Whether chats run inside an OS sandbox here (bwrap present). */
    sandboxed: boolean;
}

/**
 * The read-only chats (the hidden Chats project's threads), newest first, and
 * whether they are sandboxed here. Null until the first read lands.
 */
export function useChatList(enabled = true) {
    const [list, setList] = useState<ChatList | null>(null);
    const [error, setError] = useState<string | null>(null);
    const reload = useCallback(async () => {
        if (!enabled) return;
        try {
            const r = await unwrap(api.GET('/api/v1/chats'));
            setList({ chats: r.chats ?? [], sandboxed: !!r.sandboxed });
            setError(null);
        } catch (e) {
            setError((e as Error).message);
        }
    }, [enabled]);
    useEffect(() => {
        void reload();
    }, [reload]);
    return { list, error, reload };
}
