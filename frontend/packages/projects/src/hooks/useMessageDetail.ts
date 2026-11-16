import { useCallback, useEffect, useState } from 'react';
import type { MessageDetail } from '../transcript/messageDetail';
import { readLocal, writeLocal } from '../utils/storage';

export type DetailSurface = 'conversation' | 'thread';
const CHANGED = 'tingly-project.message-detail';
const keyOf = (surface: DetailSurface) => `${surface}.messageDetail`;

export function readMessageDetail(surface: DetailSurface): MessageDetail {
    const saved = readLocal(keyOf(surface));
    if (saved === 'detailed' || saved === 'concise') return saved;
    // The old report switch was the conversation's only visibility preference.
    // An invalid new value falls back safely, rather than reviving old settings.
    if (saved === null && surface === 'conversation' && readLocal('conversation.threadReports') === '0') return 'concise';
    return 'detailed';
}

/** Browser-wide per surface, like the report switch it replaces. Separate
 * surfaces prevent a manager preference from hiding calls in an open thread. */
export function useMessageDetail(surface: DetailSurface) {
    const [detail, setDetailState] = useState(() => readMessageDetail(surface));
    useEffect(() => {
        const changed = (event: Event) => {
            const update = (event as CustomEvent<{ surface: DetailSurface; detail: MessageDetail }>).detail;
            if (update.surface === surface) setDetailState(update.detail);
        };
        const stored = () => setDetailState(readMessageDetail(surface));
        window.addEventListener(CHANGED, changed);
        window.addEventListener('storage', stored);
        return () => {
            window.removeEventListener(CHANGED, changed);
            window.removeEventListener('storage', stored);
        };
    }, [surface]);
    const setDetail = useCallback((next: MessageDetail) => {
        writeLocal(keyOf(surface), next);
        setDetailState(next);
        window.dispatchEvent(new CustomEvent(CHANGED, { detail: { surface, detail: next } }));
    }, [surface]);
    return [detail, setDetail] as const;
}
