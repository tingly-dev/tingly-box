import { useEffect, useRef, useState } from 'react';
import {
    Box,
    Paper,
    Typography,
    Chip,
    Tooltip,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TablePagination,
    ToggleButtonGroup,
    ToggleButton,
    CircularProgress,
    useTheme,
} from '@mui/material';
import { WaveSine as StreamIcon } from '@/components/icons';
import { TOKEN_COLORS, formatNumber, hasCacheWrites } from './chartStyles';
import api from '@/services/api';

// ─── Types ───────────────────────────────────────────────────────────────────

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
    cache_read_tokens: number;
    cache_write_tokens?: number;
    status: string;
    error_code?: string;
    latency_ms: number;
    ttft_ms?: number;
    tokens_per_second?: number;
    streamed: boolean;
}

// ─── Constants ────────────────────────────────────────────────────────────────

const SUCCESS_COLOR = '#10B981';
const ERROR_COLOR = '#EF4444';

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmtTokens = formatNumber;

const fmtLatency = (ms: number) => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
    return `${ms}ms`;
};

const getTokensPerSecond = (record: UsageRecord) => {
    if (!record.streamed) return 0;
    if ((record.tokens_per_second ?? 0) > 0) return record.tokens_per_second!;
    const decodeMs = record.latency_ms - (record.ttft_ms ?? 0);
    if (record.output_tokens <= 1 || (record.ttft_ms ?? 0) <= 0 || decodeMs <= 0) return 0;
    return (record.output_tokens - 1) * 1000 / decodeMs;
};

const getTPSFormula = (record: UsageRecord) => {
    const decodeMs = record.latency_ms - (record.ttft_ms ?? 0);
    if (getTokensPerSecond(record) <= 0) return '';
    return `${record.output_tokens - 1} decode intervals / ${decodeMs}ms after TTFT`;
};

const fmtTime = (ts: string) =>
    new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

// ─── Request Table ────────────────────────────────────────────────────────────

const getLatencyColor = (ms: number, theme: any) => {
    if (ms > 2000) return theme.palette.error.main;
    if (ms > 1000) return theme.palette.warning.main;
    return theme.palette.success.main;
};

interface TableSectionProps {
    records: UsageRecord[];
    total: number;
    page: number;
    rowsPerPage: number;
    statusFilter: 'all' | 'success' | 'error';
    loading: boolean;
    onStatusFilterChange: (s: 'all' | 'success' | 'error') => void;
    onPageChange: (p: number) => void;
    onRowsPerPageChange: (r: number) => void;
}

