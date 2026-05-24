import CardGrid from '@/components/CardGrid';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Logout } from '@/components/icons';
import { Refresh as RefreshIcon } from '@/components/icons';
import { IconCircleCheck, IconCircleX, IconInfoCircle, IconLock, IconStar, IconLicense, IconBrandGithub, IconLanguage, IconBrush, IconWorld, IconCheck, IconClock } from '@tabler/icons-react';
import { Box, Button, CircularProgress, IconButton, InputAdornment, Link, Stack, TextField, Tooltip, Typography, Chip } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { useAuth } from '@/contexts/AuthContext';
import { useThemeMode } from '@/contexts/ThemeContext';
import { useNotify } from '@/hooks/useNotify';
import { api } from '@/services/api';
import { getThemeOptions } from '@/theme/options';

const System = () => {
    const { t, i18n } = useTranslation();
    const { currentVersion, hasUpdate, latestVersion, showUpdateDialog } = useVersion();
    const { isHealthy, checking, checkHealth } = useHealth();
    const { logout: authLogout } = useAuth();
    const { mode: themeMode, setTheme } = useThemeMode();
    const themeOptions = useMemo(() => getThemeOptions(t), [t]);
    const notify = useNotify();
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [loading, setLoading] = useState(true);
    const [respectEnvProxy, setRespectEnvProxy] = useState<boolean | null>(null);
    const [globalProxyUrl, setGlobalProxyUrl] = useState('');
    const [globalProxyInput, setGlobalProxyInput] = useState('');
    const [proxyUrlSaving, setProxyUrlSaving] = useState(false);
    const isServerStatusAvailable = Boolean(serverStatus);
    const serverStatusLabel = !isServerStatusAvailable
        ? t('system.status.unavailable')
        : serverStatus.server_running ? t('system.status.running') : t('system.status.stopped');

    const handleForceLogout = () => {
        authLogout();
        notify.info('Logged out successfully');
        setTimeout(() => {
            window.location.href = '/login';
        }, 500);
    };

    const changeLanguage = (lng: string) => {
        i18n.changeLanguage(lng);
        // Save language preference to localStorage
        localStorage.setItem('i18nextLng', lng);
        notify.success(t('system.language.saveSuccess'));
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
            setRespectEnvProxy(value === null ? false : value);
            const gpUrl = result.data.http_transport?.global_proxy_url ?? '';
            setGlobalProxyUrl(gpUrl);
            setGlobalProxyInput(gpUrl);
        }
    };

    const saveGlobalProxyUrl = async () => {
        setProxyUrlSaving(true);
        const result = await api.updateConfig({
            http_transport: { global_proxy_url: globalProxyInput },
        });
        if (result.success) {
            setGlobalProxyUrl(globalProxyInput);
            notify.success(t('system.proxy.globalProxyUrl.saveSuccess'));
        } else {
            notify.error(t('system.proxy.globalProxyUrl.saveFailed'));
        }
        setProxyUrlSaving(false);
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
        <PageLayout loading={loading}>
            <CardGrid>
                {/* Server Status - Simplified one-line-per-status design */}
                <UnifiedCard
                    title={t('system.serverStatus.title')}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={0.5}>
                            <Tooltip title={t('system.serverStatus.forceLogout')} arrow>
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
                                aria-label={t('system.serverStatus.refreshStatus')}
                            >
                                {checking ? <CircularProgress size={16} /> : <RefreshIcon />}
                            </IconButton>
                        </Stack>
                    }
                >
                    <Stack spacing={1.5}>
                        {/* Server Status */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                {serverStatus?.server_running ? (
                                    <IconCircleCheck size={16} style={{ color: 'var(--mui-palette-success-main)' }} />
                                ) : (
                                    <IconCircleX
                                        size={16}
                                        style={{
                                            color: isServerStatusAvailable
                                                ? 'var(--mui-palette-error-main)'
                                                : 'var(--mui-palette-text-secondary)',
                                        }}
                                    />
                                )}
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('system.serverStatus.server')}
                                </Typography>
                            </Box>
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                    {serverStatusLabel}
                                    {isHealthy && (
                                        <Typography component="span" variant="body2" color="success.main" sx={{ ml: 1 }}>
                                            · {t('system.status.connected')}
                                        </Typography>
                                    )}
                                </Typography>
                            </Box>
                        </Box>

                        {/* Uptime */}
                        {serverStatus?.uptime && (
                            <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                    <IconClock size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                        {t('system.status.uptime')}
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
                                    {t('system.proxy.label')}
                                </Typography>
                            </Box>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                                {respectEnvProxy !== null && (
                                    <Tooltip title={t('system.proxy.respectEnvProxy.helper')} arrow>
                                        <Chip
                                            label={`${respectEnvProxy ? t('system.proxy.respectEnvProxy.label') : t('common.direct')} · ${respectEnvProxy ? t('common.on') : t('common.off')}`}
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

                        {/* Language — merged from the standalone Language Settings card */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconLanguage size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('system.language.title')}
                                </Typography>
                            </Box>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                                <Chip
                                    label={t('system.language.en')}
                                    onClick={() => changeLanguage('en')}
                                    size="small"
                                    sx={{
                                        bgcolor: i18n.language === 'en' ? 'primary.main' : 'action.hover',
                                        color: i18n.language === 'en' ? 'primary.contrastText' : 'text.primary',
                                        fontWeight: i18n.language === 'en' ? 600 : 400,
                                        border: i18n.language === 'en' ? 'none' : '1px solid',
                                        borderColor: 'divider',
                                        cursor: 'pointer',
                                        '&:hover': {
                                            bgcolor: i18n.language === 'en' ? 'primary.dark' : 'action.selected',
                                        },
                                    }}
                                />
                                <Chip
                                    label={t('system.language.zh')}
                                    onClick={() => changeLanguage('zh')}
                                    size="small"
                                    sx={{
                                        bgcolor: i18n.language === 'zh' ? 'primary.main' : 'action.hover',
                                        color: i18n.language === 'zh' ? 'primary.contrastText' : 'text.primary',
                                        fontWeight: i18n.language === 'zh' ? 600 : 400,
                                        border: i18n.language === 'zh' ? 'none' : '1px solid',
                                        borderColor: 'divider',
                                        cursor: 'pointer',
                                        '&:hover': {
                                            bgcolor: i18n.language === 'zh' ? 'primary.dark' : 'action.selected',
                                        },
                                    }}
                                />
                            </Box>
                        </Box>

                        {/* Theme — moved from the activity bar */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconBrush size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('common.theme')}
                                </Typography>
                            </Box>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, flexWrap: 'wrap' }}>
                                {themeOptions.map(({ value, label, renderIcon }) => {
                                    const selected = themeMode === value;
                                    return (
                                        <Chip
                                            key={value}
                                            icon={renderIcon({ size: 14 })}
                                            label={label}
                                            onClick={() => setTheme(value)}
                                            size="small"
                                            sx={{
                                                bgcolor: selected ? 'primary.main' : 'action.hover',
                                                color: selected ? 'primary.contrastText' : 'text.primary',
                                                fontWeight: selected ? 600 : 400,
                                                border: selected ? 'none' : '1px solid',
                                                borderColor: 'divider',
                                                cursor: 'pointer',
                                                '& .MuiChip-icon': {
                                                    color: 'inherit',
                                                },
                                                '&:hover': {
                                                    bgcolor: selected ? 'primary.dark' : 'action.selected',
                                                },
                                            }}
                                        />
                                    );
                                })}
                            </Box>
                        </Box>
                    </Stack>
                </UnifiedCard>

                {/* Quick Proxy — dedicated card for the reusable proxy preset */}
                <UnifiedCard
                    title={
                        <Stack direction="row" alignItems="center" spacing={1}>
                            <IconWorld size={18} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                            <Typography variant="subtitle1" fontWeight={600}>
                                {t('system.proxy.globalProxyUrl.label')}
                            </Typography>
                        </Stack>
                    }
                    size="full"
                >
                    <Stack spacing={1.5}>
                        <Typography variant="body2" color="text.secondary">
                            {t('system.proxy.globalProxyUrl.description', { defaultValue: t('system.proxy.globalProxyUrl.helper') })}
                        </Typography>
                        <Stack direction="row" spacing={1} alignItems="center">
                            <TextField
                                size="small"
                                fullWidth
                                value={globalProxyInput}
                                onChange={(e) => setGlobalProxyInput(e.target.value)}
                                placeholder="http://127.0.0.1:7890"
                                sx={{ maxWidth: 480 }}
                                InputProps={globalProxyUrl && globalProxyInput === globalProxyUrl ? {
                                    endAdornment: (
                                        <InputAdornment position="end">
                                            <Tooltip title={t('common.saved', { defaultValue: 'Saved' })} arrow>
                                                <IconCheck size={16} style={{ color: 'var(--mui-palette-success-main)' }} />
                                            </Tooltip>
                                        </InputAdornment>
                                    )
                                } : undefined}
                            />
                            <Button
                                size="small"
                                variant="contained"
                                onClick={saveGlobalProxyUrl}
                                disabled={proxyUrlSaving || globalProxyInput === globalProxyUrl}
                                sx={{ whiteSpace: 'nowrap', minWidth: 72 }}
                            >
                                {proxyUrlSaving ? <CircularProgress size={14} color="inherit" /> : t('common.save')}
                            </Button>
                        </Stack>
                    </Stack>
                </UnifiedCard>

                {/* About - Simplified one-line-per-status design */}
                <UnifiedCard
                    title={t('system.about.title')}
                    size="full"
                >
                    <Stack spacing={1.5}>
                        {/* Version */}
                        <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 100 }}>
                                <IconInfoCircle size={14} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('system.about.version')}
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
                                                {hasUpdate ? `${latestVersion} ${t('system.about.available')}` : t('system.about.devMode')}
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
                                    {t('system.about.license')}
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
                                    {t('system.about.github')}
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

            </CardGrid>
        </PageLayout>
    );
};

export default System;
