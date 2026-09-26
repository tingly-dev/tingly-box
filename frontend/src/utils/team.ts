import type { Team } from '@/types/team';

/** Teams in display order: the default Team first, the rest as returned. */
export function orderTeams(teams: Team[]): Team[] {
    return [...teams.filter((team) => team.is_default), ...teams.filter((team) => !team.is_default)];
}

/** Route of a Team's workspace page. */
export function teamPath(team: Pick<Team, 'is_default' | 'slug'>): string {
    return team.is_default ? '/agent/team' : `/agent/team/${team.slug}`;
}

export interface TeamGroup<T> {
    team: Team;
    items: T[];
}

/**
 * Buckets items by owning Team, one group per Team in `orderTeams` order
 * (empty groups included — callers that don't want them filter). Items whose
 * Team isn't among `teams` — e.g. before the Team list has loaded — are
 * returned separately so callers can decide whether to show them.
 */
export function groupByTeam<T>(
    items: T[],
    teams: Team[],
    getTeamId: (item: T) => string | undefined,
): { groups: TeamGroup<T>[]; unmatched: T[] } {
    const groups = orderTeams(teams).map((team) => ({ team, items: [] as T[] }));
    const byId = new Map(groups.map((group) => [group.team.id, group]));
    const unmatched: T[] = [];
    for (const item of items) {
        const teamId = getTeamId(item);
        const group = teamId ? byId.get(teamId) : undefined;
        if (group) group.items.push(item);
        else unmatched.push(item);
    }
    return { groups, unmatched };
}
