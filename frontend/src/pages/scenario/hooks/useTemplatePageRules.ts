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

export interface CreateRuleOptions {
    providerUuid: string;
    model: string;
}

export interface UseTemplatePageRulesReturn {
    providerModelsByUuid: ProviderModelsDataByUuid;
    refreshingProviders: string[];
    handleRuleChange: (updatedRule: Rule) => void;
    handleProviderModelsChange: (providerUuid: string, models: any) => void;
    handleRefreshModels: (providerUuid: string) => Promise<void>;
    handleCreateRules: (options: CreateRuleOptions[]) => Promise<string[]>;
}

export const resolveRuleName = (desiredName: string, existingNames: Set<string>): string => {
    if (!existingNames.has(desiredName)) return desiredName;
    return `${desiredName}-${uuidv4().slice(0, 6)}`;
};

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

    // Creates one rule per model, in order, so names stay unique within the
    // batch (a single pick is a batch of one); the list reloads once at the
    // end and one toast sums it up. Returns the created uuids.
    const handleCreateRules = useCallback(async (options: CreateRuleOptions[]): Promise<string[]> => {
        if (!scenario) {
            showNotification('Cannot create rule: scenario not specified', 'error');
            return [];
        }

        const existingNames = new Set(rules.map(r => r.request_model).filter(Boolean) as string[]);
        const created: Rule[] = [];
        const failures: string[] = [];
        for (const { providerUuid, model } of options) {
            try {
                const newRuleData = {
                    scenario: scenario,
                    request_model: resolveRuleName(model, existingNames),
                    response_model: '',
                    active: true,
                    services: [{ provider: providerUuid, model, weight: 0, active: true, time_window: 0 }],
                };

                // Create rule via API (empty string for UUID signals backend to generate new UUID)
                let result = await api.createRule('', newRuleData);

                // If backend reports name conflict, retry once with a random suffix
                if (!result?.success && result?.error?.includes('already exists')) {
                    newRuleData.request_model = `${newRuleData.request_model}-${uuidv4().slice(0, 6)}`;
                    result = await api.createRule('', newRuleData);
                }

                if (!result?.success) {
                    failures.push(`${model}: ${result?.error || 'Unknown error'}`);
                } else if (!result.data?.uuid) {
                    console.error('Invalid createRule response:', result);
                    failures.push(`${model}: Invalid response from server (missing UUID)`);
                } else {
                    existingNames.add(result.data.request_model);
                    created.push(result.data);
                }
            } catch (error) {
                console.error('Error creating rule:', error);
                failures.push(`${model}: ${(error as Error).message || 'Unknown error'}`);
            }
        }

        if (created.length > 0) {
            // Reload rules from backend to verify persistence
            if (loadRules) {
                await loadRules(scenario);
            } else if (onRulesChange) {
                // Fallback: update local state if loadRules not provided
                onRulesChange([...rules, ...created]);
            }
        }

        if (failures.length === 0) {
            showNotification(created.length === 1 ? 'Routing rule created successfully!' : `Added ${created.length} models`, 'success');
        } else if (options.length === 1) {
            showNotification(`Failed to create rule: ${failures[0]}`, 'error');
        } else {
            showNotification(
                `Added ${created.length} of ${options.length} models. Failed: ${failures.join('; ')}`,
                created.length > 0 ? 'warning' : 'error',
            );
        }
        return created.map(r => r.uuid);
    }, [scenario, rules, onRulesChange, showNotification, loadRules]);

    return {
        providerModelsByUuid,
        refreshingProviders,
        handleRuleChange,
        handleProviderModelsChange,
        handleRefreshModels,
        handleCreateRules,
    };
};
