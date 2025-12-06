import Refresh from '@mui/icons-material/Refresh';
import { Alert, Box, CircularProgress, IconButton, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import ServerStatusControl from '../components/ServerStatusControl';
import UnifiedCard from '../components/UnifiedCard';
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

    if (loading) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box>
            {message && (
                <Alert
                    severity={message.type}
                    sx={{ mb: 2 }}
                    onClose={() => setMessage(null)}
                >
                    {message.text}
                </Alert>
            )}

            <CardGrid>
                {/* Server Status - Consolidated */}
                <CardGridItem xs={12} md={6}>
                    <UnifiedCard
                        title="Server Status & Control"
                        subtitle={serverStatus ? (serverStatus.server_running ? "Server is running" : "Server is stopped") : "Loading..."}
                        size="large"
                        rightAction={
                            <IconButton onClick={loadServerStatus} size="small" title="Refresh Status">
                                <Refresh />
                            </IconButton>
                        }
                    >
                        {serverStatus ? (
                            <>
                                <ServerStatusControl
                                    serverStatus={serverStatus}
                                    onStartServer={handleStartServer}
                                    onStopServer={handleStopServer}
                                    onRestartServer={handleRestartServer}
                                    onGenerateToken={handleGenerateToken}
                                />
                                {serverStatus.request_count !== undefined && (
                                    <Box sx={{ mt: 2, p: 2, backgroundColor: 'grey.50', borderRadius: 2 }}>
                                        <Typography variant="body2" color="text.secondary" gutterBottom>
                                            Total Requests
                                        </Typography>
                                        <Typography variant="h6" sx={{ fontFamily: 'monospace', fontWeight: 600 }}>
                                            {serverStatus.request_count}
                                        </Typography>
                                    </Box>
                                )}
                            </>
                        ) : (
                            <div>Loading...</div>
                        )}
                    </UnifiedCard>
                </CardGridItem>


              </CardGrid>
        </Box>
    );
};

export default System;
