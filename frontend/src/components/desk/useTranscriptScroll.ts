import {useCallback, useLayoutEffect, useRef, useState} from 'react';

// Only the transcript scrolls. Delayed Markdown layout follows the tail
// while reading live output, but never interrupts reading earlier messages.
export function useTranscriptScroll(revision: string) {
    const viewport = useRef<HTMLDivElement>(null);
    const content = useRef<HTMLDivElement>(null);
    const following = useRef(true);
    const previous = useRef(revision);
    const [away, setAway] = useState(false);
    const [unread, setUnread] = useState(false);
    const follow = useCallback(() => {
        const el = viewport.current;
        if (el && following.current) el.scrollTop = el.scrollHeight;
    }, []);
    useLayoutEffect(() => {
        if (previous.current !== revision && !following.current) setUnread(true);
        previous.current = revision;
        follow();
    }, [revision, follow]);
    useLayoutEffect(() => {
        if (!content.current || typeof ResizeObserver === 'undefined') return;
        const observer = new ResizeObserver(follow);
        observer.observe(content.current);
        return () => observer.disconnect();
    }, [follow]);
    const onScroll = () => {
        const el = viewport.current;
        if (!el) return;
        following.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        setAway(!following.current);
        if (following.current) setUnread(false);
    };
    const latest = () => {
        following.current = true;
        setAway(false);
        setUnread(false);
        follow();
    };
    const reveal = (el: HTMLElement) => {
        const container = viewport.current;
        if (!container) return;
        following.current = false;
        setAway(true);
        container.scrollTo({top: container.scrollTop + el.getBoundingClientRect().top - container.getBoundingClientRect().top - 24, behavior: 'smooth'});
    };
    return {viewport, content, onScroll, latest, reveal, away, unread};
}
