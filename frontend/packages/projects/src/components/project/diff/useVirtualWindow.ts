import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';

/** Windowing for a scroller of equal-height rows: only the rows in view (plus
 * a margin) are rendered, however long the list is. The caller renders
 * `start..end` inside a box `totalHeight` tall, offset by `start * rowHeight`. */
export function useVirtualWindow(count: number, rowHeight: number, overscan = 10) {
    const ref = useRef<HTMLDivElement>(null);
    const [firstRow, setFirstRow] = useState(0);
    const [viewport, setViewport] = useState(600);

    const measure = useCallback(() => {
        const el = ref.current;
        if (!el) return;
        setFirstRow(Math.floor(el.scrollTop / rowHeight));
        if (el.clientHeight > 0) setViewport(el.clientHeight);
    }, [rowHeight]);

    useLayoutEffect(measure, [measure, count]);
    useEffect(() => {
        const el = ref.current;
        if (!el || typeof ResizeObserver === 'undefined') return;
        const ro = new ResizeObserver(measure);
        ro.observe(el);
        return () => ro.disconnect();
    }, [measure]);

    /** Scrolls the minimum that brings row `i` fully into view. */
    const reveal = useCallback((i: number) => {
        const el = ref.current;
        if (!el) return;
        const top = i * rowHeight;
        if (top < el.scrollTop) el.scrollTop = top;
        else if (top + rowHeight > el.scrollTop + el.clientHeight) el.scrollTop = top + rowHeight - el.clientHeight;
    }, [rowHeight]);

    const start = Math.max(0, Math.min(firstRow, Math.max(0, count - 1)) - overscan);
    const end = Math.min(count, firstRow + Math.ceil(viewport / rowHeight) + overscan);
    return { ref, onScroll: measure, start, end, totalHeight: count * rowHeight, reveal };
}
