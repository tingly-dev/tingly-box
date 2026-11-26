import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Box, CircularProgress, Paper, Stack, Typography } from '@mui/material';
import { alpha, useTheme } from '@mui/material/styles';
import type { Theme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import {
    Bar,
    CartesianGrid,
    Cell,
    ComposedChart,
    Line,
    LineChart,
    ReferenceArea,
    ReferenceLine,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from 'recharts';
import { fetchUIAPI } from '@/services/api';
import { ProviderLabel } from '@/components/ProviderLogo';
import { QuotaRawResponseButton } from '@/components/credential/QuotaRawResponseButton';
import {
    formatQuotaUsage,
    isCountable,
    quotaRemainingPercent,
    quotaToWindows,
    type ProviderQuota,
} from '@/types/quota';
import { quotaTone, type StatusTone } from '@tingly/ui/theme/status';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius, radiusPx } from '@tingly/ui/theme/radius';
import {
    dayBands,
    gapThreshold,
    hasInternalGap,
    seriesDelta,
    splitByGaps,
    type QuotaSample,
    type QuotaSeries,
    type QuotaMode,
} from './quotaHistorySeries';

interface QuotaHistoryResponse { data?: ProviderQuota[] }
interface Props { startTime: string; endTime: string; provider: string; daily: boolean; refreshKey?: number }

interface ProviderSeries {
    uuid: string;
    name: string;
    count: number;
    latestError?: string;
    latestRaw?: unknown;
    windows: QuotaSeries[];
}

const number = (value: number) => value.toLocaleString(undefined, { maximumFractionDigits: 2 });
const DANGER_PERCENT = 20;

// Quota colours come from the theme's own status palette (MUI
// success/warning/error), not the brighter @tingly/ui "fill" set, so the charts
// read as part of the theme. `main` clears the 3:1 graphics target on each
// surface in both modes (light 3.19–4.83:1, dark 4.89–5.78:1); caption text
// uses the adjacent variant to clear 4.5:1 (see the PR's contrast table).
type StatusKey = 'success' | 'warning' | 'error' | 'info';
const statusKey = (tone: StatusTone): StatusKey | null =>
    tone === 'success' || tone === 'warning' || tone === 'error' || tone === 'info' ? tone : null;

/** Solid mark colour (line, band stroke, chip dot, sparkline). */
const markColor = (theme: Theme, tone: StatusTone): string => {
    const key = statusKey(tone);
    return key ? theme.palette[key].main : theme.palette.text.secondary;
};

/** Coloured *text* needs a stronger step than a mark: dark on light, light on dark. */
const statusText = (theme: Theme, key: StatusKey): string =>
    theme.palette.mode === 'dark' ? theme.palette[key].light : theme.palette[key].dark;

function groupSnapshots(snapshots: ProviderQuota[]): ProviderSeries[] {
    const groups = new Map<string, ProviderQuota[]>();
    for (const snapshot of snapshots) {
        const records = groups.get(snapshot.provider_uuid) ?? [];
        records.push(snapshot);
        groups.set(snapshot.provider_uuid, records);
    }
    return Array.from(groups, ([uuid, records]) => {
        records.sort((a, b) => Date.parse(a.fetched_at) - Date.parse(b.fetched_at));
        const windows = new Map<string, QuotaSeries>();
        for (const record of records) {
            if (record.last_error) continue;
            const time = Date.parse(record.fetched_at);
            if (!Number.isFinite(time)) continue;
            for (const { key, label, window } of quotaToWindows(record)) {
                const mode: QuotaMode | null = isCountable(window) ? 'percent' : window.available != null ? 'available' : null;
                if (!mode) continue;
                // Keep changes in a window's unit or meaning on separate axes.
                const seriesKey = [key, window.window_minutes ?? 0, mode, window.unit, window.currency_code ?? ''].join(':');
                const series = windows.get(seriesKey) ?? { key: seriesKey, label, mode, samples: [] };
                series.label = label;
                series.samples.push({ time, value: mode === 'percent' ? quotaRemainingPercent(window) : window.available!, window });
                windows.set(seriesKey, series);
            }
        }
        const latest = records[records.length - 1];
        return {
            uuid, name: latest.provider_name || uuid, count: records.length,
            latestError: latest.last_error || undefined, latestRaw: latest.raw_response, windows: Array.from(windows.values()),
        };
    }).sort((a, b) => Number(b.windows.length > 0) - Number(a.windows.length > 0) || a.name.localeCompare(b.name));
}

// ── small presentational pieces ─────────────────────────────────────────────

function useValueText() {
    const { t } = useTranslation();
    return (series: QuotaSeries, sample: QuotaSample): string =>
        series.mode === 'percent'
            ? t('dashboard.quotaHistory.percentLeft', { value: number(sample.value), defaultValue: '{{value}}% left' })
            : t('dashboard.quotaHistory.balanceLeft', {
                value: number(sample.value),
                unit: sample.window.currency_code || sample.window.unit || '',
                defaultValue: '{{value}} {{unit}} left',
            });
}

function formatCountdown(ms: number): string {
    const minutes = Math.max(0, Math.round(ms / 60000));
    const days = Math.floor(minutes / 1440);
    const hours = Math.floor((minutes % 1440) / 60);
    const mins = minutes % 60;
    if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
    if (hours > 0) return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
    return `${mins}m`;
}

function StatusChip({ tone, label }: { tone: StatusTone; label: string }) {
    const theme = useTheme();
    const key = statusKey(tone);
    const fill = markColor(theme, tone);
    // Neutral (balances) use a theme surface tint rather than alpha() of the
    // translucent secondary text colour.
    const background = key ? alpha(fill, 0.12) : theme.palette.action.hover;
    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 0.75,
                px: 1,
                py: 0.25,
                borderRadius: radius.pill,
                backgroundColor: background,
                whiteSpace: 'nowrap',
            }}
        >
            <Box sx={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: fill, flex: '0 0 auto' }} />
            <Typography variant="caption" sx={{ fontWeight: 600, fontSize: fontSizes.micro, lineHeight: 1, color: 'text.primary' }}>
                {label}
            </Typography>
        </Box>
    );
}

