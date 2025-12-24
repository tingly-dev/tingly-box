import { Add as AddIcon, PlayArrow as ProbeIcon } from '@mui/icons-material';
import { Box, Button, Dialog, DialogContent, DialogTitle, Typography } from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import type { ProbeResponse } from '../client';
import ApiKeyModal from '../components/ApiKeyModal';
import CardGrid from '../components/CardGrid.tsx';
import PresetProviderFormDialog, { type EnhancedProviderFormData } from '../components/PresetProviderFormDialog.tsx';
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
    emptyState?: React.ReactNode;
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
    emptyState
}) => {
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [loading, setLoading] = useState(true);
    const [configRecord, setConfigRecord] = useState<ConfigRecord | null>(null);
    const [saving, setSaving] = useState(false);
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);

    // Probe state
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);

    // Add provider dialog state
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: '',
        token: '',
    });

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

    const loadProviders = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        }
    };

    const loadData = async () => {
        setLoading(true);
        await loadProviders();
        setLoading(false);
    };

    useEffect(() => {
        loadData();
    }, []);

    // Convert rule to ConfigRecord format when rule or providers change
    useEffect(() => {
        if (!loading && rule && providers.length > 0 && !configRecord) {
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

            setConfigRecord({
                uuid: rule.uuid || crypto.randomUUID(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                active: rule.active !== undefined ? rule.active : true,
                providers: providersList,
            });
        }
    }, [loading, rule, providers, configRecord]);

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
            const providerName = providerUuidToName[providerUuid];
            if (!providerName) {
                throw new Error('Provider not found');
            }

            const result = await api.probeModel(providerName, model);
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

    // RuleGraphV2 handlers
    const handleUpdateRecord = (field: keyof ConfigRecord, value: any) => {
        if (configRecord) {
            setConfigRecord({
                ...configRecord,
                [field]: value
            });
        }
    };

    const handleUpdateProvider = (recordId: string, providerId: string, field: keyof ConfigProvider, value: any) => {
        if (configRecord) {
            setConfigRecord({
                ...configRecord,
                providers: configRecord.providers.map(p => {
                    if (p.uuid === providerId) {
                        const updated = { ...p, [field]: value };
                        // If provider changed, reset model
                        if (field === 'provider') {
                            updated.model = '';
                        }
                        return updated;
                    }
                    return p;
                })
            });
        }
    };

    const handleAddProviderWithValues = (providerUuid: string, model: string) => {
        if (configRecord) {
            setConfigRecord({
                ...configRecord,
                providers: [
                    ...configRecord.providers,
                    { uuid: crypto.randomUUID(), provider: providerUuid, model: model, isManualInput: false },
                ],
            });
        }
    };

    const handleDeleteProvider = (_recordId: string, providerId: string) => {
        if (configRecord) {
            setConfigRecord({
                ...configRecord,
                providers: configRecord.providers.filter(p => p.uuid !== providerId),
            });
        }
    };

    const handleSave = async () => {
        if (!configRecord || !configRecord.requestModel) {
            showNotification('Request model name is required', 'error');
            return;
        }

        // Validate providers have both provider and model
        for (const provider of configRecord.providers) {
            if (provider.provider && !provider.model) {
                showNotification(`Please select a model for all providers`, 'error');
                return;
            }
        }

        setSaving(true);
        try {
            // Update the rule on the server
            const ruleData = {
                uuid: rule.uuid,
                request_model: configRecord.requestModel,
                response_model: configRecord.responseModel,
                active: configRecord.active,
                services: configRecord.providers
                    .filter(p => p.provider) // Only include providers with provider selected
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
                showNotification(`Successfully updated ${configRecord.requestModel} configuration`, 'success');
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

    const handleReset = async () => {
        // Reload from server
        await loadData();
        showNotification('Reset to last saved state', 'success');
    };

    const handleDelete = () => {
        showNotification('Delete not supported in template mode', 'warning');
    };

    const handleAddProviderClick = () => {
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
        });
        setAddDialogOpen(true);
    };

    const handleEnhanceProviderFormChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setProviderFormData(prev => ({
            ...prev,
            [field]: value,
        }));
    };

    const handleAddProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification('Provider added successfully!', 'success');
            setProviderFormData({
                name: '',
                apiBase: '',
                apiStyle: undefined,
                token: '',
            });
            setAddDialogOpen(false);
            await loadProviders();

            // Refresh models for the newly added provider
            await handleRefreshModels(result.data.uuid);
        } else {
            showNotification(`Failed to add provider: ${result.error}`, 'error');
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

    // Handle model selection from ModelSelectTab
    const handleModelSelect = (option: ProviderSelectTabOption) => {
        if (modelSelectMode === 'add') {
            // Add new provider with selected values
            handleAddProviderWithValues(option.provider.uuid, option.model || '');
        } else if (modelSelectMode === 'edit' && editingProviderUuid) {
            // Update existing provider
            handleUpdateProvider(configRecord!.uuid, editingProviderUuid, 'provider', option.provider.uuid);
            handleUpdateProvider(configRecord!.uuid, editingProviderUuid, 'model', option.model || '');
        }

        // Fetch models for the selected provider
        if (option.provider.uuid) {
            handleFetchModels(option.provider.uuid);
        }

        setModelSelectDialogOpen(false);
    };

    // Handle provider tab change in ModelSelectTab
    const handleProviderTabChange = (provider: Provider) => {
        handleFetchModels(provider.uuid);
    };

    const defaultEmptyState = (
        <Box textAlign="center" py={8} width={"100%"}>
            <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleAddProviderClick}
                size="large"
                sx={{
                    backgroundColor: 'primary.main',
                    color: 'white',
                    width: 80,
                    height: 80,
                    borderRadius: 2,
                    mb: 3,
                    '&:hover': {
                        backgroundColor: 'primary.dark',
                        transform: 'scale(1.05)',
                    },
                }}
            >
                <AddIcon sx={{ fontSize: 40 }} />
            </Button>
            <Typography variant="h5" sx={{ fontWeight: 600, mb: 2 }}>
                No API Keys Available
            </Typography>
            <Typography variant="body1" color="text.secondary" sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                Get started by adding your first AI API Key.
            </Typography>
            <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleAddProviderClick}
                size="large"
            >
                Add Your First API Key
            </Button>
        </Box>
    );

    return (
        <CardGrid loading={loading}>
            {/* Header Card */}
            <UnifiedCard
                title={title}
                size="header"
            >
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
            </UnifiedCard>

            {/* Rule Configuration using RuleGraphV2 */}
            {providers.length > 0 && configRecord ? (
                <>
                    <UnifiedCard
                        title="Configuration"
                        size="full"
                        rightAction={
                            <Box sx={{ display: 'flex', gap: 1 }}>
                                <Button
                                    variant="outlined"
                                    onClick={handleProbe}
                                    disabled={!configRecord.providers[0]?.provider || !configRecord.providers[0]?.model || isProbing}
                                    startIcon={<ProbeIcon />}
                                >
                                    Test Connection
                                </Button>
                                <Button
                                    variant="contained"
                                    onClick={handleAddProviderClick}
                                >
                                    Add API Key
                                </Button>
                            </Box>
                        }
                    >
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
                            onSave={handleSave}
                            onDelete={handleDelete}
                            onReset={handleReset}
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
                </>
            ) : (
                <UnifiedCard
                    title="Configuration"
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            onClick={handleAddProviderClick}
                        >
                            Add API Key
                        </Button>
                    }
                >
                    {emptyState || defaultEmptyState}
                </UnifiedCard>
            )}

            <PresetProviderFormDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProviderSubmit}
                data={providerFormData}
                onChange={handleEnhanceProviderFormChange}
                mode="add"
            />

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
                <DialogTitle>
                    {modelSelectMode === 'add' ? 'Add Provider' : 'Edit Provider'}
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
