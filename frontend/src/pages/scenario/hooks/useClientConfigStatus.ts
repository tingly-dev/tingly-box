import { useCallback, useEffect, useRef, useState } from 'react';
import type { components } from '@/client';
import { controlApi } from '@/services/openapi';

export type ClientConfigStatus = components['schemas']['ClientConfigStatusResponse'];

// Keep the status honest without a push channel: re-read it whenever the
// page changes something that feeds it (the caller's `deps`), when the
// window regains focus (the user may have edited the file or run Auto
// Config elsewhere), and on a slow interval while the page is open.
const REFRESH_MS = 15_000;

// Tools whose config file the gateway can read back and compare
// (GET /api/v1/config/<tool>/status).
export type ClientConfigTool = 'claude' | 'codex' | 'dsh' | 'workbuddy';

const STATUS_PATHS = {
    claude: '/api/v1/config/claude/status',
    codex: '/api/v1/config/codex/status',
    dsh: '/api/v1/config/dsh/status',
    workbuddy: '/api/v1/config/workbuddy/status',
} as const;

/** `tool` null = this page has no readable config; nothing is fetched. */
export function useClientConfigStatus(tool: ClientConfigTool | null, deps: unknown[]) {
    const [state, setState] = useState<{ tool: ClientConfigTool; status: ClientConfigStatus | null } | null>(null);
    // Reads overlap (an edit, a focus and the timer can all fire at once) and
    // can come back out of order; only the newest one may set the status, or an
    // old "outdated" read lands after the fresh "applied" one.
    const latest = useRef(0);

    const refresh = useCallback(async () => {
        if (!tool) return;
        const seq = ++latest.current;
        const res = await controlApi((client, headers) => client.GET(STATUS_PATHS[tool], { headers }))
            .catch(() => undefined);
        if (seq !== latest.current) return;
        // An older server without the endpoint (or an error) shows nothing
        // rather than a guess.
        setState({ tool, status: res?.success ? res : null });
    }, [tool]);

    useEffect(() => {
        void refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);

    useEffect(() => {
        if (!tool) return;
        const onFocus = () => void refresh();
        window.addEventListener('focus', onFocus);
        const timer = window.setInterval(() => {
            if (document.visibilityState === 'visible') void refresh();
        }, REFRESH_MS);
        return () => {
            window.removeEventListener('focus', onFocus);
            window.clearInterval(timer);
        };
    }, [refresh, tool]);

    // Another tool's status, still held while this tool's first read is in
    // flight, is not this tool's status.
    const status = state && state.tool === tool ? state.status : null;
    return { status, refresh };
}
