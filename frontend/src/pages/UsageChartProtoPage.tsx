// TEMPORARY PROTOTYPE — not linked from the app nav and not the default
// dashboard path. Renders the same deterministic dataset twice (line/area vs
// bar) so the two forms can be compared side by side with real theme colours
// and the existing recharts dependency. Mounted only at /prototype/usage-charts
// for screenshots; delete this file + its route when the decision is made.
import { useMemo } from 'react';
import { Box, Typography } from '@mui/material';
import {
    Area,
    AreaChart,
    Bar,
    BarChart,
    CartesianGrid,
    Cell,
    Line,
    LineChart,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from 'recharts';
import { useTheme } from '@mui/material/styles';
import { formatNumber, getThemeChartStyles } from '@/components/dashboard/chartStyles';
import { chartCursorFill } from '@/theme/dataviz';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

// ── deterministic data ──────────────────────────────────────────────────────
// Seedable PRNG so every screenshot of a scenario is byte-for-byte stable.
function mulberry32(seed: number) {
    return function () {
        seed |= 0;
        seed = (seed + 0x6d2b79f5) | 0;
        let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

interface Series {
    key: string;
    name: string;
    color: string;
}

interface Point {
    label: string; // axis tick
    full: string; // tooltip heading
    // series values live at the top level so recharts `dataKey` can read them
    [key: string]: string | number;
}

interface Scenario {
    id: string;
    title: string;
    note: string;
    points: Point[];
    series: Series[];
    stacked: boolean;
    /** index of an incomplete (still-running) bucket, styled specially in the bar form */
    partialIndex?: number;
    /** area (stacked) instead of a plain line for the "line" form */
    lineAsArea?: boolean;
}

const pad = (n: number) => String(n).padStart(2, '0');
const dayLabel = (d: Date) => `${pad(d.getMonth() + 1)}/${pad(d.getDate())}`;
const hourLabel = (d: Date) => `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:00`;
const dayFull = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
const hourFull = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit' });

// Weekday rhythm: weekends quiet, a little growth toward the end.
function weekdayFactor(d: Date) {
    const dow = d.getDay();
    return dow === 0 || dow === 6 ? 0.32 : 1;
}

function buildDailyTotals(days: number, seed: number, base: number, opts?: { partialLast?: boolean }): Point[] {
    const rnd = mulberry32(seed);
    const out: Point[] = [];
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    for (let i = days - 1; i >= 0; i--) {
        const d = new Date(today);
        d.setDate(d.getDate() - i);
        const growth = 1 + ((days - 1 - i) / days) * 0.5;
        const noise = 0.82 + rnd() * 0.36;
        let total = base * weekdayFactor(d) * growth * noise;
        const isPartial = !!opts?.partialLast && i === 0;
        if (isPartial) total *= 0.3; // only ~30% of today has elapsed
        out.push({ label: dayLabel(d), full: dayFull(d), total: Math.round(total) });
    }
    return out;
}

function buildDailyStack(days: number, seed: number): Point[] {
    const rnd = mulberry32(seed);
    const out: Point[] = [];
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    for (let i = days - 1; i >= 0; i--) {
        const d = new Date(today);
        d.setDate(d.getDate() - i);
        const growth = 1 + ((days - 1 - i) / days) * 0.45;
        const wf = weekdayFactor(d);
        const cache = Math.round(9_000_000 * wf * growth * (0.9 + rnd() * 0.2));
        const input = Math.round(2_200_000 * wf * growth * (0.85 + rnd() * 0.3));
        const output = Math.round(900_000 * wf * growth * (0.8 + rnd() * 0.4));
        out.push({ label: dayLabel(d), full: dayFull(d), cache, input, output });
    }
    return out;
}

function buildHourlyTotals(days: number, seed: number): Point[] {
    const rnd = mulberry32(seed);
    const out: Point[] = [];
    const start = new Date();
    start.setMinutes(0, 0, 0);
    const count = days * 24;
    for (let i = count - 1; i >= 0; i--) {
        const d = new Date(start);
        d.setHours(d.getHours() - i);
        const hour = d.getHours();
        // Diurnal curve: near-zero overnight, peak mid-afternoon.
        const diurnal = 0.08 + 0.92 * Math.max(0, Math.sin(((hour - 6) / 24) * Math.PI * 2 - Math.PI / 2) * 0.5 + 0.5);
        const total = 520_000 * weekdayFactor(d) * diurnal * (0.75 + rnd() * 0.5);
        out.push({ label: hourLabel(d), full: hourFull(d), total: Math.round(total) });
    }
    return out;
}

function buildProviderTotals(days: number, seed: number): Point[] {
    const rnd = mulberry32(seed);
    const out: Point[] = [];
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    for (let i = days - 1; i >= 0; i--) {
        const d = new Date(today);
        d.setDate(d.getDate() - i);
        const wf = weekdayFactor(d);
        const progress = (days - 1 - i) / (days - 1);
        // Providers cross over: OpenAI grows into the leader, OpenRouter trails.
        const anthropic = 5_200_000 * (1 - progress * 0.3) * wf * (0.85 + rnd() * 0.3);
        const openai = 3_000_000 * (1 + progress * 0.9) * wf * (0.85 + rnd() * 0.3);
        const openrouter = 1_100_000 * (1 + progress * 0.2) * wf * (0.8 + rnd() * 0.4);
        out.push({
            label: dayLabel(d),
            full: dayFull(d),
            anthropic: Math.round(anthropic),
            openai: Math.round(openai),
            openrouter: Math.round(openrouter),
        });
    }
    return out;
}

const TOTAL_SERIES: Series[] = [{ key: 'total', name: 'Total tokens', color: '#3B82F6' }];

const STACK_SERIES: Series[] = [
    { key: 'cache', name: 'Cache Read', color: '#cbd5e1' },
    { key: 'input', name: 'Input', color: '#3B82F6' },
    { key: 'output', name: 'Output', color: '#10B981' },
];

// Validated categorical order (blue → orange → aqua) from the dataviz reference.
const PROVIDER_SERIES: Series[] = [
    { key: 'anthropic', name: 'Anthropic', color: '#2a78d6' },
    { key: 'openai', name: 'OpenAI', color: '#eb6834' },
    { key: 'openrouter', name: 'OpenRouter', color: '#1baf7a' },
];

// ── shared chart bits ───────────────────────────────────────────────────────
function useAxis() {
    const theme = useTheme();
    const styles = getThemeChartStyles(theme);
    return {
        tick: { fontSize: 11, fill: theme.palette.text.secondary } as const,
        grid: styles.chart.grid,
        axis: styles.chart.axis,
    };
}

function ProtoTooltip({ active, payload, label }: any) {
    if (!active || !payload || !payload.length) return null;
    const point = payload[0].payload as Point;
    const rows = payload as any[];
    const total = rows.reduce((s, r) => s + (r.value || 0), 0);
    return (
        <Box
            sx={{
                backgroundColor: 'background.paper',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: radius.card,
                p: 1.25,
                minWidth: 180,
            }}
        >
            <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 0.75 }}>
                {point.full}
            </Typography>
            {rows.map((r) => (
                <Box key={r.dataKey} sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                        <Box sx={{ width: 9, height: 9, borderRadius: '2px', backgroundColor: r.color }} />
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>{r.name}</Typography>
                    </Box>
                    <Typography variant="caption" sx={{ fontWeight: 600 }}>{formatNumber(r.value)}</Typography>
                </Box>
            ))}
            {rows.length > 1 && (
                <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, mt: 0.5, pt: 0.5, borderTop: '1px dashed', borderColor: 'divider' }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>Total</Typography>
                    <Typography variant="caption" sx={{ fontWeight: 600 }}>{formatNumber(total)}</Typography>
                </Box>
            )}
        </Box>
    );
}