function Sparkline({ samples, color }: { samples: QuotaSample[]; color: string }) {
    const W = 110;
    const H = 26;
    const P = 2;
    if (samples.length < 2) return <Box sx={{ width: W, height: H, flex: '0 0 auto' }} />;
    const sorted = [...samples].sort((a, b) => a.time - b.time);
    const runs = splitByGaps(sorted);
    const values = sorted.map((s) => s.value);
    let min = Math.min(...values);
    let max = Math.max(...values);
    if (max === min) { min -= 1; max += 1; }
    const t0 = sorted[0].time;
    const t1 = sorted[sorted.length - 1].time || t0 + 1;
    const x = (tm: number) => P + (t1 === t0 ? 0 : ((tm - t0) / (t1 - t0)) * (W - 2 * P));
    const y = (v: number) => H - P - ((v - min) / (max - min)) * (H - 2 * P);
    return (
        <Box component="svg" width={W} height={H} viewBox={`0 0 ${W} ${H}`} sx={{ flex: '0 0 auto', display: 'block' }} aria-hidden>
            {runs.map((run, i) =>
                run.length > 1 ? (
                    <polyline
                        key={i}
                        points={run.map((s) => `${x(s.time).toFixed(1)},${y(s.value).toFixed(1)}`).join(' ')}
                        fill="none"
                        stroke={color}
                        strokeWidth={1.5}
                        strokeLinejoin="round"
                    />
                ) : (
                    <circle key={i} cx={x(run[0].time)} cy={y(run[0].value)} r={1.5} fill={color} />
                ),
            )}
        </Box>
    );
}

function toneLabel(t: (k: string) => string, tone: StatusTone): string {
    if (tone === 'success') return t('dashboard.quotaHistory.statusHealthy');
    if (tone === 'warning') return t('dashboard.quotaHistory.statusLow');
    if (tone === 'error') return t('dashboard.quotaHistory.statusCritical');
    return t('dashboard.quotaHistory.statusBalance');
}

// ── charts ──────────────────────────────────────────────────────────────────

const axisTick = (theme: Theme) => ({ fontSize: 11, fill: theme.palette.text.secondary });

