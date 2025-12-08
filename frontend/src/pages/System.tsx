import { Cancel, CheckCircle, Key, PlayArrow, RestartAlt, Stop, Refresh as RefreshIcon } from '@mui/icons-material';
import { Button, IconButton, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import CardGrid from '../components/CardGrid';
import UnifiedCard from '../components/UnifiedCard';
import { PageLayout } from '../components/PageLayout';
import { api } from '../services/api';

const System = () => {
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [providersStatus, setProvidersStatus] = useState<any>(null);
    const [defaults, setDefaults] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadAllData();

        const statusInterval = setInterval(() => {
            loadServerStatus();
        }, 30000);

        return () => {
            clearInterval(statusInterval);
        };
    }, []);

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadServerStatus(),
            loadProvidersStatus(),
            loadDefaults(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadServerStatus = async () => {
        const result = await api.getStatus();
        if (result.success) {
            setServerStatus(result.data);
        }
    };


    const loadProvidersStatus = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProvidersStatus(result.data);
        }
    };


    const loadDefaults = async () => {
        const result = await api.getDefaults();
        if (result.success) {
            setDefaults(result.data);
        }
    };

    const loadProviderSelectionPanel = async () => {
        const [providersResult, modelsResult, defaultsResult] = await Promise.all([
            api.getProviders(),
            api.getProviderModels(),
            api.getDefaults(),
        ]);

        if (providersResult.success && modelsResult.success) {
            setProviders(providersResult.data);
            setProviderModels(modelsResult.data);
            if (defaultsResult.success) {
                setDefaults(defaultsResult.data);
            }
        }
    };

    const handleStartServer = async () => {
        const port = prompt('Enter port for server (8080):', '8080');
        if (port) {
            const result = await api.startServer(parseInt(port));
            if (result.success) {
                setMessage({ type: 'success', text: result.message });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setMessage({ type: 'error', text: result.error });
            }
        }
    };

    const handleStopServer = async () => {
        if (confirm('Are you sure you want to stop the server?')) {
            const result = await api.stopServer();
            if (result.success) {
                setMessage({ type: 'success', text: result.message });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setMessage({ type: 'error', text: result.error });
            }
        }
    };

    const handleRestartServer = async () => {
        const port = prompt('Enter port for server (8080):', '8080');
        if (port) {
            const result = await api.restartServer(parseInt(port));
            if (result.success) {
                setMessage({ type: 'success', text: result.message });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setMessage({ type: 'error', text: result.error });
            }
        }
    };

    const handleGenerateToken = async () => {
        const clientId = prompt('Enter client ID (web):', 'web');
        if (clientId) {
            const result = await api.generateToken(clientId);
            if (result.success) {
                localStorage.setItem('model_auth_token', result.data.token)
                // navigator.clipboard.writeText(result.data.token);
                // setMessage({ type: 'success', text: 'Token copied to clipboard!' });
            } else {
                setMessage({ type: 'error', text: result.error });
            }
        }
    };


    // This handler is kept for backward compatibility
    // The main configuration management is now done through ModelConfigCard
    const setDefaultProviderHandler = async (providerName: string) => {
        const currentDefaults = await api.getDefaults();
        if (!currentDefaults.success) {
            setMessage({ type: 'error', text: 'Failed to get current defaults' });
            return;
        }

        // Update the default RequestConfig with the selected provider
        const requestConfigs = currentDefaults.data.request_configs || [];
        if (requestConfigs.length === 0) {
            setMessage({
                type: 'error',
                text: 'No request configurations found. Please use the Model Configuration card to add one.'
            });
            return;
        }

        const payload = {
            request_configs: requestConfigs,
        };

        const result = await api.setDefaults(payload);
        if (result.success) {
            setMessage({ type: 'success', text: `Set ${providerName} as default provider` });
            await loadProviderSelectionPanel();
            await loadDefaults();
        } else {
            setMessage({ type: 'error', text: result.error });
        }
    };

    const fetchProviderModels = async (providerName: string) => {
        const result = await api.getProviderModelsByName(providerName);
        if (result.success) {
            setMessage({ type: 'success', text: `Successfully fetched models for ${providerName}` });
            await loadProviderSelectionPanel();
        } else {
            setMessage({ type: 'error', text: `Failed to fetch models: ${result.error}` });
        }
    };

    return (
        <PageLayout loading={loading} message={message} onClearMessage={() => setMessage(null)}>
            <CardGrid>
                {/* Server Status - Consolidated */}
                <UnifiedCard
                    title="Server Status & Control"
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button
                                variant="outlined"
                                size="small"
                                startIcon={<Key />}
                                onClick={handleGenerateToken}
                                title="Generate Token"
                            >
                                Token
                            </Button>
                            <Button
                                variant="contained"
                                color="success"
                                size="small"
                                startIcon={<PlayArrow />}
                                onClick={handleStartServer}
                                disabled={serverStatus?.server_running}
                                title="Start Server"
                            >
                                Start
                            </Button>
                            <Button
                                variant="contained"
                                color="error"
                                size="small"
                                startIcon={<Stop />}
                                onClick={handleStopServer}
                                disabled={!serverStatus?.server_running}
                                title="Stop Server"
                            >
                                Stop
                            </Button>
                            <Button
                                variant="contained"
                                size="small"
                                startIcon={<RestartAlt />}
                                onClick={handleRestartServer}
                                title="Restart Server"
                            >
                                Restart
                            </Button>
                            <IconButton onClick={loadServerStatus} size="small" title="Refresh Status">
                                <RefreshIcon />
                            </IconButton>
                        </Stack>
                    }
                >
                    {serverStatus ? (
                        <Stack spacing={3}>
                            {/* Status Information */}
                            <Stack spacing={1}>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    {serverStatus.server_running ? (
                                        <CheckCircle color="success" />
                                    ) : (
                                        <Cancel color="error" />
                                    )}
                                    <Typography variant="h6">
                                        Status: {serverStatus.server_running ? 'Running' : 'Stopped'}
                                    </Typography>
                                </Stack>
                                <Typography variant="body2" color="text.secondary">
                                    <strong>Port:</strong> {serverStatus.port}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    <strong>Providers:</strong> {serverStatus.providers_enabled}/{serverStatus.providers_total}
                                </Typography>
                                {serverStatus.uptime && (
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>Uptime:</strong> {serverStatus.uptime}
                                    </Typography>
                                )}
                                {serverStatus.last_updated && (
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>Last Updated:</strong> {serverStatus.last_updated}
                                    </Typography>
                                )}
                                {serverStatus.request_count !== undefined && (
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>Total Requests:</strong> {serverStatus.request_count}
                                    </Typography>
                                )}
                            </Stack>
                        </Stack>
                    ) : (
                        <div>Loading...</div>
                    )}
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default System;