function RequestTable({ records, total, page, rowsPerPage, statusFilter, loading, onStatusFilterChange, onPageChange, onRowsPerPageChange }: TableSectionProps) {
    const theme = useTheme();
    const showCacheWrite = hasCacheWrites(records);

    return (
        <Paper elevation={0} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, overflow: 'hidden', backgroundColor: 'background.paper', boxShadow: 'none', width: '100%', minWidth: 0 }}>
            {/* Header */}
            <Box sx={{ px: 2.5, py: 1.5, borderBottom: '1px solid', borderColor: 'divider', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 1.5 }}>
                <Typography sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    Requests
                    <Typography component="span" variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>
                        {!loading && `${total.toLocaleString()} total`}
                    </Typography>
                </Typography>
                <ToggleButtonGroup
                    value={statusFilter} exclusive size="small"
                    onChange={(_, v) => v && onStatusFilterChange(v)}
                    sx={{ '& .MuiToggleButton-root': { px: 1.5, py: 0.375, fontSize: '0.75rem', textTransform: 'none' } }}
                >
                    <ToggleButton value="all">All</ToggleButton>
                    <ToggleButton value="success">Success</ToggleButton>
                    <ToggleButton value="error">Error</ToggleButton>
                </ToggleButtonGroup>
            </Box>
            {/* Table */}
            <TableContainer sx={{ maxHeight: 420, maxWidth: '100%', overflow: 'auto', position: 'relative' }}>
                {loading && (
                    <Box sx={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1, backgroundColor: theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.3)' : 'rgba(255,255,255,0.6)' }}>
                        <CircularProgress size={28} />
                    </Box>
                )}
                <Table stickyHeader size="small" sx={{ tableLayout: 'auto' }}>
                    <TableHead>
                        <TableRow sx={{ '& .MuiTableCell-root': { fontWeight: 600, fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em', color: 'text.secondary', py: 1, borderBottom: '1px solid', borderColor: 'divider', backgroundColor: 'background.paper', whiteSpace: 'nowrap' } }}>
                            <TableCell>Time</TableCell>
                            <TableCell>Model</TableCell>
                            <TableCell>Scenario</TableCell>
                            <TableCell align="right" sx={{ minWidth: 96 }}>Cache Read</TableCell>
                            {showCacheWrite && <TableCell align="right">Cache Write</TableCell>}
                            <TableCell align="right">Input</TableCell>
                            <TableCell align="right">Output</TableCell>
                            <TableCell align="right">Latency</TableCell>
                            <TableCell align="right">TTFT</TableCell>
                            <TableCell align="right">TPS</TableCell>
                            <TableCell align="center">Status</TableCell>
                            <TableCell align="center">Stream</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {records.length === 0 && !loading ? (
                            <TableRow>
                                <TableCell colSpan={11 + (showCacheWrite ? 1 : 0)} align="center" sx={{ py: 5 }}>
                                    <Typography variant="body2" sx={{
                                        color: "text.secondary"
                                    }}>No requests found</Typography>
                                    <Typography variant="caption" sx={{
                                        color: "text.disabled"
                                    }}>Try changing the status filter</Typography>
                                </TableCell>
                            </TableRow>
                        ) : records.map(r => (
                            <TableRow key={r.id} hover sx={{ '& .MuiTableCell-root': { py: 0.625, borderBottom: '1px solid', borderColor: 'divider' } }}>
                                {/* Time */}
                                <TableCell>
                                    <Tooltip title={new Date(r.timestamp).toLocaleString()} placement="right">
                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary', cursor: 'default' }}>
                                            {fmtTime(r.timestamp)}
                                        </Typography>
                                    </Tooltip>
                                </TableCell>

                                {/* Model */}
                                <TableCell>
                                    <Typography sx={{ fontSize: '0.65rem', color: 'text.disabled', lineHeight: 1.2 }}>
                                        {r.provider_name || '-'}
                                    </Typography>
                                    <Tooltip title={r.model} placement="top">
                                        <Typography sx={{ fontSize: '0.78rem', maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.4 }}>
                                            {r.model || '-'}
                                        </Typography>
                                    </Tooltip>
                                </TableCell>

                                {/* Scenario */}
                                <TableCell>
                                    <Typography sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                        {r.scenario || '-'}
                                    </Typography>
                                </TableCell>

                                {/* Tokens */}
                                <TableCell align="right">
                                    {(() => {
                                        const cacheTokens = r.cache_read_tokens || 0;
                                        const inputTokens = r.input_tokens || 0;
                                        const total = cacheTokens + inputTokens;
                                        const ratio = total > 0 ? (cacheTokens / total) * 100 : 0;
                                        return (
                                            <Box sx={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 0.5, lineHeight: 1.2, whiteSpace: 'nowrap' }}>
                                                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: cacheTokens > 0 ? 'text.primary' : 'text.disabled' }}>
                                                    {cacheTokens > 0 ? fmtTokens(cacheTokens) : '-'}
                                                </Typography>
                                                {cacheTokens > 0 && total > 0 && (
                                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.65rem', color: 'text.secondary' }}>
                                                        | {ratio.toFixed(1)}%
                                                    </Typography>
                                                )}
                                            </Box>
                                        );
                                    })()}
                                </TableCell>

                                {showCacheWrite && (
                                    <TableCell align="right">
                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: (r.cache_write_tokens || 0) > 0 ? 'text.primary' : 'text.disabled' }}>
                                            {(r.cache_write_tokens || 0) > 0 ? fmtTokens(r.cache_write_tokens || 0) : '-'}
                                        </Typography>
                                    </TableCell>
                                )}
                                <TableCell align="right">
                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: TOKEN_COLORS.input.main }}>
                                        {fmtTokens(r.input_tokens)}
                                    </Typography>
                                </TableCell>
                                <TableCell align="right">
                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: TOKEN_COLORS.output.main }}>
                                        {fmtTokens(r.output_tokens)}
                                    </Typography>
                                </TableCell>

                                {/* Latency */}
                                <TableCell align="right">
                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: r.latency_ms > 0 ? getLatencyColor(r.latency_ms, theme) : 'text.disabled' }}>
                                        {r.latency_ms > 0 ? fmtLatency(r.latency_ms) : '-'}
                                    </Typography>
                                </TableCell>

                                {/* TTFT */}
                                <TableCell align="right">
                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: (r.ttft_ms ?? 0) > 0 ? getLatencyColor(r.ttft_ms!, theme) : 'text.disabled' }}>
                                        {(r.ttft_ms ?? 0) > 0 ? fmtLatency(r.ttft_ms!) : '-'}
                                    </Typography>
                                </TableCell>

                                {/* Per-request output TPS after TTFT */}
                                <TableCell align="right">
                                    <Tooltip title={getTPSFormula(r)} placement="top">
                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: getTokensPerSecond(r) > 0 ? 'text.primary' : 'text.disabled', cursor: getTokensPerSecond(r) > 0 ? 'help' : 'default' }}>
                                            {getTokensPerSecond(r) > 0 ? getTokensPerSecond(r).toFixed(1) : '-'}
                                        </Typography>
                                    </Tooltip>
                                </TableCell>

                                {/* Status */}
                                <TableCell align="center">
                                    {r.status === 'success' ? (
                                        <Chip label="OK" size="small" sx={{ height: 18, fontSize: '0.65rem', fontWeight: 700, backgroundColor: SUCCESS_COLOR, color: '#fff', '& .MuiChip-label': { px: 0.75 } }} />
                                    ) : (
                                        <Tooltip title={r.error_code || r.status} placement="top">
                                            <Chip label="ERR" size="small" sx={{ height: 18, fontSize: '0.65rem', fontWeight: 700, backgroundColor: ERROR_COLOR, color: '#fff', '& .MuiChip-label': { px: 0.75 } }} />
                                        </Tooltip>
                                    )}
                                </TableCell>

                                {/* Stream */}
                                <TableCell align="center">
                                    {r.streamed && (
                                        <Tooltip title="Streamed">
                                            <Box sx={{ display: 'inline-flex', color: 'primary.main' }}>
                                                <StreamIcon sx={{ fontSize: '0.95rem' }} />
                                            </Box>
                                        </Tooltip>
                                    )}
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </TableContainer>
            <TablePagination
                rowsPerPageOptions={[20, 50, 100]}
                component="div"
                count={total}
                rowsPerPage={rowsPerPage}
                page={page}
                onPageChange={(_, p) => onPageChange(p)}
                onRowsPerPageChange={e => onRowsPerPageChange(parseInt(e.target.value, 10))}
                sx={{ borderTop: '1px solid', borderColor: 'divider', '& .MuiTablePagination-toolbar': { minHeight: 48 }, '& .MuiTablePagination-selectLabel, & .MuiTablePagination-displayedRows': { fontSize: '0.75rem' } }}
            />
        </Paper>
    );
}

