import {
    Add as AddIcon,
    ContentCopy as CopyIcon,
    PlayArrow as ProbeIcon
} from '@mui/icons-material';
import {
    AlertTitle,
    Box,
    Button,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Typography
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ProbeResponse } from '../client';
import CardGrid from '../components/CardGrid.tsx';
import PresetProviderFormDialog, { type EnhancedProviderFormData } from '../components/PresetProviderFormDialog.tsx';
import { HomeHeader } from '../components/HomeHeader.tsx';
import ModelSelectTab, { type ProviderSelectTabOption } from "../components/ModelSelectTab.tsx";
import { PageLayout } from '../components/PageLayout';
import Probe from '../components/Probe';
import UnifiedCard from '../components/UnifiedCard';
import { api, getBaseUrl } from '../services/api';
import type { ProviderModelsDataByUuid } from '../types/provider';

const defaultRule = "tingly"
const defaultRuleUUID = "tingly"


const Home = () => {
    const navigate = useNavigate();
    const [providers, setProviders] = useState<any[]>([]);
    const [rule, setRule] = useState<any>({});
    // UUID-based provider models mapping
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [loading, setLoading] = useState(true);
    const [selectedOption, setSelectedOption] = useState<any>({ provider: "", model: "" }); // provider is now UUID
    const [baseUrl, setBaseUrl] = useState<string>("");
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]); // These are UUIDs

    // Server info states
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [activeTab, setActiveTab] = useState(0);

    // Banner state for provider/model selection
    const [bannerProvider, setBannerProvider] = useState<string>('');
    const [bannerModel, setBannerModel] = useState<string>('');
    const [showBanner, setShowBanner] = useState(false);

    // Create lookup maps for provider UUID to name
    const providerUuidToName = React.useMemo(() => {
        const map: { [uuid: string]: string } = {};
        providers.forEach(provider => {
            map[provider.uuid] = provider.name;
        });
        return map;
    }, [providers]);

    // Transform UUID-based provider models to name-based for ModelSelectTab (legacy compatibility)
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

    // Unified notification state
    const [notification, setNotification] = useState<{
        open: boolean;
        message?: string;
        severity?: 'success' | 'info' | 'warning' | 'error';
        autoHideDuration?: number;
        customContent?: React.ReactNode;
        onClose?: () => void;
    }>({ open: false });

    // Probe state and logic
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);

    const handleProbe = useCallback(async () => {
        if (!selectedOption.provider || !selectedOption.model) return;

        setIsProbing(true);
        setProbeResult(null);

        try {
            const providerName = providerUuidToName[selectedOption.provider];
            if (!providerName) {
                throw new Error('Provider not found');
            }

            console.log(providerName, selectedOption.model);
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

    // Helper function to show notifications
    const showNotification = (message: string, severity: 'success' | 'info' | 'warning' | 'error' = 'info', autoHideDuration: number = 6000) => {
        setNotification({
            open: true,
            message,
            severity,
            autoHideDuration,
            onClose: () => setNotification(prev => ({ ...prev, open: false }))
        });
    };

    // Helper function to show banner notification
    const showBannerNotification = () => {
        if (showBanner && bannerProvider && bannerModel) {
            setNotification({
                open: true,
                autoHideDuration: 0, // Don't auto-hide banner notification
                customContent: (
                    <>
                        <AlertTitle>Active Provider & Model</AlertTitle>
                        <Typography variant="body2">
                            <strong>Request:</strong> tingly {" -> "}
                            <strong>Provider:</strong> {bannerProvider} | <strong>Model:</strong> {bannerModel}
                        </Typography>
                    </>
                ),
                severity: 'info',
                onClose: () => {
        setShowBanner(false);
                    setNotification(prev => ({ ...prev, open: false }));
                }
            });
        }
    };

    // Show banner notification when banner should be displayed
    React.useEffect(() => {
        if (showBanner) {
            showBannerNotification();
        } else {
            setNotification(prev => ({ ...prev, open: false }));
        }
    }, [showBanner, bannerProvider, bannerModel]);

    // Add provider dialog state
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: '',
        token: '',
    });

    const loadBaseUrl = async () => {
        const baseUrl = await getBaseUrl();
        setBaseUrl(baseUrl);
    };

    useEffect(() => {
        loadBaseUrl()
        loadData();
        loadToken();
    }, []);


    // Update selected option when rules are loaded
    useEffect(() => {
        if (rule && rule.services && rule.services.length > 0) {
            const firstService = rule.services[0];
            setSelectedOption({
                provider: firstService.provider,
                model: firstService.model
            });
        } else {
            setSelectedOption({
                provider: '',
                model: ''
            });
        }
    }, [rule]);

    const loadToken = async () => {
        const result = await api.getToken();
        if (result.token) {
            setApiKey(result.token);
        }
    };

    const loadData = async () => {
        setLoading(true);
        await Promise.all([
            loadBaseUrl(),
            loadProviders(),
            loadRule(),
        ]);
        setLoading(false);
    };

    const loadProviders = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        }
    };

    const loadRule = async () => {
        const result = await api.getRule(defaultRule);
        if (result.success) {
            setRule(result.data);
        }
        // Remove automatic rule creation - rule should only be created when user selects a provider/model
    };


    // Server info handlers
    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    const generateToken = async () => {
        const clientId = 'web';
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            copyToClipboard(result.data.token, 'Token');
        } else {
            showNotification(`Failed to generate token: ${result.error}`, 'error');
        }
    };

    // Composition handlers for provider select
    const handleModelSelect = async (provider: any, model: string) => {
        setSelectedOption({ provider: provider.uuid, model: model });

        try {
            // Update the "tingly" rule with the selected provider and model
            const ruleData = {
                uuid: defaultRuleUUID,
                request_model: defaultRule,
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

            const existingRule = await api.getRule(defaultRuleUUID);
            let result;
            if (existingRule.success && existingRule.data.uuid) {
                // Update existing rule using uuid
                result = await api.updateRule(existingRule.data.uuid, ruleData);
            } else {
                // Create new rule if it doesn't exist
                const createResult = await api.createRule(
                    defaultRuleUUID,
                    {
                        name: 'tingly',
                        ...ruleData,
                    });
                result = createResult;
            }
            if (result.success) {
                // Show banner with selected provider and model info
                setBannerProvider(provider.name);
                setBannerModel(model);
                setShowBanner(true);
                showNotification(`Successfully updated tingly rule to use ${provider.name}:${model}`, 'success');
                // Reload rule to get updated data
                const reloadResult = await api.getRule('tingly');
                if (reloadResult.success) {
                    setRule(reloadResult.data);
                }
            } else {
                showNotification(`Failed to update tingly rule: ${result.error}`, 'error');
            }
        } catch (error) {
            console.error("Error updating tingly rule:", error);
            showNotification(`Error updating tingly rule for ${provider.name}`, 'error');
        }
    };

    const handleProviderChange = async (provider: any) => {
        // Called when user switches to a provider tab
        // Fetch models for this provider using UUID
        try {
            // Check if already refreshing to avoid duplicate requests
            if (refreshingProviders.includes(provider.uuid)) {
                return;
            }

            // Add provider UUID to refreshing list
            setRefreshingProviders(prev => [...prev, provider.uuid]);

            const result = await api.getProviderModelsByUUID(provider.uuid);
            if (result.success && result.data) {
                // Update UUID-based mapping with the fetched models
                setProviderModelsByUuid(prev => ({
                    ...prev,
                    [provider.uuid]: result.data,
                }));
            }
            // Note: We don't show notification here to avoid spamming user on tab switch
        } catch (error) {
            console.error("Error fetching models on provider change:", error);
        } finally {
            // Remove provider UUID from refreshing list
            setRefreshingProviders(prev => prev.filter(p => p !== provider.uuid));
        }
    };

    const handleModelRefresh = async (provider: any) => {
        try {
            // Add provider UUID to refreshing list
            setRefreshingProviders(prev => [...prev, provider.uuid]);

            const result = await api.updateProviderModelsByUUID(provider.uuid);
            if (result.success && result.data) {
                // Update UUID-based mapping with the fetched models
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
            // Remove provider UUID from refreshing list
            setRefreshingProviders(prev => prev.filter(p => p !== provider.uuid));
        }
    };

    // Provider dialog handlers
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

            // Automatically fetch models for the newly added provider
            const newProvider = { name: providerData.name };
            await handleModelRefresh(newProvider);
        } else {
            showNotification(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const openaiBaseUrl = `${baseUrl}/openai`;
    const anthropicBaseUrl = `${baseUrl}/anthropic`;
    const token = generatedToken || apiKey;

    const ApiKeyModal = () => {
        return (
            <Dialog
                open={showTokenModal}
                onClose={() => setShowTokenModal(false)}
                maxWidth="md"
                fullWidth
            >
                <DialogTitle>API Key</DialogTitle>
                <DialogContent>
                    <Box sx={{ mb: 2 }}>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                            Your authentication token:
                        </Typography>
                        <Box
                            onClick={() => copyToClipboard(token, 'API Key')}
                            sx={{
                                p: 2,
                                bgcolor: 'grey.100',
                                borderRadius: 1,
                                fontFamily: 'monospace',
                                fontSize: '0.85rem',
                                wordBreak: 'break-all',
                                border: '1px solid',
                                borderColor: 'grey.300',
                                cursor: 'pointer',
                                '&:hover': {
                                    backgroundColor: 'grey.200',
                                    borderColor: 'primary.main'
                                },
                                transition: 'all 0.2s ease-in-out',
                                title: 'Click to copy token'
                            }}
                        >
                            {token}
                        </Box>
                    </Box>
                    <Box sx={{ display: 'flex', gap: 1 }}>
                        <Button
                            variant="outlined"
                            onClick={() => copyToClipboard(token, 'API Key')}
                            startIcon={<CopyIcon fontSize="small" />}
                        >
                            Copy Token
                        </Button>
                    </Box>
                </DialogContent>
            </Dialog>

        )
    }

    const Guiding = () => {
        return (
            <Box textAlign="center" py={8} width={"100%"}>
                <IconButton
                    size="large"
                    onClick={handleAddProviderClick}
                    sx={{
                        backgroundColor: 'primary.main',
                        color: 'white',
                        width: 80,
                        height: 80,
                        mb: 3,
                        '&:hover': {
                            backgroundColor: 'primary.dark',
                            transform: 'scale(1.05)',
                        },
                    }}
                >
                    <AddIcon sx={{ fontSize: 40 }} />
                </IconButton>
                <Typography variant="h5" sx={{ fontWeight: 600, mb: 2 }}>
                    No API Keys Available
                </Typography>
                <Typography variant="body1" color="text.secondary"
                    sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                    Get started by adding your first AI API Key.
                    You can connect to OpenAI, Anthropic, or any compatible API endpoint.
                </Typography>
                <Typography variant="body2" color="text.secondary"
                    sx={{ mb: 4, maxWidth: 400, mx: 'auto' }}>
                    <strong>Steps to get started:</strong><br />
                    1. Click the + button to add model api key<br />
                    2. Select your preferred model
                    3. Use tingly box model config to access
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
        )
    }

    return (
        <PageLayout
            loading={loading}
            notification={notification}
        >
            <CardGrid>
                {/* Server Information Header */}
                <UnifiedCard
                    title="Proxy Configs"
                    size="header"
                >
                    <HomeHeader
                        activeTab={activeTab}
                        setActiveTab={setActiveTab}
                        openaiBaseUrl={openaiBaseUrl}
                        anthropicBaseUrl={anthropicBaseUrl}
                        token={token}
                        showTokenModal={showTokenModal}
                        setShowTokenModal={setShowTokenModal}
                        generateToken={generateToken}
                        copyToClipboard={copyToClipboard}
                    />
                </UnifiedCard>

                <UnifiedCard
                    title="Providers"
                    size="full"
                    rightAction={
                        <Box sx={{ display: 'flex', gap: 1 }}>
                            <Button
                                variant="outlined"
                                onClick={handleProbe}
                                disabled={!selectedOption.provider || !selectedOption.model || isProbing}
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

                            {/* Probe Component - only show when provider and model are selected */}
                            {selectedOption.provider && selectedOption.model && (
                                <Box sx={{ display: 'flex', justifyContent: 'center' }}>
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
                        <Guiding />
                    )}

                </UnifiedCard>
            </CardGrid>

            {/* Token Modal */}
            <ApiKeyModal />

            {/* Add Provider Dialog */}
            <PresetProviderFormDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                data={providerFormData}
                onChange={handleEnhanceProviderFormChange}
                mode="add"
            />
        </PageLayout>
    );
};

export default Home;
