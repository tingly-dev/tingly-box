import { Paper, Typography, Box, Tabs, Tab } from '@mui/material';
import {
    ComposedChart,
    Line,
    Area,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    Legend,
} from 'recharts';
import { useState } from 'react';

interface TimeSeriesData {
    timestamp: string;
    request_count: number;
    total_tokens: number;
    input_tokens: number;
    output_tokens: number;
    error_count?: number;
    avg_latency_ms?: number;
}

interface RequestErrorChartProps {
    data: TimeSeriesData[];
    interval?: string;
}

type TabValue = 'requests' | 'errors' | 'both';

export default function RequestErrorChart({ data, interval = 'hour' }: RequestErrorChartProps) {
    const [tabValue, setTabValue] = useState<TabValue>('both');

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

        switch (interval) {
            case 'minute':
                return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
            case 'hour':
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:00`;
            case 'day':
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
            case 'week':
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
            default:
                return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:00`;
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
    const chartData = data.map((item) => {
        const date = new Date(item.timestamp);
        return {
            timestamp: item.timestamp,
            time: formatTimeLabel(item.timestamp),
            timeFull: formatTooltipTime(item.timestamp),
            requests: item.request_count,
            errors: item.error_count || 0,
            successRate: item.request_count > 0
                ? ((item.request_count - (item.error_count || 0)) / item.request_count * 100)
                : 100,
        };
    });

    // Check if there are any errors in the data
    const hasErrors = chartData.some((d) => d.errors > 0);

    // Calculate smart interval for X-axis labels based on data points
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

    const formatPercent = (value: number) => {
        return `${value.toFixed(1)}%`;
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
                            {entry.name}: {entry.name === 'Success Rate' ? formatPercent(entry.value) : entry.value.toLocaleString()}
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
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
            }}
        >
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                <Typography variant="h6" sx={{ fontWeight: 600 }}>
                    Request Metrics
                </Typography>
                <Tabs
                    value={tabValue}
                    onChange={(_, value) => setTabValue(value)}
                    size="small"
                    sx={{ minHeight: 36, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    <Tab label="Requests" value="requests" sx={{ minHeight: 36, py: 0.5, fontSize: '0.875rem' }} />
                    {hasErrors && (
                        <Tab label="Errors" value="errors" sx={{ minHeight: 36, py: 0.5, fontSize: '0.875rem' }} />
                    )}
                    {hasErrors && (
                        <Tab label="Both" value="both" sx={{ minHeight: 36, py: 0.5, fontSize: '0.875rem' }} />
                    )}
                </Tabs>
            </Box>

            {chartData.length === 0 ? (
                <Box
                    sx={{
                        height: 300,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    No data available
                </Box>
            ) : (
                <ResponsiveContainer width="100%" height={300}>
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
                            yAxisId="left"
                            tickFormatter={formatYAxis}
                            tick={{ fontSize: 11 }}
                            tickLine={false}
                            axisLine={{ stroke: '#e0e0e0' }}
                        />
                        {tabValue === 'errors' && (
                            <YAxis
                                yAxisId="right"
                                orientation="right"
                                tickFormatter={formatPercent}
                                tick={{ fontSize: 11 }}
                                tickLine={false}
                                axisLine={{ stroke: '#e0e0e0' }}
                            />
                        )}
                        <Tooltip content={<CustomTooltip />} />
                        <Legend />

                        {/* Requests tab */}
                        {tabValue === 'requests' && (
                            <>
                                <Area
                                    yAxisId="left"
                                    type="monotone"
                                    dataKey="requests"
                                    name="Requests"
                                    stroke="#ff9800"
                                    fill="#fff3e0"
                                    strokeWidth={2}
                                />
                            </>
                        )}

                        {/* Errors tab */}
                        {tabValue === 'errors' && (
                            <>
                                <Line
                                    yAxisId="left"
                                    type="monotone"
                                    dataKey="errors"
                                    name="Errors"
                                    stroke="#d32f2f"
                                    strokeWidth={2}
                                    dot={{ r: 3 }}
                                />
                                <Line
                                    yAxisId="right"
                                    type="monotone"
                                    dataKey="successRate"
                                    name="Success Rate"
                                    stroke="#2e7d32"
                                    strokeWidth={2}
                                    strokeDasharray="5 5"
                                    dot={false}
                                />
                            </>
                        )}

                        {/* Both tab */}
                        {tabValue === 'both' && hasErrors && (
                            <>
                                <Area
                                    yAxisId="left"
                                    type="monotone"
                                    dataKey="requests"
                                    name="Requests"
                                    stroke="#ff9800"
                                    fill="#fff3e0"
                                    strokeWidth={2}
                                />
                                <Line
                                    yAxisId="left"
                                    type="monotone"
                                    dataKey="errors"
                                    name="Errors"
                                    stroke="#d32f2f"
                                    strokeWidth={2}
                                    dot={{ r: 3 }}
                                />
                            </>
                        )}
                    </ComposedChart>
                </ResponsiveContainer>
            )}
        </Paper>
    );
}
