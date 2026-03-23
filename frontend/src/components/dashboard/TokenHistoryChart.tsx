import { Paper, Typography, Box, alpha } from '@mui/material';
import { useState } from 'react';
import {
    ComposedChart,
    BarChart,
    Area,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    Legend,
    Cell,
} from 'recharts';
import { TOKEN_COLORS, gridStyle, tooltipStyle, barRadius } from './chartStyles';

export interface TimeSeriesData {
    timestamp: string;
    request_count: number;
    total_tokens?: number;
    input_tokens: number;
    output_tokens: number;
    cache_input_tokens?: number;
    error_count?: number;
    avg_latency_ms?: number;
}

interface TokenHistoryChartProps {
    data: TimeSeriesData[];
    interval?: string;
}

// Series visibility state type
type SeriesKey = 'cache' | 'input' | 'output';
interface SeriesVisibility {
    cache: boolean;
    input: boolean;
    output: boolean;
}

// Shared legend item component with click handler
interface LegendItemProps {
    label: string;
    color: string;
    visible: boolean;
    onToggle: () => void;
}

const LegendItem = ({ label, color, visible, onToggle }: LegendItemProps) => (
    <Box
        onClick={onToggle}
        sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            cursor: 'pointer',
            userSelect: 'none',
            opacity: visible ? 1 : 0.4,
            transition: 'opacity 0.2s ease',
            '&:hover': {
                opacity: visible ? 0.8 : 0.5,
            },
        }}
    >
        <Box sx={{ width: 12, height: 12, borderRadius: 2, backgroundColor: color }} />
        <Typography variant="caption" sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
            {label}
        </Typography>
    </Box>
);

// Shared types
export interface ChartDataPoint {
    timestamp: string;
    time: string;
    timeFull: string;
    inputTokens: number;
    outputTokens: number;
    cacheTokens: number;
}

