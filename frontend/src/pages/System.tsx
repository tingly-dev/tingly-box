import { Cancel, CheckCircle, Key, PlayArrow, Refresh as RefreshIcon, RestartAlt, Stop } from '@mui/icons-material';
import { Box, Button, IconButton, Stack, Typography, Link, Tabs, Tab } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CardGrid from '@/components/CardGrid';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import GlobalExperimentalFeatures from '@/components/GlobalExperimentalFeatures';
import RequestLog from '@/components/RequestLog';
import { api, getBaseUrl } from '../services/api';
import { useVersion } from '../contexts/VersionContext';

const System = () => {
    const { t } = useTranslation();
    const { currentVersion } = useVersion();
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [baseUrl, setBaseUrl] = useState<string>("");
    const [providersStatus, setProvidersStatus] = useState<any>(null);
    const [rules, setRules] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [notification, setNotification] = useState<{ open: boolean; message?: string; severity?: 'success' | 'error' | 'info' | 'warning' }>({ open: false });
    const [loading, setLoading] = useState(true);
    const [currentTab, setCurrentTab] = useState(0);

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



    const loadProviderSelectionPanel = async () => {
        const [providersResult] = await Promise.all([
            api.getProviders(),
        ]);

        if (providersResult.success) {
            setProviders(providersResult.data);
        }
    };

    const handleStartServer = async () => {
        const port = prompt(t('system.prompts.enterPort'), '8080');
        if (port) {
            const result = await api.startServer(parseInt(port));
            if (result.success) {
                setNotification({ open: true, message: result.message, severity: 'success' });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setNotification({ open: true, message: result.error, severity: 'error' });
            }
        }
    };

    const handleStopServer = async () => {
        if (confirm(t('system.confirmations.stopServer'))) {
            const result = await api.stopServer();
            if (result.success) {
                setNotification({ open: true, message: result.message, severity: 'success' });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setNotification({ open: true, message: result.error, severity: 'error' });
            }
        }
    };

    const handleRestartServer = async () => {
        const port = prompt(t('system.prompts.enterPort'), '8080');
        if (port) {
            const result = await api.restartServer(parseInt(port));
            if (result.success) {
                setNotification({ open: true, message: result.message, severity: 'success' });
                setTimeout(() => {
                    loadServerStatus();
                }, 1000);
            } else {
                setNotification({ open: true, message: result.error, severity: 'error' });
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
                // setNotification({ open: true, message: 'Token copied to clipboard!', severity: 'success' });
            } else {
                setNotification({ open: true, message: result.error, severity: 'error' });
            }
        }
    };

    return (
        <PageLayout loading={loading} notification={notification}>
            <Stack spacing={2} sx={{ mb: 2 }}>
                <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
                    <Tab label="System Status" />
                    <Tab label="Request Logs" />
                </Tabs>
            </Stack>

            {currentTab === 0 ? (
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

                    {/* About Card */}
                    <UnifiedCard
                        title="About"
                        size="medium"
                        width="100%"
                    >
                        <Stack spacing={1.5}>
                            <Typography variant="body2" color="text.secondary">
                                <strong>Version:</strong> {currentVersion || 'N/A'}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                <strong>License:</strong> MPL v2.0
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                <strong>GitHub:</strong>{' '}
                                <Link
                                    href="https://github.com/tingly-dev/tingly-box"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                >
                                    tingly-dev/tingly-box
                                </Link>
                            </Typography>
                        </Stack>
                    </UnifiedCard>

                    {/* Global Experimental Features */}
                    <UnifiedCard
                        title="Global Experimental Features"
                        size="full"
                    >
                        <Stack spacing={1}>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                These experimental features apply globally to all scenarios. Individual scenarios can override these settings.
                            </Typography>
                            <GlobalExperimentalFeatures />
                        </Stack>
                    </UnifiedCard>

                </CardGrid>
            ) : (
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
            )}
        </PageLayout>
    );
};

export default System;