function Legend({ series }: { series: Series[] }) {
    if (series.length < 2) return null;
    return (
        <Box sx={{ display: 'flex', gap: 2, mb: 0.5, flexWrap: 'wrap' }}>
            {series.map((s) => (
                <Box key={s.key} sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    <Box sx={{ width: 11, height: 11, borderRadius: '2px', backgroundColor: s.color }} />
                    <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: fontSizes.sm }}>{s.name}</Typography>
                </Box>
            ))}
        </Box>
    );
}

const CHART_H = 230;

interface FormProps {
    scenario: Scenario;
    form: 'line' | 'bar';
}

function ChartForm({ scenario, form }: FormProps) {
    const axis = useAxis();
    const { points, series, stacked, partialIndex } = scenario;
    const n = points.length;
    const interval = n <= 16 ? 0 : n <= 60 ? Math.ceil(n / 7) - 1 : Math.ceil(n / 8) - 1;
    const dense = points.length > 45;
    const isStackedArea = form === 'line' && scenario.lineAsArea;
    const xTick = dense ? { ...axis.tick, angle: -32, textAnchor: 'end' as const } : axis.tick;

    const commonAxes = (
        <>
            <CartesianGrid strokeDasharray="3 3" stroke={axis.grid} strokeOpacity={0.6} vertical={false} />
            <XAxis
                dataKey="label"
                tick={xTick}
                tickLine={false}
                axisLine={{ stroke: axis.axis, strokeWidth: 1.5 }}
                interval={interval}
                minTickGap={8}
                height={dense ? 54 : 38}
            />
            <YAxis
                tickFormatter={formatNumber}
                tick={axis.tick}
                tickLine={false}
                axisLine={{ stroke: axis.axis, strokeWidth: 1.5 }}
                width={54}
                domain={[0, 'auto']}
            />
            <Tooltip content={<ProtoTooltip />} cursor={{ fill: chartCursorFill }} />
        </>
    );

    if (form === 'line') {
        if (isStackedArea) {
            return (
                <ResponsiveContainer width="100%" height={CHART_H}>
                    <AreaChart data={points} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
                        <defs>
                            {series.map((s) => (
                                <linearGradient key={s.key} id={`area-${scenario.id}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={s.color} stopOpacity={0.55} />
                                    <stop offset="95%" stopColor={s.color} stopOpacity={0.15} />
                                </linearGradient>
                            ))}
                        </defs>
                        {commonAxes}
                        {series.map((s) => (
                            <Area
                                key={s.key}
                                type="monotone"
                                dataKey={s.key}
                                name={s.name}
                                stackId="1"
                                stroke={s.color}
                                strokeWidth={1.5}
                                fill={`url(#area-${scenario.id}-${s.key})`}
                                dot={false}
                                activeDot={{ r: 3 }}
                            />
                        ))}
                    </AreaChart>
                </ResponsiveContainer>
            );
        }
        return (
            <ResponsiveContainer width="100%" height={CHART_H}>
                <LineChart data={points} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
                    {commonAxes}
                    {series.map((s) => (
                        <Line
                            key={s.key}
                            type="monotone"
                            dataKey={s.key}
                            name={s.name}
                            stroke={s.color}
                            strokeWidth={2}
                            dot={dense ? false : { r: 2.5, strokeWidth: 0 }}
                            activeDot={{ r: 4 }}
                            isAnimationActive={false}
                        />
                    ))}
                </LineChart>
            </ResponsiveContainer>
        );
    }

    // bar form
    return (
        <ResponsiveContainer width="100%" height={CHART_H}>
            <BarChart data={points} margin={{ top: 8, right: 8, bottom: 0, left: 0 }} barCategoryGap={stacked ? '18%' : '12%'} barGap={2}>
                <defs>
                    {series.map((s) => (
                        <pattern
                            key={s.key}
                            id={`hatch-${scenario.id}-${s.key}`}
                            width="6"
                            height="6"
                            patternUnits="userSpaceOnUse"
                            patternTransform="rotate(45)"
                        >
                            <rect width="6" height="6" fill="transparent" />
                            <line x1="0" y1="0" x2="0" y2="6" stroke={s.color} strokeWidth="2" />
                        </pattern>
                    ))}
                </defs>
                {commonAxes}
                {series.map((s, si) => {
                    const isTop = stacked ? si === series.length - 1 : true;
                    return (
                        <Bar
                            key={s.key}
                            dataKey={s.key}
                            name={s.name}
                            stackId={stacked ? '1' : undefined}
                            fill={s.color}
                            radius={isTop && !dense ? [3, 3, 0, 0] : 0}
                            isAnimationActive={false}
                        >
                            {partialIndex !== undefined &&
                                points.map((_, i) => (
                                    <Cell
                                        key={i}
                                        fill={i === partialIndex ? `url(#hatch-${scenario.id}-${s.key})` : s.color}
                                        stroke={i === partialIndex ? s.color : 'none'}
                                        strokeDasharray={i === partialIndex ? '2 2' : undefined}
                                    />
                                ))}
                        </Bar>
                    );
                })}
            </BarChart>
        </ResponsiveContainer>
    );
}

function FormCard({ scenario, form }: FormProps) {
    const label = form === 'line' ? (scenario.lineAsArea ? 'Line / area (existing style)' : 'Line') : 'Bar (prototype)';
    const partialLine = scenario.partialIndex !== undefined;
    return (
        <Box
            data-shot={`${form}-${scenario.id}`}
            sx={{
                px: 2,
                pt: 1.5,
                pb: 1.5,
                borderRadius: radius.card,
                border: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                minWidth: 0,
            }}
        >
            <Typography sx={{ fontWeight: 600, fontSize: fontSizes.sm, color: 'text.secondary', mb: 1 }}>
                {label}
            </Typography>
            <Legend series={scenario.series} />
            <ChartForm scenario={scenario} form={form} />
            {partialLine && (
                <Typography variant="caption" sx={{ color: 'text.disabled', display: 'block', mt: 0.5 }}>
                    {form === 'bar'
                        ? 'Hatched bar = today, still running (not a real drop).'
                        : 'Solid segment down to today’s partial value — reads as a crash unless the bucket is marked.'}
                </Typography>
            )}
        </Box>
    );
}

function ScenarioSection({ scenario }: { scenario: Scenario }) {
    return (
        <Box sx={{ mb: 4 }} data-scenario={scenario.id}>
            <Typography sx={{ fontWeight: 700, fontSize: fontSizes.title, color: 'text.primary' }}>
                {scenario.title}
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.25, mb: 1.5, maxWidth: 1000 }}>
                {scenario.note}
            </Typography>
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: 'minmax(0, 1fr)',
                    '@media (min-width: 1100px)': { gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)' },
                    gap: 2,
                }}
            >
                <FormCard scenario={scenario} form="line" />
                <FormCard scenario={scenario} form="bar" />
            </Box>
        </Box>
    );
}

