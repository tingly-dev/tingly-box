import { useEffect, useState } from 'react';
import { Box, Button, Chip, CircularProgress, Stack, Tooltip, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { useProviderQuota } from '@/hooks/useProviderQuota';
import { QuotaBarItem } from '@/components/credential/QuotaBarItem';
import { quotaToWindows } from '@/types/quota';
import { api } from '@/services/api';

interface HubProvider {
    uuid: string;
    name?: string;
}

// HubPage is the tray's compact landing surface — shown in a small, fixed-size
// window (see gui/wails3/run.go's showHubWindow). It intentionally does not
// duplicate what's one click away in the full app (agent list, usage charts);
// it surfaces only what's time-sensitive to check before opening anything:
// server health, update availability, and provider quota.
export default function HubPage() {
    const { t } = useTranslation();
    const navigate = useNavigate();
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

    const { quotaData, refreshing } = useProviderQuota(providers, { fetchOnMount: true });

    const quotaRows = providers
        .map((p) => ({ provider: p, windows: quotaToWindows(quotaData[p.uuid]) }))
        .filter((row) => row.windows.length > 0);

    return (
        <Box sx={{ height: '100vh', display: 'flex', flexDirection: 'column', p: 2, gap: 2 }}>
            {/* Status bar */}
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                <Box
                    sx={{
                        width: 8,
                        height: 8,
                        borderRadius: '50%',
                        bgcolor: isHealthy ? 'success.main' : 'error.main',
                        flexShrink: 0,
                    }}
                />
                <Typography variant="subtitle1" sx={{ fontWeight: 700, flex: 1 }}>
                    {t('hub.title')}
                </Typography>
                <Tooltip title={isHealthy ? t('hub.status.healthy') : t('hub.status.unhealthy')} arrow>
                    <Chip
                        size="small"
                        label={`v${currentVersion}`}
                        color={isHealthy ? 'default' : 'error'}
                        variant="outlined"
                    />
                </Tooltip>
                {hasUpdate && (
                    <Chip size="small" label={t('hub.status.updateAvailable')} color="info" />
                )}
            </Stack>

            {/* Provider quota */}
            <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                    {t('hub.quota.title')}
                </Typography>
                {loadingProviders ? (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
                        <CircularProgress size={20} />
                    </Box>
                ) : quotaRows.length === 0 ? (
                    <Typography variant="caption" color="text.secondary">
                        {t('hub.quota.empty')}
                    </Typography>
                ) : (
                    <Stack spacing={1.25}>
                        {quotaRows.map(({ provider, windows }) => (
                            <Stack key={provider.uuid} direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                <Typography
                                    variant="body2"
                                    sx={{ minWidth: 96, flexShrink: 0, color: 'text.secondary', fontSize: '0.8rem' }}
                                    noWrap
                                >
                                    {provider.name || provider.uuid}
                                </Typography>
                                <Stack direction="row" spacing={1.5} sx={{ overflowX: 'auto', flex: 1 }}>
                                    {windows.slice(0, 2).map(({ key, window }) => (
                                        <QuotaBarItem key={key} window={window} />
                                    ))}
                                </Stack>
                                {refreshing.has(provider.uuid) && <CircularProgress size={12} />}
                            </Stack>
                        ))}
                    </Stack>
                )}
            </Box>

            {/* Quick actions */}
            <Stack direction="row" spacing={1}>
                <Button fullWidth variant="outlined" onClick={() => navigate('/agent')}>
                    {t('hub.actions.home')}
                </Button>
                <Button fullWidth variant="contained" onClick={() => navigate('/dashboard')}>
                    {t('hub.actions.dashboard')}
                </Button>
            </Stack>
        </Box>
    );
}
