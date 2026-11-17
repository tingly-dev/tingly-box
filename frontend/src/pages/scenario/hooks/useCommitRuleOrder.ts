import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes.ts';
import { api } from '@/services/api';

interface Options {
    scenario?: string;
    rules: Rule[];
    onRulesChange: (rules: Rule[]) => void;
    loadRules?: (scenario: string) => Promise<void> | void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

/**
 * Persists a whole new rule order (drag and drop, "Pin to top"). Optimistic
 * locally; on failure restores the pre-move list, toasts, then reloads to the
 * server truth — another tab may have changed the list meanwhile.
 */
export function useCommitRuleOrder({ scenario, rules, onRulesChange, loadRules, showNotification }: Options) {
    const { t } = useTranslation();

    return useCallback(async (orderedUuids: string[], movedUuid?: string) => {
        if (!scenario) return;
        const byUuid = new Map(rules.map(r => [r.uuid, r]));
        const next = orderedUuids.flatMap(uuid => byUuid.get(uuid) ?? []);
        // The list changed under the gesture (rule added/removed by a refetch): drop it.
        if (next.length !== rules.length) return;

        onRulesChange(next);
        try {
            const result = await api.reorderRules(scenario, orderedUuids);
            if (!result?.success) {
                throw new Error(result?.error || 'Unknown error');
            }
            const moved = movedUuid ? byUuid.get(movedUuid) : undefined;
            showNotification(
                moved
                    ? t('templatePage.reorder.saved', {
                        name: moved.request_model || moved.uuid,
                        position: orderedUuids.indexOf(moved.uuid) + 1,
                        total: orderedUuids.length,
                    })
                    : t('templatePage.reorder.savedPlain'),
                'success',
            );
        } catch (error) {
            console.error('Error reordering rules:', error);
            onRulesChange(rules);
            showNotification(t('templatePage.reorder.saveFailed', { message: (error as Error).message }), 'error');
            await loadRules?.(scenario);
        }
    }, [rules, scenario, onRulesChange, loadRules, showNotification, t]);
}
