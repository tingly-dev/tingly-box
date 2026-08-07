import { useMemo, useState } from 'react';
import type { Rule } from '@/components/RoutingGraphTypes.ts';

export type RuleSortMode = 'original' | 'name';

export interface UseRuleSortReturn {
    sortMode: RuleSortMode;
    toggleSortMode: () => void;
    sortedRules: Rule[];
}

/**
 * Purely a display concern: reorders the Model Rules list for viewing only.
 * The rules array's actual order (which the backend may use for match
 * priority) is never mutated — 'original' just renders `rules` as given.
 */
export function useRuleSort(rules: Rule[]): UseRuleSortReturn {
    const [sortMode, setSortMode] = useState<RuleSortMode>('original');

    const toggleSortMode = () => {
        setSortMode((prev) => (prev === 'original' ? 'name' : 'original'));
    };

    const sortedRules = useMemo(() => {
        if (sortMode === 'original') return rules;
        return [...rules].sort((a, b) =>
            (a.request_model || '').localeCompare(b.request_model || '', undefined, { sensitivity: 'base' }),
        );
    }, [rules, sortMode]);

    return { sortMode, toggleSortMode, sortedRules };
}