function LineTooltip({ active, payload, series, language }: any) {
    const { t } = useTranslation();
    const valueText = useValueText();
    if (!active || !payload?.length) return null;
    const sample: QuotaSample | undefined = payload.find((p: any) => p.value != null)?.payload?.sample;
    if (!sample) return null;
    return (
        <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: radiusPx.row, p: 1.25 }}>
            <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
                {new Date(sample.time).toLocaleString(language)}
            </Typography>
            <Typography variant="caption" sx={{ display: 'block' }}>{valueText(series, sample)}</Typography>
            {series.mode === 'percent' && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                    {t('dashboard.quotaHistory.usedInline', { value: formatQuotaUsage(sample.window, { formatNumber: number }), defaultValue: 'used {{value}}' })}
                </Typography>
            )}
        </Box>
    );
}

function BandTooltip({ active, payload }: any) {
    const { t } = useTranslation();
    if (!active || !payload?.length) return null;
    const datum = payload[0].payload;
    if (!datum?.label) return null;
    return (
        <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: radiusPx.row, p: 1.25 }}>
            <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>{datum.label}</Typography>
            {datum.hasData ? (
                <>
                    <Typography variant="caption" sx={{ display: 'block' }}>
                        {t('dashboard.quotaHistory.lowHigh', { low: number(datum.lowVal), high: number(datum.highVal), defaultValue: 'low {{low}} · high {{high}}' })}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                        {t('dashboard.quotaHistory.latestValue', { value: number(datum.latest), defaultValue: 'latest {{value}}' })}
                    </Typography>
                </>
            ) : (
                <Typography variant="caption" color="text.secondary">{t('dashboard.quotaHistory.noData', { defaultValue: 'No data' })}</Typography>
            )}
        </Box>
    );
}

