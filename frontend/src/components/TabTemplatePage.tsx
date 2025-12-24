import {Add as AddIcon, PlayArrow as ProbeIcon} from '@mui/icons-material';
import {Box, Button, Stack, Typography} from '@mui/material';
import React, {useCallback, useEffect, useState} from 'react';
import CardGrid from '../components/CardGrid.tsx';
import PresetProviderFormDialog, {type EnhancedProviderFormData} from '../components/PresetProviderFormDialog.tsx';
import Probe from '../components/Probe';
import type {ProviderSelectTabOption} from '../components/ModelSelectTab';
import ModelSelectTab from '../components/ModelSelectTab';
import UnifiedCard from '../components/UnifiedCard';
import ApiKeyModal from '../components/ApiKeyModal';
import {api} from '../services/api';
import type {ProviderModelsDataByUuid} from '../types/provider';
import type {ProbeResponse} from '../client';

export interface TabTemplatePageProps {
    // Card title
    title: string;
    // Optional subtitle/description
    subtitle?: string;
    // Rule name for API
    rule: object;
    // Custom header component (optional - if not provided, will create default)
    header?: React.ReactNode;
    // Token modal state
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    // Custom model select handler (optional)
    onModelSelect?: (provider: any, model: string) => void;
    // Empty state content (optional)
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
                                                             onModelSelect,
                                                             emptyState
                                                         }) => {
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [loading, setLoading] = useState(true);
    const [selectedOption, setSelectedOption] = useState<any>({provider: "", model: ""});
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

    const providerUuidToName = React.useMemo(() => {
        const map: { [uuid: string]: string } = {};
        providers.forEach(provider => {
            map[provider.uuid] = provider.name;
        });
        return map;
    }, [providers]);

    const providerModelsByName = React.useMemo(() => {
        const nameBased: { [name: string]: any } = {};
        Object.entries(providerModelsByUuid).forEach(([uuid, modelData]) => {
            const providerName = providerUuidToName[uuid];
            if (providerName) {
                nameBased[providerName] = modelData;
            }
        });
        return nameBased;
    }, [providerModelsByUuid, providerUuidToName]);

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

    useEffect(() => {
        if (providers.length > 0) {
            let provider;
            if (selectedOption.provider) {
                provider = providers.find(p => p.uuid === selectedOption.provider);
            } else {
                provider = providers.find(p => p.enabled);
            }
            if (provider && !providerModelsByUuid[provider.uuid]) {
                handleProviderChange(provider);
            }
        }
    }, [selectedOption.provider, providers]);

    const handleProbe = useCallback(async () => {
        if (!selectedOption.provider || !selectedOption.model) return;

        setIsProbing(true);
        setProbeResult(null);

        try {
            const providerName = providerUuidToName[selectedOption.provider];
            if (!providerName) {
                throw new Error('Provider not found');
            }

            const result = await api.probeModel(providerName, selectedOption.model);
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
    }, [selectedOption.provider, selectedOption.model, providerUuidToName]);

    const handleModelSelect = async (provider: any, model: string) => {
        setSelectedOption({provider: provider.uuid, model: model});

        if (onModelSelect) {
            onModelSelect(provider, model);
            return;
        } else {
            // Default behavior: update the rule
            try {
                const ruleData = {
                    uuid: rule.uuid,
                    request_model: rule.request_model,
                    active: true,
                    services: [
                        {
                            provider: provider.uuid,
                            model: model,
                            weight: 0,
                            active: true,
                            time_window: 0,
                        }
                    ],
                };

                const existingRule = await api.getRule(rule.uuid);
                let result;
                if (existingRule.success && existingRule.data.uuid) {
                    result = await api.updateRule(rule.uuid, ruleData);
                }
                if (result.success) {
                    showNotification(`Successfully updated ${rule.request_model} forwarding to use ${provider.name}:${model}`, 'success');
                } else {
                    showNotification(`Failed to update rule ${rule.request_model}: ${result.error}`, 'error');
                }
            } catch (error) {
                console.error(`Error updating rule ${rule.request_model}:`, error);
                showNotification(`Error updating rule ${rule.request_model} for ${provider.name}`, 'error');
            }
        }
    };

    const handleProviderChange = async (provider: any) => {
        try {
            if (refreshingProviders.includes(provider.uuid)) {
                return;
            }

            setRefreshingProviders(prev => [...prev, provider.uuid]);

            const result = await api.getProviderModelsByUUID(provider.uuid);
            if (result.success && result.data) {
                setProviderModelsByUuid(prev => ({
                    ...prev,
                    [provider.uuid]: result.data,
                }));
            }
        } catch (error) {
            console.error("Error fetching models on provider change:", error);
        } finally {
            setRefreshingProviders(prev => prev.filter(p => p !== provider.uuid));
        }
    };

    const handleModelRefresh = async (provider: any) => {
        try {
            setRefreshingProviders(prev => [...prev, provider.uuid]);

            const result = await api.updateProviderModelsByUUID(provider.uuid);
            if (result.success && result.data) {
                setProviderModelsByUuid(prev => ({
                    ...prev,
                    [provider.uuid]: result.data,
                }));
                showNotification(`Models for ${provider.name} refreshed successfully!`, 'success');
            } else {
                showNotification(`Failed to refresh models for ${provider.name}.\nPlease check base_url and api_key.`, 'error');
            }
        } catch (error) {
            console.error("Error refreshing models:", error);
            showNotification(`Error refreshing models for ${provider.name}`, 'error');
        } finally {
            setRefreshingProviders(prev => prev.filter(p => p !== provider.uuid));
        }
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

    const handleAddProvider = async (e: React.FormEvent) => {
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

            const newProvider = {name: providerData.name};
            await handleModelRefresh(newProvider);
        } else {
            showNotification(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const defaultEmptyState = (
        <Box textAlign="center" py={8} width={"100%"}>
            <Button
                variant="contained"
                startIcon={<AddIcon/>}
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
                <AddIcon sx={{fontSize: 40}}/>
            </Button>
            <Typography variant="h5" sx={{fontWeight: 600, mb: 2}}>
                No API Keys Available
            </Typography>
            <Typography variant="body1" color="text.secondary" sx={{mb: 3, maxWidth: 500, mx: 'auto'}}>
                Get started by adding your first AI API Key.
            </Typography>
            <Button
                variant="contained"
                startIcon={<AddIcon/>}
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
                    <Box sx={{p: 2}}>
                        <Typography variant="h6" sx={{fontWeight: 600}}>
                            {title}
                        </Typography>
                        {subtitle && (
                            <Typography variant="body2" color="text.secondary" sx={{mt: 0.5}}>
                                {subtitle}
                            </Typography>
                        )}
                    </Box>
                )}
            </UnifiedCard>

            {/* Provider Selection */}
            <UnifiedCard
                title="Providers"
                size="full"
                rightAction={
                    <Box sx={{display: 'flex', gap: 1}}>
                        <Button
                            variant="outlined"
                            onClick={handleProbe}
                            disabled={!selectedOption.provider || !selectedOption.model || isProbing}
                            startIcon={<ProbeIcon/>}
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
                {providers.length > 0 ? (
                    <Stack spacing={3}>
                        <ModelSelectTab
                            providers={providers}
                            providerModels={providerModelsByName}
                            selectedProvider={selectedOption?.provider}
                            selectedModel={selectedOption?.model}
                            onSelected={(opt: ProviderSelectTabOption) => handleModelSelect(opt.provider, opt.model || "")}
                            onProviderChange={handleProviderChange}
                            onRefresh={handleModelRefresh}
                            refreshingProviders={refreshingProviders}
                        />

                        {selectedOption.provider && selectedOption.model && (
                            <Box sx={{display: 'flex', justifyContent: 'center'}}>
                                <Probe
                                    provider={selectedOption.provider}
                                    model={selectedOption.model}
                                    isProbing={isProbing}
                                    probeResult={probeResult}
                                    onToggleDetails={() => setDetailsExpanded(!detailsExpanded)}
                                    detailsExpanded={detailsExpanded}
                                />
                            </Box>
                        )}
                    </Stack>
                ) : (
                    emptyState || defaultEmptyState
                )}
            </UnifiedCard>

            <PresetProviderFormDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                data={providerFormData}
                onChange={handleEnhanceProviderFormChange}
                mode="add"
            />

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
