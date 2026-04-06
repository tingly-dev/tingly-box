import CardGrid from '@/components/CardGrid';
import GlobalExperimentalFeatures from '@/components/GlobalExperimentalFeatures';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Logout } from '@mui/icons-material';
import { Refresh as RefreshIcon } from '@mui/icons-material';
import { IconCircleCheck, IconCircleX, IconInfoCircle, IconKey, IconLock, IconStar, IconLicense, IconBrandGithub } from '@tabler/icons-react';
import { Box, CircularProgress, IconButton, Link, Stack, Tooltip, Typography, Chip } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { useAuth } from '@/contexts/AuthContext';
import { api } from '@/services/api';

const System = () => {
    const { t } = useTranslation();
    const { currentVersion, hasUpdate, latestVersion, showUpdateDialog } = useVersion();
    const { isHealthy, checking, checkHealth } = useHealth();
    const { logout: authLogout } = useAuth();
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [notification, setNotification] = useState<{ open: boolean; message?: string; severity?: 'success' | 'error' | 'info' | 'warning' }>({ open: false });
    const [loading, setLoading] = useState(true);
    const [respectEnvProxy, setRespectEnvProxy] = useState<boolean | null>(null);

    const handleForceLogout = () => {
        authLogout();
        setNotification({ open: true, message: 'Logged out successfully', severity: 'info' });
        setTimeout(() => {
            window.location.href = '/login';
        }, 500);
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
            loadServerStatus(),
            loadProxyConfig(),
        ]);
        setLoading(false);
    };

    const loadProxyConfig = async () => {
        const result = await api.getConfig();
        if (result.success && result.data) {
            const value = result.data.http_transport?.respect_env_proxy;
            setRespectEnvProxy(value === null ? true : value);
        }
    };

    const loadServerStatus = async () => {
        const result = await api.getStatus();
        if (result.success) {
            setServerStatus(result.data);
        }
    };

    const toggleProxy = () => {
        const newValue = !respectEnvProxy;
        setRespectEnvProxy(newValue);

        api.updateConfig({
            http_transport: {
                respect_env_proxy: newValue,
            },
        }).then((result) => {
            if (!result.success) {
                setRespectEnvProxy(!newValue);
            }
        });
    };

    return (
        <PageLayout loading={loading} notification={notification}>
            <CardGrid>
                {/* Server Status - Simplified one-line-per-status design */}
                <UnifiedCard
                    title="Server Status"
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={0.5}>
                            <Tooltip title="Force Logout" arrow>
                                <IconButton
                                    onClick={handleForceLogout}
                                    size="small"
                                    aria-label="Force logout"
                                >
                                    <Logout fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <IconButton
                                onClick={() => { loadServerStatus(); checkHealth(); }}
                                size="small"
                                aria-label="Refresh status"
                            >
                                {checking ? <CircularProgress size={16} /> : <RefreshIcon />}
                            </IconButton>
                        </Stack>
                    }
                >
                    {serverStatus ? (
                        <Stack spacing={1.5}>
                            {/* Server Status */}
                            <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                    {serverStatus.server_running ? (
                                        <IconCircleCheck size={16} style={{ color: 'var(--mui-palette-success-main)' }} />
                                    ) : (
                                        <IconCircleX size={16} style={{ color: 'var(--mui-palette-error-main)' }} />
                                    )}
                                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                        Server
                                    </Typography>
                                </Box>
                                <Box sx={{ flex: 1 }}>
                                    <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                        {serverStatus.server_running ? t('system.status.running') : t('system.status.stopped')}
                                        {isHealthy && (
                                            <Typography component="span" variant="body2" color="success.main" sx={{ ml: 1 }}>
                                                · Connected
                                            </Typography>
                                        )}
                                    </Typography>
                                </Box>
                            </Box>

                            {/* Keys */}
                            <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                    <IconKey size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                        Keys
                                    </Typography>
                                </Box>
                                <Box sx={{ flex: 1 }}>
                                    <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                        {serverStatus.providers_enabled} / {serverStatus.providers_total}
                                    </Typography>
                                </Box>
                            </Box>

                            {/* Uptime */}
                            {serverStatus.uptime && (
                                <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                            Uptime
                                        </Typography>
                                    </Box>
                                    <Box sx={{ flex: 1 }}>
                                        <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                            {serverStatus.uptime}
                                        </Typography>
                                    </Box>
                                </Box>
                            )}

                            {/* Proxy Settings */}
                            <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                    <IconLock size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                        Proxy
                                    </Typography>
                                </Box>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                                    {respectEnvProxy !== null && (
                                        <Tooltip
                                            title={t('system.proxy.respectEnvProxy.helper') + (respectEnvProxy ? ' (enabled)' : ' (disabled)')}
                                            arrow
                                        >
                                            <Chip
                                                label={`${respectEnvProxy ? 'System Proxy' : 'Direct'} · ${respectEnvProxy ? 'On' : 'Off'}`}
                                                onClick={toggleProxy}
                                                size="small"
                                                sx={(theme) => ({
                                                    bgcolor: respectEnvProxy ? 'primary.main' : 'action.hover',
                                                    color: respectEnvProxy ? 'primary.contrastText' : 'text.primary',
                                                    fontWeight: respectEnvProxy ? 600 : 400,
                                                    border: respectEnvProxy ? 'none' : '1px solid',
                                                    borderColor: 'divider',
                                                    '&:hover': {
                                                        bgcolor: respectEnvProxy ? 'primary.dark' : 'action.selected',
                                                    },
                                                })}
                                            />
                                        </Tooltip>
                                    )}
                                </Box>
                            </Box>
                        </Stack>
                    ) : (
                        <Typography color="text.secondary">{t('system.status.loading')}</Typography>
                    )}
                </UnifiedCard>

                {/* About - Simplified one-line-per-status design */}
                <UnifiedCard
                    title="About"
                    size="full"
                >
                    <Stack spacing={1.5}>
                        {/* Version */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconInfoCircle size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    Version
                                </Typography>
                            </Box>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                                <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                    {currentVersion || 'N/A'}
                                </Typography>
                                {(hasUpdate || import.meta.env.DEV) && (
                                    <Tooltip title={`Click to view ${hasUpdate ? 'update details' : 'dev info'}`} arrow>
                                        <Box
                                            onClick={showUpdateDialog}
                                            sx={{
                                                display: 'flex',
                                                alignItems: 'center',
                                                gap: 0.5,
                                                color: import.meta.env.DEV && !hasUpdate ? 'success.main' : 'info.main',
                                                cursor: 'pointer',
                                                px: 1,
                                                py: 0.5,
                                                borderRadius: 1,
                                                transition: 'all 150ms ease-in-out',
                                                '&:hover': { bgcolor: 'action.hover' },
                                            }}
                                            role="button"
                                            aria-label="View update details"
                                            tabIndex={0}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter' || e.key === ' ') {
                                                    e.preventDefault();
                                                    showUpdateDialog();
                                                }
                                            }}
                                        >
                                            <IconStar size={16} />
                                            <Typography variant="caption" color={import.meta.env.DEV && !hasUpdate ? 'success.main' : 'info.main'}>
                                                {hasUpdate ? `${latestVersion} available` : 'Dev Mode'}
                                            </Typography>
                                        </Box>
                                    </Tooltip>
                                )}
                            </Box>
                        </Box>

                        {/* License */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconLicense size={16} style={{ color: 'text.secondary' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    License
                                </Typography>
                            </Box>
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                    MPL-2.0 + Commercial
                                </Typography>
                            </Box>
                        </Box>

                        {/* GitHub */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconBrandGithub size={16} style={{ color: 'text.secondary' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    GitHub
                                </Typography>
                            </Box>
                            <Box sx={{ flex: 1 }}>
                                <Link
                                    href="https://github.com/tingly-dev/tingly-box"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    sx={{ typography: 'body2', color: 'primary.main', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
                                >
                                    tingly-dev/tingly-box
                                </Link>
                            </Box>
                        </Box>
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
        </PageLayout>
    );
};

export default System;
