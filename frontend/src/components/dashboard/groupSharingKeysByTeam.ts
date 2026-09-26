import type { UsageIdentity } from '@/hooks/useDashboardData';
import type { Team } from '@/types/team';

export interface SharingKeyGroup {
    /** Owning Team, or null for keys whose Team isn't (yet) known. */
    team: Team | null;
    identities: UsageIdentity[];
}

/**
 * Groups sharing-key identities by Team, in the Team sidebar's order (default
 * Team first). Teams without keys are omitted. Keys that can't be matched to a
 * loaded Team — e.g. before the Team list has arrived — land in a trailing
 * Team-less group so they stay selectable.
 */
export function groupSharingKeysByTeam(identities: UsageIdentity[], teams: Team[]): SharingKeyGroup[] {
    const ordered = [...teams.filter((team) => team.is_default), ...teams.filter((team) => !team.is_default)];
    const defaultTeamId = ordered.find((team) => team.is_default)?.id;
    const byTeam = new Map<string, UsageIdentity[]>();
    const unmatched: UsageIdentity[] = [];
    const known = new Set(ordered.map((team) => team.id));

    for (const identity of identities) {
        const teamId = identity.teamId || defaultTeamId;
        if (teamId && known.has(teamId)) {
            byTeam.set(teamId, [...(byTeam.get(teamId) ?? []), identity]);
        } else {
            unmatched.push(identity);
        }
    }

    const groups: SharingKeyGroup[] = ordered
        .filter((team) => byTeam.has(team.id))
        .map((team) => ({ team, identities: byTeam.get(team.id)! }));
    if (unmatched.length > 0) groups.push({ team: null, identities: unmatched });
    return groups;
}
