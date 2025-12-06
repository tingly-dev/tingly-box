import { Add as AddIcon, ContentCopy as CopyIcon, Info as InfoIcon, Refresh as RefreshIcon, Terminal as TerminalIcon } from '@mui/icons-material';
import {
    Alert,
    AlertTitle,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Snackbar,
    Stack,
    Tooltip,
    Typography
} from '@mui/material';
import { useEffect, useState } from 'react';
import { ProviderDialog } from '../components/ProviderDialog';
import { ProviderSelect } from '../components/ProviderSelect';
import ProviderSelectTab, { type ProviderSelectTabOption } from "../components/ProviderSelectTab.tsx";
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const Dashboard = () => {
    const [providers, setProviders] = useState<any[]>([]);
    const [defaults, setDefaults] = useState<any>({});
    const [providerModels, setProviderModels] = useState<any>({});
    const [loading, setLoading] = useState(true);
    const [selectedOption, setSelectedOption] = useState<any>({ provider: "", model: "" });

    // Composition state for provider select
    const [expandedProviders, setExpandedProviders] = useState<string[]>([]);
    const [searchTerms, setSearchTerms] = useState<{ [key: string]: string }>({});
    const [currentPage, setCurrentPage] = useState<{ [key: string]: number }>({});

    // Server info states
    const [snackbarOpen, setSnackbarOpen] = useState(false);
    const [snackbarMessage, setSnackbarMessage] = useState('');
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [modelToken, setModelToken] = useState<string>('');
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [llmModel, setLLMModel] = useState<string>("tingly");

    // Banner state for provider/model selection
    const [showBanner, setShowBanner] = useState(false);
    const [bannerProvider, setBannerProvider] = useState<string>('');
    const [bannerModel, setBannerModel] = useState<string>('');

    // Add provider dialog state
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [providerName, setProviderName] = useState('');
    const [providerApiBase, setProviderApiBase] = useState('');
    const [providerApiVersion, setProviderApiVersion] = useState('openai');
    const [providerToken, setProviderToken] = useState('');

    useEffect(() => {
        loadAllData();
        loadToken();
    }, []);


    const loadToken = async () => {
        const result = await api.getToken();
        if (result.success && result.data && result.data.token) {
            setModelToken(result.data.token);
        }
    };

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadProviders(),
            loadDefaults(),
            loadProviderModels(),
        ]);
        setLoading(false);
    };

    const loadProviders = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        }
    };

    const loadDefaults = async () => {
        const result = await api.getDefaults();
        if (result.success) {
            setDefaults(result.data);
        }
    };

    const loadProviderModels = async () => {
        const result = await api.getProviderModels();
        if (result.success) {
            setProviderModels(result.data);
        }
    };

    const setDefaultProviderHandler = async (_providerName: string) => {
        const currentDefaults = await api.getDefaults();
        if (!currentDefaults.success) {
            return;
        }

        // Update the default RequestConfig with the selected provider
        const requestConfigs = currentDefaults.data.request_configs || [];
        if (requestConfigs.length === 0) {
            return;
        }

        const payload = {
            request_configs: requestConfigs,
        };

        const result = await api.setDefaults(payload);
        if (result.success) {
            await loadDefaults();
        }
    };

    const fetchProviderModels = async (_providerName: string) => {
        const result = await api.getProviderModelsByName(_providerName);
        if (result.success) {
            await loadProviders();
            await loadProviderModels();
        }
    };

    // Server info handlers
    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            setSnackbarMessage(`${label} copied to clipboard!`);
            setSnackbarOpen(true);
        } catch (err) {
            setSnackbarMessage('Failed to copy to clipboard');
            setSnackbarOpen(true);
        }
    };

    const generateToken = async () => {
        const clientId = 'web';
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            copyToClipboard(result.data.token, 'Token');
        } else {
            setSnackbarMessage(`Failed to generate token: ${result.error}`);
            setSnackbarOpen(true);
        }
    };

    // Composition handlers for provider select
    const handleModelSelect = (provider: any, model: string) => {
        console.log("on select", provider, model);
        setSelectedOption({ provider: provider.name, model: model });

        // Show banner with selected provider and model info
        setBannerProvider(provider.name);
        setBannerModel(model);
        setShowBanner(true);
    };

    const handleRefresh = async (provider: any) => {
        console.log("Refreshing models for", provider.name);
        try {
            const result = await api.getProviderModelsByName(provider.name);
            if (result.success) {
                await loadProviders();
                await loadProviderModels();
                setSnackbarMessage(`Models for ${provider.name} refreshed successfully!`);
                setSnackbarOpen(true);
            } else {
                setSnackbarMessage(`Failed to refresh models for ${provider.name}`);
                setSnackbarOpen(true);
            }
        } catch (error) {
            console.error("Error refreshing models:", error);
            setSnackbarMessage(`Error refreshing models for ${provider.name}`);
            setSnackbarOpen(true);
        }
    };

    const handleExpandToggle = (providerName: string, expanded: boolean) => {
        if (expanded) {
            setExpandedProviders(prev => [...prev, providerName]);
        } else {
            setExpandedProviders(prev => prev.filter(name => name !== providerName));
        }
    };

    const handleSearchChange = (providerName: string, searchTerm: string) => {
        setSearchTerms(prev => ({ ...prev, [providerName]: searchTerm }));
        // Reset to first page when searching
        setCurrentPage(prev => ({ ...prev, [providerName]: 1 }));
    };

    const handlePageChange = (providerName: string, page: number) => {
        setCurrentPage(prev => ({ ...prev, [providerName]: page }));
    };

    // Provider dialog handlers
    const handleAddProviderClick = () => {
        setProviderName('');
        setProviderApiBase('');
        setProviderApiVersion('openai');
        setProviderToken('');
        setAddDialogOpen(true);
    };

    const handleAddProvider = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerName,
            api_base: providerApiBase,
            api_version: providerApiVersion,
            token: providerToken,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            setSnackbarMessage('Provider added successfully!');
            setSnackbarOpen(true);
            setProviderName('');
            setProviderApiBase('');
            setProviderApiVersion('openai');
            setProviderToken('');
            setAddDialogOpen(false);
            await loadProviders();
        } else {
            setSnackbarMessage(`Failed to add provider: ${result.error}`);
            setSnackbarOpen(true);
        }
    };

    if (loading) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
                <CircularProgress />
            </Box>
        );
    }

    const baseUrl = import.meta.env.VITE_API_BASE_URL || window.location.origin;
    const openaiBaseUrl = `${baseUrl}/openai/v1`;
    const anthropicBaseUrl = `${baseUrl}/anthropic/v1`;
    const token = generatedToken || modelToken;

    return (
        <Box >


            {/* Server Information Header */}
            <UnifiedCard
                title="Switch Provider & Model"
                subtitle={`Total: ${providers.length} providers | Enabled: ${providers.filter((p: any) => p.enabled).length}`}
                size="full"
                width="100%"
                height="100%"
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

                    <Grid size={{ xs: 12, md: 12 }}>
                        {/* Provider/Model Selection Banner */}
                        {showBanner && (
                            <Alert
                                severity="info"
                                icon={<InfoIcon />}
                                sx={{
                                    mb: 2,
                                    '& .MuiAlert-message': {
                                        width: '100%'
                                    }
                                }}
                                action={
                                    <IconButton
                                        aria-label="close"
                                        color="inherit"
                                        size="small"
                                        onClick={() => setShowBanner(false)}
                                    >
                                        ×
                                    </IconButton>
                                }
                            >
                                <AlertTitle>Active Provider & Model</AlertTitle>
                                <Typography variant="body2">
                                    <strong>Request:</strong> {llmModel} {" -> "}
                                    <strong>Provider:</strong> {bannerProvider} | <strong>Model:</strong> {bannerModel}
                                </Typography>
                            </Alert>
                        )}

                    </Grid>

                    {providers.length > 0 ? (

                        <Grid size={{ xs: 12, md: 12 }}>
                            {/* Providers Quick Settings */}
                            {/* <Stack spacing={2}>
                                {providers.map((provider: any) => (
                                    <ProviderSelect
                                        key={provider.name}
                                        provider={provider}
                                        providerModels={providerModels}
                                        selectedProvider={selectedOption?.provider}
                                        selectedModel={selectedOption?.model}
                                        isExpanded={expandedProviders.includes(provider.name)}
                                        searchTerms={searchTerms}
                                        currentPage={currentPage}
                                        onModelSelect={handleModelSelect}
                                        onExpandToggle={handleExpandToggle}
                                        onSearchChange={handleSearchChange}
                                        onPageChange={handlePageChange}
                                    />
                                ))}
                            </Stack> */}
                            <Stack spacing={2}>
                                <ProviderSelectTab
                                    providers={providers}
                                    providerModels={providerModels}
                                    selectedProvider={selectedOption?.provider}
                                    selectedModel={selectedOption?.model}
                                    onSelected={(opt: ProviderSelectTabOption) => handleModelSelect(opt.provider, opt.model || "")}
                                    onRefresh={handleRefresh}
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
                            <Typography variant="body1" color="text.secondary" sx={{ mb: 3, maxWidth: 500, mx: 'auto' }}>
                                Get started by adding your first AI provider. You can connect to OpenAI, Anthropic, or any compatible API endpoint.
                            </Typography>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 4, maxWidth: 400, mx: 'auto' }}>
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
                providerApiVersion={providerApiVersion}
                onProviderApiVersionChange={setProviderApiVersion}
                providerToken={providerToken}
                onProviderTokenChange={setProviderToken}
            />

            <Snackbar
                open={snackbarOpen}
                autoHideDuration={3000}
                onClose={() => setSnackbarOpen(false)}
                message={snackbarMessage}
            />
        </Box>
    );
};

export default Dashboard;
