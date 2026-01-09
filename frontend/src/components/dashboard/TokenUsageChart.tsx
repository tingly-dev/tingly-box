import { Box, Paper, Typography } from '@mui/material';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

interface TokenUsageData {
    name: string;
    inputTokens: number;
    outputTokens: number;
}

interface TokenUsageChartProps {
    data: TokenUsageData[];
}

export default function TokenUsageChart({ data }: TokenUsageChartProps) {
    const hasData = data.length > 0;

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
                Token Usage by Model
            </Typography>
            {hasData ? (
                <Box sx={{ width: '100%', height: 280 }}>
                    <ResponsiveContainer>
                        <BarChart data={data} margin={{ top: 10, right: 30, left: 0, bottom: 0 }}>
                            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                            <XAxis dataKey="name" tick={{ fontSize: 12 }} />
                            <YAxis tick={{ fontSize: 12 }} />
                            <Tooltip
                                contentStyle={{
                                    borderRadius: 8,
                                    border: '1px solid #e5e7eb',
                                    boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)',
                                }}
                            />
                            <Legend
                                verticalAlign="top"
                                align="right"
                                height={36}
                                iconType="circle"
                                iconSize={8}
                            />
                            <Bar dataKey="inputTokens" name="Input" stackId="a" fill="#3b82f6" radius={[4, 4, 0, 0]} />
                            <Bar dataKey="outputTokens" name="Output" stackId="a" fill="#10b981" radius={[4, 4, 0, 0]} />
                        </BarChart>
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
                    <Typography>No data available</Typography>
                </Box>
            )}
        </Paper>
    );
}
