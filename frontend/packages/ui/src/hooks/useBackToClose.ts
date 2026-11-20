import { useEffect, useRef } from 'react';

let nextId = 0;

/**
 * While `open`, the sheet owns a history entry, so the system Back closes it
 * instead of leaving the page (`.design/mobile-ui.md`, Style). Closing it any
 * other way gives the entry back. Off when `enabled` is false (desktop).
 */
export function useBackToClose(open: boolean, onClose: () => void, enabled = true) {
    const onCloseRef = useRef(onClose);
    useEffect(() => { onCloseRef.current = onClose; });

    useEffect(() => {
        if (!open || !enabled) return;
        const id = ++nextId;
        window.history.pushState({ ...window.history.state, phoneSheet: id }, '');
        let popped = false;
        const onPop = () => {
            popped = true;
            onCloseRef.current();
        };
        window.addEventListener('popstate', onPop);
        return () => {
            window.removeEventListener('popstate', onPop);
            if (!popped && window.history.state?.phoneSheet === id) window.history.back();
        };
    }, [open, enabled]);
}
