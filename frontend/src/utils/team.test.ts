import { describe, expect, it } from 'vitest';
import type { Team } from '@/types/team';
import { groupByTeam, orderTeams, teamPath } from './team';

const team = (id: string, isDefault = false): Team => ({
    id, name: id, slug: id, enabled: true, quota_visible: false, is_default: isDefault, created_at: '', updated_at: '',
});
const teams = [team('t2'), team('default', true), team('t1')];

describe('orderTeams', () => {
    it('puts the default Team first and keeps the rest in order', () => {
        expect(orderTeams(teams).map((t) => t.id)).toEqual(['default', 't2', 't1']);
    });
});

describe('teamPath', () => {
    it('maps the default Team to the bare route and others to their slug', () => {
        expect(teamPath(teams[1])).toBe('/agent/team');
        expect(teamPath(teams[0])).toBe('/agent/team/t2');
    });
});

describe('groupByTeam', () => {
    const items = [{ k: 'a', team: 't2' }, { k: 'b', team: 'default' }, { k: 'c', team: 'gone' }, { k: 'd' }];
    const { groups, unmatched } = groupByTeam(items, teams, (i) => (i as { team?: string }).team);

    it('returns every Team in display order, empty ones included', () => {
        expect(groups.map((g) => [g.team.id, g.items.map((i) => i.k)])).toEqual([
            ['default', ['b']], ['t2', ['a']], ['t1', []],
        ]);
    });

    it('returns items of unknown or missing Teams separately', () => {
        expect(unmatched.map((i) => i.k)).toEqual(['c', 'd']);
    });

    it('treats everything as unmatched before Teams have loaded', () => {
        expect(groupByTeam(items, [], (i) => (i as { team?: string }).team).unmatched).toHaveLength(4);
    });
});