function QuotaChart({ series, language, daily }: { series: QuotaSeries; language: string; daily: boolean }) {
    const theme = useTheme();
    const { t } = useTranslation();
    const valueText = useValueText();
    const isPercent = series.mode === 'percent';

    const chart = useMemo(() => {
        const sorted = [...series.samples].sort((a, b) => a.time - b.time);
        if (!sorted.length) return { kind: 'line' as const, bands: [] as ReturnType<typeof dayBands>, data: [] as any[], t0: 0, t1: 1, hasGap: false };
        if (daily) {
            const bands = dayBands(sorted);
            const data = bands.map((b) => (b.hasData
                ? { label: b.label, low: b.low, band: b.high - b.low, latest: b.latest, lowVal: b.low, highVal: b.high, hasData: true }
                : { label: b.label, highVal: 0, lowVal: 0, latest: 0, hasData: false }));
            return { kind: 'band' as const, bands, data, t0: 0, t1: 0, hasGap: hasInternalGap(bands) };
        }
        const threshold = gapThreshold(sorted);
        const data: any[] = [];
        for (let i = 0; i < sorted.length; i++) {
            data.push({ time: sorted[i].time, value: sorted[i].value, sample: sorted[i] });
            if (i < sorted.length - 1 && sorted[i + 1].time - sorted[i].time > threshold) {
                data.push({ time: (sorted[i].time + sorted[i + 1].time) / 2, value: null, gap: true });
            }
        }
        let t0 = sorted[0].time;
        let t1 = sorted[sorted.length - 1].time;
        if (t1 === t0) { t0 -= 30 * 60 * 1000; t1 += 30 * 60 * 1000; }
        return { kind: 'line' as const, bands: [] as ReturnType<typeof dayBands>, data, t0, t1, hasGap: data.some((d) => d.gap) };
    }, [series.samples, daily]);

    const latest = useMemo(
        () => [...series.samples].sort((a, b) => b.time - a.time)[0],
        [series.samples],
    );
    if (!latest) return null;

    const tone: StatusTone = isPercent ? quotaTone(latest.value) : 'neutral';
    const lineColor = isPercent ? markColor(theme, tone) : theme.palette.primary.main;
    const dangerTint = alpha(theme.palette.error.main, 0.07);
    const axisColor = theme.palette.divider;
    const tickStyle = axisTick(theme);

    const delta = seriesDelta([...series.samples].sort((a, b) => a.time - b.time));
    const deltaUnit = isPercent ? '%' : ` ${latest.window.currency_code || latest.window.unit || ''}`;
    const deltaText = `${delta > 0 ? '+' : delta < 0 ? '-' : ''}${number(Math.abs(delta))}${deltaUnit}`;
    const deltaColor = delta > 0 ? statusText(theme, 'success') : delta < 0 ? statusText(theme, 'error') : theme.palette.text.secondary;

    const resetsAt = latest.window.resets_at ? Date.parse(latest.window.resets_at) : NaN;
    const countdown = Number.isFinite(resetsAt) && resetsAt > Date.now() ? formatCountdown(resetsAt - Date.now()) : undefined;
    const resetMarker = chart.kind === 'line' && Number.isFinite(resetsAt) && resetsAt >= chart.t0 && resetsAt <= chart.t1 ? resetsAt : undefined;

    const tickTime = (tm: number) => new Date(tm).toLocaleString(language, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    const yTick = isPercent ? (v: number) => `${v}%` : (v: number) => number(v);
    const legendParts = [
        chart.kind === 'band' ? t('dashboard.quotaHistory.bandLegend', { defaultValue: 'Band = daily low–high · line = day’s latest' }) : '',
        chart.hasGap ? t('dashboard.quotaHistory.gapLegend', { defaultValue: 'Gaps = no data' }) : '',
    ].filter(Boolean);

    return (
        <Box sx={{ minWidth: 0, p: 2, border: '1px solid', borderColor: 'divider', borderRadius: radius.card, backgroundColor: 'background.paper' }}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1, mb: 1 }}>
                <Stack direction="row" sx={{ alignItems: 'center', gap: 1, minWidth: 0 }}>
                    <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>{series.label}</Typography>
                    <StatusChip tone={tone} label={toneLabel(t as any, tone)} />
                </Stack>
                <Box sx={{ textAlign: 'right', flex: '0 0 auto' }}>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>{valueText(series, latest)}</Typography>
                    <Box sx={{ display: 'flex', gap: 0.75, justifyContent: 'flex-end', alignItems: 'baseline', flexWrap: 'wrap' }}>
                        <Typography variant="caption" sx={{ color: deltaColor }}>{t('dashboard.quotaHistory.rangeDelta', { value: deltaText, defaultValue: '{{value}} over range' })}</Typography>
                        {countdown && (
                            <Typography variant="caption" color="text.secondary">
                                · {t('dashboard.quotaHistory.resetsIn', { time: countdown, defaultValue: 'resets in {{time}}' })}
                            </Typography>
                        )}
                    </Box>
                </Box>
            </Stack>
            <Box sx={{ width: '100%', height: 200 }}>
                <ResponsiveContainer width="100%" height="100%">
                    {chart.kind === 'band' ? (
                        <ComposedChart data={chart.data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }} barCategoryGap="18%">
                            {isPercent && <ReferenceArea y1={0} y2={DANGER_PERCENT} fill={dangerTint} fillOpacity={1} />}
                            <CartesianGrid vertical={false} stroke={axisColor} strokeDasharray="3 3" strokeOpacity={0.6} />
                            <XAxis dataKey="label" tick={tickStyle} tickLine={false} minTickGap={16} height={26} />
                            <YAxis domain={isPercent ? [0, 100] : [0, 'auto']} tickFormatter={yTick} tick={tickStyle} tickLine={false} width={46} />
                            <Tooltip cursor={{ fill: theme.palette.action.hover }} content={<BandTooltip />} />
                            <Bar dataKey="low" stackId="band" fill="transparent" isAnimationActive={false} />
                            <Bar dataKey="band" stackId="band" maxBarSize={22} radius={[2, 2, 2, 2]} isAnimationActive={false}>
                                {chart.data.map((d: any, i: number) => (
                                    <Cell
                                        key={i}
                                        fill={d.hasData ? alpha(isPercent ? markColor(theme, quotaTone(d.latest)) : theme.palette.primary.main, 0.3) : 'transparent'}
                                    />
                                ))}
                            </Bar>
                            <Line type="linear" dataKey="latest" stroke={theme.palette.text.secondary} strokeWidth={1.5} dot={false} connectNulls={false} isAnimationActive={false} />
                            {chart.bands.filter((b) => !b.hasData).map((b) => (
                                <ReferenceLine key={b.key} x={b.label} stroke={axisColor} strokeDasharray="2 3" />
                            ))}
                        </ComposedChart>
                    ) : (
                        <LineChart data={chart.data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
                            {isPercent && <ReferenceArea y1={0} y2={DANGER_PERCENT} fill={dangerTint} fillOpacity={1} />}
                            <CartesianGrid vertical={false} stroke={axisColor} strokeDasharray="3 3" strokeOpacity={0.6} />
                            <XAxis dataKey="time" type="number" domain={[chart.t0, chart.t1]} allowDataOverflow tickFormatter={tickTime} tick={tickStyle} tickLine={false} minTickGap={32} height={26} />
                            <YAxis domain={isPercent ? [0, 100] : [0, 'auto']} tickFormatter={yTick} tick={tickStyle} tickLine={false} width={46} />
                            <Tooltip cursor={{ stroke: axisColor }} content={<LineTooltip series={series} language={language} />} />
                            {resetMarker && <ReferenceLine x={resetMarker} stroke={theme.palette.text.disabled} strokeDasharray="4 4" />}
                            <Line type="linear" dataKey="value" stroke={lineColor} strokeWidth={2} dot={false} activeDot={{ r: 3 }} connectNulls={false} isAnimationActive={false} />
                        </LineChart>
                    )}
                </ResponsiveContainer>
            </Box>
            {legendParts.length > 0 && (
                <Typography variant="caption" color="text.secondary">{legendParts.join(' · ')}</Typography>
            )}
        </Box>
    );
}

