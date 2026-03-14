import { Paper, Typography, Box, alpha } from '@mui/material';
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
import { TOKEN_COLORS, gridStyle, tooltipStyle, barRadius } from './chartStyles';

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
            <Box sx={tooltipStyle}>
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
                <Box sx={{ mt: 1, pt: 1, borderTop: '1px solid #e2e8f0', fontSize: '0.75rem' }}>
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
            <Box sx={{ mb: 2, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '1rem' }}>
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
                        <BarChart data={top5Data} layout="vertical" barCategoryGap={12}>
                            <CartesianGrid strokeDasharray="4 4" stroke={gridStyle.stroke} strokeOpacity={gridStyle.strokeOpacity} />
                            <XAxis
                                type="number"
                                tickFormatter={formatYAxis}
                                tick={{ fontSize: 11, fill: '#64748b' }}
                                tickLine={false}
                                axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                            />
                            <YAxis
                                dataKey="name"
                                type="category"
                                tick={{ fontSize: 11, fill: '#64748b' }}
                                tickLine={false}
                                axisLine={{ stroke: '#e2e8f0', strokeWidth: 1 }}
                                width={160}
                            />
                            <Tooltip content={<CustomTooltip />} />
                            <Bar dataKey="cacheTokens" name="Cache Tokens" fill={TOKEN_COLORS.cache.main} stackId="tokens" radius={barRadius} hide={!visibleSeries.cache}>
                                {top5Data.map((entry, index) => (
                                    <Cell
                                        key={`cache-${index}`}
                                        fill={entry.cacheTokens > 0 ? TOKEN_COLORS.cache.gradient : 'transparent'}
                                    />
                                ))}
                            </Bar>
                            <Bar dataKey="inputTokens" name="Input Tokens" fill={TOKEN_COLORS.input.gradient} stackId="tokens" radius={barRadius} hide={!visibleSeries.input} />
                            <Bar dataKey="outputTokens" name="Output Tokens" fill={TOKEN_COLORS.output.gradient} stackId="tokens" radius={barRadius} hide={!visibleSeries.output} />
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
