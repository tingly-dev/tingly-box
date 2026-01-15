import { Paper, Typography, Box } from '@mui/material';
import {
    ComposedChart,
    Area,
    Line,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    Legend,
} from 'recharts';

interface TimeSeriesData {
    timestamp: string;
    request_count: number;
    total_tokens: number;
    input_tokens: number;
    output_tokens: number;
    error_count?: number;
    avg_latency_ms?: number;
}

interface TokenHistoryChartProps {
    data: TimeSeriesData[];
}

export default function TokenHistoryChart({ data }: TokenHistoryChartProps) {
    // Format data for chart
    const chartData = data.map((item) => ({
        time: new Date(item.timestamp).toLocaleString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
        }),
        inputTokens: item.input_tokens,
        outputTokens: item.output_tokens,
        requests: item.request_count,
        errors: item.error_count || 0,
    }));

    // Check if there are any errors in the data
    const hasErrors = chartData.some((d) => d.errors > 0);

    const formatYAxis = (value: number) => {
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
        return value.toString();
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
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
                Token Usage Over Time
            </Typography>
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
                            tick={{ fontSize: 12 }}
                            tickLine={false}
                            axisLine={{ stroke: '#e0e0e0' }}
                        />
                        <YAxis
                            yAxisId="left"
                            tickFormatter={formatYAxis}
                            tick={{ fontSize: 12 }}
                            tickLine={false}
                            axisLine={{ stroke: '#e0e0e0' }}
                        />
                        {hasErrors && (
                            <YAxis
                                yAxisId="right"
                                orientation="right"
                                tick={{ fontSize: 12 }}
                                tickLine={false}
                                axisLine={{ stroke: '#e0e0e0' }}
                            />
                        )}
                        <Tooltip
                            formatter={(value: number, name: string) => [
                                value.toLocaleString(),
                                name,
                            ]}
                            contentStyle={{
                                borderRadius: 8,
                                border: '1px solid #e0e0e0',
                                boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                            }}
                        />
                        <Legend />
                        <Area
                            yAxisId="left"
                            type="monotone"
                            dataKey="inputTokens"
                            name="Input Tokens"
                            stackId="1"
                            stroke="#1976d2"
                            fill="#bbdefb"
                        />
                        <Area
                            yAxisId="left"
                            type="monotone"
                            dataKey="outputTokens"
                            name="Output Tokens"
                            stackId="1"
                            stroke="#2e7d32"
                            fill="#c8e6c9"
                        />
                        <Line
                            yAxisId="left"
                            type="monotone"
                            dataKey="requests"
                            name="Requests"
                            stroke="#ff9800"
                            strokeWidth={2}
                            dot={false}
                        />
                        {hasErrors && (
                            <Line
                                yAxisId="right"
                                type="monotone"
                                dataKey="errors"
                                name="Errors"
                                stroke="#d32f2f"
                                strokeWidth={2}
                                strokeDasharray="5 5"
                                dot={false}
                            />
                        )}
                    </ComposedChart>
                </ResponsiveContainer>
            )}
        </Paper>
    );
}
