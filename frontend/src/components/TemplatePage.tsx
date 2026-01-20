import { Dialog, DialogContent, DialogTitle } from '@mui/material';
import React, { useCallback, useState } from 'react';
import ApiKeyModal from '../components/ApiKeyModal';
import RuleCard from './RuleCard.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import ModelSelectTab, { type ProviderSelectTabOption } from './ModelSelectTab';
import type { ConfigRecord, Rule } from './RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';

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

    // ModelSelectTab dialog state
    const [modelSelectDialogOpen, setModelSelectDialogOpen] = useState(false);
    const [modelSelectMode, setModelSelectMode] = useState<'edit' | 'add'>('add');
    const [editingProviderUuid, setEditingProviderUuid] = useState<string | null>(null);
    const [currentRuleUuid, setCurrentRuleUuid] = useState<string | null>(null);
    const [currentConfigRecord, setCurrentConfigRecord] = useState<ConfigRecord | null>(null);

    const handleFetchModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid || providerModelsByUuid[providerUuid]) {
            return;
        }

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                // If GET returns empty list, auto-fetch from Provider API
                if (!result.data.models || result.data.models.length === 0) {
                    const refreshResult = await api.updateProviderModelsByUUID(providerUuid);
                    if (refreshResult.success && refreshResult.data) {
                        setProviderModelsByUuid((prev: any) => ({
                            ...prev,
                            [providerUuid]: refreshResult.data,
                        }));
                    }
                    return;
                }
                setProviderModelsByUuid((prev: any) => ({
                    ...prev,
                    [providerUuid]: result.data,
                }));
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        }
    }, [providerModelsByUuid]);

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

    const openModelSelectDialog = useCallback((
        ruleUuid: string,
        configRecord: ConfigRecord,
        mode: 'edit' | 'add',
        providerUuid?: string
    ) => {
        setCurrentRuleUuid(ruleUuid);
        setCurrentConfigRecord(configRecord);
        setModelSelectMode(mode);
        setEditingProviderUuid(providerUuid || null);
        setModelSelectDialogOpen(true);

        // Auto-fetch models for the first provider when dialog opens
        if (providers.length > 0) {
            handleFetchModels(providers[0].uuid);
        }
    }, [providers, handleFetchModels]);

    const handleModelSelect = useCallback((option: ProviderSelectTabOption) => {
        if (!currentConfigRecord || !currentRuleUuid) return;

        let updated: ConfigRecord;

        if (modelSelectMode === 'add') {
            updated = {
                ...currentConfigRecord,
                providers: [
                    ...currentConfigRecord.providers,
                    { uuid: uuidv4(), provider: option.provider.uuid, model: option.model || '', isManualInput: false },
                ],
            };
        } else if (modelSelectMode === 'edit' && editingProviderUuid) {
            updated = {
                ...currentConfigRecord,
                providers: currentConfigRecord.providers.map(p => {
                    if (p.uuid === editingProviderUuid) {
                        return { ...p, provider: option.provider.uuid, model: option.model || '' };
                    }
                    return p;
                }),
            };
        } else {
            updated = currentConfigRecord;
        }

        const rule = rules.find(r => r.uuid === currentRuleUuid);
        if (rule && updated) {
            const ruleData = {
                uuid: rule.uuid,
                scenario: rule.scenario,
                request_model: updated.requestModel,
                response_model: updated.responseModel,
                active: updated.active,
                description: updated.description,
                services: updated.providers
                    .filter(p => p.provider && p.model)
                    .map(provider => ({
                        provider: provider.provider,
                        model: provider.model,
                        weight: provider.weight || 0,
                        active: provider.active !== undefined ? provider.active : true,
                        time_window: provider.time_window || 0,
                    })),
                smart_enabled: updated.smartEnabled || false,
                smart_routing: updated.smartRouting || [],
            };

            api.updateRule(rule.uuid, ruleData).then((result) => {
                if (result.success) {
                    showNotification(`Configuration saved successfully`, 'success');
                    handleRuleChange({
                        ...rule,
                        scenario: ruleData.scenario,
                        request_model: ruleData.request_model,
                        response_model: ruleData.response_model,
                        active: ruleData.active,
                        description: ruleData.description,
                        services: ruleData.services,
                        smart_enabled: ruleData.smart_enabled,
                        smart_routing: ruleData.smart_routing,
                    });
                } else {
                    showNotification(`Failed to save: ${result.error || 'Unknown error'}`, 'error');
                }
            });
        }

        setModelSelectDialogOpen(false);
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);

        if (option.provider.uuid) {
            handleFetchModels(option.provider.uuid);
        }
    }, [currentConfigRecord, currentRuleUuid, modelSelectMode, editingProviderUuid, rules, handleRuleChange, showNotification, handleFetchModels]);

    if (!providers.length || !rules?.length) {
        return null;
    }

    console.log("rules", rules)

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

            <Dialog
                open={modelSelectDialogOpen}
                onClose={() => setModelSelectDialogOpen(false)}
                maxWidth="lg"
                fullWidth
                PaperProps={{
                    sx: { height: '80vh' }
                }}
            >
                <DialogTitle sx={{ textAlign: 'center' }}>
                    {modelSelectMode === 'add' ? 'Add API Key' : 'Choose Model'}
                </DialogTitle>
                <DialogContent>
                    <ModelSelectTab
                        providers={providers}
                        providerModels={providerModelsByUuid}
                        selectedProvider={modelSelectMode === 'edit' && editingProviderUuid
                            ? currentConfigRecord?.providers.find(p => p.uuid === editingProviderUuid)?.provider
                            : undefined}
                        selectedModel={modelSelectMode === 'edit' && editingProviderUuid
                            ? currentConfigRecord?.providers.find(p => p.uuid === editingProviderUuid)?.model
                            : undefined}
                        onSelected={handleModelSelect}
                        onProviderChange={(p) => handleFetchModels(p.uuid)}
                        onRefresh={(p) => handleRefreshModels(p.uuid)}
                        refreshingProviders={refreshingProviders}
                    />
                </DialogContent>
            </Dialog>

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
