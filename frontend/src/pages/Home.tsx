import {
    Add as AddIcon,
    ContentCopy as CopyIcon,
    PlayArrow as ProbeIcon,
    Refresh as RefreshIcon,
    Terminal as TerminalIcon
} from '@mui/icons-material';
import {
    AlertTitle,
    Box,
    Button,
    Dialog,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Stack,
    Tooltip,
    Typography
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ProbeResponse } from '../client';
import CredentialFormDialog, { type ProviderFormData } from '../components/CredentialFormDialog.tsx';
import ModelSelectTab, { type ProviderSelectTabOption } from "../components/ModelSelectTab.tsx";
import { PageLayout } from '../components/PageLayout';
import Probe from '../components/Probe';
import UnifiedCard from '../components/UnifiedCard';
import { api, getBaseUrl } from '../services/api';

const defaultRule = "tingly"
const defaultRuleUUID = "tingly"


const Home = () => {
    const navigate = useNavigate();
    const [providers, setProviders] = useState<any[]>([]);
    const [rule, setRule] = useState<any>({});
    const [providerModels, setProviderModels] = useState<any>({});
    const [loading, setLoading] = useState(true);
    const [selectedOption, setSelectedOption] = useState<any>({ provider: "", model: "" });
    const [baseUrl, setBaseUrl] = useState<string>("");

    // Server info states
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    const [showTokenModal, setShowTokenModal] = useState(false);

    // Banner state for provider/model selection
    const [bannerProvider, setBannerProvider] = useState<string>('');
    const [bannerModel, setBannerModel] = useState<string>('');
    const [showBanner, setShowBanner] = useState(false);

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
            console.log(selectedOption.provider, selectedOption.model);
            const result = await api.probeModel(selectedOption.provider, selectedOption.model);
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
    }, [selectedOption.provider, selectedOption.model]);

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
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: 'openai',
        token: '',
    });

    const loadBaseUrl = async () => {
        const baseUrl = await getBaseUrl()
        setBaseUrl(baseUrl)
    }

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
        console.log(result)
        if (result.token) {
            setApiKey(result.token);
        }
    };

    const loadData = async () => {
        setLoading(true);
        await Promise.all([
            loadBaseUrl(),
            loadProviders(),
            loadProviderModels(),
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

    const loadProviderModels = async () => {
        const result = await api.getProviderModels();
        if (result.success) {
            setProviderModels(result.data);
        }
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
        setSelectedOption({ provider: provider.name, model: model });

        try {
            // Update the "tingly" rule with the selected provider and model
            const ruleData = {
                uuid: defaultRuleUUID,
                request_model: defaultRule,
                active: true,
                services: [
                    {
                        provider: provider.name,
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

    const handleModelRefresh = async (provider: any) => {
        try {
            const result = await api.getProviderModelsByName(provider.name);
            if (result.success) {
                await loadProviders();
                await loadProviderModels();
                showNotification(`Models for ${provider.name} refreshed successfully!`, 'success');
            } else {
                showNotification(`Failed to refresh models for ${provider.name}`, 'error');
            }
        } catch (error) {
            console.error("Error refreshing models:", error);
            showNotification(`Error refreshing models for ${provider.name}`, 'error');
        }
    };

    // Provider dialog handlers
    const handleAddProviderClick = () => {
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: 'openai',
            token: '',
        });
        setAddDialogOpen(true);
    };

    const handleProviderFormChange = (field: keyof ProviderFormData, value: any) => {
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
                apiStyle: 'openai',
                token: '',
            });
            setAddDialogOpen(false);
            await loadProviders();
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
                    No Providers Available
                </Typography>
                <Typography variant="body1" color="text.secondary"
                    sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                    Get started by adding your first AI provider. You can connect to OpenAI, Anthropic, or
                    any compatible API endpoint.
                </Typography>
                <Typography variant="body2" color="text.secondary"
                    sx={{ mb: 4, maxWidth: 400, mx: 'auto' }}>
                    <strong>Steps to get started:</strong><br />
                    1. Click the + button to add a provider<br />
                    2. Configure your API keys<br />
                    3. Select your preferred model
                </Typography>
                <Button
                    variant="contained"
                    startIcon={<AddIcon />}
                    onClick={handleAddProviderClick}
                    size="large"
                >
                    Add Your First Provider
                </Button>
            </Box>
        )
    }


    const Header = () => {
        return (
            <>
                <Grid container spacing={3}>
                    <Grid size={{ xs: 12, md: 6 }}>
                        <Stack spacing={2}>
                            {/* OpenAI Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                                <Typography
                                    variant="body2"
                                    color="text.secondary"
                                    sx={{
                                        minWidth: 120,
                                        flexShrink: 0,
                                        fontWeight: 500
                                    }}
                                >
                                    OpenAI Base URL:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    onClick={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')}
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.75rem',
                                        color: 'primary.main',
                                        flex: 1,
                                        cursor: 'pointer',
                                        '&:hover': {
                                            textDecoration: 'underline',
                                            backgroundColor: 'action.hover'
                                        },
                                        padding: 1,
                                        borderRadius: 1,
                                        transition: 'all 0.2s ease-in-out'
                                    }}
                                    title="Click to copy OpenAI Base URL"
                                >
                                    {baseUrl}/openai
                                </Typography>
                                <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                    <IconButton
                                        onClick={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')}
                                        size="small"
                                        title="Copy OpenAI Base URL"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                    <IconButton
                                        onClick={() => {
                                            const openaiCurl = `curl -X POST "${openaiBaseUrl}/v1/chat/completions" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}]}'`;
                                            copyToClipboard(openaiCurl, 'OpenAI cURL command');
                                        }}
                                        size="small"
                                        title="Copy OpenAI cURL Example"
                                    >
                                        <TerminalIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>

                            {/* Anthropic Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                                <Typography
                                    variant="body2"
                                    color="text.secondary"
                                    sx={{
                                        minWidth: 120,
                                        flexShrink: 0,
                                        fontWeight: 500
                                    }}
                                >
                                    Anthropic Base URL:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    onClick={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.75rem',
                                        color: 'primary.main',
                                        flex: 1,
                                        cursor: 'pointer',
                                        '&:hover': {
                                            textDecoration: 'underline',
                                            backgroundColor: 'action.hover'
                                        },
                                        padding: 1,
                                        borderRadius: 1,
                                        transition: 'all 0.2s ease-in-out'
                                    }}
                                    title="Click to copy Anthropic Base URL"
                                >
                                    {baseUrl}/anthropic
                                </Typography>
                                <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                    <IconButton
                                        onClick={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                                        size="small"
                                        title="Copy Anthropic Base URL"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                    <IconButton
                                        onClick={() => {
                                            const anthropicCurl = `curl -X POST "${anthropicBaseUrl}/v1/messages" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}], "max_tokens": 100}'`;
                                            copyToClipboard(anthropicCurl, 'Anthropic cURL command');
                                        }}
                                        size="small"
                                        title="Copy Anthropic cURL Example"
                                    >
                                        <TerminalIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>
                        </Stack>
                    </Grid>

                    <Grid size={{ xs: 12, md: 6 }}>
                        <Stack spacing={2}>
                            {/* Token Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                                <Typography
                                    variant="body2"
                                    color="text.secondary"
                                    sx={{
                                        minWidth: 120,
                                        flexShrink: 0,
                                        fontWeight: 500
                                    }}
                                >
                                    LLM API KEY:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.8rem',
                                        color: 'text.secondary',
                                        letterSpacing: '2px',
                                        flex: 1,
                                        cursor: 'default',
                                        userSelect: 'none'
                                    }}
                                >
                                    ••••••••••••••••
                                </Typography>
                                <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                    <Tooltip title="View Token">
                                        <IconButton
                                            onClick={() => setShowTokenModal(true)}
                                            size="small"
                                        >
                                            <Typography variant="caption">
                                                👁️
                                            </Typography>
                                        </IconButton>
                                    </Tooltip>
                                    <IconButton
                                        onClick={generateToken}
                                        size="small"
                                        title="Generate New Token"
                                    >
                                        <RefreshIcon fontSize="small" />
                                    </IconButton>
                                    <IconButton
                                        onClick={() => copyToClipboard(token, 'API Key')}
                                        size="small"
                                        title="Copy Token"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>

                            {/* Model Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                                <Typography
                                    variant="body2"
                                    color="text.secondary"
                                    sx={{
                                        minWidth: 120,
                                        flexShrink: 0,
                                        fontWeight: 500
                                    }}
                                >
                                    LLM API Model:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    onClick={() => copyToClipboard('tingly', 'LLM API Model')}
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.8rem',
                                        color: 'text.secondary',
                                        letterSpacing: '2px',
                                        flex: 1,
                                        cursor: 'pointer',
                                        '&:hover': {
                                            textDecoration: 'underline',
                                            backgroundColor: 'action.hover'
                                        },
                                        padding: 1,
                                        borderRadius: 1,
                                        transition: 'all 0.2s ease-in-out'
                                    }}
                                    title="Click to copy LLM API Model"
                                >
                                    tingly
                                </Typography>
                                <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                    <IconButton
                                        onClick={() => copyToClipboard('tingly', 'LLM API Model')}
                                        size="small"
                                        title="Copy Model"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>
                        </Stack>
                    </Grid>
                </Grid>
            </>
        )
    }
    return (
        <PageLayout
            loading={loading}
            notification={notification}
        >
            {/* Server Information Header */}
            <UnifiedCard
                title="Model Proxy Config"
                // subtitle={`Total: ${providers.length} providers | Enabled: ${providers.filter((p: any) => p.enabled).length}`}
                size="header"

            >
                <Header></Header>
            </UnifiedCard>

            <UnifiedCard
                title="Choose Model"
                size={"full"}
                height={"100%"}
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
                            onClick={() => navigate('/provider')}
                        >
                            Manage Credentials
                        </Button>
                    </Box>
                }
            >

                {providers.length > 0 ? (
                    <Stack spacing={3}>
                        <ModelSelectTab
                            providers={providers}
                            providerModels={providerModels}
                            selectedProvider={selectedOption?.provider}
                            selectedModel={selectedOption?.model}
                            onSelected={(opt: ProviderSelectTabOption) => handleModelSelect(opt.provider, opt.model || "")}
                            onRefresh={handleModelRefresh}
                        />

                        {/* Probe Component - only show when provider and model are selected */}
                        {selectedOption.provider && selectedOption.model && (
                            <Box sx={{ display: 'flex', justifyContent: 'center' }}>
                                {/* <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1, fontSize: '0.875rem' }}>
                                    Connection Status
                                </Typography> */}
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
                    <Guiding></Guiding>
                )}

            </UnifiedCard>

            {/* Token Modal */}
            <ApiKeyModal></ApiKeyModal>

            {/* Add Provider Dialog */}
            <CredentialFormDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                data={providerFormData}
                onChange={handleProviderFormChange}
                mode="add"
            />
        </PageLayout>
    );
};

export default Home;
