import {useCallback, useEffect, useRef, useState} from 'react';
import {api} from '@/services/api';

// Phases of a one-click update (POST /info/version/update):
//   installing      — npm is installing the new version
//   restarting      — the server is restarting itself; polling for the new version
//   restartRequired — installed, but a service manager must restart the server
//   timedOut        — installed, but the new version did not answer in time
//   error           — the update was refused or failed
export type SelfUpdateState =
    | {phase: 'idle'}
    | {phase: 'installing'}
    | {phase: 'restarting'; version: string}
    | {phase: 'restartRequired'; version: string}
    | {phase: 'timedOut'; version: string}
    | {phase: 'error'; message: string};

interface Options {
    pollIntervalMs?: number;
    timeoutMs?: number;
    reload?: () => void;
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// Release builds report "v0.261001.1+abc" while npm versions are "0.261001.1".
const normalizeVersion = (version: string) => version.replace(/^v/, '').split('+')[0];

export function useSelfUpdate({
    pollIntervalMs = 2000,
    timeoutMs = 120_000,
    reload = () => window.location.reload(),
}: Options = {}) {
    const [state, setState] = useState<SelfUpdateState>({phase: 'idle'});
    const mounted = useRef(true);
    useEffect(() => () => {
        mounted.current = false;
    }, []);
    const update = useCallback((next: SelfUpdateState) => {
        if (mounted.current) setState(next);
    }, []);

    const start = useCallback(async () => {
        update({phase: 'installing'});
        const result = await api.applyUpdate();
        if (!result?.success || !result.data) {
            update({phase: 'error', message: result?.error || 'Update failed'});
            return;
        }

        const {version, restarting} = result.data;
        if (!restarting) {
            update({phase: 'restartRequired', version});
            return;
        }

        // The page reloads once the restarted server reports the new
        // version — even if the dialog was closed meanwhile.
        update({phase: 'restarting', version});
        const deadline = Date.now() + timeoutMs;
        while (Date.now() < deadline) {
            await sleep(pollIntervalMs);
            if (normalizeVersion(await api.getVersion()) === normalizeVersion(version)) {
                reload();
                return;
            }
        }
        update({phase: 'timedOut', version});
    }, [update, pollIntervalMs, timeoutMs, reload]);

    return {state, start};
}