function DeltaText({ series }: { series: QuotaSeries }) {
    const theme = useTheme();
    const { t } = useTranslation();
    const sorted = [...series.samples].sort((a, b) => a.time - b.time);
    const delta = seriesDelta(sorted);
    const unit = series.mode === 'percent' ? '%' : ` ${sorted[0]?.window.currency_code || sorted[0]?.window.unit || ''}`;
    const text = `${delta > 0 ? '+' : delta < 0 ? '-' : ''}${number(Math.abs(delta))}${unit}`;
    const color = delta > 0 ? statusText(theme, 'success') : delta < 0 ? statusText(theme, 'error') : theme.palette.text.secondary;
    return (
        <Typography variant="caption" sx={{ color }}>
            {t('dashboard.quotaHistory.rangeDelta', { value: text, defaultValue: '{{value}} over range' })}
        </Typography>
    );
}

function providerUrgency(p: ProviderSeries): { rank: number; min: number } {
    const percent = p.windows.filter((w) => w.mode === 'percent');
    if (!percent.length) return { rank: 3, min: Number.POSITIVE_INFINITY };
    const min = Math.min(...percent.map((w) => w.samples[w.samples.length - 1]?.value ?? 100));
    const tone = quotaTone(min);
    return { rank: tone === 'error' ? 0 : tone === 'warning' ? 1 : 2, min };
}

function Watchboard({ providers }: { providers: ProviderSeries[] }) {
    const theme = useTheme();
    const { t } = useTranslation();
    const valueText = useValueText();

    const rows = useMemo(() => {
        const list: Array<{ key: string; provider: ProviderSeries; series: QuotaSeries; sample: QuotaSample; tone: StatusTone; rank: number }> = [];
        for (const p of providers) {
            for (const s of p.windows) {
                const sample = s.samples[s.samples.length - 1];
                if (!sample) continue;
                const tone: StatusTone = s.mode === 'percent' ? quotaTone(sample.value) : 'neutral';
                const rank = tone === 'error' ? 0 : tone === 'warning' ? 1 : tone === 'success' ? 2 : 3;
                list.push({ key: `${p.uuid}:${s.key}`, provider: p, series: s, sample, tone, rank });
            }
        }
        list.sort((a, b) =>
            a.rank - b.rank
            || (a.series.mode === 'percent' && b.series.mode === 'percent' ? a.sample.value - b.sample.value : 0)
            || a.provider.name.localeCompare(b.provider.name)
            || a.series.label.localeCompare(b.series.label));
        return list;
    }, [providers]);

    if (!rows.length) return null;

    return (
        <Box sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: radius.card, backgroundColor: 'background.paper' }}>
            <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', gap: 1, mb: 0.5 }}>
                <Typography sx={{ fontWeight: 600, fontSize: fontSizes.base }}>{t('dashboard.quotaHistory.watchTitle', { defaultValue: 'Quota watch' })}</Typography>
                <Typography variant="caption" color="text.secondary">{t('dashboard.quotaHistory.watchHint', { defaultValue: 'Lowest remaining first' })}</Typography>
            </Stack>
            <Box sx={{ maxHeight: 320, overflowY: 'auto', pr: 0.5 }}>
            {rows.map((r) => (
                <Box
                    key={r.key}
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: 'auto minmax(0, 1fr) auto 110px',
                        alignItems: 'center',
                        columnGap: 2,
                        py: 0.75,
                        borderTop: '1px solid',
                        borderColor: 'divider',
                    }}
                >
                    <StatusChip tone={r.tone} label={toneLabel(t as any, r.tone)} />
                    <Typography variant="body2" noWrap sx={{ minWidth: 0 }}>
                        <Box component="span" sx={{ fontWeight: 600 }}>{r.provider.name}</Box>
                        <Box component="span" sx={{ color: 'text.secondary' }}>{` · ${r.series.label}`}</Box>
                    </Typography>
                    <Box sx={{ textAlign: 'right' }}>
                        <Typography variant="body2" sx={{ fontWeight: 600 }} noWrap>{valueText(r.series, r.sample)}</Typography>
                        <DeltaText series={r.series} />
                    </Box>
                    <Sparkline samples={r.series.samples} color={markColor(theme, r.tone)} />
                </Box>
            ))}
            </Box>
        </Box>
    );
}

