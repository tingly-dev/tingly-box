import { Cancel, CheckCircle, Key, PlayArrow, Refresh as RefreshIcon, RestartAlt, Stop, Visibility as ViewIcon, Info as InfoIcon } from '@mui/icons-material';
import { Button, IconButton, Stack, Typography, Switch, Box, Alert, AlertTitle, Dialog, DialogTitle, DialogContent, DialogActions } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CardGrid from '@/components/CardGrid';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api, getBaseUrl } from '../services/api';
import { useAnalytics } from '@/contexts/AnalyticsContext';
import { useVersion } from '@/contexts/VersionContext';

const System = () => {
    const { t } = useTranslation();
    const { enabled: analyticsEnabled, hasConsent, grantConsent, revokeConsent, getDataPreview } = useAnalytics();
    const { currentVersion } = useVersion();
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [baseUrl, setBaseUrl] = useState<string>("");
    const [providersStatus, setProvidersStatus] = useState<any>(null);
    const [rules, setRules] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);
    const [previewOpen, setPreviewOpen] = useState(false);

    const handleAnalyticsToggle = (event: React.ChangeEvent<HTMLInputElement>) => {
        if (event.target.checked) {
            grantConsent();
        } else {
            revokeConsent();
        }
    };

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

                {/* Analytics Settings */}
                <UnifiedCard title="Analytics & Usage Data" size="full">
                    <Stack spacing={3}>
                        <Alert severity="info" icon={<InfoIcon />}>
                            <AlertTitle>Help us improve Tingly-Box</AlertTitle>
                            <Typography variant="body2">
                                We collect only anonymous usage data to understand which features are used most.
                                No personal information, no request content, no provider credentials.
                            </Typography>
                        </Alert>

                        <Box
                            sx={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                p: 2,
                                borderRadius: 2,
                                bgcolor: 'background.default',
                            }}
                        >
                            <Box>
                                <Typography variant="subtitle1" fontWeight={600}>
                                    Share anonymous usage data
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Help improve Tingly-Box by sharing anonymous usage statistics
                                </Typography>
                            </Box>
                            <Switch
                                checked={hasConsent}
                                onChange={handleAnalyticsToggle}
                                disabled={!analyticsEnabled}
                                color="primary"
                            />
                        </Box>

                        {!analyticsEnabled && (
                            <Alert severity="warning">
                                <Typography variant="body2">
                                    Analytics is disabled in this build. No data will be collected.
                                </Typography>
                            </Alert>
                        )}

                        <Box sx={{ pl: 2 }}>
                            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
                                What we collect:
                            </Typography>
                            <Stack spacing={1}>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <CheckCircle color="success" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>Page visits:</strong> Which pages you visit (e.g., Dashboard, System)
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <CheckCircle color="success" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        <strong>System info:</strong> App version, OS type (macOS/Windows/Linux)
                                    </Typography>
                                </Stack>
                            </Stack>
                        </Box>

                        <Box sx={{ pl: 2 }}>
                            <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
                                What we DON'T collect:
                            </Typography>
                            <Stack spacing={1}>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <Cancel color="disabled" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        IP addresses, location data
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <Cancel color="disabled" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        Request content, API keys, tokens
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <Cancel color="disabled" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        Provider names, model names
                                    </Typography>
                                </Stack>
                                <Stack direction="row" alignItems="flex-start" spacing={1}>
                                    <Cancel color="disabled" sx={{ fontSize: 16, mt: 0.3 }} />
                                    <Typography variant="body2" color="text.secondary">
                                        Error messages or crash details
                                    </Typography>
                                </Stack>
                            </Stack>
                        </Box>

                        <Box>
                            <Button
                                variant="outlined"
                                size="small"
                                startIcon={<ViewIcon />}
                                onClick={() => setPreviewOpen(true)}
                            >
                                View sample data
                            </Button>
                        </Box>
                    </Stack>
                </UnifiedCard>

                {/* About */}
                <UnifiedCard title="About" size="full">
                    <Stack spacing={2}>
                        <Box>
                            <Typography variant="body2" color="text.secondary">
                                <strong>Version:</strong> {currentVersion}
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary">
                                <strong>License:</strong> MPL v2.0
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary">
                                <strong>Repository:</strong>{' '}
                                <Typography
                                    component="a"
                                    href="https://github.com/tingly-dev/tingly-box"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    sx={{ color: 'primary.main', textDecoration: 'none' }}
                                >
                                    github.com/tingly-dev/tingly-box
                                </Typography>
                            </Typography>
                        </Box>
                    </Stack>
                </UnifiedCard>
            </CardGrid>

            {/* Data Preview Dialog */}
            <Dialog
                open={previewOpen}
                onClose={() => setPreviewOpen(false)}
                maxWidth="md"
                fullWidth
            >
                <DialogTitle>Sample data collected</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Here are examples of the data that would be sent to Google Analytics when you enable
                        analytics. We collect ONLY:
                    </Typography>
                    <Box
                        sx={{
                            bgcolor: 'grey.900',
                            color: 'grey.100',
                            p: 2,
                            borderRadius: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.875rem',
                            overflow: 'auto',
                        }}
                    >
                        <pre style={{ margin: 0 }}>{getDataPreview()}</pre>
                    </Box>
                    <Alert severity="success" sx={{ mt: 2 }}>
                        <Typography variant="body2">
                            <strong>That's it!</strong> We only collect page visits and basic system info.
                            No IP addresses, no request content, no provider data, no error messages.
                        </Typography>
                    </Alert>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setPreviewOpen(false)}>Close</Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default System;
