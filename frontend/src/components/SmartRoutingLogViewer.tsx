import {
    Box,
    Button,
    Chip,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    IconButton,
    Collapse,
    Alert,
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import CancelIcon from '@mui/icons-material/Cancel';
import RefreshIcon from '@mui/icons-material/Refresh';
import DeleteSweepIcon from '@mui/icons-material/DeleteSweep';

interface OpEvalResult {
    uuid?: string;
    position: string;
    operation: string;
    value?: string;
    actual?: string;
    matched: boolean;
    reason?: string;
}

interface RuleEvalResult {
    rule_index: number;
    description?: string;
    matched: boolean;
    ops_evaluated: number;
    ops_total: number;
    ops?: OpEvalResult[];
}

interface RequestSnapshot {
    model?: string;
    thinking_enabled?: boolean;
    latest_role?: string;
    latest_type?: string;
    estimated_tokens?: number;
    tool_uses?: string[];
    latest_user_head?: string;
    system_msg_count?: number;
    user_msg_count?: number;
}

interface SmartRoutingLogFields {
    rule_uuid?: string;
    request_model?: string;
    matched?: boolean;
    matched_rule_index?: number;
    matched_rule_description?: string;
    matched_services?: number;
    final_active_count?: number;
    rules_total?: number;
    selected_provider?: string;
    selected_model?: string;
    outcome?: string;
    reason?: string;
    client_ip?: string;
    request_id?: string;
    trace?: RuleEvalResult[];
    request?: RequestSnapshot;
    [k: string]: unknown;
}

interface SmartRoutingLogEntry {
    time: string;
    level: string;
    message: string;
    fields?: SmartRoutingLogFields;
}

interface SmartRoutingLogViewerProps {
    getLogs: (params?: { limit?: number }) => Promise<{ total: number; logs: SmartRoutingLogEntry[] }>;
    clearLogs?: () => Promise<void>;
}

const outcomeColor = (outcome?: string): 'default' | 'success' | 'warning' | 'error' => {
    switch (outcome) {
        case 'selected':
            return 'success';
        case 'no_match':
        case 'no_candidates':
        case 'no_active_services':
            return 'warning';
        case 'lb_failed':
        case 'router_invalid':
        case 'extract_failed':
        case 'no_context':
            return 'error';
        default:
            return 'default';
    }
};

const SmartRoutingLogViewer = ({ getLogs, clearLogs }: SmartRoutingLogViewerProps) => {
    const [logs, setLogs] = useState<SmartRoutingLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(false);
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [error, setError] = useState<string | null>(null);
    const tableContainerRef = useRef<HTMLDivElement>(null);

    const loadLogs = async () => {
        setLoading(true);
        setError(null);
        try {
            const response = await getLogs({ limit: 200 });
            if (response && response.logs) {
                // sort oldest -> newest so the latest sits at the bottom
                const sorted = [...response.logs].sort(
                    (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime(),
                );
                setLogs(sorted);
            }
        } catch (e: any) {
            setError(e instanceof Error ? e.message : 'Failed to load smart routing logs');
        } finally {
            setLoading(false);
        }
    };

    const handleClear = async () => {
        if (!clearLogs) return;
        try {
            await clearLogs();
            await loadLogs();
        } catch (e: any) {
            setError(e instanceof Error ? e.message : 'Failed to clear logs');
        }
    };

    useEffect(() => {
        loadLogs();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useEffect(() => {
        if (autoRefresh) {
            const id = setInterval(loadLogs, 5000);
            return () => clearInterval(id);
        }
    }, [autoRefresh]);

    useEffect(() => {
        if (!tableContainerRef.current || logs.length === 0) return;
        const el = tableContainerRef.current;
        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
                el.scrollTop = el.scrollHeight;
            });
        });
    }, [logs]);

    const toggleRow = (i: number) => {
        const next = new Set(expandedRows);
        if (next.has(i)) next.delete(i);
        else next.add(i);
        setExpandedRows(next);
    };

    const formatTime = (s: string): string => {
        try {
            return new Date(s).toLocaleString();
        } catch {
            return s;
        }
    };

    const renderOp = (op: OpEvalResult, i: number) => (
        <Stack
            key={i}
            direction="row"
            spacing={1}
            alignItems="center"
            sx={{
                py: 0.5,
                px: 1,
                borderLeft: 3,
                borderColor: op.matched ? 'success.main' : 'error.main',
                backgroundColor: op.matched ? 'rgba(16,185,129,0.08)' : 'rgba(239,68,68,0.08)',
                borderRadius: 0.5,
                mb: 0.25,
            }}
        >
            {op.matched ? (
                <CheckCircleIcon sx={{ fontSize: 16, color: 'success.main' }} />
            ) : (
                <CancelIcon sx={{ fontSize: 16, color: 'error.main' }} />
            )}
            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', minWidth: 130 }}>
                <strong>{op.position}</strong>.{op.operation}
            </Typography>
            {op.value !== undefined && op.value !== '' && (
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary' }}>
                    = "{op.value}"
                </Typography>
            )}
            <Box sx={{ flex: 1 }} />
            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary', wordBreak: 'break-word' }}>
                {op.reason}
            </Typography>
        </Stack>
    );

    const renderRule = (rule: RuleEvalResult, matchedIdx?: number) => {
        const isMatchedWinner = matchedIdx !== undefined && matchedIdx === rule.rule_index;
        return (
            <Box
                key={rule.rule_index}
                sx={{
                    p: 1,
                    borderRadius: 1,
                    border: 1,
                    borderColor: isMatchedWinner ? 'success.main' : 'divider',
                    backgroundColor: isMatchedWinner ? 'rgba(16,185,129,0.05)' : 'transparent',
                    mb: 1,
                }}
            >
                <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
                    <Chip
                        size="small"
                        label={`Rule #${rule.rule_index}`}
                        sx={{ fontSize: '0.7rem', height: 20, fontWeight: 'bold' }}
                    />
                    <Typography sx={{ fontWeight: 'bold', fontSize: '0.8rem', flex: 1 }}>
                        {rule.description || '(no description)'}
                    </Typography>
                    <Chip
                        size="small"
                        label={rule.matched ? 'MATCH' : 'SKIP'}
                        color={rule.matched ? 'success' : 'default'}
                        sx={{ fontSize: '0.65rem', height: 18, fontWeight: 'bold' }}
                    />
                    <Typography sx={{ fontSize: '0.7rem', color: 'text.secondary' }}>
                        {rule.ops_evaluated}/{rule.ops_total} ops
                    </Typography>
                </Stack>
                {rule.ops && rule.ops.map(renderOp)}
            </Box>
        );
    };

    const renderRequestSnapshot = (req?: RequestSnapshot) => {
        if (!req) return null;
        const fieldRow = (label: string, value: any) => (
            <Stack direction="row" spacing={1} key={label}>
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary', minWidth: 140 }}>
                    {label}
                </Typography>
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem', wordBreak: 'break-all' }}>
                    {value === undefined || value === null
                        ? '-'
                        : Array.isArray(value)
                            ? value.length === 0 ? '[]' : value.join(', ')
                            : typeof value === 'object'
                                ? JSON.stringify(value)
                                : String(value)}
                </Typography>
            </Stack>
        );
        return (
            <Stack spacing={0.25} sx={{ p: 1, backgroundColor: 'rgba(0,0,0,0.03)', borderRadius: 1 }}>
                {fieldRow('model', req.model)}
                {fieldRow('thinking_enabled', req.thinking_enabled)}
                {fieldRow('estimated_tokens', req.estimated_tokens)}
                {fieldRow('latest_role', req.latest_role)}
                {fieldRow('latest_type', req.latest_type)}
                {fieldRow('tool_uses', req.tool_uses)}
                {fieldRow('system_msgs', req.system_msg_count)}
                {fieldRow('user_msgs', req.user_msg_count)}
                {req.latest_user_head && fieldRow('latest_user', req.latest_user_head)}
            </Stack>
        );
    };

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap sx={{ flexShrink: 0 }}>
                <Stack direction="row" spacing={1} alignItems="center">
                    <Button
                        variant={autoRefresh ? 'contained' : 'outlined'}
                        size="small"
                        onClick={() => setAutoRefresh(!autoRefresh)}
                        sx={{ fontSize: '0.75rem' }}
                    >
                        Auto
                    </Button>
                    <Button
                        variant="outlined"
                        size="small"
                        onClick={loadLogs}
                        disabled={loading}
                        startIcon={<RefreshIcon />}
                        sx={{ fontSize: '0.75rem' }}
                    >
                        Refresh
                    </Button>
                    {clearLogs && (
                        <Button
                            variant="outlined"
                            size="small"
                            color="warning"
                            onClick={handleClear}
                            startIcon={<DeleteSweepIcon />}
                            sx={{ fontSize: '0.75rem' }}
                        >
                            Clear
                        </Button>
                    )}
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                        {logs.length} entries
                    </Typography>
                </Stack>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" color="text.secondary">
                    Each row is one routing decision. Expand to see per-op evaluation and the request fields.
                </Typography>
            </Stack>

            {error && (
                <Alert severity="error" onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            <Box
                ref={tableContainerRef}
                sx={{
                    flex: 1,
                    overflow: 'auto',
                    minHeight: 0,
                    backgroundColor: 'background.paper',
                    borderRadius: 1,
                    border: 1,
                    borderColor: 'divider',
                }}
            >
                <TableContainer sx={{ maxHeight: 'none' }}>
                    <Table stickyHeader size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell padding="checkbox" />
                                <TableCell sx={{ width: 180 }}>Time</TableCell>
                                <TableCell sx={{ width: 110 }}>Outcome</TableCell>
                                <TableCell sx={{ width: 180 }}>Model</TableCell>
                                <TableCell sx={{ width: 90 }}>Rule</TableCell>
                                <TableCell>Selected → Provider/Model</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {logs.length === 0 ? (
                                <TableRow>
                                    <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                                        <Typography color="text.secondary">
                                            {loading ? 'Loading...' : 'No smart routing logs yet — send a request to a smart-routed rule.'}
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                logs.map((log, index) => {
                                    const f = log.fields || {};
                                    const matchedIdx = typeof f.matched_rule_index === 'number' ? f.matched_rule_index : -1;
                                    return (
                                        <>
                                            <TableRow
                                                key={index}
                                                hover
                                                sx={{ cursor: 'pointer' }}
                                                onClick={() => toggleRow(index)}
                                            >
                                                <TableCell padding="checkbox">
                                                    <IconButton size="small">
                                                        {expandedRows.has(index) ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                                                    </IconButton>
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                                    {formatTime(log.time)}
                                                </TableCell>
                                                <TableCell>
                                                    <Chip
                                                        size="small"
                                                        label={(f.outcome || '-').toUpperCase()}
                                                        color={outcomeColor(f.outcome)}
                                                        sx={{ fontSize: '0.65rem', height: 20, fontWeight: 'bold' }}
                                                    />
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', fontFamily: 'monospace' }}>
                                                    {f.request_model || f.request?.model || '-'}
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem' }}>
                                                    {matchedIdx >= 0 ? (
                                                        <Chip
                                                            size="small"
                                                            label={`#${matchedIdx}`}
                                                            sx={{ fontSize: '0.65rem', height: 20, fontWeight: 'bold' }}
                                                        />
                                                    ) : (
                                                        <Typography sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>-</Typography>
                                                    )}
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem' }}>
                                                    {f.selected_provider ? (
                                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                                            {f.selected_provider} → {f.selected_model}
                                                        </Typography>
                                                    ) : (
                                                        <Typography sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                                            {f.reason || log.message}
                                                        </Typography>
                                                    )}
                                                </TableCell>
                                            </TableRow>
                                            <TableRow key={`${index}-expanded`}>
                                                <TableCell colSpan={6} sx={{ pb: 0, pt: 0, border: 'none' }}>
                                                    <Collapse in={expandedRows.has(index)} timeout="auto" unmountOnExit>
                                                        <Box sx={{ p: 1.5, backgroundColor: 'rgba(0,0,0,0.02)' }}>
                                                            <Stack spacing={1.5}>
                                                                <Box>
                                                                    <Typography variant="caption" sx={{ fontWeight: 'bold', textTransform: 'uppercase', color: 'text.secondary' }}>
                                                                        Request snapshot
                                                                    </Typography>
                                                                    {renderRequestSnapshot(f.request)}
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" sx={{ fontWeight: 'bold', textTransform: 'uppercase', color: 'text.secondary' }}>
                                                                        Rule evaluation ({f.rules_total ?? f.trace?.length ?? 0} rules)
                                                                    </Typography>
                                                                    {f.trace && f.trace.length > 0 ? (
                                                                        f.trace.map((rule) => renderRule(rule, matchedIdx))
                                                                    ) : (
                                                                        <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                                                                            No rule trace recorded (e.g. context extraction failed before evaluation).
                                                                        </Typography>
                                                                    )}
                                                                </Box>
                                                                {(f.matched_services !== undefined || f.final_active_count !== undefined) && (
                                                                    <Box>
                                                                        <Typography variant="caption" sx={{ fontWeight: 'bold', textTransform: 'uppercase', color: 'text.secondary' }}>
                                                                            Selection
                                                                        </Typography>
                                                                        <Stack direction="row" spacing={2}>
                                                                            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }}>
                                                                                matched_services: {f.matched_services ?? '-'}
                                                                            </Typography>
                                                                            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }}>
                                                                                active_after_filter: {f.final_active_count ?? '-'}
                                                                            </Typography>
                                                                            {f.client_ip && (
                                                                                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }}>
                                                                                    client_ip: {f.client_ip}
                                                                                </Typography>
                                                                            )}
                                                                        </Stack>
                                                                    </Box>
                                                                )}
                                                            </Stack>
                                                        </Box>
                                                    </Collapse>
                                                </TableCell>
                                            </TableRow>
                                        </>
                                    );
                                })
                            )}
                        </TableBody>
                    </Table>
                </TableContainer>
            </Box>
        </Stack>
    );
};

export default SmartRoutingLogViewer;