export default function QuotaHistoryView({ startTime, endTime, provider, daily, refreshKey = 0 }: Props) {
    const { t, i18n } = useTranslation();
    const [snapshots, setSnapshots] = useState<ProviderQuota[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(false);
    const requestSeq = useRef(0);

    useEffect(() => {
        const seq = ++requestSeq.current;
        const params = new URLSearchParams({ start_time: startTime, end_time: endTime, limit: '1000' });
        if (daily) params.set('daily', 'true');
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
    }, [startTime, endTime, provider, daily, refreshKey]);

    const providers = useMemo(() => groupSnapshots(snapshots), [snapshots]);
    // Most urgent provider first (lowest remaining across its percent windows).
    const ordered = useMemo(
        () => [...providers].sort((a, b) => {
            const ua = providerUrgency(a);
            const ub = providerUrgency(b);
            return ua.rank - ub.rank || ua.min - ub.min || a.name.localeCompare(b.name);
        }),
        [providers],
    );

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
            {snapshots.length >= 1000 && <Alert severity="info">{t('dashboard.quotaHistory.limitHint', { defaultValue: 'Showing the latest 1,000 points. Choose a provider or shorter time range to see more.' })}</Alert>}
            <Watchboard providers={providers} />
            {ordered.map((item) => {
                const percentWindows = item.windows.filter((w) => w.mode === 'percent');
                const worstTone: StatusTone = percentWindows.length
                    ? quotaTone(Math.min(...percentWindows.map((w) => w.samples[w.samples.length - 1]?.value ?? 100)))
                    : 'neutral';
                return (
                    <Box key={item.uuid}>
                        <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', gap: 1, mb: 1.25, px: 0.5 }}>
                            <Stack direction="row" sx={{ alignItems: 'center', gap: 1, minWidth: 0 }}>
                                <ProviderLabel uuid={item.uuid} size={18}>
                                    <Typography variant="h6" sx={{ fontWeight: 600 }}>{item.name}</Typography>
                                </ProviderLabel>
                                <StatusChip tone={worstTone} label={toneLabel(t as any, worstTone)} />
                            </Stack>
                            <Stack direction="row" sx={{ alignItems: 'center', gap: 1 }}>
                                <QuotaRawResponseButton providerName={item.name} response={item.latestRaw} />
                                <Typography variant="caption" color="text.secondary">
                                    {t('dashboard.quotaHistory.sampleCount', { defaultValue: '{{count}} samples', count: item.count })}
                                </Typography>
                            </Stack>
                        </Stack>
                        {item.latestError && <Alert severity="warning" sx={{ mb: 2 }}>{item.latestError}</Alert>}
                        {item.windows.length ? (
                            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'repeat(2, minmax(0, 1fr))' }, gap: 2 }}>
                                {item.windows.map((series) => <QuotaChart key={series.key} series={series} language={i18n.language} daily={daily} />)}
                            </Box>
                        ) : (
                            <Typography variant="body2" color="text.secondary">{t('dashboard.quotaHistory.noWindows', { defaultValue: 'No quota windows reported' })}</Typography>
                        )}
                    </Box>
                );
            })}
        </Stack>
    );
}



