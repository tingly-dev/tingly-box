import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Box, CircularProgress, Paper, Stack, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { fetchUIAPI } from '@/services/api';
import { isCountable, quotaToWindows, type ProviderQuota, type QuotaWindow } from '@/types/quota';

interface QuotaHistoryResponse { data?: ProviderQuota[] }
interface Props { startTime: string; endTime: string; provider: string; refreshKey?: number }
interface Sample { time: number; value: number; window: QuotaWindow }
interface Series { key: string; label: string; mode: 'percent' | 'available'; samples: Sample[] }
interface ProviderSeries { uuid: string; name: string; count: number; latestError?: string; windows: Series[] }

const number = (value: number) => value.toLocaleString(undefined, { maximumFractionDigits: 2 });

function groupSnapshots(snapshots: ProviderQuota[]): ProviderSeries[] {
    const groups = new Map<string, ProviderQuota[]>();
    for (const snapshot of snapshots) {
        const records = groups.get(snapshot.provider_uuid) ?? [];
        records.push(snapshot);
        groups.set(snapshot.provider_uuid, records);
    }
    return Array.from(groups, ([uuid, records]) => {
        records.sort((a, b) => Date.parse(a.fetched_at) - Date.parse(b.fetched_at));
        const windows = new Map<string, Series>();
        for (const record of records) {
            if (record.last_error) continue;
            const time = Date.parse(record.fetched_at);
            if (!Number.isFinite(time)) continue;
            for (const { key, label, window } of quotaToWindows(record)) {
                const mode = isCountable(window) ? 'percent' : window.available != null ? 'available' : null;
                if (!mode) continue;
                // Keep changes in a window's unit or meaning on separate axes.
                const seriesKey = [key, window.window_minutes ?? 0, mode, window.unit, window.currency_code ?? ''].join(':');
                const series = windows.get(seriesKey) ?? { key: seriesKey, label, mode, samples: [] };
                series.label = label;
                series.samples.push({ time, value: mode === 'percent' ? window.used_percent : window.available!, window });
                windows.set(seriesKey, series);
            }
        }
        const latest = records[records.length - 1];
        return {
            uuid, name: latest.provider_name || uuid, count: records.length,
            latestError: latest.last_error || undefined, windows: Array.from(windows.values()),
        };
    }).sort((a, b) => Number(b.windows.length > 0) - Number(a.windows.length > 0) || a.name.localeCompare(b.name));
}

function QuotaChart({ series, language, startTime, endTime }: { series: Series; language: string; startTime: string; endTime: string }) {
    const theme = useTheme();
    const { t } = useTranslation();
    const latest = series.samples[series.samples.length - 1];
    const valueLabel = (sample: Sample) => series.mode === 'percent'
        ? `${number(sample.window.used_percent)}% · ${number(sample.window.used)} / ${number(sample.window.limit)}${sample.window.unit === 'percent' ? '' : ` ${sample.window.unit}`}`
        : `${number(sample.value)} ${sample.window.currency_code || sample.window.unit}`;
    const tickTime = (time: number) => new Date(time).toLocaleString(language, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });

    return (
        <Box sx={{ minWidth: 0, p: 2, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', gap: 1, mb: 1 }}>
                <Typography variant="body2" sx={{ fontWeight: 600 }}>{series.label}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ textAlign: 'right' }}>{valueLabel(latest)}</Typography>
            </Stack>
            <Box sx={{ width: '100%', height: 180 }}>
                <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={series.samples} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
                        <CartesianGrid vertical={false} stroke={theme.palette.divider} strokeDasharray="3 3" />
                        <XAxis dataKey="time" type="number" domain={[Date.parse(startTime), Date.parse(endTime)]} allowDataOverflow tickFormatter={tickTime} tick={{ fontSize: 11, fill: theme.palette.text.secondary }} tickLine={false} minTickGap={32} />
                        <YAxis domain={series.mode === 'percent' ? [0, 100] : ['auto', 'auto']} tickFormatter={(value: number) => series.mode === 'percent' ? `${value}%` : number(value)} tick={{ fontSize: 11, fill: theme.palette.text.secondary }} tickLine={false} width={54} />
                        <Tooltip
                            labelFormatter={(value) => new Date(Number(value)).toLocaleString(language)}
                            formatter={(_value, _name, item) => [valueLabel(item.payload as Sample), series.label]}
                            contentStyle={{ background: theme.palette.background.paper, borderColor: theme.palette.divider, borderRadius: 8 }}
                        />
                        <Line type="linear" dataKey="value" stroke={theme.palette.primary.main} strokeWidth={2} dot={{ r: 3 }} activeDot={{ r: 5 }} isAnimationActive={false} />
                    </LineChart>
                </ResponsiveContainer>
            </Box>
            <Typography variant="caption" color="text.secondary">
                {t('dashboard.quotaHistory.sampleCount', { defaultValue: '{{count}} samples', count: series.samples.length })}
            </Typography>
        </Box>
    );
}

