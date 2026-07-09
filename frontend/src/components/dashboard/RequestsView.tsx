import { useState } from 'react';
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
import {
    PieChart,
    Pie,
    Cell,
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip as ReTooltip,
    ResponsiveContainer,
} from 'recharts';
import { WaveSine as StreamIcon } from '@/components/icons';
import { getThemeChartStyles, TOKEN_COLORS, formatNumber } from './chartStyles';

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
    cache_input_tokens: number;
    status: string;
    error_code?: string;
    latency_ms: number;
    ttft_ms?: number;
    streamed: boolean;
}

// ─── Constants ────────────────────────────────────────────────────────────────

const SUCCESS_COLOR = '#10B981';
const ERROR_COLOR  = '#EF4444';

const LATENCY_BUCKETS = [
    { label: '<0.2s',   min: 0,    max: 200  },
    { label: '0.2-0.5', min: 200,  max: 500  },
    { label: '0.5-1s',  min: 500,  max: 1000 },
    { label: '1-2s',    min: 1000, max: 2000 },
    { label: '2-5s',    min: 2000, max: 5000 },
    { label: '>5s',     min: 5000, max: Infinity },
];

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmtTokens = formatNumber;

const fmtLatency = (ms: number) => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
    return `${ms}ms`;
};

const fmtTime = (ts: string) =>
    new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

// ─── Token Donut ─────────────────────────────────────────────────────────────

function DonutTooltip({ active, payload }: any) {
    const theme = useTheme();
    if (!active || !payload?.length) return null;
    const d = payload[0].payload;
    return (
        <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 1.5, p: 1.25, boxShadow: theme.shadows[4] }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: d.color, flexShrink: 0 }} />
                <Typography sx={{ fontSize: '0.75rem', fontWeight: 600 }}>{d.name}</Typography>
            </Box>
            <Typography sx={{ fontSize: '0.72rem', color: 'text.secondary', mt: 0.25 }}>
                {fmtTokens(d.value)} ({d.pct}%)
            </Typography>
        </Box>
    );
}

function TokenDonut({ records }: { records: UsageRecord[] }) {
    const totalInput  = records.reduce((s, r) => s + r.input_tokens, 0);
    const totalOutput = records.reduce((s, r) => s + r.output_tokens, 0);
    const totalCache  = records.reduce((s, r) => s + r.cache_input_tokens, 0);
    const total = totalInput + totalOutput + totalCache;

    const pieData = [
        { name: 'Input',  value: totalInput,  color: TOKEN_COLORS.input.main,  pct: total > 0 ? ((totalInput  / total) * 100).toFixed(1) : '0' },
        { name: 'Output', value: totalOutput, color: TOKEN_COLORS.output.main, pct: total > 0 ? ((totalOutput / total) * 100).toFixed(1) : '0' },
        { name: 'Cache',  value: totalCache,  color: TOKEN_COLORS.cache.main,  pct: total > 0 ? ((totalCache  / total) * 100).toFixed(1) : '0' },
    ].filter(d => d.value > 0);

    return (
        <Paper elevation={0} sx={{ p: 2.5, border: '1px solid', borderColor: 'divider', borderRadius: 2, backgroundColor: 'background.paper', boxShadow: 'none' }}>
            <Typography sx={{ fontWeight: 600, fontSize: '0.875rem', mb: 2 }}>Token Breakdown</Typography>
            {total === 0 ? (
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 140 }}>
                    <Typography variant="caption" color="text.disabled">No data</Typography>
                </Box>
            ) : (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2.5 }}>
                    {/* Donut */}
                    <Box sx={{ position: 'relative', flexShrink: 0, width: 140, height: 140 }}>
                        <PieChart width={140} height={140}>
                            <Pie data={pieData} cx={70} cy={70} innerRadius={42} outerRadius={62} dataKey="value" paddingAngle={2} isAnimationActive={false}>
                                {pieData.map((entry, i) => <Cell key={i} fill={entry.color} />)}
                            </Pie>
                            <ReTooltip content={<DonutTooltip />} />
                        </PieChart>
                        {/* Center label */}
                        <Box sx={{ position: 'absolute', top: '50%', left: '50%', transform: 'translate(-50%, -50%)', textAlign: 'center', pointerEvents: 'none' }}>
                            <Typography sx={{ fontSize: '0.82rem', fontWeight: 700, lineHeight: 1.2 }}>
                                {fmtTokens(total)}
                            </Typography>
                            <Typography sx={{ fontSize: '0.58rem', color: 'text.secondary', lineHeight: 1.2 }}>
                                total
                            </Typography>
                        </Box>
                    </Box>

                    {/* Legend */}
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.25, minWidth: 0 }}>
                        {pieData.map(d => (
                            <Box key={d.name} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Box sx={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: d.color, flexShrink: 0 }} />
                                <Box sx={{ minWidth: 0 }}>
                                    <Typography sx={{ fontSize: '0.7rem', color: 'text.secondary', lineHeight: 1.2 }}>{d.name}</Typography>
                                    <Typography sx={{ fontSize: '0.78rem', fontWeight: 600, lineHeight: 1.3 }}>
                                        {fmtTokens(d.value)}
                                        <Typography component="span" sx={{ fontSize: '0.65rem', color: 'text.secondary', ml: 0.5 }}>
                                            {d.pct}%
                                        </Typography>
                                    </Typography>
                                </Box>
                            </Box>
                        ))}
                        <Box sx={{ mt: 0.5, pt: 0.75, borderTop: '1px solid', borderColor: 'divider' }}>
                            <Typography sx={{ fontSize: '0.68rem', color: 'text.secondary' }}>
                                {records.length} requests
                            </Typography>
                        </Box>
                    </Box>
                </Box>
            )}
        </Paper>
    );
}

