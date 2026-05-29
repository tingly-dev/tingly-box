import { useState } from 'react';
import {
    Box,
    Typography,
    ToggleButtonGroup,
    ToggleButton,
    CircularProgress,
    useTheme,
} from '@mui/material';
import {
    ScatterChart,
    Scatter,
    XAxis,
    YAxis,
    ZAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
} from 'recharts';
import { ChartWrapper, LegendItem } from './TokenHistoryChart/components';
import { getThemeChartStyles } from './chartStyles';

export interface UsageRecord {
    id: number;
    provider_uuid: string;
    provider_name: string;
    model: string;
    scenario: string;
    rule_uuid?: string;
    user_id?: string;
    request_model?: string;
    timestamp: string;
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
    cache_input_tokens: number;
    status: string;
    error_code?: string;
    latency_ms: number;
    streamed: boolean;
}

type YMetric = 'tokens' | 'latency';

interface ScatterPoint {
    x: number;
    y: number;
    time: string;
    model: string;
    provider: string;
    status: string;
    error_code?: string;
    input_tokens: number;
    output_tokens: number;
    cache_input_tokens: number;
    total_tokens: number;
    latency_ms: number;
    streamed: boolean;
}

interface RequestScatterChartProps {
    records: UsageRecord[];
    loading: boolean;
}

const SUCCESS_COLOR = '#10B981';
const ERROR_COLOR = '#EF4444';

const formatXTick = (ms: number) =>
    new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

const formatYTokens = (n: number) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
    return n.toString();
};

const formatYLatency = (ms: number) => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
    return `${ms}ms`;
};

function TooltipRow({ label, value, color }: { label: string; value: string; color?: string }) {
    return (
        <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
            <Typography sx={{ fontSize: '0.7rem', color: 'text.secondary' }}>{label}:</Typography>
            <Typography sx={{ fontSize: '0.7rem', fontWeight: 600, color: color ?? 'text.primary' }}>
                {value}
            </Typography>
        </Box>
    );
}

function ScatterTooltip({ active, payload }: any) {
    const theme = useTheme();
    if (!active || !payload?.length) return null;
    const d: ScatterPoint = payload[0].payload;
    const total = d.total_tokens || d.input_tokens + d.output_tokens + d.cache_input_tokens;

    return (
        <Box
            sx={{
                backgroundColor: 'background.paper',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                p: 1.75,
                minWidth: 210,
                boxShadow: theme.shadows[4],
            }}
        >
            <Typography sx={{ fontWeight: 600, fontSize: '0.8rem', mb: 0.25 }}>{d.time}</Typography>
            <Typography sx={{ fontSize: '0.68rem', color: 'text.disabled', mb: 0.75 }}>{d.provider}</Typography>
            <Typography
                sx={{
                    fontSize: '0.78rem',
                    fontWeight: 500,
                    mb: 1,
                    maxWidth: 220,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                }}
            >
                {d.model}
            </Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.3 }}>
                <TooltipRow label="Total Tokens" value={total.toLocaleString()} />
                <TooltipRow label="Input" value={d.input_tokens.toLocaleString()} />
                <TooltipRow label="Output" value={d.output_tokens.toLocaleString()} />
                {d.cache_input_tokens > 0 && (
                    <TooltipRow label="Cache" value={d.cache_input_tokens.toLocaleString()} />
                )}
                <TooltipRow
                    label="Latency"
                    value={d.latency_ms > 0 ? `${d.latency_ms}ms` : '-'}
                />
                <TooltipRow
                    label="Status"
                    value={d.status}
                    color={d.status === 'success' ? SUCCESS_COLOR : ERROR_COLOR}
                />
                {d.error_code && (
                    <TooltipRow label="Error" value={d.error_code} color={ERROR_COLOR} />
                )}
                {d.streamed && <TooltipRow label="Streamed" value="yes" />}
            </Box>
        </Box>
    );
}

