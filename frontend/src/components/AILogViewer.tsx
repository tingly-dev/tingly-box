import {
    Alert,
    Box,
    Chip,
    Collapse,
    FormControlLabel,
    IconButton,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    TableSortLabel,
    Switch,
    Tooltip,
} from '@mui/material';
import { Fragment, useEffect, useRef, useState } from 'react';
import { KeyboardArrowDown as KeyboardArrowDownIcon, KeyboardArrowUp as KeyboardArrowUpIcon, Refresh as RefreshIcon, ErrorOutline as ErrorOutlineIcon } from '@/components/icons';
import RequestJourney, { type TraceDetail } from '@/components/RequestJourney';

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
    trace_id?: string;
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
    // Fetches one trace from the in-memory span buffer; null when evicted.
    getTrace: (traceId: string) => Promise<TraceDetail | null>;
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

const formatTime = (s: string): string => {
    try {
        return new Date(s).toLocaleString();
    } catch {
        return s;
    }
};

const AILogViewer = ({ getRequests, getRequestDetail, getTrace, initialScenario }: RequestsViewerProps) => {
    const [requests, setRequests] = useState<ModelRequestSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [expandedId, setExpandedId] = useState<string | null>(null);
    const [details, setDetails] = useState<Record<string, ModelRequestDetail>>({});
    const [error, setError] = useState<string | null>(null);
    // Always start unfiltered so the user sees the full picture.
    // initialScenario is kept as a prop only to power the quick-filter chip.
    const [scenario, setScenario] = useState('');
    const tableContainerRef = useRef<HTMLDivElement>(null);
    // Sorting state
    const [sortField, setSortField] = useState<SortField>('time');
    const [sortOrder, setSortOrder] = useState<SortOrder>('desc');

    const loadRequests = async () => {
        setLoading(true);
        setError(null);
        try {
            const response = await getRequests({
                limit: 200,
                scenario: scenario || undefined,
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
    }, [scenario, sortField, sortOrder]);

    useEffect(() => {
        if (autoRefresh) {
            const id = setInterval(loadRequests, 5000);
            return () => clearInterval(id);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [autoRefresh, scenario]);

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

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            <Stack
                direction="row"
                spacing={1.5}
                useFlexGap
                sx={{
                    alignItems: "center",
                    flexWrap: "wrap",
                    flexShrink: 0,
                    minHeight: 40,
                    py: 0.75
                }}>
                <Stack direction="row" spacing={1} sx={{
                    alignItems: "center"
                }}>
                    <FormControlLabel
                        control={(
                            <Switch
                                size="small"
                                checked={autoRefresh}
                                onChange={(_, checked) => setAutoRefresh(checked)}
                            />
                        )}
                        label={<Typography variant="body2">Auto-refresh</Typography>}
                        sx={{ m: 0, mr: 0.25 }}
                    />
                    <Tooltip title="Refresh now" arrow>
                        <span>
                            <IconButton
                                size="small"
                                aria-label="Refresh requests"
                                onClick={loadRequests}
                                disabled={loading}
                            >
                                <RefreshIcon fontSize="small" />
                            </IconButton>
                        </span>
                    </Tooltip>
                    <Typography
                        variant="caption"
                        sx={{
                            color: "text.secondary",
                            whiteSpace: 'nowrap'
                        }}>
                        {loading ? 'Refreshing…' : `${requests.length} requests`}
                    </Typography>
                </Stack>
                <Stack direction="row" spacing={1} sx={{
                    alignItems: "center"
                }}>
                    {/* Quick-filter chip: one click to apply/remove the context scenario */}
                    {initialScenario && (
                        <Chip
                            label={`filter: ${initialScenario}`}
                            size="small"
                            color={scenario === initialScenario ? 'primary' : 'default'}
                            variant={scenario === initialScenario ? 'filled' : 'outlined'}
                            onClick={() => setScenario(prev => prev === initialScenario ? '' : initialScenario)}
                            onDelete={scenario === initialScenario ? () => setScenario('') : undefined}
                            sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }}
                        />
                    )}
                </Stack>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" sx={{
                    color: "text.secondary"
                }}>
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
                {/* Log rows are timestamps, model ids, URLs and stack traces —
                    machine text, so the table stays LTR under an RTL locale
                    (see index.css). */}
                <TableContainer data-ltr sx={{ maxHeight: 'none' }}>
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
                                        <Typography sx={{
                                            color: "text.secondary"
                                        }}>
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
                                                    <Stack direction="row" spacing={0.5} sx={{
                                                        alignItems: "center"
                                                    }}>
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
                                                            {detail ? (
                                                                <RequestJourney
                                                                    events={detail.events}
                                                                    traceId={detail.trace_id}
                                                                    getTrace={getTrace}
                                                                />
                                                            ) : (
                                                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                                                    Loading journey...
                                                                </Typography>
                                                            )}
                                                            <Stack
                                                                direction="row"
                                                                spacing={1.5}
                                                                sx={{ mt: 1, pt: 0.75, borderTop: 1, borderColor: 'divider', flexWrap: 'wrap' }}
                                                            >
                                                                {/* Correlation ids live at the bottom: needed when filing a
                                                                    bug or grepping server logs, never when reading the journey. */}
                                                                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.65rem', color: 'text.disabled' }}>
                                                                    request {req.request_id}
                                                                </Typography>
                                                                {detail?.trace_id && (
                                                                    <Typography sx={{ fontFamily: 'monospace', fontSize: '0.65rem', color: 'text.disabled' }}>
                                                                        trace {detail.trace_id}
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
