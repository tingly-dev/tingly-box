import {
    Alert,
    Box,
    Button,
    Chip,
    Collapse,
    IconButton,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Typography,
    TableSortLabel,
} from '@mui/material';
import { Fragment, useEffect, useRef, useState } from 'react';
import { KeyboardArrowDown as KeyboardArrowDownIcon, KeyboardArrowUp as KeyboardArrowUpIcon, Refresh as RefreshIcon, ErrorOutline as ErrorOutlineIcon } from '@/components/icons';

export interface ModelRequestSummary {
    request_id: string;
    time: string;
    scenario?: string;
    request_model?: string;
    routed_model?: string;
    provider?: string;
    method?: string;
    path?: string;
    status?: number;
    latency_ms?: number;
    has_error: boolean;
    max_level?: string;
    event_count: number;
}

export interface ModelRequestEvent {
    time: string;
    source: string;
    level: string;
    stage?: string;
    message: string;
    fields?: Record<string, any>;
}

export interface ModelRequestDetail extends ModelRequestSummary {
    events: ModelRequestEvent[];
}

export interface RequestFilters {
    limit?: number;
    scenario?: string;
    provider?: string;
    status?: string;
}

type SortField = 'time' | 'scenario' | 'model' | 'provider' | 'status' | 'latency';
type SortOrder = 'asc' | 'desc';

interface RequestsViewerProps {
    getRequests: (params?: RequestFilters) => Promise<{ total: number; requests: ModelRequestSummary[] }>;
    getRequestDetail: (id: string) => Promise<ModelRequestDetail | null>;
    // When set, the scenario filter is initialized to this value but can be changed/cleared.
    // Used by the per-scenario quick-open dialog to provide context without locking the view.
    initialScenario?: string;
}

const statusColor = (status?: number): 'default' | 'success' | 'warning' | 'error' => {
    if (!status) return 'default';
    if (status >= 500) return 'error';
    if (status >= 400) return 'warning';
    if (status >= 200) return 'success';
    return 'default';
};

const sourceColor = (source: string): 'default' | 'primary' | 'secondary' | 'info' => {
    switch (source) {
        case 'http':
            return 'primary';
        case 'model_request':
            return 'secondary';
        case 'smart_routing':
            return 'info';
        default:
            return 'default';
    }
};

const levelColor = (level: string): 'default' | 'warning' | 'error' => {
    switch (level) {
        case 'error':
        case 'fatal':
        case 'panic':
            return 'error';
        case 'warning':
            return 'warning';
        default:
            return 'default';
    }
};

const formatTime = (s: string): string => {
    try {
        return new Date(s).toLocaleString();
    } catch {
        return s;
    }
};

const formatTimeShort = (s: string): string => {
    try {
        return new Date(s).toLocaleTimeString();
    } catch {
        return s;
    }
};

// Fields surfaced as the row summary / handled specially — hidden from the raw dump.
const SUPPRESSED_FIELDS = new Set([
    'request_id',
    'source',
    'stage',
    'type',
    'time',
    'level',
    'msg',
    'trace',
    'request',
]);

