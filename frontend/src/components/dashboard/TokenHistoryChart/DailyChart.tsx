// Daily Token History Chart (Bar Chart) - for multi-day view

import { useState } from 'react';
import { Box } from '@mui/material';
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
import { useTheme } from '@mui/material/styles';
import { getThemeChartStyles } from '../chartStyles';
import { ChartWrapper, LegendItem, CustomTooltip } from './components';
import {
    formatChartData,
    calculateLabelInterval,
    formatYAxis,
} from './utils';
import type { TimeSeriesData, SeriesVisibility, SeriesKey } from './types';

interface DailyTokenHistoryChartProps {
    data: TimeSeriesData[];
}

export function DailyTokenHistoryChart({ data }: DailyTokenHistoryChartProps) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);

    const chartData = formatChartData(data, true);
    const labelInterval = calculateLabelInterval(chartData.length);

    const [visibleSeries, setVisibleSeries] = useState<SeriesVisibility>({
        cache: true,
        input: true,
        output: true,
    });

    // Calculate adaptive bar configuration based on data length
    const getBarConfig = (dataLength: number) => {
        if (dataLength <= 7) {
            return { barGap: 10, barCategoryGap: '15%' };
        }
        if (dataLength <= 14) {
            return { barGap: 8, barCategoryGap: '12%' };
        }
        if (dataLength <= 30) {
            return { barGap: 5, barCategoryGap: '8%' };
        }
        return { barGap: 3, barCategoryGap: '5%' };
    };

    const barConfig = getBarConfig(chartData.length);

    const toggleSeries = (key: SeriesKey) => {
        setVisibleSeries(prev => ({ ...prev, [key]: !prev[key] }));
    };

    const hasData = chartData.length > 0;

    return (
        <ChartWrapper title="Token Usage Over Time (Daily)" hasData={hasData}>
            {hasData ? (
                <>
                    <Box sx={{ display: 'flex', justifyContent: 'center', gap: 3, mb: 2 }}>
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
                    <ResponsiveContainer width="100%" height={280}>
                        <BarChart
                            key={chartData.length ? `${chartData.length}-${chartData[0].timestamp}` : 'empty'}
                            data={chartData}
                            barGap={barConfig.barGap}
                            barCategoryGap={barConfig.barCategoryGap}
                        >
                            <defs>
                                <linearGradient id="colorInput" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.input.main} stopOpacity={0.95} />
                                    <stop offset="95%" stopColor={chartStyles.token.input.main} stopOpacity={0.75} />
                                </linearGradient>
                                <linearGradient id="colorOutput" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.output.main} stopOpacity={0.95} />
                                    <stop offset="95%" stopColor={chartStyles.token.output.main} stopOpacity={0.75} />
                                </linearGradient>
                                <linearGradient id="colorCache" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.cache.main} stopOpacity={0.95} />
                                    <stop offset="95%" stopColor={chartStyles.token.cache.main} stopOpacity={0.75} />
                                </linearGradient>
                            </defs>
                            <CartesianGrid
                                strokeDasharray="3 3"
                                stroke={chartStyles.chart.grid}
                                strokeOpacity={0.6}
                                vertical={false}
                            />
                            <XAxis
                                dataKey="time"
                                tick={{ fontSize: 12, fill: 'text.secondary', fontWeight: 500 }}
                                tickLine={false}
                                axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1.5 }}
                                interval={labelInterval}
                                height={50}
                            />
                            <YAxis
                                tickFormatter={formatYAxis}
                                tick={{ fontSize: 12, fill: 'text.secondary', fontWeight: 500 }}
                                tickLine={false}
                                axisLine={{ stroke: chartStyles.chart.axis, strokeWidth: 1.5 }}
                                width={60}
                            />
                            <Tooltip
                                content={<CustomTooltip />}
                                cursor={{ fill: 'rgba(0, 0, 0, 0.05)' }}
                            />
                            <Bar
                                dataKey="cacheTokens"
                                name="Cache Tokens"
                                stackId="tokens"
                                hide={!visibleSeries.cache}
                                stroke={chartStyles.token.cache.main}
                                strokeWidth={0.5}
                                strokeOpacity={0.8}
                                isAnimationActive={true}
                                animationBegin={0}
                                animationDuration={800}
                                animationEasing="ease-out"
                            >
                                {chartData.map((entry, index) => (
                                    <Cell
                                        key={`cache-${index}`}
                                        fill={entry.cacheTokens > 0 ? 'url(#colorCache)' : 'transparent'}
                                        stroke={entry.cacheTokens > 0 ? chartStyles.token.cache.main : 'transparent'}
                                        strokeWidth={0.5}
                                    />
                                ))}
                            </Bar>
                            <Bar
                                dataKey="inputTokens"
                                name="Input Tokens"
                                fill="url(#colorInput)"
                                stroke={chartStyles.token.input.main}
                                strokeWidth={0.5}
                                strokeOpacity={0.8}
                                stackId="tokens"
                                isAnimationActive={true}
                                animationBegin={100}
                                animationDuration={800}
                                animationEasing="ease-out"
                            />
                            <Bar
                                dataKey="outputTokens"
                                name="Output Tokens"
                                fill="url(#colorOutput)"
                                stroke={chartStyles.token.output.main}
                                strokeWidth={0.5}
                                strokeOpacity={0.8}
                                stackId="tokens"
                                isAnimationActive={true}
                                animationBegin={200}
                                animationDuration={800}
                                animationEasing="ease-out"
                            />
                        </BarChart>
                    </ResponsiveContainer>
                </>
            ) : null}
        </ChartWrapper>
    );
}
