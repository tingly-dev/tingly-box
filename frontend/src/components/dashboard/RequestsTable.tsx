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
    Chip,
    Tooltip,
    ToggleButtonGroup,
    ToggleButton,
    CircularProgress,
    useTheme,
} from '@mui/material';
import { tablerMui } from '@/components/icons';
import { IconWaveSine } from '@tabler/icons-react';

const StreamIcon = tablerMui(IconWaveSine);

export interface UsageRecord {
    id: number;
    provider_uuid: string;
    provider_name: string;
    model: string;
    scenario: string;
    rule_uuid?: string;
    user_id?: string;
    request_model?: string;
    timestamp: string;
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
    cache_input_tokens: number;
    status: string;
    error_code?: string;
    latency_ms: number;
    streamed: boolean;
}

interface RequestsTableProps {
    records: UsageRecord[];
    total: number;
    page: number;
    rowsPerPage: number;
    statusFilter: 'all' | 'success' | 'error';
    loading: boolean;
    onPageChange: (page: number) => void;
    onRowsPerPageChange: (rowsPerPage: number) => void;
    onStatusFilterChange: (status: 'all' | 'success' | 'error') => void;
}

export default function RequestsTable({
    records,
    total,
    page,
    rowsPerPage,
    statusFilter,
    loading,
    onPageChange,
    onRowsPerPageChange,
    onStatusFilterChange,
}: RequestsTableProps) {
    const theme = useTheme();

    const formatTime = (timestamp: string): string => {
        const date = new Date(timestamp);
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    };

    const formatTokens = (num: number): string => {
        if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
        if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
        return num.toString();
    };

    const getLatencyColor = (ms: number): string => {
        if (ms > 2000) return theme.palette.error.main;
        if (ms > 1000) return theme.palette.warning.main;
        return theme.palette.success.main;
    };

    return (
        <Paper
            elevation={0}
            sx={{
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                overflow: 'hidden',
                backgroundColor: 'background.paper',
                boxShadow: 'none',
                width: '100%',
            }}
        >
            {/* Header */}
            <Box
                sx={{
                    px: 2.5,
                    py: 1.5,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    gap: 2,
                    flexWrap: 'wrap',
                }}
            >
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Requests
                    {!loading && (
                        <Typography component="span" variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>
                            {total.toLocaleString()} total
                        </Typography>
                    )}
                </Typography>
                <ToggleButtonGroup
                    value={statusFilter}
                    exclusive
                    onChange={(_, v) => v && onStatusFilterChange(v)}
                    size="small"
                    sx={{
                        '& .MuiToggleButton-root': {
                            px: 1.5,
                            py: 0.375,
                            fontSize: '0.75rem',
                            textTransform: 'none',
                            lineHeight: 1.4,
                        },
                    }}
                >
                    <ToggleButton value="all">All</ToggleButton>
                    <ToggleButton value="success">Success</ToggleButton>
                    <ToggleButton value="error">Error</ToggleButton>
                </ToggleButtonGroup>
            </Box>

            {/* Table */}
            <TableContainer sx={{ maxHeight: 420, position: 'relative' }}>
                {loading && (
                    <Box
                        sx={{
                            position: 'absolute',
                            inset: 0,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            backgroundColor: theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.3)' : 'rgba(255,255,255,0.6)',
                            zIndex: 1,
                        }}
                    >
                        <CircularProgress size={28} />
                    </Box>
                )}
                <Table stickyHeader size="small">
                    <TableHead>
                        <TableRow
                            sx={{
                                '& .MuiTableCell-root': {
                                    fontWeight: 600,
                                    fontSize: '0.7rem',
                                    textTransform: 'uppercase',
                                    letterSpacing: '0.05em',
                                    color: 'text.secondary',
                                    py: 1,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                    whiteSpace: 'nowrap',
                                    backgroundColor: 'background.paper',
                                },
                            }}
                        >
                            <TableCell>Time</TableCell>
                            <TableCell>Provider / Model</TableCell>
                            <TableCell>Scenario</TableCell>
                            <TableCell align="right">Input</TableCell>
                            <TableCell align="right">Output</TableCell>
                            <TableCell align="right">Cache</TableCell>
                            <TableCell align="right">Latency</TableCell>
                            <TableCell align="center">Status</TableCell>
                            <TableCell align="center">Stream</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {records.length === 0 && !loading ? (
                            <TableRow>
                                <TableCell colSpan={9} align="center" sx={{ py: 6 }}>
                                    <Typography variant="body2" color="text.secondary">
                                        No requests found
                                    </Typography>
                                    <Typography variant="caption" color="text.disabled">
                                        Try changing the status filter or time range
                                    </Typography>
                                </TableCell>
                            </TableRow>
                        ) : (
                            records.map((record) => (
                                <TableRow
                                    key={record.id}
                                    hover
                                    sx={{
                                        '& .MuiTableCell-root': {
                                            py: 0.625,
                                            borderBottom: '1px solid',
                                            borderColor: 'divider',
                                        },
                                    }}
                                >
                                    {/* Time */}
                                    <TableCell>
                                        <Tooltip title={new Date(record.timestamp).toLocaleString()} placement="right">
                                            <Typography
                                                variant="caption"
                                                sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary', cursor: 'default' }}
                                            >
                                                {formatTime(record.timestamp)}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>

                                    {/* Provider / Model */}
                                    <TableCell>
                                        <Typography variant="caption" sx={{ display: 'block', color: 'text.disabled', fontSize: '0.65rem', lineHeight: 1.2 }}>
                                            {record.provider_name || '-'}
                                        </Typography>
                                        <Tooltip title={record.model} placement="top">
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    fontSize: '0.78rem',
                                                    maxWidth: 180,
                                                    overflow: 'hidden',
                                                    textOverflow: 'ellipsis',
                                                    whiteSpace: 'nowrap',
                                                    lineHeight: 1.4,
                                                }}
                                            >
                                                {record.model || '-'}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>

                                    {/* Scenario */}
                                    <TableCell>
                                        <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.75rem' }}>
                                            {record.scenario || '-'}
                                        </Typography>
                                    </TableCell>

                                    {/* Input tokens */}
                                    <TableCell align="right">
                                        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                            {formatTokens(record.input_tokens)}
                                        </Typography>
                                    </TableCell>

                                    {/* Output tokens */}
                                    <TableCell align="right">
                                        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                            {formatTokens(record.output_tokens)}
                                        </Typography>
                                    </TableCell>

                                    {/* Cache tokens */}
                                    <TableCell align="right">
                                        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'text.secondary' }}>
                                            {record.cache_input_tokens > 0 ? formatTokens(record.cache_input_tokens) : '-'}
                                        </Typography>
                                    </TableCell>

                                    {/* Latency */}
                                    <TableCell align="right">
                                        <Typography
                                            variant="caption"
                                            sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: record.latency_ms > 0 ? getLatencyColor(record.latency_ms) : 'text.disabled' }}
                                        >
                                            {record.latency_ms > 0 ? `${record.latency_ms}ms` : '-'}
                                        </Typography>
                                    </TableCell>

                                    {/* Status */}
                                    <TableCell align="center">
                                        {record.status === 'success' ? (
                                            <Chip
                                                label="OK"
                                                size="small"
                                                sx={{
                                                    height: 18,
                                                    fontSize: '0.65rem',
                                                    fontWeight: 700,
                                                    backgroundColor: 'success.main',
                                                    color: '#fff',
                                                    '& .MuiChip-label': { px: 0.75 },
                                                }}
                                            />
                                        ) : (
                                            <Tooltip title={record.error_code || record.status} placement="top">
                                                <Chip
                                                    label="ERR"
                                                    size="small"
                                                    sx={{
                                                        height: 18,
                                                        fontSize: '0.65rem',
                                                        fontWeight: 700,
                                                        backgroundColor: 'error.main',
                                                        color: '#fff',
                                                        '& .MuiChip-label': { px: 0.75 },
                                                    }}
                                                />
                                            </Tooltip>
                                        )}
                                    </TableCell>

                                    {/* Stream */}
                                    <TableCell align="center">
                                        {record.streamed && (
                                            <Tooltip title="Streamed">
                                                <Box sx={{ display: 'inline-flex', color: 'primary.main' }}>
                                                    <StreamIcon sx={{ fontSize: '0.95rem' }} />
                                                </Box>
                                            </Tooltip>
                                        )}
                                    </TableCell>
                                </TableRow>
                            ))
                        )}
                    </TableBody>
                </Table>
            </TableContainer>

            <TablePagination
                rowsPerPageOptions={[50, 100, 200]}
                component="div"
                count={total}
                rowsPerPage={rowsPerPage}
                page={page}
                onPageChange={(_, p) => onPageChange(p)}
                onRowsPerPageChange={(e) => onRowsPerPageChange(parseInt(e.target.value, 10))}
                sx={{
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    '& .MuiTablePagination-toolbar': { minHeight: 48 },
                    '& .MuiTablePagination-selectLabel, & .MuiTablePagination-displayedRows': {
                        fontSize: '0.75rem',
                    },
                }}
            />
        </Paper>
    );
}
