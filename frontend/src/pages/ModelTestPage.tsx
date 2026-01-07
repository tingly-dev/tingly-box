import { ArrowBack as ArrowBackIcon, ContentCopy, Refresh as RefreshIcon } from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Paper,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import api from '../services/api';
import { ApiStyleBadge } from '@/components/ApiStyleBadge';
import ProbeModal from '@/components/ProbeModal';
import type { Provider, ProviderModelsData } from '../types/provider';
import type { ProbeResponse } from '../client';

interface ModelCardProps {
    model: string;
    provider: Provider;
    isTesting: boolean;
    onTest: (model: string) => void;
    onViewResult: (model: string) => void;
    hasResult: boolean;
}

const ModelCard = ({ model, provider, isTesting, onTest, onViewResult, hasResult }: ModelCardProps) => {
    return (
        <Card
            sx={{
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                cursor: 'pointer',
                transition: 'all 0.2s',
                border: '1px solid',
                borderColor: 'divider',
                '&:hover': {
                    boxShadow: 4,
                    transform: 'translateY(-2px)',
                },
            }}
        >
            <CardContent sx={{ flexGrow: 1, p: 2 }}>
                <Stack spacing={1.5}>
                    <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <ApiStyleBadge apiStyle={provider.api_style} />
                        {hasResult && (
                            <Tooltip title="View test result">
                                <IconButton
                                    size="small"
                                    color="primary"
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        onViewResult(model);
                                    }}
                                >
                                    <ContentCopy fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        )}
                    </Stack>
                    <Typography
                        variant="body2"
                        sx={{
                            fontFamily: 'monospace',
                            wordBreak: 'break-all',
                            fontWeight: 500,
                        }}
                    >
                        {model}
                    </Typography>
                    <Button
                        variant="outlined"
                        size="small"
                        disabled={isTesting}
                        onClick={(e) => {
                            e.stopPropagation();
                            onTest(model);
                        }}
                        startIcon={isTesting ? <CircularProgress size={14} /> : undefined}
                        sx={{
                            textTransform: 'none',
                            borderRadius: 1.5,
                            fontSize: '0.75rem',
                        }}
                    >
                        {isTesting ? 'Testing...' : 'Test'}
                    </Button>
                </Stack>
            </CardContent>
        </Card>
    );
};

interface TestResultDialogProps {
    open: boolean;
    onClose: () => void;
    probeResult: ProbeResponse | null;
    model: string;
    provider: Provider;
}

const TestResultDialog = ({ open, onClose, probeResult, model, provider }: TestResultDialogProps) => {
    const handleCopyCurl = async () => {
        if (probeResult?.data?.curl_command) {
            try {
                await navigator.clipboard.writeText(probeResult.data.curl_command);
            } catch (err) {
                console.error('Failed to copy curl command:', err);
            }
        }
    };

    const curlCommand = probeResult?.data?.curl_command || '';

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="h6">Connection Test Result</Typography>
                <IconButton onClick={onClose} size="small">
                    <ArrowBackIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ pb: 2 }}>
                <Stack spacing={2}>
                    {/* Provider Info */}
                    <Paper variant="outlined" sx={{ p: 2 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                            Provider
                        </Typography>
                        <Stack direction="row" spacing={1} alignItems="center">
                            <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                {provider.name}
                            </Typography>
                            <ApiStyleBadge apiStyle={provider.api_style} />
                        </Stack>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            Model: {model}
                        </Typography>
                    </Paper>

                    {/* Test Result */}
                    <Box>
                        <ProbeModal
                            provider={provider}
                            model={model}
                            probeResult={probeResult}
                            isProbing={false}
                        />
                    </Box>

                    {/* Curl Command */}
                    <Paper variant="outlined" sx={{ p: 2 }}>
                        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1 }}>
                            <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                curl Command
                            </Typography>
                            <Tooltip title="Copy curl command">
                                <IconButton size="small" color="primary" onClick={handleCopyCurl}>
                                    <ContentCopy fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                        <Box
                            sx={{
                                p: 2,
                                bgcolor: 'grey.50',
                                borderRadius: 1,
                                fontFamily: 'monospace',
                                fontSize: '0.8rem',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-all',
                                border: '1px solid',
                                borderColor: 'divider',
                            }}
                        >
                            {curlCommand}
                        </Box>
                    </Paper>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} variant="contained">
                    Close
                </Button>
            </DialogActions>
        </Dialog>
    );
};

