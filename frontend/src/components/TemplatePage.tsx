import React, { useCallback, useState } from 'react';
import ApiKeyModal from '../components/ApiKeyModal';
import RuleCard from './RuleCard.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import type { Rule } from './RoutingGraphTypes.ts';
import { useModelSelectDialog } from '../hooks/useModelSelectDialog';

export interface TabTemplatePageProps {
    title?: string | React.ReactNode;
    rules: Rule[];
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    collapsible?: boolean;
    allowDeleteRule?: boolean;
    onRuleDelete?: (ruleUuid: string) => void;
    allowToggleRule?: boolean;
    newlyCreatedRuleUuids?: Set<string>;
}

const TemplatePage: React.FC<TabTemplatePageProps> = ({
    rules,
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers,
    onRulesChange,
    title="",
    collapsible = false,
    allowDeleteRule = false,
    onRuleDelete,
    allowToggleRule = true,
    newlyCreatedRuleUuids,
}) => {
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

    // Use the model select dialog hook
    const { openModelSelect, ModelSelectDialog, isOpen: modelSelectDialogOpen } = useModelSelectDialog({
        providers,
        providerModels: providerModelsByUuid,
        onProviderModelsChange: handleProviderModelsChange,
        rules,
        onRuleChange: handleRuleChange,
        showNotification,
        onRefreshProvider: handleRefreshModels,
        refreshingProviders,
    });

    // Wrapper to maintain compatibility with existing RuleCard interface
    const openModelSelectDialog = useCallback((
        ruleUuid: string,
        configRecord: any,
        mode: 'edit' | 'add',
        providerUuid?: string
    ) => {
        openModelSelect({ ruleUuid, configRecord, providerUuid, mode });
    }, [openModelSelect]);

    if (!providers.length || !rules?.length) {
        return null;
    }

    return (
        <>
            <UnifiedCard size="full" title={title}>
            {rules.map((rule) => (
                rule && rule.uuid &&
                    <RuleCard
                        key={rule.uuid}
                        rule={rule}
                        providers={providers}
                        providerModelsByUuid={providerModelsByUuid}
                        saving={refreshingProviders.length > 0}
                        showNotification={showNotification}
                        onRuleChange={handleRuleChange}
                        onProviderModelsChange={handleProviderModelsChange}
                        onRefreshProvider={handleRefreshModels}
                        collapsible={collapsible}
                        initiallyExpanded={
                            collapsible
                        }
                        onModelSelectOpen={openModelSelectDialog}
                        allowDeleteRule={allowDeleteRule}
                        onRuleDelete={onRuleDelete}
                        allowToggleRule={allowToggleRule}
                    />
            ))}
            </UnifiedCard>

            <ModelSelectDialog open={modelSelectDialogOpen} onClose={() => {}} />

            <ApiKeyModal
                open={showTokenModal}
                onClose={() => setShowTokenModal(false)}
                token={token}
                onCopy={async (text, label) => {
                    try {
                        await navigator.clipboard.writeText(text);
                        showNotification(`${label} copied to clipboard!`, 'success');
                    } catch (err) {
                        showNotification('Failed to copy to clipboard', 'error');
                    }
                }}
            />
        </>
    );
};

export default TemplatePage;
