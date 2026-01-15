import {
    Paper,
    Typography,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Box,
} from '@mui/material';

interface AggregatedStat {
    key: string;
    provider_uuid?: string;
    provider_name?: string;
    model?: string;
    scenario?: string;
    request_count: number;
    total_tokens: number;
    total_input_tokens: number;
    total_output_tokens: number;
    avg_latency_ms: number;
    error_count: number;
    error_rate: number;
}

interface ServiceStatsTableProps {
    stats: AggregatedStat[];
}

export default function ServiceStatsTable({ stats }: ServiceStatsTableProps) {
    const formatNumber = (num: number): string => {
        if (num >= 1000000) return `${(num / 1000000).toFixed(2)}M`;
        if (num >= 1000) return `${(num / 1000).toFixed(2)}K`;
        return num.toLocaleString();
    };

    return (
        <Paper
            elevation={0}
            sx={{
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                overflow: 'hidden',
            }}
        >
            <Box sx={{ p: 3, borderBottom: '1px solid', borderColor: 'divider' }}>
                <Typography variant="h6" sx={{ fontWeight: 600 }}>
                    Usage by Model
                </Typography>
            </Box>
            <TableContainer>
                <Table>
                    <TableHead>
                        <TableRow sx={{ backgroundColor: '#fafafa' }}>
                            <TableCell sx={{ fontWeight: 600 }}>Provider</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Model</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Requests</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Input Tokens</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Output Tokens</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Avg Latency</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Error Rate</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {stats.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={7} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                                    No usage data available
                                </TableCell>
                            </TableRow>
                        ) : (
                            stats.map((stat, index) => (
                                <TableRow key={index} hover>
                                    <TableCell>{stat.provider_name || '-'}</TableCell>
                                    <TableCell>
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                maxWidth: 200,
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                            }}
                                            title={stat.model}
                                        >
                                            {stat.model || stat.key}
                                        </Typography>
                                    </TableCell>
                                    <TableCell align="right">{formatNumber(stat.request_count)}</TableCell>
                                    <TableCell align="right">{formatNumber(stat.total_input_tokens)}</TableCell>
                                    <TableCell align="right">{formatNumber(stat.total_output_tokens)}</TableCell>
                                    <TableCell align="right">
                                        {stat.avg_latency_ms > 0 ? `${stat.avg_latency_ms.toFixed(0)}ms` : '-'}
                                    </TableCell>
                                    <TableCell align="right">
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                color: stat.error_rate > 0.05 ? 'error.main' : 'text.secondary',
                                            }}
                                        >
                                            {(stat.error_rate * 100).toFixed(2)}%
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ))
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
        </Paper>
    );
}
