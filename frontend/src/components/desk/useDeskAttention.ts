import type {SessionInfo} from '@/services/deskApi';
import {useEffect, useRef, useState} from 'react';
import {isBusyStatus, sessionTitle} from './deskUtils';

export type AttentionEvent = {id: string; kind: 'needs-input' | 'finished' | 'failed'};

// attentionEvents compares two polls of the session list and reports what
// changed that the user should hear about: a turn that started waiting on
// them, or one that ended. Sessions absent from the previous poll are new
// to this page, not changes, so they report nothing.
export const attentionEvents = (prev: Map<string, SessionInfo>, next: SessionInfo[]): AttentionEvent[] => {
    const out: AttentionEvent[] = [];
    for (const s of next) {
        const before = prev.get(s.id);
        if (!before) continue;
        if (s.awaiting_input && !before.awaiting_input) {
            out.push({id: s.id, kind: 'needs-input'});
        } else if (isBusyStatus(before.status) && !isBusyStatus(s.status) && s.status !== 'closed') {
            out.push({id: s.id, kind: s.status === 'failed' ? 'failed' : 'finished'});
        }
    }
    return out;
};

const canNotify = () => typeof Notification !== 'undefined';

// requestNotifications asks once, from a user gesture (browsers ignore the
// prompt otherwise).
export const requestNotifications = () => {
    if (canNotify() && Notification.permission === 'default') {
        void Notification.requestPermission().catch(() => {});
    }
};

// useDeskAttention tells the user about sessions they aren't looking at:
// a browser notification while the page is in the background, a count in
// the tab title, and the set of sessions that finished unseen (the sidebar
// marks them until opened). Scoped to the Desk page (ux-principles #12):
// the title is restored when the page unmounts.
export const useDeskAttention = (sessions: SessionInfo[], selectedId: string | null, open: (id: string) => void) => {
    const prev = useRef<Map<string, SessionInfo> | null>(null);
    const [unseen, setUnseen] = useState<Set<string>>(new Set());
    const [visible, setVisible] = useState(() => typeof document === 'undefined' || document.visibilityState === 'visible');
    const openRef = useRef(open);
    useEffect(() => {
        openRef.current = open;
    }, [open]);

    useEffect(() => {
        const onChange = () => setVisible(document.visibilityState === 'visible');
        document.addEventListener('visibilitychange', onChange);
        return () => document.removeEventListener('visibilitychange', onChange);
    }, []);

    useEffect(() => {
        const before = prev.current;
        prev.current = new Map(sessions.map((s) => [s.id, s]));
        if (!before) return;
        const watching = (id: string) => visible && id === selectedId;
        const events = attentionEvents(before, sessions).filter((e) => !watching(e.id));
        if (events.length === 0) return;

        const finished = events.filter((e) => e.kind !== 'needs-input').map((e) => e.id);
        if (finished.length > 0) setUnseen((u) => new Set([...u, ...finished]));

        if (visible || !canNotify() || Notification.permission !== 'granted') return;
        for (const e of events) {
            const s = sessions.find((x) => x.id === e.id);
            if (!s) continue;
            const title = e.kind === 'needs-input' ? 'Desk: waiting for you' : e.kind === 'failed' ? 'Desk: turn failed' : 'Desk: turn finished';
            const n = new Notification(title, {body: sessionTitle(s), tag: `desk-${s.id}`});
            n.onclick = () => {
                window.focus();
                openRef.current(s.id);
                n.close();
            };
        }
    }, [sessions, selectedId, visible]);

    // Opening a session is seeing it (adjusted during render rather than in
    // an effect, so the mark never paints on the open session).
    if (selectedId && visible && unseen.has(selectedId)) {
        const next = new Set(unseen);
        next.delete(selectedId);
        setUnseen(next);
    }

    const count = sessions.filter((s) => s.awaiting_input).length + unseen.size;
    useEffect(() => {
        const base = document.title.replace(/^\(\d+\) /, '');
        document.title = count > 0 ? `(${count}) ${base}` : base;
        return () => {
            document.title = document.title.replace(/^\(\d+\) /, '');
        };
    }, [count]);

    return unseen;
};
