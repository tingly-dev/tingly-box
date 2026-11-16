import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useRosterAxis } from './RosterAxis';
import type { AggregatedStat } from './ServiceStatsTable';

// The selection-repair effect is why a mismatched click payload "bounces":
// any selectedKey outside the roster's key space is replaced by the first
// visible row. These tests pin that contract for every caller.
// Sorted by the default sort (total tokens desc): user-1, user-2, user-3.
const roster: AggregatedStat[] = [
    { key: 'user-1', request_count: 3, total_input_tokens: 70, total_output_tokens: 30 },
    { key: 'user-2', request_count: 2, total_input_tokens: 40, total_output_tokens: 20 },
    { key: 'user-3', request_count: 1, total_input_tokens: 20, total_output_tokens: 10 },
];
const getKey = (row: AggregatedStat) => row.key;
const stableLoadDetail = async () => [];

const renderAxis = () => renderHook(() => useRosterAxis<AggregatedStat, never, string>({
    roster,
    getKey,
    nameOf: (row) => row.key,
    searchText: (row) => [row.key],
    rowsPerPage: 10,
    range: '7d',
    loadDetail: stableLoadDetail,
}));

describe('useRosterAxis selection', () => {
    it('starts with the first visible row selected', () => {
        const { result } = renderAxis();
        expect(result.current.selectedKey).toBe('user-1');
        expect(result.current.selected?.key).toBe('user-1');
    });

    it('keeps a selection that is inside the roster (no bounce)', () => {
        const { result } = renderAxis();
        act(() => result.current.setSelectedKey('user-2'));
        expect(result.current.selectedKey).toBe('user-2');
        expect(result.current.selected?.key).toBe('user-2');
    });

    it('snaps an out-of-roster selection back to the first visible row', () => {
        const { result } = renderAxis();
        act(() => result.current.setSelectedKey('not-a-roster-key'));
        expect(result.current.selectedKey).toBe('user-1');
        expect(result.current.selected?.key).toBe('user-1');
    });
});
