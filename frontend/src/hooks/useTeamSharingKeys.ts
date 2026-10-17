import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '@/services/api';
import type { SharingKey } from '@/components/SharingKeysTable';

/**
 * The Team Keys of one Team, loaded once for the Team page so the always-on
 * overview and the Team Keys dialog read the same list: a change made in the
 * dialog shows up in the overview as soon as `reload` returns.
 *
 * `loading` is only true until the first list of a Team arrives; a reload
 * keeps the current list on screen instead of flashing a spinner.
 */
export function useTeamSharingKeys(teamId: string | undefined) {
    // Tagged with the Team they belong to, so switching Teams shows the
    // loading state rather than the previous Team's keys.
    const [loaded, setLoaded] = useState<{ teamId: string; keys: SharingKey[] } | null>(null);

    // Guards against a stale response for a previously selected team
    // overwriting the keys of the team the user has since switched to.
    const requestedTeamIdRef = useRef<string | undefined>(undefined);

    const reload = useCallback(async () => {
        if (!teamId) return;
        requestedTeamIdRef.current = teamId;
        const result = await api.listAPITokens({ team_id: teamId });
        if (requestedTeamIdRef.current !== teamId) return;
        setLoaded((prev) => ({
            teamId,
            // On a failed reload keep what is on screen for this Team.
            keys: result.success && result.data
                ? result.data.tokens || []
                : prev?.teamId === teamId ? prev.keys : [],
        }));
    }, [teamId]);

    useEffect(() => { void reload(); }, [reload]);

    const current = loaded?.teamId === teamId ? loaded : null;
    return { keys: current?.keys ?? [], loading: !current, reload };
}
