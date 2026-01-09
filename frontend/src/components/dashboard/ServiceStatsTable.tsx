import {
    Paper,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    Box,
} from '@mui/material';

interface ServiceStat {
    service_id: string;
    request_count: number;
    window_request_count: number;
    window_input_tokens: number;
    window_output_tokens: number;
    window_tokens_consumed: number;
    last_used: string;
}

interface ServiceStatsTableProps {
    stats: Record<string, ServiceStat>;
    providerMap?: Record<string, string>;
}

function formatNumber(num: number): string {
    if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M';
    }
    if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
}

function formatTimeAgo(dateStr: string): string {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d ago`;
}

function parseServiceId(serviceId: string): { provider: string; model: string } {
    const parts = serviceId.split(':');
    if (parts.length >= 2) {
        return { provider: parts[0], model: parts.slice(1).join(':') };
    }
    return { provider: serviceId, model: '-' };
}

export default function ServiceStatsTable({ stats, providerMap = {} }: ServiceStatsTableProps) {
    const entries = Object.entries(stats);
    const hasData = entries.length > 0;

    // Helper to get provider name from UUID
    const getProviderName = (providerUuid: string): string => {
        return providerMap[providerUuid] || providerUuid;
    };

    return (
        <Paper
            elevation={0}
            sx={{
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                overflow: 'hidden',
            }}
        >
            <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider' }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                    Service Statistics
                </Typography>
            </Box>
            <TableContainer>
                <Table size="small">
                    <TableHead>
                        <TableRow sx={{ bgcolor: 'grey.50' }}>
                            <TableCell sx={{ fontWeight: 600 }}>Provider</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Model</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Requests</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Input Tokens</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Output Tokens</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>Last Used</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {hasData ? (
                            entries.map(([key, stat]) => {
                                const { provider, model } = parseServiceId(stat.service_id || key);
                                const providerName = getProviderName(provider);
                                return (
                                    <TableRow key={key} hover>
                                        <TableCell>{providerName}</TableCell>
                                        <TableCell sx={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                                            {model}
                                        </TableCell>
                                        <TableCell align="right">{formatNumber(stat.request_count || 0)}</TableCell>
                                        <TableCell align="right">{formatNumber(stat.window_input_tokens || 0)}</TableCell>
                                        <TableCell align="right">{formatNumber(stat.window_output_tokens || 0)}</TableCell>
                                        <TableCell align="right">{formatTimeAgo(stat.last_used)}</TableCell>
                                    </TableRow>
                                );
                            })
                        ) : (
                            <TableRow>
                                <TableCell colSpan={6} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                                    No service statistics available
                                </TableCell>
                            </TableRow>
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
        </Paper>
    );
}
