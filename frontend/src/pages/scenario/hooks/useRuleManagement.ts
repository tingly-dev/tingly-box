import { useCallback, useEffect, useState } from 'react';
import { rulesDataCache } from '@/services/scenarioDataCache';

/**
 * Hook for managing rules in scenario pages
 * Handles rule loading, changes, and deletion with proper state tracking
 *
 * @param scenario - Known up front when the caller already has it (most
 * do), used only to seed initial state synchronously from the shared
 * rules cache so revisiting a page in the same session paints instantly
 * instead of showing a loading spinner again. Safe to omit.
 */
export const useRuleManagement = (scenario?: string) => {
    const [rules, setRules] = useState<any[]>(() => (scenario ? rulesDataCache.getCached(scenario) ?? [] : []));
    const [loadingRule, setLoadingRule] = useState(() => !scenario || rulesDataCache.getCached(scenario) === undefined);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = useState<Set<string>>(new Set());

    const handleRuleDelete = useCallback((deletedRuleUuid: string) => {
        setRules((prevRules) => prevRules.filter(r => r.uuid !== deletedRuleUuid));
    }, []);

    const handleRulesChange = useCallback((updatedRules: any[]) => {
        setRules(updatedRules);
        // If a new rule was added (length increased), add it to newlyCreatedRuleUuids
        if (updatedRules.length > rules.length) {
            const newRule = updatedRules[updatedRules.length - 1];
            setNewlyCreatedRuleUuids(prev => new Set(prev).add(newRule.uuid));
        }
    }, [rules.length]);

    // Always fetches fresh (correct after rule mutations), but concurrent
    // mounts of this hook for the same scenario (the page + TemplatePage
    // both call it) share one in-flight request instead of firing one
    // apiece, and a scenario revisited this session paints from cache
    // first (see the useState initializer above) while this quietly
    // revalidates.
    const loadRules = useCallback(async (scenarioId: string) => {
        if (!scenarioId.trim()) {
            setRules([]);
            setLoadingRule(false);
            return;
        }

        const ruleData = await rulesDataCache.refresh(scenarioId);
        setRules(ruleData);
        setLoadingRule(false);
    }, []);

    // Keep in sync with a fetch/revalidation for the current scenario
    // started by another mount of this hook.
    useEffect(() => {
        if (!scenario) return;
        return rulesDataCache.subscribe(scenario, setRules);
    }, [scenario]);

    return {
        rules,
        setRules,
        loadingRule,
        newlyCreatedRuleUuids,
        setNewlyCreatedRuleUuids,
        handleRuleDelete,
        handleRulesChange,
        loadRules,
    };
};
