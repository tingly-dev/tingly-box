import { describe, expect, it } from 'vitest';
import type { UsageIdentity } from '@/hooks/useDashboardData';
import type { Team } from '@/types/team';
import { groupSharingKeysByTeam } from './groupSharingKeysByTeam';

const team = (id: string, isDefault = false): Team => ({
    id, name: id, slug: id, enabled: true, is_default: isDefault, created_at: '', updated_at: '',
});
const key = (userId: string, teamId?: string): UsageIdentity => ({
    userId, label: userId, type: 'sharing_key', enabled: true, teamId,
});

describe('groupSharingKeysByTeam', () => {
    const teams = [team('t2'), team('default', true), team('t1')];

    it('orders groups default-first and omits Teams without keys', () => {
        const groups = groupSharingKeysByTeam([key('a', 't2'), key('b', 'default')], teams);
        expect(groups.map((g) => g.team?.id)).toEqual(['default', 't2']);
    });

    it('puts keys without a team_id under the default Team', () => {
        const groups = groupSharingKeysByTeam([key('a')], teams);
        expect(groups).toEqual([{ team: teams[1], identities: [key('a')] }]);
    });

    it('keeps keys of unknown Teams selectable in a trailing Team-less group', () => {
        const groups = groupSharingKeysByTeam([key('a', 'gone'), key('b', 't1')], teams);
        expect(groups.map((g) => g.team?.id ?? null)).toEqual(['t1', null]);
        expect(groups[1].identities).toEqual([key('a', 'gone')]);
    });

    it('returns one Team-less group before Teams have loaded', () => {
        expect(groupSharingKeysByTeam([key('a', 't1')], [])).toEqual([{ team: null, identities: [key('a', 't1')] }]);
    });
});