// ─── Latency Histogram ────────────────────────────────────────────────────────

function HistoTooltip({ active, payload, label }: any) {
    const theme = useTheme();
    if (!active || !payload?.length) return null;
    const success = payload.find((p: any) => p.dataKey === 'success')?.value ?? 0;
    const error   = payload.find((p: any) => p.dataKey === 'error')?.value   ?? 0;
    return (
        <Box sx={{ backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 1.5, p: 1.25, boxShadow: theme.shadows[4] }}>
            <Typography sx={{ fontSize: '0.75rem', fontWeight: 600, mb: 0.5 }}>{label}</Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25 }}>
                <Typography sx={{ fontSize: '0.7rem', color: SUCCESS_COLOR }}>Success: {success}</Typography>
                {error > 0 && <Typography sx={{ fontSize: '0.7rem', color: ERROR_COLOR }}>Error: {error}</Typography>}
                <Typography sx={{ fontSize: '0.7rem', color: 'text.secondary', fontWeight: 600 }}>Total: {success + error}</Typography>
            </Box>
        </Box>
    );
}

function LatencyHistogram({ records }: { records: UsageRecord[] }) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);
    const [metric, setMetric] = useState<'latency' | 'ttft'>('latency');

    const getValue = (r: UsageRecord) => metric === 'latency' ? r.latency_ms : (r.ttft_ms ?? 0);
    // TTFT is only meaningful for streamed requests
    const pool = metric === 'ttft' ? records.filter(r => r.streamed && (r.ttft_ms ?? 0) > 0) : records;

    const histData = LATENCY_BUCKETS.map(b => ({
        label: b.label,
        success: pool.filter(r => getValue(r) >= b.min && getValue(r) < b.max && r.status === 'success').length,
        error:   pool.filter(r => getValue(r) >= b.min && getValue(r) < b.max && r.status !== 'success').length,
    }));

    const hasData = pool.length > 0;

    return (
        <Paper elevation={0} sx={{ p: 2.5, border: '1px solid', borderColor: 'divider', borderRadius: 2, backgroundColor: 'background.paper', boxShadow: 'none' }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                <Typography sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    {metric === 'latency' ? 'Latency' : 'TTFT'} Distribution
                </Typography>
                <ToggleButtonGroup
                    value={metric} exclusive size="small"
                    onChange={(_, v) => v && setMetric(v)}
                    sx={{ '& .MuiToggleButton-root': { px: 1.25, py: 0.25, fontSize: '0.7rem', textTransform: 'none' } }}
                >
                    <ToggleButton value="latency">Latency</ToggleButton>
                    <ToggleButton value="ttft">TTFT</ToggleButton>
                </ToggleButtonGroup>
            </Box>
            {!hasData ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 140, gap: 0.5 }}>
                    <Typography variant="caption" color="text.disabled">
                        {metric === 'ttft' ? 'No TTFT data — backend field not yet exposed' : 'No data'}
                    </Typography>
                </Box>
            ) : (
                <ResponsiveContainer width="100%" height={140}>
                    <BarChart data={histData} margin={{ top: 0, right: 4, bottom: 0, left: -20 }} barCategoryGap="20%">
                        <CartesianGrid strokeDasharray="3 3" stroke={chartStyles.chart.grid} strokeOpacity={0.6} vertical={false} />
                        <XAxis
                            dataKey="label"
                            tick={{ fontSize: 10, fill: theme.palette.text.secondary }}
                            tickLine={false}
                            axisLine={{ stroke: chartStyles.chart.axis }}
                        />
                        <YAxis
                            tick={{ fontSize: 10, fill: theme.palette.text.secondary }}
                            tickLine={false}
                            axisLine={false}
                            allowDecimals={false}
                        />
                        <ReTooltip content={<HistoTooltip />} cursor={{ fill: theme.palette.action.hover }} />
                        <Bar dataKey="success" stackId="a" fill={SUCCESS_COLOR} fillOpacity={0.75} isAnimationActive={false} radius={[0, 0, 0, 0]} />
                        <Bar dataKey="error"   stackId="a" fill={ERROR_COLOR}   fillOpacity={0.85} isAnimationActive={false} radius={[3, 3, 0, 0]} />
                    </BarChart>
                </ResponsiveContainer>
            )}
        </Paper>
    );
}

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
                            <TableCell align="right" sx={{ minWidth: 96 }}>Cache</TableCell>
                            <TableCell align="right">Input</TableCell>
                            <TableCell align="right">Output</TableCell>
                            <TableCell align="right">Latency</TableCell>
                            <TableCell align="right">TTFT</TableCell>
                            <TableCell align="center">Status</TableCell>
                            <TableCell align="center">Stream</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {records.length === 0 && !loading ? (
                            <TableRow>
                                <TableCell colSpan={10} align="center" sx={{ py: 5 }}>
                                    <Typography variant="body2" color="text.secondary">No requests found</Typography>
                                    <Typography variant="caption" color="text.disabled">Try changing the status filter</Typography>
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
                                        const cacheTokens = r.cache_input_tokens || 0;
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

interface RequestsViewProps {
    records: UsageRecord[];
    loading: boolean;
}

export default function RequestsView({ records, loading }: RequestsViewProps) {
    const [statusFilter, setStatusFilter] = useState<'all' | 'success' | 'error'>('all');
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(50);

    const filtered = statusFilter === 'all'
        ? records
        : records.filter(r => statusFilter === 'success' ? r.status === 'success' : r.status !== 'success');

    const paged = filtered.slice(page * rowsPerPage, (page + 1) * rowsPerPage);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {/* Charts row */}
            <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2 }}>
                <TokenDonut records={records} />
                <LatencyHistogram records={records} />
            </Box>

            {/* Table */}
            <RequestTable
                records={paged}
                total={filtered.length}
                page={page}
                rowsPerPage={rowsPerPage}
                statusFilter={statusFilter}
                loading={loading && records.length === 0}
                onStatusFilterChange={s => { setStatusFilter(s); setPage(0); }}
                onPageChange={setPage}
                onRowsPerPageChange={r => { setRowsPerPage(r); setPage(0); }}
            />
        </Box>
    );
}
