import { useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    LinearProgress,
    Paper,
    Stack,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fetchUIAPI } from '@/services/api';
import { isCountable, quotaToWindows, type ProviderQuota } from '@/types/quota';

interface QuotaHistoryResponse {
    meta?: { total?: number };
    data?: ProviderQuota[];
}

interface QuotaHistoryViewProps {
    startTime: string;
    endTime: string;
    provider: string;
    refreshKey?: number;
    showHeading?: boolean;
}

const formatValue = (value: number, unit: string): string => {
    const formatted = value.toLocaleString(undefined, { maximumFractionDigits: 2 });
    if (unit === 'percent') return `${formatted}%`;
    return `${formatted} ${unit}`.trim();
};

export default function QuotaHistoryView({ startTime, endTime, provider, refreshKey = 0, showHeading = true }: QuotaHistoryViewProps) {
    const { t, i18n } = useTranslation();
    const [snapshots, setSnapshots] = useState<ProviderQuota[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(false);
    const requestSeq = useRef(0);

    useEffect(() => {
        const seq = ++requestSeq.current;
        const params = new URLSearchParams({ start_time: startTime, end_time: endTime, limit: '1000' });
        if (provider !== 'all') params.set('provider', provider);
        setLoading(true);
        setError(false);
        fetchUIAPI(`/provider-quota/history?${params.toString()}`)
            .then((result: QuotaHistoryResponse) => {
                if (seq === requestSeq.current) setSnapshots(result.data ?? []);
            })
            .catch(() => {
                if (seq === requestSeq.current) {
                    setSnapshots([]);
                    setError(true);
                }
            })
            .finally(() => {
                if (seq === requestSeq.current) setLoading(false);
            });
    }, [startTime, endTime, provider, refreshKey]);

    const providerCount = useMemo(
        () => new Set(snapshots.map((snapshot) => snapshot.provider_uuid)).size,
        [snapshots],
    );

    return (
        <Paper variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, borderRadius: 2, minHeight: 280 }}>
            <Stack sx={{ flexDirection: { xs: 'column', sm: 'row' }, justifyContent: 'space-between', gap: 1, mb: 2 }}>
                {showHeading ? <Box>
                    <Typography variant="h6" sx={{ fontWeight: 600 }}>
                        {t('dashboard.quotaHistory.title', { defaultValue: 'Quota history' })}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {t('dashboard.quotaHistory.subtitle', { defaultValue: 'Stored provider quota snapshots for the selected time range.' })}
                    </Typography>
                </Box> : <Box />}
                {!loading && snapshots.length > 0 && (
                    <Chip
                        size="small"
                        label={t('dashboard.quotaHistory.snapshotCount', {
                            defaultValue: '{{snapshots}} snapshots · {{providers}} providers',
                            snapshots: snapshots.length,
                            providers: providerCount,
                        })}
                    />
                )}
            </Stack>

            {loading ? (
                <Box sx={{ display: 'grid', placeItems: 'center', minHeight: 190 }}><CircularProgress size={28} /></Box>
            ) : error ? (
                <Alert severity="warning">{t('dashboard.quotaHistory.loadError', { defaultValue: 'Quota history could not be loaded.' })}</Alert>
            ) : snapshots.length === 0 ? (
                <Box sx={{ display: 'grid', placeItems: 'center', textAlign: 'center', minHeight: 190 }}>
                    <Box>
                        <Typography sx={{ fontWeight: 600 }}>{t('dashboard.quotaHistory.empty', { defaultValue: 'No quota snapshots in this period' })}</Typography>
                        <Typography variant="body2" color="text.secondary">
                            {t('dashboard.quotaHistory.emptyHint', { defaultValue: 'Snapshots appear after a supported provider quota is refreshed.' })}
                        </Typography>
                    </Box>
                </Box>
            ) : (
                <Stack sx={{ gap: 1.25 }}>
                    {snapshots.map((snapshot, snapshotIndex) => {
                        const windows = quotaToWindows(snapshot);
                        return (
                            <Box
                                key={`${snapshot.provider_uuid}-${snapshot.fetched_at}-${snapshotIndex}`}
                                sx={{ p: 1.5, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}
                            >
                                <Stack sx={{ flexDirection: { xs: 'column', md: 'row' }, gap: 1.5, alignItems: { md: 'center' } }}>
                                    <Box sx={{ width: { md: 210 }, flexShrink: 0 }}>
                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>{snapshot.provider_name || snapshot.provider_uuid}</Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            {new Date(snapshot.fetched_at).toLocaleString(i18n.language)}
                                        </Typography>
                                    </Box>
                                    {snapshot.last_error ? (
                                        <Alert severity="error" sx={{ py: 0, flex: 1 }}>{snapshot.last_error}</Alert>
                                    ) : windows.length === 0 ? (
                                        <Typography variant="body2" color="text.secondary">
                                            {t('dashboard.quotaHistory.noWindows', { defaultValue: 'No quota windows reported' })}
                                        </Typography>
                                    ) : (
                                        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, minmax(0, 1fr))' }, gap: 1.5, flex: 1 }}>
                                            {windows.map(({ key, label, window }) => {
                                                const countable = isCountable(window);
                                                return (
                                                    <Box key={key} sx={{ minWidth: 0 }}>
                                                        <Stack sx={{ flexDirection: 'row', justifyContent: 'space-between', gap: 1, mb: 0.5 }}>
                                                            <Typography variant="caption" noWrap>{label}</Typography>
                                                            <Typography variant="caption" color="text.secondary" noWrap>
                                                                {countable
                                                                    ? `${formatValue(window.used, window.unit)} / ${formatValue(window.limit, window.unit)}`
                                                                    : t('dashboard.quotaHistory.notCountable', { defaultValue: 'Not limited' })}
                                                            </Typography>
                                                        </Stack>
                                                        <LinearProgress
                                                            variant="determinate"
                                                            value={countable ? Math.min(100, Math.max(0, window.used_percent)) : 0}
                                                            color={countable && window.used_percent >= 80 ? 'error' : countable && window.used_percent >= 50 ? 'warning' : 'success'}
                                                            sx={{ height: 6, borderRadius: 999 }}
                                                        />
                                                    </Box>
                                                );
                                            })}
                                        </Box>
                                    )}
                                </Stack>
                            </Box>
                        );
                    })}
                </Stack>
            )}
        </Paper>
    );
}