export default function QuotaHistoryView({ startTime, endTime, provider, refreshKey = 0 }: Props) {
    const { t, i18n } = useTranslation();
    const [snapshots, setSnapshots] = useState<ProviderQuota[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(false);
    const requestSeq = useRef(0);

    useEffect(() => {
        const seq = ++requestSeq.current;
        const params = new URLSearchParams({ start_time: startTime, end_time: endTime, limit: '5000' });
        if (provider !== 'all') params.set('provider', provider);
        setLoading(true);
        setError(false);
        fetchUIAPI(`/provider-quota/history?${params.toString()}`)
            .then((result: QuotaHistoryResponse) => {
                if (seq === requestSeq.current) setSnapshots(result.data ?? []);
            })
            .catch(() => {
                if (seq === requestSeq.current) { setSnapshots([]); setError(true); }
            })
            .finally(() => {
                if (seq === requestSeq.current) setLoading(false);
            });
    }, [startTime, endTime, provider, refreshKey]);

    const providers = useMemo(() => groupSnapshots(snapshots), [snapshots]);
    if (loading) return <Paper variant="outlined" sx={{ display: 'grid', placeItems: 'center', minHeight: 220 }}><CircularProgress size={28} /></Paper>;
    if (error) return <Alert severity="warning">{t('dashboard.quotaHistory.loadError', { defaultValue: 'Quota history could not be loaded.' })}</Alert>;
    if (providers.length === 0) return (
        <Paper variant="outlined" sx={{ display: 'grid', placeItems: 'center', textAlign: 'center', minHeight: 220, p: 2 }}>
            <Box>
                <Typography sx={{ fontWeight: 600 }}>{t('dashboard.quotaHistory.empty', { defaultValue: 'No quota snapshots in this period' })}</Typography>
                <Typography variant="body2" color="text.secondary">{t('dashboard.quotaHistory.emptyHint', { defaultValue: 'Snapshots appear after a supported provider quota is refreshed.' })}</Typography>
            </Box>
        </Paper>
    );
    return (
        <Stack sx={{ gap: 2 }}>
            {snapshots.length >= 5000 && <Alert severity="info">{t('dashboard.quotaHistory.limitHint', { defaultValue: 'Showing the latest 5,000 samples. Choose a provider or shorter time range to see older samples.' })}</Alert>}
            {providers.map((item) => (
                <Paper key={item.uuid} variant="outlined" sx={{ p: { xs: 2, sm: 2.5 }, borderRadius: 2 }}>
                    <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', gap: 1, mb: 2 }}>
                        <Typography variant="h6" sx={{ fontWeight: 600 }}>{item.name}</Typography>
                        <Typography variant="caption" color="text.secondary">
                            {t('dashboard.quotaHistory.sampleCount', { defaultValue: '{{count}} samples', count: item.count })}
                        </Typography>
                    </Stack>
                    {item.latestError && <Alert severity="warning" sx={{ mb: 2 }}>{item.latestError}</Alert>}
                    {item.windows.length ? (
                        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'repeat(2, minmax(0, 1fr))' }, gap: 2 }}>
                            {item.windows.map((series) => <QuotaChart key={series.key} series={series} language={i18n.language} startTime={startTime} endTime={endTime} />)}
                        </Box>
                    ) : (
                        <Typography variant="body2" color="text.secondary">{t('dashboard.quotaHistory.noWindows', { defaultValue: 'No quota windows reported' })}</Typography>
                    )}
                </Paper>
            ))}
        </Stack>
    );
}
