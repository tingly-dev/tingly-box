import type { Team } from '@/types/team';

/** Qualify sharing identities without changing their user_id grouping/filter. */
export function usageAccountLabel(name: string, teamId: string | undefined, teams: Pick<Team, 'id' | 'name'>[]): string {
    if (!teamId) return name;
    // Preserve a distinguishable identity while teams load or after a Team is deleted.
    const team = teams.find((candidate) => candidate.id === teamId);
    return `${name} · ${team?.name || teamId}`;
}
