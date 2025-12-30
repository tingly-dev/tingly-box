import { Cancel, CheckCircle, Key, PlayArrow, Refresh as RefreshIcon, RestartAlt, Stop } from '@mui/icons-material';
import { Button, IconButton, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CardGrid from '../components/CardGrid';
import { PageLayout } from '../components/PageLayout';
import UnifiedCard from '../components/UnifiedCard';
import RequestLog from '../components/RequestLog';
import { api, getBaseUrl } from '../services/api';

const System = () => {
    const { t } = useTranslation();
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [baseUrl, setBaseUrl] = useState<string>("");
    const [providersStatus, setProvidersStatus] = useState<any>(null);
    const [rules, setRules] = useState<any>({});
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
            loadBaseUrl(),
            loadServerStatus(),
            loadProvidersStatus(),
            loadDefaults(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadBaseUrl = async () => {
        const reuslt = await getBaseUrl();
        setBaseUrl(reuslt)
    }

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
        const result = await api.getRules();
        if (result.success) {
            setRules(result.data);
        }
    };

    const loadProviderSelectionPanel = async () => {
        const [providersResult, defaultsResult] = await Promise.all([
            api.getProviders(),
            api.getRules(),
        ]);

        if (providersResult.success) {
            setProviders(providersResult.data);
            if (defaultsResult.success) {
                setRules(defaultsResult.data);
            }
        }
    };

    const handleStartServer = async () => {
        const port = prompt(t('system.prompts.enterPort'), '8080');
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
        if (confirm(t('system.confirmations.stopServer'))) {
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
        const port = prompt(t('system.prompts.enterPort'), '8080');
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
        const clientId = prompt(t('system.prompts.enterClientId'), 'web');
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

    return (
        <PageLayout loading={loading}>
            <CardGrid>
                {/* Server Status - Consolidated */}
                <UnifiedCard
                    title={t('system.pageTitle')}
                    size="full"
                    // rightAction={
                    //     <Stack direction="row" spacing={1}>
                    //         <Button
                    //             variant="outlined"
                    //             size="small"
                    //             startIcon={<Key />}
                    //             onClick={handleGenerateToken}
                    //             title="Generate Token"
                    //         >
                    //             Token
                    //         </Button>
                    //         <Button
                    //             variant="contained"
                    //             color="success"
                    //             size="small"
                    //             startIcon={<PlayArrow />}
                    //             onClick={handleStartServer}
                    //             disabled={serverStatus?.server_running}
                    //             title="Start Server"
                    //         >
                    //             Start
                    //         </Button>
                    //         <Button
                    //             variant="contained"
                    //             color="error"
                    //             size="small"
                    //             startIcon={<Stop />}
                    //             onClick={handleStopServer}
                    //             disabled={!serverStatus?.server_running}
                    //             title="Stop Server"
                    //         >
                    //             Stop
                    //         </Button>
                    //         <Button
                    //             variant="contained"
                    //             size="small"
                    //             startIcon={<RestartAlt />}
                    //             onClick={handleRestartServer}
                    //             title="Restart Server"
                    //         >
                    //             Restart
                    //         </Button>
                    //         <IconButton onClick={loadServerStatus} size="small" title="Refresh Status">
                    //             <RefreshIcon />
                    //         </IconButton>
                    //     </Stack>
                    // }
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
                                        Status: {serverStatus.server_running ? t('system.status.running') : t('system.status.stopped')}
                                    </Typography>
                                </Stack>
                                <Typography variant="body2" color="text.secondary">
                                    <strong>Server:</strong> {baseUrl}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    <strong>Keys:</strong> {serverStatus.providers_enabled}/{serverStatus.providers_total}
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
                                {/* {serverStatus.request_count !== undefined && (
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>Total Requests:</strong> {serverStatus.request_count}
                                    </Typography>
                                )} */}
                            </Stack>
                        </Stack>
                    ) : (
                        <div>{t('system.status.loading')}</div>
                    )}
                </UnifiedCard>

                {/* Request Logs */}
                <UnifiedCard title="Request Logs" size="full">
                    <RequestLog
                        getLogs={async (params) => {
                            try {
                                const { logsApi } = await api.instances();
                                const response = await logsApi.apiV1LogGet();
                                return {
                                    total: response.data.total || 0,
                                    logs: response.data.logs || [],
                                };
                            } catch (error: any) {
                                console.error('Failed to get logs:', error);
                                return { total: 0, logs: [] };
                            }
                        }}
                        clearLogs={async () => {
                            try {
                                const { logsApi } = await api.instances();
                                await logsApi.apiV1LogDelete();
                                return { success: true, message: 'Logs cleared' };
                            } catch (error: any) {
                                console.error('Failed to clear logs:', error);
                                return { success: false, message: error.message || 'Failed to clear logs' };
                            }
                        }}
                    />
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default System;
