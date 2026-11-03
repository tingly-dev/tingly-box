import { useEffect, useState } from 'react';
import { Box, Button, ButtonBase, CircularProgress, Divider, IconButton, Menu, MenuItem, Paper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ChevronRight, Refresh, Sort } from '@/components/icons';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { ProviderLabel } from '@/components/ProviderLogo';
import { useProviderIconId, type IconSource } from '@/utils/providerIcon';
import { QuotaCell } from '@/components/credential/QuotaCell';
import { quotaRemainingPercent, quotaToWindows, tightestWindow, type ProviderQuota } from '@/types/quota';
import { api } from '@/services/api';
import { SHELL_ROUTES } from '@/routes/shellRoutes';

interface HubProvider {
    uuid: string;
    name?: string;
    api_base?: string;
    oauth_detail?: IconSource['oauth_detail'];
}

type QuotaSortMode = 'remaining' | 'name' | 'type';
const SORT_MODES: QuotaSortMode[] = ['remaining', 'name', 'type'];
const SORT_STORAGE_KEY = 'hub.quotaSort';

// The panel is reopened constantly, so the chosen order is remembered; storage
// can be unavailable, in which case the default order applies.
function readSortMode(): QuotaSortMode {
    try {
        const saved = localStorage.getItem(SORT_STORAGE_KEY);
        return SORT_MODES.find((m) => m === saved) ?? 'remaining';
    } catch {
        return 'remaining';
    }
}

/** Shared by the desktop tray hub and the navigation rail's quota popover. */
export function ProviderQuotaPanel({ onNavigate }: { onNavigate: (path: string) => void }) {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<HubProvider[]>([]);
    const [loadingProviders, setLoadingProviders] = useState(true);
    const iconId = useProviderIconId();
    const [sortMode, setSortMode] = useState<QuotaSortMode>(readSortMode);
    const [sortAnchor, setSortAnchor] = useState<HTMLElement | null>(null);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const result = await api.getProviders();
                if (cancelled) return;
                const list = Array.isArray(result?.data) ? result.data : [];
                setProviders(list.map((p: any) => ({ uuid: p.uuid, name: p.name, api_base: p.api_base, oauth_detail: p.oauth_detail })));
            } catch {
                // Quota is a bonus, not core function — a failed provider list
                // just leaves the quota section empty, not the whole page broken.
            } finally {
                if (!cancelled) setLoadingProviders(false);
            }
        })();
        return () => { cancelled = true; };
    }, []);

    const { quotaData, loading: loadingQuota, refreshing, refreshQuota, redeemReward, refreshAllQuotas } = useProviderQuota(providers, { fetchOnMount: true });

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
        .map((row) => ({ ...row, type: iconId(row.provider) ?? '' }));
    const displayName = (row: { provider: HubProvider }) => row.provider.name || row.provider.uuid;
    // Every order falls back to name so equal keys never shuffle between renders.
    const byName = (a: typeof quotaRows[number], b: typeof quotaRows[number]) =>
        displayName(a).localeCompare(displayName(b));
    quotaRows.sort((a, b) => {
        if (sortMode === 'name') return byName(a, b);
        if (sortMode === 'type') {
            // Providers with no recognised type go last, not first.
            if (!a.type !== !b.type) return a.type ? -1 : 1;
            return a.type.localeCompare(b.type) || byName(a, b);
        }
        return a.remaining - b.remaining || byName(a, b);
    });

    return (
        <Paper
            elevation={0}
            sx={{
                borderRadius: 2,
                bgcolor: 'background.paper',
                flex: 1,
                minWidth: 0,
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
                <Box>
                    <IconButton
                        size="small"
                        aria-label={t('hub.quota.sort.label')}
                        onClick={(e) => setSortAnchor(e.currentTarget)}
                        disabled={quotaRows.length < 2}
                    >
                        <Sort sx={{ fontSize: 16 }} />
                    </IconButton>
                    <Menu
                        anchorEl={sortAnchor}
                        open={Boolean(sortAnchor)}
                        onClose={() => setSortAnchor(null)}
                        slotProps={{ list: { dense: true } }}
                    >
                        {SORT_MODES.map((mode) => (
                            <MenuItem
                                key={mode}
                                selected={mode === sortMode}
                                onClick={() => {
                                    setSortMode(mode);
                                    try { localStorage.setItem(SORT_STORAGE_KEY, mode); } catch { /* not remembered */ }
                                    setSortAnchor(null);
                                }}
                            >
                                {t(`hub.quota.sort.${mode}`)}
                            </MenuItem>
                        ))}
                    </Menu>
                    <IconButton
                        size="small"
                        aria-label={t('hub.quota.refresh')}
                        onClick={() => void refreshAllQuotas()}
                        disabled={providers.length === 0 || refreshing.size > 0}
                    >
                        <Refresh sx={{ fontSize: 16 }} />
                    </IconButton>
                </Box>
            </Stack>
            <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0, px: 1.5 }}>
                {(loadingProviders || loadingQuota) ? (
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
                                sx={{ alignItems: 'center', justifyContent: 'space-between', py: 1 }}
                            >
                                {/* The name is the anchor and leads to the provider's
                                    credentials; the cell beside it refreshes on click. */}
                                <ButtonBase
                                    onClick={() => onNavigate(SHELL_ROUTES.credentials)}
                                    sx={{
                                        flex: 1,
                                        minWidth: 0,
                                        justifyContent: 'flex-start',
                                        borderRadius: 1,
                                        '&:hover .hub-provider-name': { color: 'primary.main' },
                                    }}
                                >
                                    <ProviderLabel provider={provider} gap={0.75} sx={{ width: '100%' }}>
                                        <Typography
                                            className="hub-provider-name"
                                            variant="body2"
                                            title={provider.name || provider.uuid}
                                            sx={{ minWidth: 0, fontWeight: 500 }}
                                            noWrap
                                        >
                                            {provider.name || provider.uuid}
                                        </Typography>
                                    </ProviderLabel>
                                </ButtonBase>
                                {/* Fixed width so rings and figures line up down the list */}
                                <Box sx={{ flexShrink: 0, width: 156 }}>
                                    <QuotaCell
                                        quota={quota}
                                        refreshing={refreshing.has(provider.uuid)}
                                        onRefresh={() => void refreshQuota(provider.uuid)}
                                        onRedeem={rewardId => redeemReward(provider.uuid, rewardId)}
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
                    onClick={() => onNavigate(SHELL_ROUTES.quotaHistory)}
                    sx={{ justifyContent: 'space-between', textTransform: 'none', color: 'text.secondary' }}
                >
                    {t('layout.quotaHistory')}
                </Button>
            </Box>
        </Paper>
    );
}
