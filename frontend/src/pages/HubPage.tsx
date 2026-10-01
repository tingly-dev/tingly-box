import { useEffect, useState } from 'react';
import { displayVersion } from '@/utils/version';
import {
    Box,
    Button,
    ButtonBase,
    Chip,
    CircularProgress,
    Divider,
    IconButton,
    Paper,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { host } from '@/host';
import { AiAgents, BarChart, ChevronRight, Lock, Refresh, Settings, TextSnippet } from '@/components/icons';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { QuotaCell } from '@/components/credential/QuotaCell';
import { quotaRemainingPercent, quotaToWindows, tightestWindow, type ProviderQuota } from '@/types/quota';
import { api, fetchUIAPI } from '@/services/api';
import { SHELL_ROUTES } from '@/routes/shellRoutes';

interface HubProvider {
    uuid: string;
    name?: string;
}

// HubPage is the tray's compact panel — a dedicated small window (see
// gui/wails3/systray.go's useSystray) separate from the main app window. It
// never navigates itself; its jumps open the main window instead, so this
// page only ever renders /hub. Content is scoped to what's
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

    const { quotaData, refreshing, refreshQuota, refreshAllQuotas } = useProviderQuota(providers, { fetchOnMount: true });

    // The panel answers "am I about to run out?", so the provider closest to
    // its limit comes first. Providers with nothing to count (a balance, a
    // failed read) follow; ones with no reading at all are left out.
    const quotaRows = providers
        .map((provider) => ({ provider, quota: quotaData[provider.uuid] }))
        .filter((row): row is { provider: HubProvider; quota: ProviderQuota } =>
            !!row.quota && (quotaToWindows(row.quota).length > 0 || !!row.quota.last_error))
        .map((row) => {
            const tightest = tightestWindow(row.quota);
            return { ...row, remaining: tightest ? quotaRemainingPercent(tightest) : Infinity };
        })
        .sort((a, b) => a.remaining - b.remaining);

    // Opens the separate main app window at the given path. HTTP-first: the
    // same-origin /api/v1/gui/open route reaches the exact same Go handler
    // and — unlike the wails bound call (host.openMainWindow), which has silently failed in the
    // panel webview before — rides the plain fetch path that every other
    // API call on this page already uses successfully. The bound method
    // stays as fallback in case the HTTP route is unreachable.
    const openMainWindow = async (path: string) => {
        try {
            await fetchUIAPI(`/gui/open?path=${encodeURIComponent(path)}`, { method: 'POST' });
        } catch {
            await host.openMainWindow(path);
        }
    };

    const versionKnown = Boolean(currentVersion) && currentVersion !== 'Unknown';

    // The main window's rail, in its order and with its icons and labels, so a
    // jump from the panel lands where the same icon would have taken you.
    const navItems = [
        { path: SHELL_ROUTES.agent, label: t('layout.nav.home'), icon: <AiAgents sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.dashboard, label: t('layout.dashboard'), icon: <BarChart sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.credentials, label: t('layout.nav.credential'), icon: <Lock sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.logs, label: t('layout.logs'), icon: <TextSnippet sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.system, label: t('layout.system'), icon: <Settings sx={{ fontSize: 20 }} /> },
    ];

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
                            {versionKnown && ` · ${displayVersion(currentVersion)}`}
                        </Typography>
                    </Stack>
                </Box>
                {/* Only claim an update when we actually know what we're on —
                    dev builds report an unknown version and would otherwise
                    always flag one. */}
                {hasUpdate && versionKnown && (
                    <Chip
                        size="small"
                        label={t('hub.status.updateAvailable')}
                        color="info"
                        variant="outlined"
                        onClick={() => openMainWindow(SHELL_ROUTES.system)}
                    />
                )}
            </Stack>

            {/* Jumps into the main window, mirroring its rail */}
            <Paper elevation={0} sx={{ borderRadius: 2, bgcolor: 'background.paper', display: 'flex', justifyContent: 'space-around', py: 0.5 }}>
                {navItems.map(({ path, label, icon }) => (
                    <Tooltip key={path} title={label} arrow>
                        <IconButton aria-label={label} onClick={() => openMainWindow(path)} sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}>
                            {icon}
                        </IconButton>
                    </Tooltip>
                ))}
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
                        onClick={() => void refreshAllQuotas()}
                        disabled={providers.length === 0 || refreshing.size > 0}
                    >
                        <Refresh sx={{ fontSize: 16 }} />
                    </IconButton>
                </Stack>
                <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0, px: 1.5 }}>
                    {loadingProviders ? (
                        <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                            <CircularProgress size={20} />
                        </Box>
                    ) : quotaRows.length === 0 ? (
                        <Typography variant="caption" color="text.secondary">
                            {t('hub.quota.empty')}
                        </Typography>
                    ) : (
                        <Stack divider={<Divider flexItem />}>
                            {quotaRows.map(({ provider, quota }) => (
                                <Stack
                                    key={provider.uuid}
                                    direction="row"
                                    spacing={1}
                                    sx={{ alignItems: 'flex-start', justifyContent: 'space-between', py: 1 }}
                                >
                                    {/* The name is the anchor and leads to the provider's
                                        credentials; the cell beside it refreshes on click. */}
                                    <ButtonBase
                                        onClick={() => openMainWindow(SHELL_ROUTES.credentials)}
                                        sx={{
                                            minWidth: 0,
                                            justifyContent: 'flex-start',
                                            borderRadius: 1,
                                            '&:hover .hub-provider-name': { color: 'primary.main' },
                                        }}
                                    >
                                        <Typography className="hub-provider-name" variant="body2" sx={{ fontWeight: 500 }} noWrap>
                                            {provider.name || provider.uuid}
                                        </Typography>
                                    </ButtonBase>
                                    {/* Fixed width so rings and figures line up down the list */}
                                    <Box sx={{ flexShrink: 0, width: 156 }}>
                                        <QuotaCell
                                            quota={quota}
                                            refreshing={refreshing.has(provider.uuid)}
                                            onRefresh={() => void refreshQuota(provider.uuid)}
                                        />
                                    </Box>
                                </Stack>
                            ))}
                        </Stack>
                    )}
                </Box>
                <Box sx={{ px: 1, py: 0.5, borderTop: '1px solid', borderColor: 'divider' }}>
                    <Button
                        size="small"
                        fullWidth
                        endIcon={<ChevronRight sx={{ fontSize: 16 }} />}
                        onClick={() => openMainWindow(SHELL_ROUTES.quotaHistory)}
                        sx={{ justifyContent: 'space-between', textTransform: 'none', color: 'text.secondary' }}
                    >
                        {t('layout.quotaHistory')}
                    </Button>
                </Box>
            </Paper>
        </Box>
    );
}