export default function RequestScatterChart({ records, loading }: RequestScatterChartProps) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);
    const [yMetric, setYMetric] = useState<YMetric>('tokens');
    const [showSuccess, setShowSuccess] = useState(true);
    const [showError, setShowError] = useState(true);

    const toPoint = (r: UsageRecord): ScatterPoint => ({
        x: new Date(r.timestamp).getTime(),
        y:
            yMetric === 'tokens'
                ? r.total_tokens || r.input_tokens + r.output_tokens + r.cache_input_tokens
                : r.latency_ms,
        time: new Date(r.timestamp).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
        }),
        model: r.model || '-',
        provider: r.provider_name || '-',
        status: r.status,
        error_code: r.error_code,
        input_tokens: r.input_tokens,
        output_tokens: r.output_tokens,
        cache_input_tokens: r.cache_input_tokens,
        total_tokens: r.total_tokens,
        latency_ms: r.latency_ms,
        streamed: r.streamed,
    });

    const successCount = records.filter((r) => r.status === 'success').length;
    const errorCount = records.filter((r) => r.status !== 'success').length;

    const successPoints = showSuccess
        ? records.filter((r) => r.status === 'success').map(toPoint)
        : [];
    const errorPoints = showError
        ? records.filter((r) => r.status !== 'success').map(toPoint)
        : [];

    const hasData = records.length > 0;
    const showChart = hasData || loading;

    return (
        <ChartWrapper title="Requests Over Time" hasData={showChart}>
            {showChart ? (
                <>
                    {/* Controls */}
                    <Box
                        sx={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            mb: 2,
                            flexWrap: 'wrap',
                            gap: 1,
                        }}
                    >
                        {/* Legend toggles */}
                        <Box sx={{ display: 'flex', gap: 0.5 }}>
                            <LegendItem
                                label={`Success (${successCount})`}
                                color={SUCCESS_COLOR}
                                visible={showSuccess}
                                onToggle={() => setShowSuccess((v) => !v)}
                            />
                            <LegendItem
                                label={`Error (${errorCount})`}
                                color={ERROR_COLOR}
                                visible={showError}
                                onToggle={() => setShowError((v) => !v)}
                            />
                        </Box>

                        {/* Y-axis metric toggle */}
                        <ToggleButtonGroup
                            value={yMetric}
                            exclusive
                            onChange={(_, v) => v && setYMetric(v)}
                            size="small"
                            sx={{
                                '& .MuiToggleButton-root': {
                                    px: 1.25,
                                    py: 0.25,
                                    fontSize: '0.72rem',
                                    textTransform: 'none',
                                },
                            }}
                        >
                            <ToggleButton value="tokens">Tokens</ToggleButton>
                            <ToggleButton value="latency">Latency</ToggleButton>
                        </ToggleButtonGroup>
                    </Box>

                    {/* Chart area */}
                    <Box sx={{ position: 'relative' }}>
                        {loading && (
                            <Box
                                sx={{
                                    position: 'absolute',
                                    inset: 0,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    zIndex: 1,
                                    backgroundColor:
                                        theme.palette.mode === 'dark'
                                            ? 'rgba(0,0,0,0.35)'
                                            : 'rgba(255,255,255,0.65)',
                                }}
                            >
                                <CircularProgress size={28} />
                            </Box>
                        )}
                        <ResponsiveContainer width="100%" height={280}>
                            <ScatterChart margin={{ top: 4, right: 12, bottom: 0, left: 0 }}>
                                <CartesianGrid
                                    strokeDasharray="3 3"
                                    stroke={chartStyles.chart.grid}
                                    strokeOpacity={0.6}
                                    vertical={false}
                                />
                                <XAxis
                                    dataKey="x"
                                    type="number"
                                    domain={['auto', 'auto']}
                                    tickFormatter={formatXTick}
                                    scale="time"
                                    tick={{
                                        fontSize: 11,
                                        fill: theme.palette.text.secondary,
                                        fontWeight: 500,
                                    }}
                                    tickLine={false}
                                    axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1.5 }}
                                    height={36}
                                />
                                <YAxis
                                    dataKey="y"
                                    type="number"
                                    tickFormatter={yMetric === 'tokens' ? formatYTokens : formatYLatency}
                                    tick={{
                                        fontSize: 11,
                                        fill: theme.palette.text.secondary,
                                        fontWeight: 500,
                                    }}
                                    tickLine={false}
                                    axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1.5 }}
                                    width={56}
                                />
                                <ZAxis range={[30, 30]} />
                                <Tooltip
                                    content={<ScatterTooltip />}
                                    cursor={{
                                        strokeDasharray: '3 3',
                                        stroke: chartStyles.chart.grid,
                                    }}
                                />
                                <Scatter
                                    name="Success"
                                    data={successPoints}
                                    fill={SUCCESS_COLOR}
                                    fillOpacity={0.65}
                                    isAnimationActive={false}
                                />
                                <Scatter
                                    name="Error"
                                    data={errorPoints}
                                    fill={ERROR_COLOR}
                                    fillOpacity={0.8}
                                    isAnimationActive={false}
                                />
                            </ScatterChart>
                        </ResponsiveContainer>
                    </Box>
                </>
            ) : null}
        </ChartWrapper>
    );
}
