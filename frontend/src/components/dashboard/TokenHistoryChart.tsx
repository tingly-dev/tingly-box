import { Paper, Typography, Box } from '@mui/material';
import {
    AreaChart,
    Area,
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
    }));

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
                    <AreaChart data={chartData}>
                        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                        <XAxis
                            dataKey="time"
                            tick={{ fontSize: 12 }}
                            tickLine={false}
                            axisLine={{ stroke: '#e0e0e0' }}
                        />
                        <YAxis
                            tickFormatter={formatYAxis}
                            tick={{ fontSize: 12 }}
                            tickLine={false}
                            axisLine={{ stroke: '#e0e0e0' }}
                        />
                        <Tooltip
                            formatter={(value: number) => [value.toLocaleString(), '']}
                            contentStyle={{
                                borderRadius: 8,
                                border: '1px solid #e0e0e0',
                                boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                            }}
                        />
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
                    </AreaChart>
                </ResponsiveContainer>
            )}
        </Paper>
    );
}