const AILogViewer = ({ getRequests, getRequestDetail, initialScenario }: RequestsViewerProps) => {
    const [requests, setRequests] = useState<ModelRequestSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [expandedId, setExpandedId] = useState<string | null>(null);
    const [details, setDetails] = useState<Record<string, ModelRequestDetail>>({});
    const [error, setError] = useState<string | null>(null);
    // Initialize scenario filter from initialScenario prop on mount
    const [scenario, setScenario] = useState(initialScenario ?? '');
    const [provider, setProvider] = useState('');
    const [status, setStatus] = useState('');
    const tableContainerRef = useRef<HTMLDivElement>(null);
    // Sorting state
    const [sortField, setSortField] = useState<SortField>('time');
    const [sortOrder, setSortOrder] = useState<SortOrder>('desc');

    // Initialize scenario from initialScenario when prop changes
    useEffect(() => {
        if (initialScenario !== undefined) {
            setScenario(initialScenario);
        }
    }, [initialScenario]);

    const loadRequests = async () => {
        setLoading(true);
        setError(null);
        try {
            const response = await getRequests({
                limit: 200,
                scenario: scenario || undefined,
                provider: provider || undefined,
                status: status || undefined,
            });
            if (response && response.requests) {
                const sorted = [...response.requests].sort((a, b) => {
                    let comparison = 0;
                    switch (sortField) {
                        case 'time':
                            comparison = new Date(a.time).getTime() - new Date(b.time).getTime();
                            break;
                        case 'scenario':
                            comparison = (a.scenario || '').localeCompare(b.scenario || '');
                            break;
                        case 'model':
                            comparison = (a.request_model || '').localeCompare(b.request_model || '');
                            break;
                        case 'provider':
                            comparison = (a.provider || '').localeCompare(b.provider || '');
                            break;
                        case 'status':
                            comparison = (a.status || 0) - (b.status || 0);
                            break;
                        case 'latency':
                            comparison = (a.latency_ms || 0) - (b.latency_ms || 0);
                            break;
                    }
                    return sortOrder === 'asc' ? comparison : -comparison;
                });
                setRequests(sorted);
            }
        } catch (e: any) {
            setError(e instanceof Error ? e.message : 'Failed to load requests');
        } finally {
            setLoading(false);
        }
    };

    // Reload whenever the filters change (and on mount).
    useEffect(() => {
        loadRequests();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [scenario, provider, status, sortField, sortOrder]);

    useEffect(() => {
        if (autoRefresh) {
            const id = setInterval(loadRequests, 5000);
            return () => clearInterval(id);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [autoRefresh, scenario, provider, status]);

    const handleSort = (field: SortField) => {
        if (sortField === field) {
            // Toggle between asc/desc
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            // New field, default to desc for time, asc for others
            setSortField(field);
            setSortOrder(field === 'time' ? 'desc' : 'asc');
        }
    };

    const toggleRow = async (id: string) => {
        if (expandedId === id) {
            setExpandedId(null);
            return;
        }
        setExpandedId(id);
        if (!details[id]) {
            try {
                const detail = await getRequestDetail(id);
                if (detail) {
                    setDetails((prev) => ({ ...prev, [id]: detail }));
                }
            } catch (e: any) {
                setError(e instanceof Error ? e.message : 'Failed to load request detail');
            }
        }
    };

    const renderRoutingTrace = (fields?: Record<string, any>) => {
        if (!fields) return null;
        const trace: any[] = Array.isArray(fields.trace) ? fields.trace : [];
        const matchedIdx = typeof fields.matched_rule_index === 'number' ? fields.matched_rule_index : -1;
        return (
            <Stack spacing={0.75} sx={{ mt: 0.5 }}>
                <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    {fields.outcome && (
                        <Chip size="small" label={`outcome: ${fields.outcome}`} sx={{ fontSize: '0.65rem', height: 18 }} />
                    )}
                    {fields.selected_provider && (
                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }}>
                            {fields.selected_provider} → {fields.selected_model}
                        </Typography>
                    )}
                </Stack>
                {trace.map((rule: any) => {
                    const isWinner = matchedIdx === rule.rule_index;
                    return (
                        <Box
                            key={rule.rule_index}
                            sx={{
                                p: 0.75,
                                borderRadius: 1,
                                border: 1,
                                borderColor: isWinner ? 'success.main' : 'divider',
                                backgroundColor: isWinner ? 'rgba(16,185,129,0.05)' : 'transparent',
                            }}
                        >
                            <Stack direction="row" spacing={1} alignItems="center">
                                <Chip size="small" label={`#${rule.rule_index}`} sx={{ fontSize: '0.65rem', height: 18 }} />
                                <Typography sx={{ fontSize: '0.75rem', flex: 1 }}>
                                    {rule.description || '(no description)'}
                                </Typography>
                                <Chip
                                    size="small"
                                    label={rule.matched ? 'MATCH' : 'SKIP'}
                                    color={rule.matched ? 'success' : 'default'}
                                    sx={{ fontSize: '0.6rem', height: 16 }}
                                />
                            </Stack>
                            {Array.isArray(rule.ops) &&
                                rule.ops.map((op: any, i: number) => (
                                    <Stack
                                        key={i}
                                        direction="row"
                                        spacing={1}
                                        sx={{
                                            pl: 1,
                                            mt: 0.25,
                                            borderLeft: 2,
                                            borderColor: op.matched ? 'success.main' : 'error.main',
                                        }}
                                    >
                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', minWidth: 120 }}>
                                            {op.position}.{op.operation}
                                        </Typography>
                                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary' }}>
                                            {op.reason}
                                        </Typography>
                                    </Stack>
                                ))}
                        </Box>
                    );
                })}
            </Stack>
        );
    };

    const renderFields = (fields?: Record<string, any>) => {
        if (!fields) return null;
        const keys = Object.keys(fields).filter((k) => !SUPPRESSED_FIELDS.has(k));
        if (keys.length === 0) return null;
        return (
            <Stack spacing={0.1} sx={{ mt: 0.25 }}>
                {keys.map((k) => (
                    <Typography key={k} sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary', wordBreak: 'break-all' }}>
                        {k}={typeof fields[k] === 'object' ? JSON.stringify(fields[k]) : String(fields[k])}
                    </Typography>
                ))}
            </Stack>
        );
    };

    const renderEvent = (ev: ModelRequestEvent, i: number) => (
        <Box
            key={i}
            sx={{
                p: 0.75,
                borderRadius: 1,
                backgroundColor: 'background.paper',
                border: 1,
                borderColor: 'divider',
            }}
        >
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary', minWidth: 90 }}>
                    {formatTimeShort(ev.time)}
                </Typography>
                <Chip size="small" label={ev.source} color={sourceColor(ev.source)} sx={{ fontSize: '0.6rem', height: 18 }} />
                {ev.level && ev.level !== 'info' && (
                    <Chip size="small" label={ev.level} color={levelColor(ev.level)} sx={{ fontSize: '0.6rem', height: 18 }} />
                )}
                {ev.stage && (
                    <Chip size="small" variant="outlined" label={ev.stage} sx={{ fontSize: '0.6rem', height: 18 }} />
                )}
                <Typography sx={{ fontSize: '0.75rem', wordBreak: 'break-word' }}>{ev.message}</Typography>
            </Stack>
            {ev.source === 'smart_routing' ? renderRoutingTrace(ev.fields) : renderFields(ev.fields)}
        </Box>
    );

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap sx={{ flexShrink: 0, minHeight: 40, py: 0.75 }}>
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
                        onClick={loadRequests}
                        disabled={loading}
                        startIcon={<RefreshIcon />}
                        sx={{ fontSize: '0.75rem' }}
                    >
                        Refresh
                    </Button>
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                        {requests.length} requests
                    </Typography>
                </Stack>
                <Stack direction="row" spacing={1} alignItems="center">
                    <TextField
                        size="small"
                        label="Scenario"
                        value={scenario}
                        onChange={(e) => setScenario(e.target.value.trim())}
                        sx={{ width: 130 }}
                    />
                    <TextField
                        size="small"
                        label="Provider"
                        value={provider}
                        onChange={(e) => setProvider(e.target.value.trim())}
                        sx={{ width: 130 }}
                    />
                    <TextField
                        size="small"
                        label="Status"
                        value={status}
                        onChange={(e) => setStatus(e.target.value.trim())}
                        sx={{ width: 90 }}
                    />
                    {(scenario || provider || status) && (
                        <Button
                            size="small"
                            variant="outlined"
                            onClick={() => { setScenario(''); setProvider(''); setStatus(''); }}
                            sx={{ fontSize: '0.7rem', py: 0.5, px: 1 }}
                        >
                            Clear
                        </Button>
                    )}
                </Stack>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" color="text.secondary">
                    One row per request. Expand for the full pipeline timeline (routing → conversion → upstream).
                </Typography>
            </Stack>

            {error && (
                <Alert severity="error" onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            <Box
                ref={tableContainerRef}
                sx={{ flex: 1, overflow: 'auto', minHeight: 0, backgroundColor: 'background.paper', borderRadius: 1, border: 1, borderColor: 'divider' }}
            >
                <TableContainer sx={{ maxHeight: 'none' }}>
                    <Table stickyHeader size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell padding="checkbox" />
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'time'}
                                        direction={sortField === 'time' ? sortOrder : 'desc'}
                                        onClick={() => handleSort('time')}
                                    >
                                        Time
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'scenario'}
                                        direction={sortField === 'scenario' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('scenario')}
                                    >
                                        Scenario
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'model'}
                                        direction={sortField === 'model' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('model')}
                                    >
                                        Model
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'provider'}
                                        direction={sortField === 'provider' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('provider')}
                                    >
                                        Provider
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'status'}
                                        direction={sortField === 'status' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('status')}
                                    >
                                        Status
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell>
                                    <TableSortLabel
                                        active={sortField === 'latency'}
                                        direction={sortField === 'latency' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('latency')}
                                    >
                                        Latency
                                    </TableSortLabel>
                                </TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {requests.length === 0 ? (
                                <TableRow>
                                    <TableCell colSpan={7} align="center" sx={{ py: 4 }}>
                                        <Typography color="text.secondary">
                                            {loading ? 'Loading...' : 'No model requests yet — send a request through the gateway.'}
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                requests.map((req) => {
                                    const expanded = expandedId === req.request_id;
                                    const detail = details[req.request_id];
                                    return (
                                        <Fragment key={req.request_id}>
                                            <TableRow
                                                hover
                                                sx={{ cursor: 'pointer' }}
                                                onClick={() => toggleRow(req.request_id)}
                                            >
                                                <TableCell padding="checkbox">
                                                    <IconButton size="small">
                                                        {expanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                                                    </IconButton>
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                                    <Stack direction="row" spacing={0.5} alignItems="center">
                                                        {req.has_error && <ErrorOutlineIcon sx={{ fontSize: 16, color: 'error.main' }} />}
                                                        <span>{formatTime(req.time)}</span>
                                                    </Stack>
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem' }}>
                                                    {req.scenario ? (
                                                        <Chip size="small" label={req.scenario} sx={{ fontSize: '0.65rem', height: 20 }} />
                                                    ) : (
                                                        '-'
                                                    )}
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', fontFamily: 'monospace' }}>
                                                    {req.request_model || '-'}
                                                    {req.routed_model && req.routed_model !== req.request_model && (
                                                        <Typography component="span" sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary' }}>
                                                            {' → '}{req.routed_model}
                                                        </Typography>
                                                    )}
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', fontFamily: 'monospace' }}>
                                                    {req.provider || '-'}
                                                </TableCell>
                                                <TableCell>
                                                    {req.status != null ? (
                                                        <Chip
                                                            size="small"
                                                            label={req.status}
                                                            color={statusColor(req.status)}
                                                            sx={{ fontSize: '0.65rem', height: 20, fontWeight: 'bold' }}
                                                        />
                                                    ) : (
                                                        '-'
                                                    )}
                                                </TableCell>
                                                <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                                    {req.latency_ms != null ? `${req.latency_ms} ms` : '-'}
                                                </TableCell>
                                            </TableRow>
                                            <TableRow>
                                                <TableCell colSpan={7} sx={{ pb: 0, pt: 0, border: 'none' }}>
                                                    <Collapse in={expanded} timeout="auto" unmountOnExit>
                                                        <Box sx={{ p: 1.5, backgroundColor: 'rgba(0,0,0,0.02)' }}>
                                                            <Typography variant="caption" sx={{ fontWeight: 'bold', textTransform: 'uppercase', color: 'text.secondary' }}>
                                                                Timeline ({detail?.events.length ?? req.event_count} events) · {req.request_id}
                                                            </Typography>
                                                            <Stack spacing={0.75} sx={{ mt: 0.75 }}>
                                                                {detail ? (
                                                                    detail.events.length > 0 ? (
                                                                        detail.events.map(renderEvent)
                                                                    ) : (
                                                                        <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                                                                            No events recorded.
                                                                        </Typography>
                                                                    )
                                                                ) : (
                                                                    <Typography variant="body2" color="text.secondary">
                                                                        Loading timeline...
                                                                    </Typography>
                                                                )}
                                                            </Stack>
                                                        </Box>
                                                    </Collapse>
                                                </TableCell>
                                            </TableRow>
                                        </Fragment>
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

export default AILogViewer;
