import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '@/services/api';
import ClaudeCodeSlotsCard from '@/pages/scenario/components/ClaudeCodeSlotsCard';
import {
    type ClaudeCodeSlotResolution,
    type ClaudeCodeSlotRules,
    slotRulesOf,
} from '@/pages/scenario/components/claudeCodeSlots';

export interface SlotRouting {
    slots: ClaudeCodeSlotResolution[];
    /** Slot → rule UUID, for deriving the Claude Code env. */
    slotRules: ClaudeCodeSlotRules;
    /** The rules some slot requests, in slot order: the ones worth showing. */
    rules: any[];
    setRules: (rules: any[]) => void;
    loading: boolean;
    /** The slot table. */
    slotsCard: React.ReactNode;
}

/**
 * Routing for clients with fixed model slots (Claude Code): each slot is
 * bound to a rule of the scenario, independently. The page shows the slot
 * table and the rules in use; `enabled: false` keeps the hook inert for
 * agents without slots (hooks can't be called conditionally).
 */
export const useSlotRouting = (scenario: string, enabled: boolean, isProfile = false): SlotRouting => {
    const [slots, setSlots] = useState<ClaudeCodeSlotResolution[]>([]);
    const [allRules, setAllRules] = useState<any[]>([]);
    const [loading, setLoading] = useState(enabled);

    const loadRules = useCallback(async () => {
        const result = await api.getRules(scenario);
        setAllRules(result?.success ? result.data || [] : []);
    }, [scenario]);

    useEffect(() => {
        if (!enabled) return;
        let isMounted = true;
        setLoading(true);
        Promise.all([api.getClaudeCodeSlots(scenario), api.getRules(scenario)]).then(([slotResult, ruleResult]) => {
            if (!isMounted) return;
            setSlots(slotResult?.success ? slotResult.data?.slots || [] : []);
            setAllRules(ruleResult?.success ? ruleResult.data || [] : []);
            setLoading(false);
        });
        return () => { isMounted = false; };
    }, [scenario, enabled]);

    const slotRules = useMemo(() => slotRulesOf(slots), [slots]);
    const rules = useMemo(() => {
        const used = new Set(Object.values(slotRules));
        return allRules.filter(r => used.has(r.uuid))
            .sort((a, b) => slots.findIndex(s => s.rule_uuid === a.uuid) - slots.findIndex(s => s.rule_uuid === b.uuid));
    }, [allRules, slotRules, slots]);

    // The rules card edits a subset; merge its changes back into the full list.
    const setRules = useCallback((next: any[]) => {
        setAllRules(prev => prev.map(r => next.find(n => n.uuid === r.uuid) ?? r));
    }, []);

    const handleSlotsChange = useCallback((next: ClaudeCodeSlotResolution[]) => {
        setSlots(next);
        // A slot may have just got a new rule, or switched one on.
        void loadRules();
    }, [loadRules]);

    const slotsCard = enabled && !loading ? (
        <ClaudeCodeSlotsCard
            scenario={scenario}
            slots={slots}
            rules={allRules}
            onChange={handleSlotsChange}
            isProfile={isProfile}
        />
    ) : null;

    return { slots, slotRules, rules, setRules, loading, slotsCard };
};
