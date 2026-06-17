import { ArrowBack as ArrowBackIcon, ContentCopy, Refresh as RefreshIcon } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    CircularProgress,
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
import { ProbeV2Dialog } from '@/components/probe/ProbeV2Dialog';
import type { Provider, ProviderModelsData } from '../types/provider';

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
                        {provider.api_base_openai && provider.api_base_anthropic ? (
                            <Stack direction="row" spacing={0.5}>
                                <ApiStyleBadge compact apiStyle="openai" />
                                <ApiStyleBadge compact apiStyle="anthropic" />
                            </Stack>
                        ) : (
                            <ApiStyleBadge apiStyle={provider.api_style} />
                        )}
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

const ModelTestPage = () => {
    const { providerUuid } = useParams<{ providerUuid: string }>();
    const navigate = useNavigate();

    const [provider, setProvider] = useState<Provider | null>(null);
    const [modelsData, setModelsData] = useState<string[]>([]);
    const [loading, setLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [testingModel, setTestingModel] = useState<string | null>(null);
    const [probeDialogOpen, setProbeDialogOpen] = useState(false);
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
        setSelectedModel(model);
        setProbeDialogOpen(true);

        // Note: ProbeV2Dialog handles the API call internally
        // We just need to open the dialog with the right parameters
        setTestingModel(null);
    };

    const handleViewResult = (model: string) => {
        setSelectedModel(model);
        setProbeDialogOpen(true);
    };

    const handleCloseProbeDialog = () => {
        setProbeDialogOpen(false);
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
                        {provider.api_base_openai && provider.api_base_anthropic ? (
                            <Stack direction="row" spacing={0.5}>
                                <ApiStyleBadge compact apiStyle="openai" />
                                <ApiStyleBadge compact apiStyle="anthropic" />
                            </Stack>
                        ) : (
                            <ApiStyleBadge apiStyle={provider.api_style} />
                        )}
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
                        <Grid key={model} xs={12} sm={6} md={4} lg={3} {...({ item: true } as any)}>
                            <ModelCard
                                model={model}
                                provider={provider}
                                isTesting={testingModel === model}
                                onTest={handleTestModel}
                                onViewResult={handleViewResult}
                                hasResult={selectedModel === model}
                            />
                        </Grid>
                    ))}
                </Grid>
            )}

            {/* Probe V2 Dialog */}
            {selectedModel && provider && (
                <ProbeV2Dialog
                    open={probeDialogOpen}
                    onClose={handleCloseProbeDialog}
                    targetType="provider"
                    targetId={provider.uuid}
                    targetName={provider.name}
                    model={selectedModel}
                    testMode="streaming"
                />
            )}
        </Box>
    );
};

export default ModelTestPage;
