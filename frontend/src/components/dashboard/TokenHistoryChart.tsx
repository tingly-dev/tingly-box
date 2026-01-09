import { Box, Paper, Typography } from '@mui/material';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

interface HistoryDataPoint {
    hour: string;
    input_tokens: number;
    output_tokens: number;
    request_count: number;
}

interface TokenHistoryChartProps {
    data: HistoryDataPoint[];
}

function formatHour(hourStr: string): string {
    const date = new Date(hourStr);
    const now = new Date();
    const isToday = date.toDateString() === now.toDateString();

    if (isToday) {
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }
    return date.toLocaleDateString([], { month: 'short', day: 'numeric', hour: '2-digit' });
}

function formatTokens(value: number): string {
    if (value >= 1000000) {
        return (value / 1000000).toFixed(1) + 'M';
    }
    if (value >= 1000) {
        return (value / 1000).toFixed(1) + 'K';
    }
    return value.toString();
}

// Pad data with zero points before and after to make the chart look better
function padChartData(data: HistoryDataPoint[]): HistoryDataPoint[] {
    if (data.length === 0) return [];
    if (data.length >= 3) return data;

    const result: HistoryDataPoint[] = [];
    const firstPoint = data[0];
    const lastPoint = data[data.length - 1];

    // Add a zero point 1 hour before the first data point
    const beforeDate = new Date(firstPoint.hour);
    beforeDate.setHours(beforeDate.getHours() - 1);
    result.push({
        hour: beforeDate.toISOString(),
        input_tokens: 0,
        output_tokens: 0,
        request_count: 0,
    });

    // Add original data
    result.push(...data);

    // Add a zero point 1 hour after the last data point
    const afterDate = new Date(lastPoint.hour);
    afterDate.setHours(afterDate.getHours() + 1);
    result.push({
        hour: afterDate.toISOString(),
        input_tokens: 0,
        output_tokens: 0,
        request_count: 0,
    });

    return result;
}

export default function TokenHistoryChart({ data }: TokenHistoryChartProps) {
    const hasData = data.length > 0;

    const paddedData = padChartData(data);
    const chartData = paddedData.map(d => ({
        ...d,
        time: formatHour(d.hour),
        total: d.input_tokens + d.output_tokens,
    }));

    return (
        <Paper
            elevation={0}
            sx={{
                p: 3,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                height: '100%',
            }}
        >
            <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
                Token Usage Trend
            </Typography>
            {hasData ? (
                <Box sx={{ width: '100%', height: 280 }}>
                    <ResponsiveContainer>
                        <AreaChart data={chartData} margin={{ top: 10, right: 30, left: 0, bottom: 0 }}>
                            <defs>
                                <linearGradient id="inputGradient" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                                    <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                                </linearGradient>
                                <linearGradient id="outputGradient" x1="0" y1="0" x2="0" y2="1">
                                    <stop offset="5%" stopColor="#10b981" stopOpacity={0.3}/>
                                    <stop offset="95%" stopColor="#10b981" stopOpacity={0}/>
                                </linearGradient>
                            </defs>
                            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" vertical={false} />
                            <XAxis
                                dataKey="time"
                                tick={{ fontSize: 11 }}
                                interval="preserveStartEnd"
                                axisLine={false}
                                tickLine={false}
                            />
                            <YAxis
                                tick={{ fontSize: 12 }}
                                tickFormatter={formatTokens}
                                axisLine={false}
                                tickLine={false}
                            />
                            <Tooltip
                                contentStyle={{
                                    borderRadius: 8,
                                    border: '1px solid #e5e7eb',
                                    boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)',
                                }}
                                formatter={(value: number) => formatTokens(value)}
                            />
                            <Legend
                                verticalAlign="top"
                                align="right"
                                height={36}
                                iconType="circle"
                                iconSize={8}
                            />
                            <Area
                                type="monotone"
                                dataKey="input_tokens"
                                name="Input"
                                stroke="#3b82f6"
                                strokeWidth={2}
                                fill="url(#inputGradient)"
                            />
                            <Area
                                type="monotone"
                                dataKey="output_tokens"
                                name="Output"
                                stroke="#10b981"
                                strokeWidth={2}
                                fill="url(#outputGradient)"
                            />
                        </AreaChart>
                    </ResponsiveContainer>
                </Box>
            ) : (
                <Box
                    sx={{
                        height: 280,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    <Typography>No historical data yet</Typography>
                </Box>
            )}
        </Paper>
    );
}
