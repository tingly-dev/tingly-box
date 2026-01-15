import { Close as CloseIcon, ContentCopy } from '@mui/icons-material';
import {
    Box,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import api from '../services/api';
import ModelSelectTab from './ModelSelectTab';
import ProbeModal from '@/components/ProbeModal';
import type { Provider } from '../types/provider';
import type { ProbeResponse } from '../client';

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
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ pb: 2 }}>
                <Stack spacing={2}>
                    {/* Provider Info */}
                    <Box sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                            Provider
                        </Typography>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            {provider.name}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            Model: {model}
                        </Typography>
                    </Box>

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
                    <Box sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
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
                    </Box>
                </Stack>
            </DialogContent>
        </Dialog>
    );
};

interface ModelListDialogProps {
    open: boolean;
    onClose: () => void;
    provider: Provider | null;
}

const ModelListDialog = ({ open, onClose, provider }: ModelListDialogProps) => {
    const [providerModels, setProviderModels] = useState<{ [key: string]: any }>({});
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);
    const [selectedModel, setSelectedModel] = useState<string>('');
    const [testing, setTesting] = useState(false);
    const [testResults, setTestResults] = useState<Map<string, ProbeResponse>>(new Map());
    const [resultDialogOpen, setResultDialogOpen] = useState(false);
    const [viewResultModel, setViewResultModel] = useState<string | null>(null);

    // Ref to track if dialog is still open (to avoid showing results after closing)
    const isDialogOpenRef = useRef(true);

    // Fetch models when dialog opens
    useEffect(() => {
        if (open && provider) {
            isDialogOpenRef.current = true;
            fetchProviderModels(provider);
        } else if (!open) {
            // Reset when closed
            isDialogOpenRef.current = false;
            setTesting(false);
            setProviderModels({});
            setSelectedModel('');
            setTestResults(new Map());
            setViewResultModel(null);
        }
    }, [open, provider]);

    const fetchProviderModels = async (prov: Provider, forceRefresh = false) => {
        setRefreshingProviders(prev => [...prev, prov.uuid]);
        try {
            // Use updateProviderModelsByUUID for refresh (POST), getProviderModelsByUUID for initial load (GET)
            const response = forceRefresh
                ? await api.updateProviderModelsByUUID(prov.uuid)
                : await api.getProviderModelsByUUID(prov.uuid);

            if (response.success && response.data) {
                setProviderModels({ [prov.uuid]: response.data });
            }
        } catch (err) {
            console.error('Failed to fetch models:', err);
        } finally {
            setRefreshingProviders(prev => prev.filter(uuid => uuid !== prov.uuid));
        }
    };

    const handleRefresh = (prov: Provider) => {
        fetchProviderModels(prov, true); // Force refresh from provider
    };

    const handleCustomModelSave = async (prov: Provider, customModel: string) => {
        // Re-fetch models to get the latest state
        await fetchProviderModels(prov);
    };

    const handleTest = async (model: string) => {
        if (!provider || testing) return;

        setTesting(true);
        try {
            const result = await api.probeModel(provider.uuid, model);
            // Only show results if dialog is still open
            if (isDialogOpenRef.current) {
                setTestResults(prev => new Map(prev).set(model, result));
                setViewResultModel(model);
                setResultDialogOpen(true);
            }
        } catch (err: any) {
            // Only show error if dialog is still open
            if (isDialogOpenRef.current) {
                const errorResult: ProbeResponse = {
                    success: false,
                    error: { message: err?.message || 'Test failed' },
                };
                setTestResults(prev => new Map(prev).set(model, errorResult));
                setViewResultModel(model);
                setResultDialogOpen(true);
            }
        } finally {
            // Only reset testing state if dialog is still open
            // (if dialog was closed, useEffect already reset it)
            if (isDialogOpenRef.current) {
                setTesting(false);
            }
        }
    };

    const handleCloseResultDialog = () => {
        setResultDialogOpen(false);
        setViewResultModel(null);
    };

    const handleClose = () => {
        onClose();
    };

    return (
        <>
            <Dialog
                open={open}
                onClose={handleClose}
                maxWidth="lg"
                fullWidth
                PaperProps={{
                    sx: { height: '80vh', display: 'flex', flexDirection: 'column' }
                }}
            >
                <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Typography variant="h6">Model List</Typography>
                    <IconButton onClick={handleClose} size="small">
                        <CloseIcon />
                    </IconButton>
                </DialogTitle>
                <DialogContent sx={{ p: 0 }}>
                    <Box sx={{ height: '70vh', overflow: 'auto', p: 2 }}>
                        <ModelSelectTab
                            providers={provider ? [provider] : []}
                            providerModels={providerModels}
                            selectedProvider={provider?.uuid}
                            selectedModel={selectedModel}
                            onSelected={(option) => setSelectedModel(option.model || '')}
                            onProviderChange={handleRefresh}
                            onRefresh={handleRefresh}
                            onCustomModelSave={handleCustomModelSave}
                            singleProvider={provider}
                            onTest={handleTest}
                            testing={testing}
                            refreshingProviders={refreshingProviders}
                        />
                    </Box>
                </DialogContent>
            </Dialog>

            {/* Test Result Dialog */}
            {viewResultModel && provider && (
                <TestResultDialog
                    open={resultDialogOpen}
                    onClose={handleCloseResultDialog}
                    probeResult={testResults.get(viewResultModel) || null}
                    model={viewResultModel}
                    provider={provider}
                />
            )}
        </>
    );
};

export default ModelListDialog;