const ModelTestPage = () => {
    const { providerUuid } = useParams<{ providerUuid: string }>();
    const navigate = useNavigate();

    const [provider, setProvider] = useState<Provider | null>(null);
    const [modelsData, setModelsData] = useState<string[]>([]);
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [testingModel, setTestingModel] = useState<string | null>(null);
    const [testResults, setTestResults] = useState<Map<string, ProbeResponse>>(new Map());
    const [resultDialogOpen, setResultDialogOpen] = useState(false);
    const [selectedModel, setSelectedModel] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    const fetchProvider = useCallback(async () => {
        if (!providerUuid) return;
        try {
            const response = await api.getProvider(providerUuid);
            if (response.success && response.data) {
                setProvider(response.data);
                return response.data;
            }
            throw new Error('Failed to fetch provider');
        } catch (err: any) {
            setError(err.message || 'Failed to fetch provider');
            return null;
        }
    }, [providerUuid]);

    const fetchModels = useCallback(async (providerData?: Provider) => {
        if (!providerUuid) return;
        try {
            setRefreshing(true);
            const p = providerData || provider;
            if (!p) return;

            const response = await api.getProviderModelsByUUID(providerUuid);
            if (response.success && response.data?.models) {
                setModelsData(response.data.models);
            } else {
                setModelsData([]);
            }
            setError(null);
        } catch (err: any) {
            setError(err.message || 'Failed to fetch models');
            setModelsData([]);
        } finally {
            setRefreshing(false);
        }
    }, [providerUuid, provider]);

    useEffect(() => {
        const init = async () => {
            setLoading(true);
            const p = await fetchProvider();
            if (p) {
                await fetchModels(p);
            }
            setLoading(false);
        };
        init();
    }, [fetchProvider, fetchModels]);

    const handleTestModel = async (model: string) => {
        if (!provider || testingModel) return;

        setTestingModel(model);
        try {
            const result = await api.probeModel(provider.uuid, model);
            setTestResults(prev => new Map(prev).set(model, result));
            setSelectedModel(model);
            setResultDialogOpen(true);
        } catch (err: any) {
            const errorResult: ProbeResponse = {
                success: false,
                error: { message: err?.message || 'Test failed' },
            };
            setTestResults(prev => new Map(prev).set(model, errorResult));
            setSelectedModel(model);
            setResultDialogOpen(true);
        } finally {
            setTestingModel(null);
        }
    };

    const handleViewResult = (model: string) => {
        setSelectedModel(model);
        setResultDialogOpen(true);
    };

    const handleCloseResultDialog = () => {
        setResultDialogOpen(false);
        setSelectedModel(null);
    };

    if (loading) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
                <Stack spacing={2} alignItems="center">
                    <CircularProgress />
                    <Typography variant="body2" color="text.secondary">
                        Loading provider and models...
                    </Typography>
                </Stack>
            </Box>
        );
    }

    if (error || !provider) {
        return (
            <Box sx={{ p: 3 }}>
                <Button
                    startIcon={<ArrowBackIcon />}
                    onClick={() => navigate('/api-keys')}
                    sx={{ mb: 2 }}
                >
                    Back to API Keys
                </Button>
                <Alert severity="error">{error || 'Provider not found'}</Alert>
            </Box>
        );
    }

    return (
        <Box sx={{ p: 3 }}>
            {/* Header */}
            <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 3 }}>
                <Stack spacing={1}>
                    <Button
                        startIcon={<ArrowBackIcon />}
                        onClick={() => navigate('/api-keys')}
                        sx={{ alignSelf: 'flex-start' }}
                    >
                        Back to API Keys
                    </Button>
                    <Typography variant="h5" sx={{ fontWeight: 600 }}>
                        Choose Model
                    </Typography>
                    <Stack direction="row" spacing={2} alignItems="center">
                        <Typography variant="body1" sx={{ fontWeight: 500 }}>
                            {provider.name}
                        </Typography>
                        <ApiStyleBadge apiStyle={provider.api_style} />
                    </Stack>
                </Stack>
                <Button
                    variant="outlined"
                    startIcon={refreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
                    onClick={() => fetchModels()}
                    disabled={refreshing}
                    sx={{
                        textTransform: 'none',
                        borderRadius: 1.5,
                    }}
                >
                    {refreshing ? 'Refreshing...' : 'Refresh Models'}
                </Button>
            </Stack>

            {/* Error Alert */}
            {error && (
                <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            {/* Models Grid */}
            {modelsData.length === 0 ? (
                <Paper sx={{ p: 4, textAlign: 'center' }}>
                    <Typography variant="body1" color="text.secondary">
                        No models found for this provider. Click "Refresh Models" to fetch the latest models.
                    </Typography>
                </Paper>
            ) : (
                <Grid container spacing={2}>
                    {modelsData.map((model) => (
                        <Grid item xs={12} sm={6} md={4} lg={3} key={model}>
                            <ModelCard
                                model={model}
                                provider={provider}
                                isTesting={testingModel === model}
                                onTest={handleTestModel}
                                onViewResult={handleViewResult}
                                hasResult={testResults.has(model)}
                            />
                        </Grid>
                    ))}
                </Grid>
            )}

            {/* Test Result Dialog */}
            {selectedModel && (
                <TestResultDialog
                    open={resultDialogOpen}
                    onClose={handleCloseResultDialog}
                    probeResult={testResults.get(selectedModel) || null}
                    model={selectedModel}
                    provider={provider}
                />
            )}
        </Box>
    );
};

export default ModelTestPage;
