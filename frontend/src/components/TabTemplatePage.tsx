import { PlayArrow as ProbeIcon } from '@mui/icons-material';
import { Box, Button, Dialog, DialogContent, DialogTitle, Typography } from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import type {ProbeResponse, ProbeResponseData} from '../client';
import ApiKeyModal from '../components/ApiKeyModal';
import CardGrid from '../components/CardGrid.tsx';
import Probe from '../components/Probe';
import RuleGraphV2 from '../components/RuleGraphV2';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import ModelSelectTab, { type ProviderSelectTabOption } from './ModelSelectTab';
import type { ConfigProvider, ConfigRecord, Rule } from './RuleGraphTypes.ts';

export interface TabTemplatePageProps {
    title: string | React.ReactNode;
    subtitle?: string;
    rule: Rule;
    header?: React.ReactNode;
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
    onRuleChange?: (updatedRule: Rule) => void;
}

const TabTemplatePage: React.FC<TabTemplatePageProps> = ({
    title,
    subtitle,
    rule,
    header,
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers,
    onRuleChange
}) => {
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [configRecord, setConfigRecord] = useState<ConfigRecord | null>(null);
    const [saving, setSaving] = useState(false);
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);
    const [isInitialized, setIsInitialized] = useState(false);

    // Probe state
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);

    // ModelSelectTab dialog state for provider selection
    const [modelSelectDialogOpen, setModelSelectDialogOpen] = useState(false);
    const [modelSelectMode, setModelSelectMode] = useState<'edit' | 'add'>('add');
    const [editingProviderUuid, setEditingProviderUuid] = useState<string | null>(null);

    const providerUuidToName = React.useMemo(() => {
        const map: { [uuid: string]: string } = {};
        providers.forEach(provider => {
            map[provider.uuid] = provider.name;
        });
        return map;
    }, [providers]);

    // Convert rule to ConfigRecord format when rule or providers change
    useEffect(() => {
        if (rule && providers.length > 0) {
            const services = rule.services || [];
            const providersList: ConfigProvider[] = services.map((service: any) => ({
                uuid: service.id || service.uuid || crypto.randomUUID(),
                provider: service.provider || '',
                model: service.model || '',
                isManualInput: false,
                weight: service.weight || 0,
                active: service.active !== undefined ? service.active : true,
                time_window: service.time_window || 0,
            }));

            // If no providers, add an empty one
            if (providersList.length === 0) {
                providersList.push({
                    uuid: crypto.randomUUID(),
                    provider: '',
                    model: '',
                });
            }

            const newConfigRecord: ConfigRecord = {
                uuid: rule.uuid || crypto.randomUUID(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                active: rule.active !== undefined ? rule.active : true,
                providers: providersList,
            };

            setConfigRecord(newConfigRecord);
            setIsInitialized(true);
        }
    }, [rule, providers]);

    // Fetch models for providers when configRecord changes
    useEffect(() => {
        if (configRecord && providers.length > 0) {
            const usedProviderUuids = new Set<string>();
            configRecord.providers.forEach(p => {
                if (p.provider) {
                    usedProviderUuids.add(p.provider);
                }
            });

            usedProviderUuids.forEach(uuid => {
                if (!providerModelsByUuid[uuid]) {
                    handleFetchModels(uuid);
                }
            });
        }
    }, [configRecord]);

    const handleProbe = useCallback(async () => {
        if (!configRecord?.providers.length || !configRecord.providers[0].provider || !configRecord.providers[0].model) {
            return;
        }

        const providerUuid = configRecord.providers[0].provider;
        const model = configRecord.providers[0].model;

        setIsProbing(true);
        setProbeResult(null);

        try {
            const result = await api.probeModel(providerUuid, model);
            setProbeResult(result);
        } catch (error) {
            console.error('Probe error:', error);
            setProbeResult({
                success: false,
                error: {
                    message: (error as Error).message,
                    type: 'client_error'
                }
            });
        } finally {
            setIsProbing(false);
        }
    }, [configRecord, providerUuidToName]);

    const handleFetchModels = async (providerUuid: string) => {
        if (!providerUuid || providerModelsByUuid[providerUuid]) {
            return;
        }

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                setProviderModelsByUuid((prev: any) => ({
                    ...prev,
                    [providerUuid]: result.data,
                }));
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        }
    };

    const handleRefreshModels = async (providerUuid: string) => {
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
                showNotification(`Failed to refresh models: ${result.error}`, 'error');
            }
        } catch (error) {
            console.error('Error refreshing models:', error);
            showNotification(`Error refreshing models`, 'error');
        } finally {
            setRefreshingProviders(prev => prev.filter(p => p !== providerUuid));
        }
    };

    // Auto-save function
    const autoSave = async (newConfigRecord: ConfigRecord) => {
        if (!newConfigRecord.requestModel) {
            return;
        }

        // Validate providers have both provider and model before saving
        for (const provider of newConfigRecord.providers) {
            if (provider.provider && !provider.model) {
                // Don't save if provider is selected but model is not
                return;
            }
        }

        setSaving(true);
        try {
            const ruleData = {
                uuid: rule.uuid,
                request_model: newConfigRecord.requestModel,
                response_model: newConfigRecord.responseModel,
                active: newConfigRecord.active,
                services: newConfigRecord.providers
                    .filter(p => p.provider && p.model) // Only include providers with both provider and model selected
                    .map(provider => ({
                        provider: provider.provider,
                        model: provider.model,
                        weight: provider.weight || 0,
                        active: provider.active !== undefined ? provider.active : true,
                        time_window: provider.time_window || 0,
                    })),
            };

            const result = await api.updateRule(rule.uuid, ruleData);

            if (result.success) {
                showNotification(`Configuration saved successfully`, 'success');
                // Notify parent of the updated rule
                onRuleChange?.({
                    ...rule,
                    request_model: ruleData.request_model,
                    response_model: ruleData.response_model,
                    active: ruleData.active,
                    services: ruleData.services,
                });
            } else {
                showNotification(`Failed to save: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error saving rule:', error);
            showNotification(`Error saving configuration`, 'error');
        } finally {
            setSaving(false);
        }
    };

    // RuleGraphV2 handlers
    const handleUpdateRecord = (field: keyof ConfigRecord, value: any) => {
        if (configRecord) {
            const updated = {
                ...configRecord,
                [field]: value
            };
            setConfigRecord(updated);
            autoSave(updated);
        }
    };

    const handleUpdateProvider = (_recordId: string, providerId: string, field: keyof ConfigProvider, value: any) => {
        if (configRecord) {
            const updatedProviders = configRecord.providers.map(p => {
                if (p.uuid === providerId) {
                    const updated = { ...p, [field]: value };
                    // If provider changed, reset model
                    if (field === 'provider') {
                        updated.model = '';
                    }
                    return updated;
                }
                return p;
            });

            const updated = {
                ...configRecord,
                providers: updatedProviders
            };
            setConfigRecord(updated);
            autoSave(updated);
        }
    };

    const handleDeleteProvider = (_recordId: string, providerId: string) => {
        if (configRecord) {
            const updated = {
                ...configRecord,
                providers: configRecord.providers.filter(p => p.uuid !== providerId),
            };
            setConfigRecord(updated);
            autoSave(updated);
        }
    };

    // Convert UUID-based providerModels to name-based for ModelSelectTab
    const nameBasedProviderModels = React.useMemo(() => {
        const converted: any = {};
        Object.entries(providerModelsByUuid || {}).forEach(([uuid, data]: [string, any]) => {
            const providerName = providerUuidToName[uuid];
            if (providerName) {
                converted[providerName] = data;
            }
        });
        return converted;
    }, [providerModelsByUuid, providerUuidToName]);

    // Handle provider node click in RuleGraphV2 - open edit dialog
    const handleProviderNodeClick = (providerUuid: string) => {
        setEditingProviderUuid(providerUuid);
        setModelSelectMode('edit');
        setModelSelectDialogOpen(true);
    };

    // Handle add provider button click in RuleGraphV2 - open add dialog
    const handleAddProviderButtonClick = () => {
        setEditingProviderUuid(null);
        setModelSelectMode('add');
        setModelSelectDialogOpen(true);
    };

    // Handle model selection from ModelSelectTab - auto save after selection
    const handleModelSelect = (option: ProviderSelectTabOption) => {
        if (!configRecord) return;

        let updated: ConfigRecord;

        if (modelSelectMode === 'add') {
            // Add new provider with selected values
            updated = {
                ...configRecord,
                providers: [
                    ...configRecord.providers,
                    { uuid: crypto.randomUUID(), provider: option.provider.uuid, model: option.model || '', isManualInput: false },
                ],
            };
        } else if (modelSelectMode === 'edit' && editingProviderUuid) {
            // Update existing provider
            updated = {
                ...configRecord,
                providers: configRecord.providers.map(p => {
                    if (p.uuid === editingProviderUuid) {
                        return { ...p, provider: option.provider.uuid, model: option.model || '' };
                    }
                    return p;
                }),
            };
        } else {
            updated = configRecord;
        }

        setConfigRecord(updated);
        setModelSelectDialogOpen(false);

        // Fetch models for the selected provider
        if (option.provider.uuid) {
            handleFetchModels(option.provider.uuid);
        }

        // Auto save after model selection
        autoSave(updated);
    };

    // Handle provider tab change in ModelSelectTab
    const handleProviderTabChange = (provider: Provider) => {
        handleFetchModels(provider.uuid);
    };

    // If no providers, don't render anything (Home.tsx will show empty state)
    if (!providers.length) {
        return null;
    }

    // Show skeleton while initializing
    if (!isInitialized || !configRecord) {
        return (
            <CardGrid>
                <UnifiedCard size="header">
                    <Box sx={{ p: 3 }}>
                        {/* Simple skeleton loader */}
                        <Box sx={{ height: 60, animation: 'pulse 1.5s ease-in-out infinite' }}>
                            <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', gap: 2 }}>
                                <Box sx={{ width: 40, height: 40, borderRadius: 1, bgcolor: 'grey.200' }} />
                                <Box sx={{ flex: 1 }}>
                                    <Box sx={{ height: 24, width: '60%', bgcolor: 'grey.200', borderRadius: 1, mb: 1 }} />
                                    <Box sx={{ height: 16, width: '40%', bgcolor: 'grey.200', borderRadius: 1 }} />
                                </Box>
                            </Box>
                        </Box>
                    </Box>
                </UnifiedCard>
            </CardGrid>
        );
    }

    return (
        <CardGrid>
            {/* Header Card */}
            <UnifiedCard
                title={title}
                size="header">
                {header || (
                    <Box sx={{ p: 2 }}>
                        <Typography variant="h6" sx={{ fontWeight: 600 }}>
                            {title}
                        </Typography>
                        {subtitle && (
                            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                                {subtitle}
                            </Typography>
                        )}
                    </Box>
                )}

                {/* Rule Configuration using RuleGraphV2 */}
                <RuleGraphV2
                    record={configRecord}
                    recordUuid={configRecord.uuid}
                    providers={providers}
                    providerUuidToName={providerUuidToName}
                    saving={saving}
                    expanded={true}
                    onUpdateRecord={handleUpdateRecord}
                    onDeleteProvider={handleDeleteProvider}
                    onRefreshModels={handleRefreshModels}
                    onToggleExpanded={() => { }}
                    onProviderNodeClick={handleProviderNodeClick}
                    onAddProviderButtonClick={handleAddProviderButtonClick}
                />
            </UnifiedCard>

            {/* Probe Result Section */}
            {configRecord.providers[0]?.provider && configRecord.providers[0]?.model && (
                <UnifiedCard
                    title="Connection Test"
                    size="full"
                    rightAction={
                        <Button
                            variant="outlined"
                            onClick={handleProbe}
                            disabled={!configRecord.providers[0]?.provider || !configRecord.providers[0]?.model || isProbing}
                            startIcon={<ProbeIcon />}
                        >
                            Test Connection
                        </Button>
                    }
                >
                    <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                        <Probe
                            provider={configRecord.providers[0].provider}
                            model={configRecord.providers[0].model}
                            isProbing={isProbing}
                            probeResult={probeResult}
                            onToggleDetails={() => setDetailsExpanded(!detailsExpanded)}
                            detailsExpanded={detailsExpanded}
                        />
                    </Box>
                </UnifiedCard>
            )}

            {/* ModelSelectTab Dialog for provider selection */}
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
                        providerModels={nameBasedProviderModels}
                        selectedProvider={modelSelectMode === 'edit' && editingProviderUuid
                            ? configRecord?.providers.find(p => p.uuid === editingProviderUuid)?.provider
                            : undefined}
                        selectedModel={modelSelectMode === 'edit' && editingProviderUuid
                            ? configRecord?.providers.find(p => p.uuid === editingProviderUuid)?.model
                            : undefined}
                        onSelected={handleModelSelect}
                        onProviderChange={handleProviderTabChange}
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
        </CardGrid>
    );
};

export default TabTemplatePage;