// ─── Main Export ─────────────────────────────────────────────────────────────

/** Base query for the records endpoint (time window + dashboard filters). */
export interface RecordsQueryParams {
    start_time: string;
    end_time: string;
    provider: string;
    model: string;
    user: string;
}

interface RequestsViewProps {
    /** Most recent records in range (capped sample) — feeds the table directly
     *  when it covers the whole range. */
    records: UsageRecord[];
    loading: boolean;
    /** Real total in range from the server; records may be capped below it. */
    totalCount?: number;
    /** Query the sample was fetched with; used to page the rest server-side. */
    queryParams?: RecordsQueryParams | null;
}

export default function RequestsView({ records, loading, totalCount, queryParams }: RequestsViewProps) {
    const [statusFilter, setStatusFilter] = useState<'all' | 'success' | 'error'>('all');
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(50);

    // When the sample holds every record in range, filter/paginate it locally
    // (no extra requests). Otherwise the table switches to server-side paging
    // so every record stays reachable instead of only the most recent slice.
    const sampleComplete = totalCount == null || records.length >= totalCount;

    const [serverRows, setServerRows] = useState<UsageRecord[]>([]);
    const [serverTotal, setServerTotal] = useState(0);
    const [serverLoading, setServerLoading] = useState(false);
    const serverSeq = useRef(0);

    // Back to the first page when the filters or the range start change —
    // but not on every auto-refresh tick (only end_time moves there).
    const resetKey = queryParams
        ? `${queryParams.provider}|${queryParams.model}|${queryParams.user}|${queryParams.start_time}`
        : '';
    useEffect(() => {
        setPage(0);
    }, [resetKey]);

    useEffect(() => {
        if (sampleComplete || !queryParams) return;
        const seq = ++serverSeq.current;
        setServerLoading(true);
        (async () => {
            try {
                const filters: Record<string, any> = {
                    start_time: queryParams.start_time,
                    end_time: queryParams.end_time,
                    limit: rowsPerPage,
                    offset: page * rowsPerPage,
                };
                if (queryParams.provider !== 'all') filters.provider = queryParams.provider;
                if (queryParams.model !== 'all') filters.model = queryParams.model;
                if (queryParams.user !== 'all') filters.user_id = queryParams.user;
                // Status values are exactly 'success' | 'error' in the store,
                // so the server-side equality filter matches the toggle 1:1.
                if (statusFilter !== 'all') filters.status = statusFilter;
                const result = await api.getUsageRecords(filters);
                if (seq !== serverSeq.current) return;
                if (result?.data) {
                    setServerRows(result.data);
                    setServerTotal(result.meta?.total ?? result.data.length);
                }
            } catch (error) {
                console.error('Failed to load records page:', error);
            } finally {
                if (seq === serverSeq.current) {
                    setServerLoading(false);
                }
            }
        })();
    }, [sampleComplete, queryParams, statusFilter, page, rowsPerPage]);

    const filtered = statusFilter === 'all'
        ? records
        : records.filter(r => statusFilter === 'success' ? r.status === 'success' : r.status !== 'success');

    const paged = filtered.slice(page * rowsPerPage, (page + 1) * rowsPerPage);

    const tableRecords = sampleComplete ? paged : serverRows;
    const tableTotal = sampleComplete ? filtered.length : serverTotal;
    const tableLoading = sampleComplete ? loading && records.length === 0 : serverLoading;

    return (
        <Box>
            <RequestTable
                records={tableRecords}
                total={tableTotal}
                page={page}
                rowsPerPage={rowsPerPage}
                statusFilter={statusFilter}
                loading={tableLoading}
                onStatusFilterChange={s => { setStatusFilter(s); setPage(0); }}
                onPageChange={setPage}
                onRowsPerPageChange={r => { setRowsPerPage(r); setPage(0); }}
            />
        </Box>
    );
}
