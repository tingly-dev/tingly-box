import { useEffect, useState } from 'react';
import {
    Box,
    Chip,
    CircularProgress,
    Divider,
    IconButton,
    List,
    ListItemButton,
    ListItemIcon,
    ListItemText,
    Paper,
    Stack,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import TinglyService from '@/bindings';
import { BarChart, ChevronRight, Home, Refresh, Settings } from '@/components/icons';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { QuotaBarItem } from '@/components/credential/QuotaBarItem';
import { quotaToWindows } from '@/types/quota';
import { api, fetchUIAPI } from '@/services/api';

interface HubProvider {
    uuid: string;
    name?: string;
}

// HubPage is the tray's compact panel — a dedicated small window (see
// gui/wails3/systray.go's useSystray) separate from the main app window. It
// never navigates itself; the action rows below open the main window
// instead, so this page only ever renders /hub. Content is scoped to what's
// not already obvious the moment you open the full app: server health,
// update availability, and provider quota.
export default function HubPage() {
    const { t } = useTranslation();
    const { isHealthy } = useHealth();
    const { currentVersion, hasUpdate } = useVersion();
    const [providers, setProviders] = useState<HubProvider[]>([]);
    const [loadingProviders, setLoadingProviders] = useState(true);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const result = await api.getProviders();
                if (cancelled) return;
                const list = Array.isArray(result?.data) ? result.data : [];
                setProviders(list.map((p: any) => ({ uuid: p.uuid, name: p.name })));
            } catch {
                // Quota is a bonus, not core function — a failed provider list
                // just leaves the quota section empty, not the whole page broken.
            } finally {
                if (!cancelled) setLoadingProviders(false);
            }
        })();
        return () => { cancelled = true; };
    }, []);

    const { quotaData, refreshing, refreshQuota } = useProviderQuota(providers, { fetchOnMount: true });

    // Force-refresh every provider's quota upstream (same per-provider
    // endpoint the credential page uses), not just re-read the cache.
    const refreshAll = () => providers.forEach((p) => { void refreshQuota(p.uuid); });

    const quotaRows = providers
        .map((p) => ({ provider: p, windows: quotaToWindows(quotaData[p.uuid]) }))
        .filter((row) => row.windows.length > 0);

    // Opens the separate main app window at the given path. HTTP-first: the
    // same-origin /api/v1/gui/open route reaches the exact same Go handler
    // and — unlike the wails bound call, which has silently failed in the
    // panel webview before — rides the plain fetch path that every other
    // API call on this page already uses successfully. The bound method
    // stays as fallback in case the HTTP route is unreachable.
    const openMainWindow = async (path: string) => {
        try {
            await fetchUIAPI(`/gui/open?path=${encodeURIComponent(path)}`, { method: 'POST' });
        } catch {
            TinglyService.OpenMainWindow(path);
        }
    };

    const versionKnown = Boolean(currentVersion) && currentVersion !== 'Unknown';

    return (
        <Box
            sx={{
                height: '100vh',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.default',
                p: 1.25,
                gap: 1.25,
            }}
        >
            {/* Header: identity + status, like a menu-bar app's masthead */}
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', px: 0.5, pt: 0.25 }}>
                {/* Same brand mark the app's activity bar uses */}
                <Box component="img" src="/assets/icon.svg" alt="" sx={{ width: 28, height: 28, borderRadius: 1 }} />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, lineHeight: 1.3 }} noWrap>
                        {t('hub.title')}
                    </Typography>
                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                        <Box
                            sx={{
                                width: 6,
                                height: 6,
                                borderRadius: '50%',
                                bgcolor: isHealthy ? 'success.main' : 'error.main',
                                flexShrink: 0,
                            }}
                        />
                        <Typography variant="caption" color="text.secondary" noWrap>
                            {isHealthy ? t('hub.status.healthy') : t('hub.status.unhealthy')}
                            {versionKnown && ` · v${currentVersion}`}
                        </Typography>
                    </Stack>
                </Box>
                {/* Only claim an update when we actually know what we're on —
                    dev builds report an unknown version and would otherwise
                    always flag one. */}
                {hasUpdate && versionKnown && (
                    <Chip size="small" label={t('hub.status.updateAvailable')} color="info" variant="outlined" />
                )}
                <IconButton
                    size="small"
                    aria-label={t('hub.actions.settings')}
                    onClick={() => openMainWindow('/system')}
                >
                    <Settings sx={{ fontSize: 18 }} />
                </IconButton>
            </Stack>

            {/* Quick actions: list rows, not oversized buttons */}
            <Paper elevation={0} sx={{ borderRadius: 2, overflow: 'hidden', bgcolor: 'background.paper' }}>
                <List disablePadding dense>
                    <ListItemButton onClick={() => openMainWindow('/agent')} sx={{ py: 1 }}>
                        <ListItemIcon sx={{ minWidth: 32 }}>
                            <Home fontSize="small" />
                        </ListItemIcon>
                        <ListItemText
                            primary={t('hub.actions.home')}
                            slotProps={{ primary: { variant: 'body2', sx: { fontWeight: 500 } } }}
                        />
                        <ChevronRight fontSize="small" sx={{ color: 'text.disabled' }} />
                    </ListItemButton>
                    <Divider component="li" />
                    <ListItemButton onClick={() => openMainWindow('/dashboard/today')} sx={{ py: 1 }}>
                        <ListItemIcon sx={{ minWidth: 32 }}>
                            <BarChart fontSize="small" />
                        </ListItemIcon>
                        <ListItemText
                            primary={t('hub.actions.dashboard')}
                            slotProps={{ primary: { variant: 'body2', sx: { fontWeight: 500 } } }}
                        />
                        <ChevronRight fontSize="small" sx={{ color: 'text.disabled' }} />
                    </ListItemButton>
                </List>
            </Paper>

            {/* Provider quota card */}
            <Paper
                elevation={0}
                sx={{
                    borderRadius: 2,
                    bgcolor: 'background.paper',
                    flex: 1,
                    minHeight: 0,
                    display: 'flex',
                    flexDirection: 'column',
                }}
            >
                <Stack
                    direction="row"
                    sx={{ alignItems: 'center', justifyContent: 'space-between', px: 1.5, pt: 0.75, pb: 0.25 }}
                >
                    <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                        {t('hub.quota.title')}
                    </Typography>
                    <IconButton
                        size="small"
                        aria-label={t('hub.quota.refresh')}
                        onClick={refreshAll}
                        disabled={providers.length === 0 || refreshing.size > 0}
                    >
                        <Refresh sx={{ fontSize: 16 }} />
                    </IconButton>
                </Stack>
                <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0, px: 1.5, pb: 1.25 }}>
                    {loadingProviders ? (
                        <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                            <CircularProgress size={20} />
                        </Box>
                    ) : quotaRows.length === 0 ? (
                        <Typography variant="caption" color="text.secondary">
                            {t('hub.quota.empty')}
                        </Typography>
                    ) : (
                        <Stack spacing={1.5} divider={<Divider flexItem />}>
                            {quotaRows.map(({ provider, windows }) => (
                                <Box key={provider.uuid}>
                                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', mb: 0.25 }}>
                                        <Typography variant="body2" sx={{ fontWeight: 500, fontSize: '0.8rem' }} noWrap>
                                            {provider.name || provider.uuid}
                                        </Typography>
                                        {refreshing.has(provider.uuid) && <CircularProgress size={10} />}
                                    </Stack>
                                    {/* A narrow strip has no room for side-by-side bars:
                                        stack the name above the bars, wrapping onto a
                                        second line if there's more than one. */}
                                    <Stack direction="row" spacing={1.5} useFlexGap sx={{ flexWrap: 'wrap' }}>
                                        {windows.slice(0, 2).map(({ key, window }) => (
                                            <QuotaBarItem key={key} window={window} />
                                        ))}
                                    </Stack>
                                </Box>
                            ))}
                        </Stack>
                    )}
                </Box>
            </Paper>
        </Box>
    );
}
