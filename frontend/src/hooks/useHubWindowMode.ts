import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';
import { Events } from '@/bindings';

/**
 * Tells the Go side (gui/wails3/run.go's useWebSystray) when the tray window
 * enters or leaves the compact hub view, so it can resize/maximise
 * accordingly. Emits are no-ops outside the Wails runtime (web/dev mode) —
 * the mock Events.Emit in bindings-web already handles that, this hook just
 * needs to avoid firing on the very first render (the window already starts
 * at the right size for whichever route it was opened with).
 */
export function useHubWindowMode() {
    const location = useLocation();
    const prevIsHubRef = useRef<boolean | null>(null);

    useEffect(() => {
        const isHub = location.pathname === '/hub';
        const prev = prevIsHubRef.current;
        prevIsHubRef.current = isHub;

        if (prev === null || prev === isHub) return;

        Events.Emit(isHub ? 'hub-entered' : 'hub-left');
    }, [location.pathname]);
}