export default function UsageChartProtoPage() {
    const scenarios = useMemo<Scenario[]>(() => {
        const s1: Scenario = {
            id: '30d',
            title: '1 · 30 days, daily, single series (total)',
            note: 'Magnitude over time. Both forms start at 0 and share one y-axis. Bars make each day’s total a discrete quantity you can compare by height; the line emphasises shape and change between days.',
            points: buildDailyTotals(30, 11, 8_000_000),
            series: TOTAL_SERIES,
            stacked: false,
        };
        const s2: Scenario = {
            id: '7d-hourly',
            title: '2 · 7 days at hourly resolution (~168 points)',
            note: 'Dense buckets. The line stays legible and shows the diurnal rhythm; bars collapse into ~1px slivers with almost no gap — this is where a bar chart degrades and should aggregate or fall back to a line.',
            points: buildHourlyTotals(7, 22),
            series: TOTAL_SERIES,
            stacked: false,
        };
        const s3: Scenario = {
            id: '30d-stacked',
            title: '3 · 30 days, daily, stacked composition (cache / input / output)',
            note: 'Part-to-whole. Stacked bars read the total height and each band as a share; the stacked area version reads as a continuous volume. Bars make the day boundaries and per-band magnitude clearer; areas smear adjacent days together.',
            points: buildDailyStack(30, 33),
            series: STACK_SERIES,
            stacked: true,
            lineAsArea: true,
        };
        const s4: Scenario = {
            id: 'providers',
            title: '4 · 14 days, multi-series (by provider)',
            note: 'Identity: Anthropic / OpenAI / OpenRouter. Grouped bars let you compare providers within a day but triple the ink at every bucket; lines are cleaner for reading crossovers and crossing trends.',
            points: buildProviderTotals(14, 44),
            series: PROVIDER_SERIES,
            stacked: false,
        };
        const s5: Scenario = {
            id: 'partial-today',
            title: '5 · Incomplete “today” bucket (last bar hatched)',
            note: 'The last bucket is today and only ~30% elapsed. Rendered naively both forms look like a crash. The bar form hatches/dashes the partial bucket; a line needs the same treatment (dashed final segment + hollow marker, or omit today) or the drop will be misread.',
            points: buildDailyTotals(14, 55, 7_000_000, { partialLast: true }),
            series: TOTAL_SERIES,
            stacked: false,
            partialIndex: 13,
        };
        return [s1, s2, s3, s4, s5];
    }, []);

    return (
        <Box sx={{ p: 3, maxWidth: 1500, mx: 'auto' }}>
            <Typography variant="h5" sx={{ fontWeight: 700 }}>
                Usage chart prototype — bar vs line
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5, mb: 2.5, maxWidth: 1000 }}>
                Identical deterministic data rendered in both forms with the app theme, existing recharts, a shared 0-based
                y-axis and the same tooltip. Note: the current dashboard already draws the daily (3d–90d) trend as a
                <b> stacked bar</b> (DailyChart) and the hourly (today/yesterday) trend as a <b>stacked area</b> (HourlyChart);
                this prototype isolates the form choice and also exercises 7d-hourly and multi-series cases the dashboard
                does not currently offer.
            </Typography>
            {scenarios.map((s) => (
                <ScenarioSection key={s.id} scenario={s} />
            ))}
        </Box>
    );
}
