import { useCallback, useEffect, useState } from 'react';
import { api } from '@/services/api.ts';

/**
 * Hook for managing rules in scenario pages
 * Handles rule loading, changes, and deletion with proper state tracking
 */
export const useRuleManagement = () => {
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
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

    const loadRules = useCallback(async (scenario: string) => {
        const result = await api.getRules(scenario);
        if (result.success) {
            // Ensure data is an array to prevent crashes
            const ruleData = Array.isArray(result.data) ? result.data : [];
            setRules(ruleData);
        }
        setLoadingRule(false);
    }, []);

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
