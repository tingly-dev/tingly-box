import { Paper, Typography, Box, alpha } from '@mui/material';
import {
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
    Legend,
} from 'recharts';

interface UsageData {
    name: string;
    inputTokens: number;
    outputTokens: number;
}

interface TokenUsageChartProps {
    data: UsageData[];
}

export default function TokenUsageChart({ data }: TokenUsageChartProps) {
    // Sort by total tokens (input + output) and take top 5
    const top5Data = [...data]
        .sort((a, b) => (b.inputTokens + b.outputTokens) - (a.inputTokens + a.outputTokens))
        .slice(0, 5);

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

    return (
        <Paper
            elevation={0}
            sx={{
                p: 3,
                borderRadius: 2.5,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
                backgroundColor: 'background.paper',
                boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <Box sx={{ mb: 2 }}>
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
                <Box sx={{ flex: 1, minHeight: 280 }}>
                    <ResponsiveContainer width="100%" height={280}>
                        <BarChart data={top5Data} layout="vertical">
                            <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                            <XAxis
                                type="number"
                                tickFormatter={formatYAxis}
                                tick={{ fontSize: 12 }}
                                tickLine={false}
                                axisLine={{ stroke: '#e0e0e0' }}
                            />
                            <YAxis
                                dataKey="name"
                                type="category"
                                tick={{ fontSize: 11 }}
                                tickLine={false}
                                axisLine={{ stroke: '#e0e0e0' }}
                                width={160}
                            />
                            <Tooltip
                                formatter={(value: number, name: string) => [formatTooltipValue(value), name]}
                                contentStyle={{
                                    borderRadius: 8,
                                    border: '1px solid #e0e0e0',
                                    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                                }}
                            />
                            <Legend />
                            <Bar dataKey="inputTokens" name="Input Tokens" fill="#1976d2" stackId="stack" />
                            <Bar dataKey="outputTokens" name="Output Tokens" fill="#2e7d32" stackId="stack" />
                        </BarChart>
                    </ResponsiveContainer>
                </Box>
            )}
        </Paper>
    );
}
