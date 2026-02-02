import { useCallback, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import { api } from '@/services/api';
import type { Provider, ProviderModelsDataByUuid } from '@/types/provider';
import type { Rule } from './RoutingGraphTypes.ts';

export interface UseTemplatePageRulesParams {
    rules: Rule[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    scenario?: string;
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
        if (!scenario) return null;

        try {
            const newRuleData = {
                scenario: scenario,
                request_model: `model-${uuidv4().slice(0, 8)}`,
                response_model: '',
                active: true,
                services: []
            };
            const result = await api.createRule('', newRuleData);
            if (result.success && result.data?.uuid) {
                onRulesChange?.([...rules, result.data]);
                showNotification('Routing rule created successfully!', 'success');
                return result.data.uuid;
            } else {
                showNotification(`Failed to create rule: ${result.error || 'Unknown error'}`, 'error');
                return null;
            }
        } catch (error) {
            console.error('Error creating rule:', error);
            showNotification('Failed to create routing rule', 'error');
            return null;
        }
    }, [scenario, rules, onRulesChange, showNotification]);

    return {
        providerModelsByUuid,
        refreshingProviders,
        handleRuleChange,
        handleProviderModelsChange,
        handleRefreshModels,
        handleCreateRule,
    };
};
