import type { Rule } from '@/components/RoutingGraphTypes.ts';
import { api } from '@/services/api.ts';
import type { ProviderModelsDataByUuid } from '@/types/provider.ts';
import { useCallback, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';

export interface UseTemplatePageRulesParams {
    rules: Rule[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    scenario?: string;
    loadRules?: (scenario: string) => Promise<void>;
}

export interface UseTemplatePageRulesReturn {
    providerModelsByUuid: ProviderModelsDataByUuid;
    refreshingProviders: string[];
    handleRuleChange: (updatedRule: Rule) => void;
    handleProviderModelsChange: (providerUuid: string, models: any) => void;
    handleRefreshModels: (providerUuid: string) => Promise<void>;
    handleCreateRule: () => Promise<string | null>;
}

export const useTemplatePageRules = ({
    rules,
    onRulesChange,
    showNotification,
    scenario,
    loadRules,
}: UseTemplatePageRulesParams): UseTemplatePageRulesReturn => {
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);

    const handleRuleChange = useCallback((updatedRule: Rule) => {
        if (onRulesChange) {
            const updatedRules = rules.map(r =>
                r.uuid === updatedRule.uuid ? updatedRule : r
            );
            onRulesChange(updatedRules);
        }
    }, [rules, onRulesChange]);

    const handleProviderModelsChange = useCallback((providerUuid: string, models: any) => {
        setProviderModelsByUuid((prev: any) => ({
            ...prev,
            [providerUuid]: models,
        }));
    }, []);

    const handleRefreshModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid) return;

        try {
            setRefreshingProviders(prev => [...prev, providerUuid]);
            const result = await api.updateProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                setProviderModelsByUuid((prev: any) => ({
                    ...prev,
                    [providerUuid]: result.data,
                }));
                showNotification(`Models refreshed successfully!`, 'success');
            } else {
                showNotification(`Failed to refresh models: ${result.message}`, 'error');
            }
        } catch (error) {
            console.error('Error refreshing models:', error);
            showNotification(`Error refreshing models`, 'error');
        } finally {
            setRefreshingProviders(prev => prev.filter(p => p !== providerUuid));
        }
    }, [showNotification]);

    const handleCreateRule = useCallback(async (): Promise<string | null> => {
        if (!scenario) {
            showNotification('Cannot create rule: scenario not specified', 'error');
            return null;
        }

        try {
            const newRuleData = {
                scenario: scenario,
                request_model: `model-${uuidv4().slice(0, 8)}`,
                response_model: '',
                active: true,
                services: []
            };

            // Create rule via API (empty string for UUID signals backend to generate new UUID)
            const result = await api.createRule('', newRuleData);

            // Validate response has required data
            if (!result.success) {
                showNotification(`Failed to create rule: ${result.error || 'Unknown error'}`, 'error');
                return null;
            }

            if (!result.data || !result.data.uuid) {
                showNotification('Failed to create rule: Invalid response from server (missing UUID)', 'error');
                console.error('Invalid createRule response:', result);
                return null;
            }

            // Verify the rule has the expected scenario
            if (result.data.scenario !== scenario) {
                showNotification(`Warning: Rule created but scenario mismatch (expected: ${scenario}, got: ${result.data.scenario})`, 'warning');
            }

            // Reload rules from backend to verify persistence
            if (loadRules) {
                await loadRules(scenario);
            } else if (onRulesChange) {
                // Fallback: update local state if loadRules not provided
                onRulesChange([...rules, result.data]);
            }

            showNotification('Routing rule created successfully!', 'success');
            return result.data.uuid;
        } catch (error) {
            console.error('Error creating rule:', error);
            showNotification(`Failed to create routing rule: ${(error as Error).message || 'Unknown error'}`, 'error');
            return null;
        }
    }, [scenario, rules, onRulesChange, showNotification, loadRules]);

    return {
        providerModelsByUuid,
        refreshingProviders,
        handleRuleChange,
        handleProviderModelsChange,
        handleRefreshModels,
        handleCreateRule,
    };
};
