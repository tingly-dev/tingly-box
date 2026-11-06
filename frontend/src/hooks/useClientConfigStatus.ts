import { useCallback, useEffect, useState } from 'react';
import type { components } from '@/client';
import { controlApi } from '@/services/openapi';

export type ClientConfigStatus = components['schemas']['ClientConfigStatusResponse'];

// Keep the status honest without a push channel: re-read it whenever the
// page changes something that feeds it (the caller's `deps`), when the
// window regains focus (the user may have edited the file or run Auto
// Config elsewhere), and on a slow interval while the page is open.

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
    const [status, setStatus] = useState<ClientConfigStatus | null>(null);

    const refresh = useCallback(async () => {
        if (!tool) return;
        const res = await controlApi((client, headers) => client.GET(STATUS_PATHS[tool], { headers }))
            .catch(() => undefined);
        // An older server without the endpoint (or an error) shows nothing
        // rather than a guess.
        setStatus(res?.success ? res : null);
    }, [tool]);

    useEffect(() => {
        void refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);

    useEffect(() => {
        if (!tool) return;
        const onFocus = () => void refresh();
        window.addEventListener('focus', onFocus);
        return () => window.removeEventListener('focus', onFocus);
    }, [refresh, tool]);

    return { status, refresh };
}
