import {
    Paper,
    Typography,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TablePagination,
    Box,
    alpha,
} from '@mui/material';
import { useState } from 'react';

export interface AggregatedStat {
    key: string;
    provider_uuid?: string;
    provider_name?: string;
    model?: string;
    scenario?: string;
    request_count: number;
    total_tokens?: number;
    total_input_tokens: number;
    total_output_tokens: number;
    avg_latency_ms: number;
    error_count: number;
    error_rate: number;
    streamed_count?: number;
}

interface ServiceStatsTableProps {
    stats: AggregatedStat[];
}

export default function ServiceStatsTable({ stats }: ServiceStatsTableProps) {
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(10);

    const formatTokens = (num: number): string => {
        if (num >= 1000000) return `${(num / 1000000).toFixed(2)}M`;
        if (num >= 1000) return `${(num / 1000).toFixed(2)}K`;
        return num.toLocaleString();
    };

    const formatRequests = (num: number): string => {
        return num.toLocaleString();
    };

    const handleChangePage = (_event: unknown, newPage: number) => {
        setPage(newPage);
    };

    const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
        setRowsPerPage(parseInt(event.target.value, 10));
        setPage(0);
    };

    // Avoid a layout jump when reaching the last page with empty rows
    const emptyRows = page > 0 ? Math.max(0, (1 + page) * rowsPerPage - stats.length) : 0;

    const visibleStats = stats.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

    return (
        <Paper
            elevation={0}
            sx={{
                borderRadius: 2.5,
                border: '1px solid',
                borderColor: 'divider',
                overflow: 'hidden',
                backgroundColor: 'background.paper',
                boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
            }}
        >
            <Box
                sx={{
                    p: 3,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                }}
            >
                <Typography variant="h6" sx={{ fontWeight: 600 }}>
                    Usage by Model
                </Typography>
            </Box>
            <TableContainer sx={{ maxHeight: 600 }}>
                <Table stickyHeader>
                    <TableHead>
                        <TableRow
                            sx={{
                                backgroundColor: alpha('#f1f5f9', 0.8),
                                '& .MuiTableCell-root': {
                                    fontWeight: 600,
                                    fontSize: '0.75rem',
                                    textTransform: 'uppercase',
                                    letterSpacing: '0.05em',
                                    color: 'text.secondary',
                                    py: 2,
                                    borderBottom: '2px solid',
                                    borderColor: 'divider',
                                },
                            }}
                        >
                            <TableCell sx={{ fontWeight: 600 }}>Provider</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Model</TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>
                                Requests
                            </TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>
                                Input Tokens
                            </TableCell>
                            <TableCell align="right" sx={{ fontWeight: 600 }}>
                                Output Tokens
                            </TableCell>
                            {/* <TableCell align="right" sx={{ fontWeight: 600 }}>Avg Latency</TableCell> */}
                            <TableCell align="right" sx={{ fontWeight: 600 }}>
                                Error Rate
                            </TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {stats.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={6} align="center" sx={{ py: 8 }}>
                                    <Box sx={{ textAlign: 'center' }}>
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
                                                mx: 'auto',
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
                                            No usage data available
                                        </Typography>
                                        <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                                            Select a different time range or check back later
                                        </Typography>
                                    </Box>
                                </TableCell>
                            </TableRow>
                        ) : (
                            <>
                                {visibleStats.map((stat, index) => (
                                    <TableRow
                                        key={index}
                                        hover
                                        sx={{
                                            transition: 'background-color 0.15s ease',
                                            '&:hover': {
                                                backgroundColor: 'action.hover',
                                            },
                                            '& .MuiTableCell-root': {
                                                py: 2,
                                                borderBottom: '1px solid',
                                                borderColor: 'divider',
                                            },
                                        }}
                                    >
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
                                        <TableCell align="right">{formatRequests(stat.request_count)}</TableCell>
                                        <TableCell align="right">{formatTokens(stat.total_input_tokens)}</TableCell>
                                        <TableCell align="right">{formatTokens(stat.total_output_tokens)}</TableCell>
                                        {/* <TableCell align="right">
                                            {stat.avg_latency_ms > 0 ? `${stat.avg_latency_ms.toFixed(0)}ms` : '-'}
                                        </TableCell> */}
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
                                ))}
                                {emptyRows > 0 && (
                                    <TableRow style={{ height: 53 * emptyRows }}>
                                        <TableCell colSpan={6} />
                                    </TableRow>
                                )}
                            </>
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
            <TablePagination
                rowsPerPageOptions={[5, 10, 25, 50]}
                component="div"
                count={stats.length}
                rowsPerPage={rowsPerPage}
                page={page}
                onPageChange={handleChangePage}
                onRowsPerPageChange={handleChangeRowsPerPage}
                sx={{
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    '& .MuiTablePagination-toolbar': {
                        minHeight: 52,
                    },
                }}
            />
        </Paper>
    );
}
