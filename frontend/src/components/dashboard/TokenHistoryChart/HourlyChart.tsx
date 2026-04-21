// Hourly Token History Chart (Area Chart) - for single day view

import {useState} from 'react';
import {Box} from '@mui/material';
import {Area, CartesianGrid, ComposedChart, ResponsiveContainer, Tooltip, XAxis, YAxis,} from 'recharts';
import {useTheme} from '@mui/material/styles';
import {getThemeChartStyles} from '../chartStyles';
import {ChartWrapper, CustomTooltip, LegendItem} from './components';
import {calculateLabelInterval, formatChartData, formatYAxis,} from './utils';
import type {SeriesKey, SeriesVisibility, TimeSeriesData} from './types';

interface HourlyTokenHistoryChartProps {
    data: TimeSeriesData[];
}

export function HourlyTokenHistoryChart({data}: HourlyTokenHistoryChartProps) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);

    const chartData = formatChartData(data, false);
    const labelInterval = calculateLabelInterval(chartData.length);

    const [visibleSeries, setVisibleSeries] = useState<SeriesVisibility>({
        cache: true,
        input: true,
        output: true,
    });

    const toggleSeries = (key: SeriesKey) => {
        setVisibleSeries(prev => ({...prev, [key]: !prev[key]}));
    };

    const hasData = chartData.length > 0;

    return (
        <ChartWrapper title="Token Usage Over Time (Hourly)" hasData={hasData}>
            {hasData ? (
                <>
                    <Box sx={{display: 'flex', justifyContent: 'center', gap: 3, mb: 2}}>
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
                        <ComposedChart data={chartData}>
                            <defs>
                                <linearGradient id="colorInputArea" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.input.main} stopOpacity={0.3}/>
                                    <stop offset="95%" stopColor={chartStyles.token.input.main} stopOpacity={0.05}/>
                                </linearGradient>
                                <linearGradient id="colorOutputArea" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.output.main} stopOpacity={0.3}/>
                                    <stop offset="95%" stopColor={chartStyles.token.output.main} stopOpacity={0.05}/>
                                </linearGradient>
                                <linearGradient id="colorCacheArea" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor={chartStyles.token.cache.main} stopOpacity={0.3}/>
                                    <stop offset="95%" stopColor={chartStyles.token.cache.main} stopOpacity={0.05}/>
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
                                tick={{fontSize: 12, fill: 'text.secondary', fontWeight: 500}}
                                tickLine={false}
                                axisLine={{stroke: chartStyles.chart.axis, strokeWidth: 1.5}}
                                interval={labelInterval}
                                height={50}
                            />
                            <YAxis
                                tickFormatter={formatYAxis}
                                tick={{fontSize: 12, fill: 'text.secondary', fontWeight: 500}}
                                tickLine={false}
                                axisLine={{stroke: chartStyles.chart.axis, strokeWidth: 1.5}}
                                width={60}
                            />
                            <Tooltip
                                content={<CustomTooltip/>}
                                cursor={{fill: 'rgba(0, 0, 0, 0.05)'}}
                            />
                            <Area
                                type="monotone"
                                dataKey="cacheTokens"
                                name="Cache Tokens"
                                stackId="1"
                                stroke={chartStyles.token.cache.main}
                                strokeWidth={1}
                                fill="url(#colorCacheArea)"
                                hide={!visibleSeries.cache}
                                isAnimationActive={true}
                                animationBegin={200}
                                animationDuration={800}
                                animationEasing="ease-out"
                            />
                            <Area
                                type="monotone"
                                dataKey="inputTokens"
                                name="Input Tokens"
                                stackId="1"
                                stroke={chartStyles.token.input.main}
                                strokeWidth={2}
                                fill="url(#colorInputArea)"
                                hide={!visibleSeries.input}
                                isAnimationActive={true}
                                animationBegin={0}
                                animationDuration={800}
                                animationEasing="ease-out"
                            />
                            <Area
                                type="monotone"
                                dataKey="outputTokens"
                                name="Output Tokens"
                                stackId="1"
                                stroke={chartStyles.token.output.main}
                                strokeWidth={2}
                                fill="url(#colorOutputArea)"
                                hide={!visibleSeries.output}
                                isAnimationActive={true}
                                animationBegin={100}
                                animationDuration={800}
                                animationEasing="ease-out"
                            />
                        </ComposedChart>
                    </ResponsiveContainer>
                </>
            ) : null}
        </ChartWrapper>
    );
}
