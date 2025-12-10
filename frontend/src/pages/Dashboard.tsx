import {
    Add as AddIcon,
    ContentCopy as CopyIcon,
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
import React, { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import Probe from '../components/Probe';
import { ProviderDialog } from '../components/ProviderDialog';
import ProviderSelectTab, { type ProviderSelectTabOption } from "../components/ProviderSelectTab.tsx";
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const defaultRule = "tingly"
const defaultRuleUUID = "tingly"


const Dashboard = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [rule, setRule] = useState<any>({});
    const [providerModels, setProviderModels] = useState<any>({});
    const [loading, setLoading] = useState(true);
    const [selectedOption, setSelectedOption] = useState<any>({ provider: "", model: "" });

    // Server info states
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [modelToken, setModelToken] = useState<string>('');
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
        customContent?: React.ReactNode;
        onClose?: () => void;
    }>({ open: false });

    // Helper function to show notifications
    const showNotification = (message: string, severity: 'success' | 'info' | 'warning' | 'error' = 'info') => {
        setNotification({
            open: true,
            message,
            severity,
            onClose: () => setNotification(prev => ({ ...prev, open: false }))
        });
    };

    // Helper function to show banner notification
    const showBannerNotification = () => {
        if (showBanner && bannerProvider && bannerModel) {
            setNotification({
                open: true,
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
    const [providerName, setProviderName] = useState('');
    const [providerApiBase, setProviderApiBase] = useState('');
    const [providerApiStyle, setProviderApiStyle] = useState('openai');
    const [providerToken, setProviderToken] = useState('');

    useEffect(() => {
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
        if (result.success && result.data && result.data.token) {
            setModelToken(result.data.token);
        }
    };

    const loadData = async () => {
        setLoading(true);
        await Promise.all([
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
        } else {
            // If the 'tingly' rule doesn't exist, create a default one
            await createDefaultTinglyRule();
        }
    };

    const createDefaultTinglyRule = async () => {
        try {
            // Create a default rule with empty services
            // This will be filled when user selects a provider and model
            const defaultRuleData = {
                response_model: undefined,
                services: [],
            };

            const result = await api.createRule({
                name: defaultRule,
                ...defaultRuleData
            });
            if (result.success) {
                // Reload the rule after creating it
                const reloadResult = await api.getRule(defaultRule);
                if (reloadResult.success) {
                    setRule(reloadResult.data);
                }
            } else {
                console.error(`Failed to create default '${defaultRule}' rule:`, result.error);
                // Show notification to user about the failure
                showNotification(`Failed to create default rule: ${result.error}`, 'error');
            }
        } catch (error) {
            console.error(`Error creating default '${defaultRule}' rule:`, error);
            showNotification(`Error creating default rule`, 'error');
        }
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
                const createResult = await api.createRule({
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
        setProviderName('');
        setProviderApiBase('');
        setProviderApiStyle('openai');
        setProviderToken('');
        setAddDialogOpen(true);
    };

    const handleAddProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerName,
            api_base: providerApiBase,
            api_style: providerApiStyle,
            token: providerToken,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification('Provider added successfully!', 'success');
            setProviderName('');
            setProviderApiBase('');
            setProviderApiStyle('openai');
            setProviderToken('');
            setAddDialogOpen(false);
            await loadProviders();
        } else {
            showNotification(`Failed to add provider: ${result.error}`, 'error');
        }
    };

    const baseUrl = import.meta.env.VITE_API_BASE_URL || window.location.origin;
    const openaiBaseUrl = `${baseUrl}/openai/v1`;
    const anthropicBaseUrl = `${baseUrl}/anthropic/v1`;
    const token = generatedToken || modelToken;

    return (
        <PageLayout
            loading={loading}
            notification={notification}
        >

            {/* Server Information Header */}
            <UnifiedCard
                title="Switch Provider & Model"
                subtitle={`Total: ${providers.length} providers | Enabled: ${providers.filter((p: any) => p.enabled).length}`}
                size="full"
                rightAction={
                    <Box>
                        <Button
                            variant="contained"
                            onClick={() => window.location.href = '/providers'}
                        >
                            Manage Providers
                        </Button>
                    </Box>
                }
            >

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, md: 6 }}>
                        <Stack spacing={1}>
                            {/* OpenAI Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 80 }}>
                                    OpenAI Base URL:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.7rem',
                                        color: 'primary.main',
                                        flex: 1,
                                        minWidth: 0
                                    }}
                                >
                                    {baseUrl}/openai/v1
                                </Typography>
                                <Stack direction="row" spacing={0.2}>
                                    <IconButton
                                        onClick={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')}
                                        size="small"
                                        title="Copy OpenAI Base URL"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                    <IconButton
                                        onClick={() => {
                                            const openaiCurl = `curl -X POST "${openaiBaseUrl}/chat/completions" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}]}'`;
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
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 80 }}>
                                    Anthropic Base URL:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.7rem',
                                        color: 'primary.main',
                                        flex: 1,
                                        minWidth: 0
                                    }}
                                >
                                    {baseUrl}/anthropic
                                </Typography>
                                <Stack direction="row" spacing={0.2}>
                                    <IconButton
                                        onClick={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                                        size="small"
                                        title="Copy Anthropic Base URL"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                    <IconButton
                                        onClick={() => {
                                            const anthropicCurl = `curl -X POST "${anthropicBaseUrl}/messages" -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"messages": [{"role": "user", "content": "Hello!"}], "max_tokens": 100}'`;
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
                        <Stack spacing={1}>
                            {/* Token Row */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 60 }}>
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
                                        minWidth: 0
                                    }}
                                >
                                    ••••••••••••••••
                                </Typography>
                                <Stack direction="row" spacing={0.2}>
                                    <Tooltip title="View Token">
                                        <IconButton
                                            onClick={() => setShowTokenModal(true)}
                                            size="small"
                                            title="View Token"
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
                                        onClick={() => copyToClipboard(token, 'API Token')}
                                        size="small"
                                        title="Copy Token"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>

                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 60 }}>
                                    LLM API Model:
                                </Typography>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.8rem',
                                        color: 'text.secondary',
                                        letterSpacing: '2px',
                                        flex: 1,
                                        minWidth: 0
                                    }}
                                >
                                    tingly
                                </Typography>
                                <Stack direction="row" spacing={0.2}>
                                    <IconButton
                                        onClick={() => copyToClipboard(token, 'API Token')}
                                        size="small"
                                        title="Copy Token"
                                    >
                                        <CopyIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </Box>
                        </Stack>
                    </Grid>


                    {providers.length > 0 ? (

                        <Grid size={{ xs: 12, md: 12 }}>
                            <Stack spacing={2}>
                                <ProviderSelectTab
                                    providers={providers}
                                    providerModels={providerModels}
                                    selectedProvider={selectedOption?.provider}
                                    selectedModel={selectedOption?.model}
                                    onSelected={(opt: ProviderSelectTabOption) => handleModelSelect(opt.provider, opt.model || "")}
                                    onRefresh={handleModelRefresh}
                                />
                            </Stack>
                        </Grid>
                    ) : (
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
                                2. Configure your API credentials<br />
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
                    )}
                </Grid>
            </UnifiedCard>

            {/* Token Modal */}
            <Dialog
                open={showTokenModal}
                onClose={() => setShowTokenModal(false)}
                maxWidth="md"
                fullWidth
            >
                <DialogTitle>API Token</DialogTitle>
                <DialogContent>
                    <Box sx={{ mb: 2 }}>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                            Your authentication token:
                        </Typography>
                        <Box sx={{
                            p: 2,
                            bgcolor: 'grey.100',
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.85rem',
                            wordBreak: 'break-all',
                            border: '1px solid',
                            borderColor: 'grey.300'
                        }}>
                            {token}
                        </Box>
                    </Box>
                    <Box sx={{ display: 'flex', gap: 1 }}>
                        <Button
                            variant="outlined"
                            onClick={() => copyToClipboard(token, 'API Token')}
                            startIcon={<CopyIcon fontSize="small" />}
                        >
                            Copy Token
                        </Button>
                    </Box>
                </DialogContent>
            </Dialog>

            {/* Add Provider Dialog */}
            <ProviderDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddProvider}
                providerName={providerName}
                onProviderNameChange={setProviderName}
                providerApiBase={providerApiBase}
                onProviderApiBaseChange={setProviderApiBase}
                providerApiStyle={providerApiStyle}
                onProviderApiStyleChange={setProviderApiStyle}
                providerToken={providerToken}
                onProviderTokenChange={setProviderToken}
            />

            {/* Probe Component */}

            <Probe rule="tingly" provider={selectedOption.provider} model={selectedOption.model} />

        </PageLayout>
    );
};

export default Dashboard;
