import CardGrid from '@/components/CardGrid.tsx';
import { PageLayout } from '@/components/PageLayout.tsx';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import { Logout, Refresh as RefreshIcon, CheckCircle as IconCircleCheck, Cancel as IconCircleX, Info as IconInfoCircle, Lock as IconLock, License as IconLicense, GitHub as IconBrandGithub, Translate as IconLanguage, Brush as IconBrush, Check as IconCheck, AccessTime as IconClock, ContentCopy as IconContentCopy, Router as IconRouter } from '@/components/icons';
import { UpdatePanelDialog } from '@/components/UpdatePanelDialog';
import { Box, Button, CircularProgress, Divider, IconButton, InputAdornment, Link, Stack, Switch, TextField, Tooltip, Typography, Chip, type SxProps, type Theme } from '@mui/material';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHealth } from '@/contexts/HealthContext.tsx';
import { useVersion } from '@/contexts/VersionContext.tsx';
import { useAuth } from '@/contexts/AuthContext.tsx';
import { useThemeMode } from '@/contexts/ThemeContext.tsx';
import { useNotify } from '@/hooks/useNotify.ts';
import { api } from '@/services/api.ts';
import { getThemeOptions } from '@/theme/options.ts';
import { SUPPORTED_LANGUAGES, resolveLanguage } from '@/i18n';

// Label column width shared by every settings row — keeps the value column
// (the actual visual anchor) vertically aligned across cards.
const LABEL_WIDTH = 140;

// Cards span the full page width (consistent with every other page); only the
// *content* inside each card is capped (via UnifiedCard.contentMaxWidth) so
// settings rows and inputs don't stretch uncomfortably wide on big screens.
const CONTENT_MAX_WIDTH = 720;

/**
 * SettingsRow — the shared label-column rhythm for this page.
 * `[icon + label (fixed)]  [children (flex)]  [optional trailing action]`
 * Replaces ~7 hand-rolled Box rows that had drifted on gap / icon size / wrapper.
 */
const SettingsRow = ({
    icon,
    label,
    children,
    action,
}: {
    icon: ReactNode;
    label: string;
    children: ReactNode;
    action?: ReactNode;
}) => (
    <Box sx={{ display: 'flex', alignItems: 'center', py: 0.5, gap: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: LABEL_WIDTH, color: 'text.secondary' }}>
            {icon}
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {label}
            </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
            {children}
        </Box>
        {action}
    </Box>
);

/**
 * chipSx — one selected/unselected Chip style, shared by Language + Theme.
 */
const chipSx = (selected: boolean): SxProps<Theme> => ({
    bgcolor: selected ? 'primary.main' : 'action.hover',
    color: selected ? 'primary.contrastText' : 'text.primary',
    fontWeight: selected ? 600 : 400,
    border: selected ? 'none' : '1px solid',
    borderColor: 'divider',
    cursor: 'pointer',
    '& .MuiChip-icon': { color: 'inherit' },
    '&:hover': {
        bgcolor: selected ? 'primary.dark' : 'action.selected',
    },
});

