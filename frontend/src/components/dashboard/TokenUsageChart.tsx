import { Paper, Typography, Box, alpha, useTheme } from '@mui/material';
import { useState } from 'react';
import {
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    Cell,
} from 'recharts';
import { TOKEN_COLORS, barRadius, getThemeChartStyles } from './chartStyles';

interface UsageData {
    name: string;
    inputTokens: number;
    outputTokens: number;
    cacheTokens?: number;
}

interface TokenUsageChartProps {
    data: UsageData[];
}

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

export default function TokenUsageChart({ data }: TokenUsageChartProps) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);

    // Sort by total tokens (input + output) and take top 5
    const top5Data = [...data]
        .sort((a, b) => (b.inputTokens + b.outputTokens) - (a.inputTokens + a.outputTokens))
        .slice(0, 5);

    const [visibleSeries, setVisibleSeries] = useState<SeriesVisibility>({ cache: true, input: true, output: true });

    const toggleSeries = (key: SeriesKey) => {
        setVisibleSeries(prev => ({ ...prev, [key]: !prev[key] }));
    };

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

    // Custom tooltip with better styling
    const CustomTooltip = ({ active, payload }: any) => {
        if (!active || !payload || !payload.length) return null;

        const data = payload[0].payload;
        return (
            <Box
                sx={{
                    borderRadius: 2,
                    border: '1px solid',
                    borderColor: chartStyles.chart.tooltipBorder,
                    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
                    backgroundColor: chartStyles.chart.tooltipBg,
                    padding: '12px',
                    minWidth: 200,
                }}
            >
                <Typography variant="body2" sx={{ fontWeight: 600, mb: 1, fontSize: '0.875rem' }}>
                    {data.name}
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
                <Box sx={{ mt: 1, pt: 1, borderTop: '1px solid', borderColor: 'divider', fontSize: '0.75rem' }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        Total: {formatTooltipValue(data.inputTokens + data.outputTokens + (data.cacheTokens || 0))}
                    </Typography>
                </Box>
            </Box>
        );
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
            <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Token Usage by Top 5 Models
                </Typography>
            </Box>
            {top5Data.length === 0 ? (
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
                            backgroundColor: chartStyles.statCard.emptyIconBg,
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
                    <ResponsiveContainer width="100%" height={280}>
                        <BarChart data={top5Data} layout="vertical" barCategoryGap={12}>
                            <CartesianGrid strokeDasharray="4 4" stroke={chartStyles.chart.grid} strokeOpacity={0.5} />
                            <XAxis
                                type="number"
                                tickFormatter={formatYAxis}
                                tick={{ fontSize: 11, fill: 'text.secondary' }}
                                tickLine={false}
                                axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1 }}
                            />
                            <YAxis
                                dataKey="name"
                                type="category"
                                tick={{ fontSize: 11, fill: 'text.secondary' }}
                                tickLine={false}
                                axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1 }}
                                width={160}
                            />
                            <Tooltip content={<CustomTooltip />} />
                            <Bar dataKey="cacheTokens" name="Cache Tokens" fill={chartStyles.token.cache.main} stackId="tokens" radius={barRadius} hide={!visibleSeries.cache}>
                                {top5Data.map((entry, index) => (
                                    <Cell
                                        key={`cache-${index}`}
                                        fill={entry.cacheTokens > 0 ? chartStyles.token.cache.gradient : 'transparent'}
                                    />
                                ))}
                            </Bar>
                            <Bar dataKey="inputTokens" name="Input Tokens" fill={chartStyles.token.input.gradient} stackId="tokens" radius={barRadius} hide={!visibleSeries.input} />
                            <Bar dataKey="outputTokens" name="Output Tokens" fill={chartStyles.token.output.gradient} stackId="tokens" radius={barRadius} hide={!visibleSeries.output} />
                        </BarChart>
                    </ResponsiveContainer>
                {/* Legend replacement - inline indicator */}
                <Box sx={{ mt: 2, display: 'flex', gap: 3, flexWrap: 'wrap' }}>
                    <LegendItem
                        label="Cache"
                        color={chartStyles.token.cache.main}
                        visible={visibleSeries.cache}
                        onToggle={() => toggleSeries('cache')}
                    />
                    <LegendItem
                        label="Input"
                        color={chartStyles.token.input.main}
                        visible={visibleSeries.input}
                        onToggle={() => toggleSeries('input')}
                    />
                    <LegendItem
                        label="Output"
                        color={chartStyles.token.output.main}
                        visible={visibleSeries.output}
                        onToggle={() => toggleSeries('output')}
                    />
                </Box>
                </>
            )}
        </Paper>
    );
}
