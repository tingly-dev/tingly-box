import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ProjectSummary } from '../../client';
import { useProjectList } from '../../hooks/useProjectList';
import { bindChannel, listChannelBindings, unbindChannel, type ChannelBinding } from '../../client/channelBindings';
import { useProjectsHost, type ImBot, type ImChat } from '../../host/ProjectsHost';

/** Fired on window when a binding was added, moved or removed, so every view of them refreshes. */
export const IM_BINDINGS_CHANGED = 'tingly-projects:im-bindings-changed';

const POLL_MS = 15_000;

/** The key a chat is known by across bindings and the host's chat list. */
export const chatKey = (platform: string, chatId: string) => `${platform}\u0000${chatId}`;

/**
 * Where a chat is bound: a project (it resolves to the project's main thread;
 * not for a folder) or one thread, which is how a folder is bound.
 */
export type BindTarget = { project_id: string } | { thread_id: string };

/** Every IM binding, refreshed on a slow poll and whenever one changes here. `null` until the first answer. */
export function useChannelBindings(enabled: boolean) {
    const [bindings, setBindings] = useState<ChannelBinding[] | null>(null);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (!enabled) return;
        // Only the latest request may land, and none after unmount.
        let latest = 0;
        const load = () => {
            if (document.visibilityState === 'hidden') return;
            const request = ++latest;
            listChannelBindings()
                .then((list) => {
                    if (request !== latest) return;
                    setBindings(list);
                    setError(null);
                })
                .catch((e: Error) => {
                    // Keep the last list; the first failure leaves an empty one and says why.
                    if (request !== latest) return;
                    setBindings((cur) => cur ?? []);
                    setError(e.message);
                });
        };
        load();
        const timer = window.setInterval(load, POLL_MS);
        window.addEventListener(IM_BINDINGS_CHANGED, load);
        document.addEventListener('visibilitychange', load);
        return () => {
            latest = -1;
            window.clearInterval(timer);
            window.removeEventListener(IM_BINDINGS_CHANGED, load);
            document.removeEventListener('visibilitychange', load);
        };
    }, [enabled]);

    const bind = useCallback(async (chat: { platform: string; chatId: string; botUuid: string }, target: BindTarget) => {
        const b = await bindChannel({ platform: chat.platform, chat_id: chat.chatId, bot_uuid: chat.botUuid, ...target });
        window.dispatchEvent(new Event(IM_BINDINGS_CHANGED));
        return b;
    }, []);

    const unbind = useCallback(async (id: string) => {
        await unbindChannel(id);
        window.dispatchEvent(new Event(IM_BINDINGS_CHANGED));
    }, []);

    const byChat = useMemo(() => new Map((bindings ?? []).map((b) => [chatKey(b.platform, b.chat_id), b])), [bindings]);

    return { bindings, byChat, error, bind, unbind };
}

/** The host's bots and the chats each has seen. Empty when the host has no IM. */
export function useImChats(enabled: boolean) {
    const { im } = useProjectsHost();
    const [state, setState] = useState<{ bots: ImBot[]; chats: ImChat[] } | null>(null);
    // Only the latest load may land, and none after unmount.
    const latest = useRef(0);

    const load = useCallback(async () => {
        if (!im) return;
        const request = ++latest.current;
        try {
            const bots = await im.listBots();
            // A bot whose chats cannot be read still shows; only its chats are missing.
            const lists = await Promise.all(bots.map((b) => im.listChats(b).catch(() => [] as ImChat[])));
            if (request === latest.current) setState({ bots, chats: lists.flat() });
        } catch {
            if (request === latest.current) setState((cur) => cur ?? { bots: [], chats: [] });
        }
    }, [im]);

    useEffect(() => {
        if (enabled) void load();
        const ref = latest;
        return () => {
            ref.current = -1;
        };
    }, [enabled, load]);

    return { bots: state?.bots ?? null, chats: state?.chats ?? null, reload: load };
}

/** Every project and folder by id, for saying where a chat is bound and which projects a chat can move to. */
export function useProjectDirectory(enabled: boolean): Map<string, ProjectSummary> {
    const { all } = useProjectList(enabled);
    return useMemo(() => new Map((all ?? []).map((p) => [p.id, p])), [all]);
}