const System = () => {
    const { t, i18n } = useTranslation();
    const { currentVersion, latestVersion, hasUpdate, showUpdateDialog, openUpdateDialog, closeUpdateDialog } = useVersion();
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
    const [copiedVersion, setCopiedVersion] = useState(false);
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

    const handleCopyVersion = () => {
        const value = (currentVersion || 'Unknown').split('+')[0];
        navigator.clipboard.writeText(value).then(() => {
            setCopiedVersion(true);
            notify.success(t('system.about.versionCopied'));
            setTimeout(() => setCopiedVersion(false), 2000);
        });
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
                {/* Server Status — "Is the gateway healthy?" Actions sit on
                    the Server row itself (trailing icons), matching how the
                    About card keeps its copy button on the Version row. */}
                <UnifiedCard
                    grid={{ xs: 12, md: 12 }}
                    title={t('system.serverStatus.title')}
                    titleHeadingLevel={1}
                    size="full"
                    contentMaxWidth={CONTENT_MAX_WIDTH}
                >
                    <Stack spacing={1.5}>
                        <SettingsRow
                            icon={
                                serverStatus?.server_running ? (
                                    <IconCircleCheck sx={{ fontSize: 16, color: 'success.main' }} />
                                ) : (
                                    <IconCircleX sx={{ fontSize: 16, color: isServerStatusAvailable ? 'error.main' : 'text.secondary' }} />
                                )
                            }
                            label={t('system.serverStatus.server')}
                            action={
                                <Stack direction="row" spacing={0.5}>
                                    <Tooltip title={t('system.serverStatus.forceLogout')} arrow>
                                        <IconButton
                                            onClick={handleForceLogout}
                                            size="small"
                                            aria-label={t('system.serverStatus.forceLogout')}
                                        >
                                            <Logout sx={{ fontSize: 16 }} />
                                        </IconButton>
                                    </Tooltip>
                                    <Tooltip title={t('system.serverStatus.refreshStatus')} arrow>
                                        <IconButton
                                            onClick={() => { loadServerStatus(); checkHealth(); }}
                                            size="small"
                                            aria-label={t('system.serverStatus.refreshStatus')}
                                        >
                                            {checking ? <CircularProgress size={16} /> : <RefreshIcon sx={{ fontSize: 16 }} />}
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            }
                        >
                            <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                {serverStatusLabel}
                                {isHealthy && (
                                    <Typography component="span" variant="body2" sx={{ color: 'success.main', ml: 1 }}>
                                        · {t('system.status.connected')}
                                    </Typography>
                                )}
                            </Typography>
                        </SettingsRow>
                        {serverStatus?.uptime && (
                            <SettingsRow icon={<IconClock sx={{ fontSize: 16 }} />} label={t('system.status.uptime')}>
                                <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                    {serverStatus.uptime}
                                </Typography>
                            </SettingsRow>
                        )}
                        <SettingsRow
                            icon={<IconInfoCircle sx={{ fontSize: 16, color: 'text.secondary' }} />}
                            label={t('system.about.version')}
                            action={
                                <Tooltip title={copiedVersion ? t('common.copied') : t('system.about.copyVersion')} placement="top" arrow>
                                    <IconButton
                                        size="small"
                                        onClick={handleCopyVersion}
                                        aria-label={t('system.about.copyVersion')}
                                        sx={{ color: 'text.secondary' }}
                                    >
                                        {copiedVersion ? <IconCheck sx={{ fontSize: 16, color: 'success.main' }} /> : <IconContentCopy sx={{ fontSize: 16 }} />}
                                    </IconButton>
                                </Tooltip>
                            }
                        >
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                                <Tooltip
                                    title={hasUpdate && latestVersion ? t('system.about.updateAvailable', { version: latestVersion.split('+')[0] }) : t('system.about.checkUpdate')}
                                    placement="top"
                                    arrow
                                >
                                    <Typography
                                        component="span"
                                        variant="body2"
                                        onClick={showUpdateDialog}
                                        sx={{
                                            color: 'text.primary',
                                            cursor: 'pointer',
                                            transition: 'color 0.2s ease',
                                            '&:hover': { color: 'primary.main' },
                                        }}
                                    >
                                        version {(currentVersion || 'Unknown').split('+')[0]}
                                    </Typography>
                                </Tooltip>
                                {hasUpdate && latestVersion && (
                                    <Typography
                                        component="span"
                                        variant="caption"
                                        onClick={showUpdateDialog}
                                        sx={{
                                            color: 'warning.main',
                                            cursor: 'pointer',
                                            '&:hover': { textDecoration: 'underline' },
                                        }}
                                    >
                                        {t('system.about.available')} → {latestVersion.split('+')[0]}
                                    </Typography>
                                )}
                            </Box>
                        </SettingsRow>
                    </Stack>
                </UnifiedCard>


                {/* Preferences — "How do I want the UI to behave?" */}
                <UnifiedCard grid={{ xs: 12, md: 12 }} title={t('system.preferences.title')} size="full" contentMaxWidth={CONTENT_MAX_WIDTH}>
                    <Stack spacing={1.5}>
                        <SettingsRow icon={<IconLanguage sx={{ fontSize: 16 }} />} label={t('system.language.title')}>
                            {SUPPORTED_LANGUAGES.map(({ code, labelKey }) => (
                                <Chip
                                    key={code}
                                    label={t(labelKey)}
                                    onClick={() => changeLanguage(code)}
                                    size="small"
                                    sx={chipSx(resolveLanguage(i18n.language) === code)}
                                />
                            ))}
                        </SettingsRow>

                        <SettingsRow icon={<IconBrush sx={{ fontSize: 16 }} />} label={t('common.theme')}>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                {themeOptions.map(({ value, label, renderIcon }) => (
                                    <Chip
                                        key={value}
                                        icon={renderIcon({ size: 14 })}
                                        label={label}
                                        onClick={() => setTheme(value)}
                                        size="small"
                                        sx={chipSx(themeMode === value)}
                                    />
                                ))}
                            </Box>
                        </SettingsRow>
                    </Stack>
                </UnifiedCard>

                {/* Proxy — "How does TB reach upstream?" Env-proxy policy +
                    reusable URL preset, kept on their own card. */}
                <UnifiedCard grid={{ xs: 12, md: 12 }} title={t('system.proxy.title')} size="full" contentMaxWidth={CONTENT_MAX_WIDTH}>
                    <Stack spacing={2}>
                        {/* Env-proxy policy — an honest switch, not a flip-chip.
                            The off-state is just "off" (no `common.direct`
                            reuse, which collided with the network "direct"). */}
                        <Box>
                            <SettingsRow icon={<IconRouter sx={{ fontSize: 16 }} />} label={t('system.proxy.respectEnvProxy.label')}>
                                {respectEnvProxy !== null && (
                                    <Box sx={{ display: 'flex', justifyContent: 'flex-end', width: '100%' }}>
                                        <Switch
                                            checked={respectEnvProxy}
                                            onChange={toggleProxy}
                                            size="small"
                                        />
                                    </Box>
                                )}
                            </SettingsRow>
                            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block' }}>
                                {t('system.proxy.respectEnvProxy.helper')}
                            </Typography>
                        </Box>

                        <Divider />

                        {/* Reusable proxy URL preset — description on its own
                            line, then the input + Save below. */}
                        <Box>
                            <SettingsRow icon={<IconLock sx={{ fontSize: 16 }} />} label={t('system.proxy.globalProxyUrl.label')}>
                                {null}
                            </SettingsRow>
                            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 1 }}>
                                {t('system.proxy.globalProxyUrl.helper')}
                            </Typography>
                            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', width: '100%' }}>
                                <TextField
                                    size="small"
                                    fullWidth
                                    value={globalProxyInput}
                                    onChange={(e) => setGlobalProxyInput(e.target.value)}
                                    placeholder="http://127.0.0.1:7890"
                                    slotProps={{
                                        input: globalProxyUrl && globalProxyInput === globalProxyUrl ? {
                                            endAdornment: (
                                                <InputAdornment position="end">
                                                    <Tooltip title={t('common.saved', { defaultValue: 'Saved' })} arrow>
                                                        <IconCheck sx={{ fontSize: 16, color: 'success.main' }} />
                                                    </Tooltip>
                                                </InputAdornment>
                                            )
                                        } : undefined
                                    }}
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
                        </Box>
                    </Stack>
                </UnifiedCard>

                {/* About — "What is this?" */}
                <UnifiedCard title={t('system.about.title')} size="full" contentMaxWidth={CONTENT_MAX_WIDTH}>
                    <Stack spacing={1.5}>
                        <SettingsRow icon={<IconLicense sx={{ fontSize: 16, color: 'text.secondary' }} />} label={t('system.about.license')}>
                            <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                MPL-2.0 + Commercial
                            </Typography>
                        </SettingsRow>

                        <SettingsRow icon={<IconBrandGithub sx={{ fontSize: 16, color: 'text.secondary' }} />} label={t('system.about.github')}>
                            <Link
                                href="https://github.com/tingly-dev/tingly-box"
                                target="_blank"
                                rel="noopener noreferrer"
                                sx={{ typography: 'body2', color: 'primary.main', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
                            >
                                tingly-dev/tingly-box
                            </Link>
                        </SettingsRow>
                    </Stack>
                </UnifiedCard>

            </CardGrid>
            {/* Update Panel Dialog */}
            <UpdatePanelDialog
                open={openUpdateDialog}
                onClose={closeUpdateDialog}
            />
        </PageLayout>
    );
};

export default System;