// Shared utilities
export const formatTimeLabel = (timestamp: string, isDayMode: boolean): string => {
    if (!timestamp) return '';

    let date: Date;
    const timestampNum = parseInt(timestamp, 10);

    if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
        date = new Date(timestampNum * 1000);
    } else {
        date = new Date(timestamp);
    }

    if (isNaN(date.getTime())) {
        console.warn('Invalid timestamp:', timestamp);
        return '';
    }

    const pad = (n: number) => String(n).padStart(2, '0');

    if (isDayMode) {
        return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
    }

    return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:00`;
};

export const formatTooltipTime = (timestamp: string, isDayMode: boolean): string => {
    if (!timestamp) return timestamp;

    let date: Date;
    const timestampNum = parseInt(timestamp, 10);

    if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
        date = new Date(timestampNum * 1000);
    } else {
        date = new Date(timestamp);
    }

    if (isNaN(date.getTime())) {
        return timestamp;
    }

    const options: Intl.DateTimeFormatOptions = {
        month: 'short',
        day: 'numeric',
    };

    // Only show time in hour mode, not in day mode
    if (!isDayMode) {
        options.hour = '2-digit';
        options.minute = '2-digit';
    }

    return date.toLocaleDateString('en-US', options);
};

export const formatChartData = (data: TimeSeriesData[], isDayMode: boolean): ChartDataPoint[] => {
    return data.map((item) => ({
        timestamp: item.timestamp,
        time: formatTimeLabel(item.timestamp, isDayMode),
        timeFull: formatTooltipTime(item.timestamp, isDayMode),
        inputTokens: item.input_tokens,
        outputTokens: item.output_tokens,
        cacheTokens: item.cache_input_tokens || 0,
    }));
};

export const calculateLabelInterval = (dataLength: number): number => {
    if (dataLength <= 7) return 0;
    if (dataLength <= 14) return 1;
    if (dataLength <= 30) return 4;
    return Math.ceil(dataLength / 6);
};

export const formatYAxis = (value: number): string => {
    if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
    return value.toString();
};

export const formatTooltipValue = (value: number): string => {
    if (value >= 1000000) return `${(value / 1000000).toFixed(2)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(2)}K`;
    return value.toLocaleString();
};

// Shared Tooltip Component
export const CustomTooltip = ({ active, payload }: any) => {
    if (!active || !payload || !payload.length) return null;

    const data = payload[0].payload;
    return (
        <Box sx={tooltipStyle}>
            <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, fontSize: '0.875rem' }}>
                {data.timeFull}
            </Typography>
            {payload.map((entry: any, index: number) => (
                <Box
                    key={index}
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        mb: 0.5,
                        fontSize: '0.75rem',
                    }}
                >
                    <Box
                        sx={{
                            width: 12,
                            height: 12,
                            borderRadius: 2,
                            backgroundColor: entry.color,
                        }}
                    />
                    <Typography variant="body2" sx={{ color: 'text.primary' }}>
                        {entry.name}: {formatTooltipValue(entry.value)}
                    </Typography>
                </Box>
            ))}
            <Box sx={{ mt: 1, pt: 1, borderTop: '1px solid #e2e8f0', fontSize: '0.75rem' }}>
                <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    Total: {formatTooltipValue(data.inputTokens + data.outputTokens + data.cacheTokens)}
                </Typography>
            </Box>
        </Box>
    );
};

// Shared wrapper component
interface ChartWrapperProps {
    title: string;
    chartData: ChartDataPoint[];
    children: React.ReactNode;
}

const ChartWrapper = ({ title, chartData, children }: ChartWrapperProps) => (
    <Paper
        elevation={0}
        sx={{
            p: 3,
            borderRadius: 2.5,
            border: '1px solid',
            borderColor: 'divider',
            flexGrow: 1,
            backgroundColor: 'background.paper',
            boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
            display: 'flex',
            flexDirection: 'column',
        }}
    >
        <Box sx={{ mb: 2 }}>
            <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '1rem' }}>
                {title}
            </Typography>
        </Box>
        {chartData.length === 0 ? (
            <Box
                sx={{
                    flex: 1,
                    minHeight: 280,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: 'text.secondary',
                }}
            >
                <Box
                    sx={{
                        width: 48,
                        height: 48,
                        borderRadius: 2,
                        backgroundColor: alpha('#64748b', 0.1),
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        mb: 2,
                    }}
                >
                    <Box
                        sx={{
                            width: 24,
                            height: 24,
                            borderRadius: '50%',
                            backgroundColor: 'text.disabled',
                            opacity: 0.3,
                        }}
                    />
                </Box>
                <Typography variant="body1" color="text.secondary">
                    No data available
                </Typography>
                <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                    Select a different time range or check back later
                </Typography>
            </Box>
        ) : (
            <Box sx={{ flex: 1, minHeight: 280 }}>
                <ResponsiveContainer width="100%" height={280}>
                    {children}
                </ResponsiveContainer>
            </Box>
        )}
    </Paper>
);

// Daily Token History Chart (Bar Chart) - for multi-day view
interface DailyTokenHistoryChartProps {
    data: TimeSeriesData[];
}

export function DailyTokenHistoryChart({ data }: DailyTokenHistoryChartProps) {
    const chartData = formatChartData(data, true);
    const labelInterval = calculateLabelInterval(chartData.length);

    const [visibleSeries, setVisibleSeries] = useState<SeriesVisibility>({ cache: true, input: true, output: true });

    const toggleSeries = (key: SeriesKey) => {
        setVisibleSeries(prev => ({ ...prev, [key]: !prev[key] }));
    };

    return (
        <Paper
            elevation={0}
            sx={{
                p: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                flexGrow: 1,
                backgroundColor: 'background.paper',
                boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <Box sx={{ mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Token Usage Over Time (Daily)
                </Typography>
            </Box>
            {chartData.length === 0 ? (
                <Box
                    sx={{
                        flex: 1,
                        minHeight: 280,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    <Box
                        sx={{
                            width: 48,
                            height: 48,
                            borderRadius: 2,
                            backgroundColor: alpha('#64748b', 0.1),
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            mb: 2,
                        }}
                    >
                        <Box
                            sx={{
                                width: 24,
                                height: 24,
                                borderRadius: '50%',
                                backgroundColor: 'text.disabled',
                                opacity: 0.3,
                            }}
                        />
                    </Box>
                    <Typography variant="body1" color="text.secondary">
                        No data available
                    </Typography>
                    <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                        Select a different time range or check back later
                    </Typography>
                </Box>
            ) : (
                <>
                    <Box sx={{ flex: 1, minHeight: 280 }}>
                        <ResponsiveContainer width="100%" height={280}>
                            <BarChart data={chartData} barCategoryGap={8}>
                                <CartesianGrid strokeDasharray="4 4" stroke={gridStyle.stroke} strokeOpacity={gridStyle.strokeOpacity} />
                                <XAxis
                                    dataKey="time"
                                    tick={{ fontSize: 11, fill: '#64748b' }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                                    interval={labelInterval}
                                />
                                <YAxis
                                    tickFormatter={formatYAxis}
                                    tick={{ fontSize: 11, fill: '#64748b' }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                                />
                                <Tooltip content={<CustomTooltip />} />
                                <Bar dataKey="cacheTokens" name="Cache Tokens" fill={TOKEN_COLORS.cache.main} stackId="tokens" hide={!visibleSeries.cache}>
                                    {chartData.map((entry, index) => (
                                        <Cell key={`cache-${index}`} fill={entry.cacheTokens > 0 ? TOKEN_COLORS.cache.gradient : 'transparent'} />
                                    ))}
                                </Bar>
                                <Bar dataKey="inputTokens" name="Input Tokens" fill={TOKEN_COLORS.input.gradient} stackId="tokens" hide={!visibleSeries.input} />
                                <Bar dataKey="outputTokens" name="Output Tokens" fill={TOKEN_COLORS.output.gradient} stackId="tokens" hide={!visibleSeries.output} />

                            </BarChart>
                        </ResponsiveContainer>
                    </Box>
                    {/* Legend replacement - inline indicator */}
                    <Box sx={{ mt: 2, display: 'flex', gap: 3, flexWrap: 'wrap' }}>
                        <LegendItem
                            label="Cache"
                            color={TOKEN_COLORS.cache.main}
                            visible={visibleSeries.cache}
                            onToggle={() => toggleSeries('cache')}
                        />
                        <LegendItem
                            label="Input"
                            color={TOKEN_COLORS.input.main}
                            visible={visibleSeries.input}
                            onToggle={() => toggleSeries('input')}
                        />
                        <LegendItem
                            label="Output"
                            color={TOKEN_COLORS.output.main}
                            visible={visibleSeries.output}
                            onToggle={() => toggleSeries('output')}
                        />
                    </Box>
                </>
            )}
        </Paper>
    );
}

// Hourly Token History Chart (Area Chart) - for today view
interface HourlyTokenHistoryChartProps {
    data: TimeSeriesData[];
}

export function HourlyTokenHistoryChart({ data }: HourlyTokenHistoryChartProps) {
    const chartData = formatChartData(data, false);
    const labelInterval = calculateLabelInterval(chartData.length);

    const [visibleSeries, setVisibleSeries] = useState<SeriesVisibility>({ cache: true, input: true, output: true });

    const toggleSeries = (key: SeriesKey) => {
        setVisibleSeries(prev => ({ ...prev, [key]: !prev[key] }));
    };

    return (
        <Paper
            elevation={0}
            sx={{
                p: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                flexGrow: 1,
                backgroundColor: 'background.paper',
                boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <Box sx={{ mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Token Usage Over Time (Hourly)
                </Typography>
            </Box>
            {chartData.length === 0 ? (
                <Box
                    sx={{
                        flex: 1,
                        minHeight: 280,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    <Box
                        sx={{
                            width: 48,
                            height: 48,
                            borderRadius: 2,
                            backgroundColor: alpha('#64748b', 0.1),
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            mb: 2,
                        }}
                    >
                        <Box
                            sx={{
                                width: 24,
                                height: 24,
                                borderRadius: '50%',
                                backgroundColor: 'text.disabled',
                                opacity: 0.3,
                            }}
                        />
                    </Box>
                    <Typography variant="body1" color="text.secondary">
                        No data available
                    </Typography>
                    <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                        Select a different time range or check back later
                    </Typography>
                </Box>
            ) : (
                <>
                    <Box sx={{ flex: 1, minHeight: 280 }}>
                        <ResponsiveContainer width="100%" height={280}>
                            <ComposedChart data={chartData}>
                                <CartesianGrid strokeDasharray="4 4" stroke={gridStyle.stroke} strokeOpacity={gridStyle.strokeOpacity} vertical={false} />
                                <XAxis
                                    dataKey="time"
                                    tick={{ fontSize: 11, fill: '#64748b' }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                                    interval={labelInterval}
                                />
                                <YAxis
                                    tickFormatter={formatYAxis}
                                    tick={{ fontSize: 11, fill: '#64748b' }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                                />
                                <Tooltip content={<CustomTooltip />} />
                                <Area
                                    type="monotone"
                                    dataKey="cacheTokens"
                                    name="Cache Tokens"
                                    stackId="1"
                                    stroke={TOKEN_COLORS.cache.main}
                                    fill={TOKEN_COLORS.cache.gradient}
                                    hide={!visibleSeries.cache}
                                />
                                <Area
                                    type="monotone"
                                    dataKey="inputTokens"
                                    name="Input Tokens"
                                    stackId="1"
                                    stroke={TOKEN_COLORS.input.main}
                                    fill={TOKEN_COLORS.input.gradient}
                                    hide={!visibleSeries.input}
                                />
                                <Area
                                    type="monotone"
                                    dataKey="outputTokens"
                                    name="Output Tokens"
                                    stackId="1"
                                    stroke={TOKEN_COLORS.output.main}
                                    fill={TOKEN_COLORS.output.gradient}
                                    hide={!visibleSeries.output}
                                />
                            </ComposedChart>
                        </ResponsiveContainer>
                    </Box>
                    {/* Legend replacement - inline indicator */}
                    <Box sx={{ mt: 2, display: 'flex', gap: 3, flexWrap: 'wrap' }}>
                        <LegendItem
                            label="Cache"
                            color={TOKEN_COLORS.cache.main}
                            visible={visibleSeries.cache}
                            onToggle={() => toggleSeries('cache')}
                        />
                        <LegendItem
                            label="Input"
                            color={TOKEN_COLORS.input.main}
                            visible={visibleSeries.input}
                            onToggle={() => toggleSeries('input')}
                        />
                        <LegendItem
                            label="Output"
                            color={TOKEN_COLORS.output.main}
                            visible={visibleSeries.output}
                            onToggle={() => toggleSeries('output')}
                        />
                    </Box>
                </>
            )}
        </Paper>
    );
}

// Original TokenHistoryChart - kept for backward compatibility
export default function TokenHistoryChart({ data, interval = 'hour' }: TokenHistoryChartProps) {
    const isDayMode = interval === 'day' || interval === 'week';

    // Format timestamp based on aggregation interval
    const formatTimeLabel = (timestamp: string) => {
        if (!timestamp) return '';

        // Handle Unix timestamp (seconds since epoch)
        let date: Date;
        const timestampNum = parseInt(timestamp, 10);

        // Check if it's a Unix timestamp (number in seconds)
        if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
            date = new Date(timestampNum * 1000);
        } else {
            // Try ISO string format
            date = new Date(timestamp);
        }

        if (isNaN(date.getTime())) {
            console.warn('Invalid timestamp:', timestamp);
            return '';
        }

        const pad = (n: number) => String(n).padStart(2, '0');

        if (isDayMode) {
            // For day mode, show simple date format
            return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
        }

        switch (interval) {
            case 'minute':
                return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
            case 'hour':
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:00`;
            default:
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
        }
    };

    // Format tooltip time with more context
    const formatTooltipTime = (timestamp: string) => {
        if (!timestamp) return timestamp;

        // Handle Unix timestamp (seconds since epoch)
        let date: Date;
        const timestampNum = parseInt(timestamp, 10);

        // Check if it's a Unix timestamp (number in seconds)
        if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
            date = new Date(timestampNum * 1000);
        } else {
            // Try ISO string format
            date = new Date(timestamp);
        }

        if (isNaN(date.getTime())) {
            return timestamp;
        }

        const options: Intl.DateTimeFormatOptions = {
            month: 'short',
            day: 'numeric',
        };

        if (interval === 'hour' || interval === 'minute') {
            options.hour = '2-digit';
            options.minute = interval === 'minute' ? '2-digit' : undefined;
        }

        if (interval === 'week') {
            return `Week of ${date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`;
        }

        return date.toLocaleDateString('en-US', options);
    };

    // Format data for chart
    const chartData = data.map((item) => ({
        timestamp: item.timestamp,
        time: formatTimeLabel(item.timestamp),
        timeFull: formatTooltipTime(item.timestamp),
        inputTokens: item.input_tokens,
        outputTokens: item.output_tokens,
        cacheTokens: item.cache_input_tokens || 0,
    }));

    // Calculate smart interval for X-axis labels
    const calculateLabelInterval = () => {
        const dataPoints = chartData.length;
        if (dataPoints <= 7) return 0;
        if (dataPoints <= 14) return 1;
        if (dataPoints <= 30) return 4;
        return Math.ceil(dataPoints / 6);
    };

    const labelInterval = calculateLabelInterval();

    const formatYAxis = (value: number) => {
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
        return value.toString();
    };

    const formatTooltipValue = (value: number) => {
        if (value >= 1000000) return `${(value / 1000000).toFixed(2)}M`;
        if (value >= 1000) return `${(value / 1000).toFixed(2)}K`;
        return value.toLocaleString();
    };

    // Custom tooltip
    const CustomTooltip = ({ active, payload }: any) => {
        if (active && payload && payload.length) {
            const data = payload[0].payload;
            return (
                <Box
                    sx={{
                        backgroundColor: 'white',
                        padding: 2,
                        borderRadius: 2,
                        border: '1px solid #e0e0e0',
                        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                    }}
                >
                    <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                        {data.timeFull}
                    </Typography>
                    {payload.map((entry: any, index: number) => (
                        <Typography key={index} variant="body2" sx={{ color: entry.color }}>
                            {entry.name}: {formatTooltipValue(entry.value)}
                        </Typography>
                    ))}
                </Box>
            );
        }
        return null;
    };

    return (
        <Paper
            elevation={0}
            sx={{
                p: 3,
                borderRadius: 2.5,
                border: '1px solid',
                borderColor: 'divider',
                flexGrow: 1,
                backgroundColor: 'background.paper',
                boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <Box sx={{ mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '1rem' }}>
                    Token Usage Over Time
                </Typography>
            </Box>
            {chartData.length === 0 ? (
                <Box
                    sx={{
                        flex: 1,
                        minHeight: 280,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    <Box
                        sx={{
                            width: 48,
                            height: 48,
                            borderRadius: 2,
                            backgroundColor: alpha('#64748b', 0.1),
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            mb: 2,
                        }}
                    >
                        <Box
                            sx={{
                                width: 24,
                                height: 24,
                                borderRadius: '50%',
                                backgroundColor: 'text.disabled',
                                opacity: 0.3,
                            }}
                        />
                    </Box>
                    <Typography variant="body1" color="text.secondary">
                        No data available
                    </Typography>
                    <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                        Select a different time range or check back later
                    </Typography>
                </Box>
            ) : (
                <Box sx={{ flex: 1, minHeight: 280 }}>
                    <ResponsiveContainer width="100%" height={280}>
                        {isDayMode ? (
                            // Bar chart for day mode
                            <BarChart data={chartData}>
                                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                                <XAxis
                                    dataKey="time"
                                    tick={{ fontSize: 11 }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e0e0e0' }}
                                    interval={labelInterval}
                                />
                                <YAxis
                                    tickFormatter={formatYAxis}
                                    tick={{ fontSize: 11 }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e0e0e0' }}
                                />
                                <Tooltip content={<CustomTooltip />} />
                                <Legend />
                                <Bar dataKey="cacheTokens" name="Cache Tokens" fill="#ed6c02" />
                                <Bar dataKey="inputTokens" name="Input Tokens" fill="#1976d2" stackId="stack" />
                                <Bar dataKey="outputTokens" name="Output Tokens" fill="#2e7d32" stackId="stack" />
                            </BarChart>
                        ) : (
                            // Area chart for hour/minute mode
                            <ComposedChart data={chartData}>
                                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                                <XAxis
                                    dataKey="time"
                                    tick={{ fontSize: 11 }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e0e0e0' }}
                                    interval={labelInterval}
                                />
                                <YAxis
                                    tickFormatter={formatYAxis}
                                    tick={{ fontSize: 11 }}
                                    tickLine={false}
                                    axisLine={{ stroke: '#e0e0e0' }}
                                />
                                <Tooltip content={<CustomTooltip />} />
                                <Legend />
                                <Area
                                    type="monotone"
                                    dataKey="inputTokens"
                                    name="Input Tokens"
                                    stackId="1"
                                    stroke="#1976d2"
                                    fill="#bbdefb"
                                />
                                <Area
                                    type="monotone"
                                    dataKey="outputTokens"
                                    name="Output Tokens"
                                    stackId="1"
                                    stroke="#2e7d32"
                                    fill="#c8e6c9"
                                />
                                <Area
                                    type="monotone"
                                    dataKey="cacheTokens"
                                    name="Cache Tokens"
                                    stroke="#ed6c02"
                                    fill="#ffe0b2"
                                />
                            </ComposedChart>
                        )}
                    </ResponsiveContainer>
                </Box>
            )}
        </Paper>
    );
}
