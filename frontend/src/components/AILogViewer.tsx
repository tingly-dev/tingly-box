import {
    Alert,
    Box,
    Chip,
    Collapse,
    Fade,
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
    TextField,
    MenuItem,
} from '@mui/material';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardArrowDown as KeyboardArrowDownIcon, KeyboardArrowUp as KeyboardArrowUpIcon, Refresh as RefreshIcon } from '@/components/icons';
// Straight from runProbe, not the probe barrel (which pulls in the probe dialogs).
import { formatLatency } from '@/components/probe/runProbe';
import RequestJourney, { type TraceDetail } from '@/components/RequestJourney';
import { holdNew, reconcile, sameRow } from '@/components/logHold';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { useTableSort } from '@/hooks/useTableSort';
import { formatLogTime } from '@/utils/datetime';
import { fontMono, fontSizes } from '@/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

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

interface RequestsViewerProps {
    getRequests: (params?: RequestFilters) => Promise<{ total: number; requests: ModelRequestSummary[] }>;
    getRequestDetail: (id: string) => Promise<ModelRequestDetail | null>;
    // Fetches one trace from the in-memory span buffer; null when evicted.
    getTrace: (traceId: string) => Promise<TraceDetail | null>;
    // When set, only this agent's requests are listed (the per-agent Log dialog):
    // the agent column and the load-size picker go, the rest stays the same.
    scenario?: string;
}

// Scrolled farther than this from the top counts as reading, not watching the live edge.
const HOLD_SCROLL_PX = 24;

const statusColor = (status?: number): 'default' | 'success' | 'warning' | 'error' => {
    if (!status) return 'default';
    if (status >= 500) return 'error';
    if (status >= 400) return 'warning';
    if (status >= 200) return 'success';
    return 'default';
};


interface RequestRowProps {
    req: ModelRequestSummary;
    expanded: boolean;
    current?: { id: string; detail?: ModelRequestDetail; error?: string };
    showScenario: boolean;
    columnCount: number;
    getTrace: RequestsViewerProps['getTrace'];
    onToggle: (id: string) => void;
}

// Memoized: a refresh re-renders only the rows whose data (or open state)
// actually changed, not the whole list.
const RequestRow = memo(({ req, expanded, current, showScenario, columnCount, getTrace, onToggle }: RequestRowProps) => {
    const { t } = useTranslation();
    const detail = current?.detail;
    return (
        <>
            <TableRow
                hover
                sx={{ cursor: 'pointer' }}
                onClick={() => onToggle(req.request_id)}
            >
                <TableCell padding="checkbox">
                    <IconButton size="small">
                        {expanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
                    </IconButton>
                </TableCell>
                <TableCell sx={{ fontSize: fontSizes.sm, color: 'text.secondary', whiteSpace: 'nowrap' }} title={req.time}>
                    {formatLogTime(req.time)}
                </TableCell>
                {showScenario && (
                    <TableCell sx={{ fontSize: fontSizes.sm }}>
                        {req.scenario ? (
                            <Chip size="small" label={req.scenario} sx={{ fontSize: fontSizes.micro, height: 20 }} />
                        ) : (
                            '-'
                        )}
                    </TableCell>
                )}
                <TableCell sx={{ fontSize: fontSizes.sm, fontFamily: fontMono }}>
                    {req.request_model || '-'}
                    {req.routed_model && req.routed_model !== req.request_model && (
                        <Typography component="span" sx={{ fontFamily: fontMono, fontSize: fontSizes.micro, color: 'text.secondary' }}>
                            {' → '}{req.routed_model}
                        </Typography>
                    )}
                </TableCell>
                <TableCell sx={{ fontSize: fontSizes.sm }}>
                    {req.provider || '-'}
                </TableCell>
                <TableCell>
                    {/* Colour only means trouble: a clean 2xx is plain text; a request that
                        failed after a 2xx (has_error, e.g. a broken stream) still shows red. */}
                    {req.status == null && !req.has_error ? '-' : req.has_error || (req.status ?? 0) >= 400 ? (
                        <Chip
                            size="small"
                            label={req.status ?? t('logViewer.error', { defaultValue: 'Error' })}
                            color={req.status != null && req.status >= 400 ? statusColor(req.status) : 'error'}
                            sx={{ fontSize: fontSizes.micro, height: 20, fontWeight: 600 }}
                        />
                    ) : (
                        <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{req.status}</Typography>
                    )}
                </TableCell>
                <TableCell align="right" sx={{ fontSize: fontSizes.sm, whiteSpace: 'nowrap' }}>
                    {req.latency_ms != null ? formatLatency(req.latency_ms) : '-'}
                </TableCell>
            </TableRow>
            <TableRow>
                <TableCell colSpan={columnCount} sx={{ pb: 0, pt: 0, border: 'none' }}>
                    <Collapse in={expanded} timeout="auto" unmountOnExit>
                        <Box sx={{ p: 1.5, backgroundColor: 'action.hover' }}>
                            {current?.error && <Alert severity="warning">{current?.error}</Alert>}
                            {detail ? (
                                <RequestJourney
                                    events={detail.events}
                                    traceId={detail.trace_id}
                                    getTrace={getTrace}
                                />
                            ) : (
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {current?.error
                                        ? t('logViewer.retryHint', { defaultValue: 'Use refresh to retry.' })
                                        : t('logViewer.loadingJourney', { defaultValue: 'Loading journey...' })}
                                </Typography>
                            )}
                            <Stack
                                direction="row"
                                spacing={1.5}
                                sx={{ mt: 1, pt: 0.75, borderTop: 1, borderColor: 'divider', flexWrap: 'wrap' }}
                            >
                                {/* Correlation ids live at the bottom: needed when filing a
                                    bug or grepping server logs, never when reading the journey. */}
                                <Typography sx={{ fontFamily: fontMono, fontSize: fontSizes.micro, color: 'text.disabled' }}>
                                    request {req.request_id}
                                </Typography>
                                {detail?.trace_id && (
                                    <Typography sx={{ fontFamily: fontMono, fontSize: fontSizes.micro, color: 'text.disabled' }}>
                                        trace {detail.trace_id}
                                    </Typography>
                                )}
                            </Stack>
                        </Box>
                    </Collapse>
                </TableCell>
            </TableRow>
        </>
    );
});
RequestRow.displayName = 'RequestRow';

const AILogViewer = ({ getRequests, getRequestDetail, getTrace, scenario }: RequestsViewerProps) => {
    const { t } = useTranslation();
    const [requests, setRequests] = useState<ModelRequestSummary[]>([]);
    const [total, setTotal] = useState(0);
    const [limit, setLimit] = useState(200);
    const [search, setSearch] = useState('');
    const [errorsOnly, setErrorsOnly] = useState(false);
    const [loading, setLoading] = useState(false);
    const [pending, setPending] = useState(0);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [expandedId, setExpandedId] = useState<string | null>(null);
    const [detailState, setDetailState] = useState<{ id: string; detail?: ModelRequestDetail; error?: string }>();
    const [detailVersion, setDetailVersion] = useState(0);
    const [error, setError] = useState<string | null>(null);
    const tableContainerRef = useRef<HTMLDivElement>(null);
    const listSequence = useRef(0);
    // What the reader sees vs what the server last sent: while they are reading
    // an open request or are scrolled away from the top, new requests are held
    // back (counted in the pill) so nothing moves under them.
    const shownRef = useRef<ModelRequestSummary[]>([]);
    const latestRef = useRef<ModelRequestSummary[]>([]);
    const expandedIdRef = useRef<string | null>(null);
    // Sorting is client-side only — never triggers a refetch.
    const { sortField, sortOrder, handleSort } = useTableSort<SortField>('time');

    const isReading = () => expandedIdRef.current != null || (tableContainerRef.current?.scrollTop ?? 0) > HOLD_SCROLL_PX;

    const showList = (list: ModelRequestSummary[], held: number) => {
        shownRef.current = list;
        setRequests(list);
        setPending(held);
    };

    // `silent` is the background poll: no "Refreshing…" flash. `flush` (filter
    // changes) replaces the list outright instead of holding new rows back.
    const loadRequests = async ({ silent = false, flush = false } = {}) => {
        const sequence = ++listSequence.current;
        if (!silent) setLoading(true);
        setError(null);
        try {
            const response = await getRequests({
                limit,
                scenario: scenario || undefined,
            });
            if (sequence !== listSequence.current) return;
            if (response && response.requests) {
                const latest = response.requests;
                latestRef.current = latest;
                const open = expandedIdRef.current;
                const before = open ? shownRef.current.find(r => r.request_id === open) : undefined;
                if (flush || !isReading()) {
                    showList(reconcile(shownRef.current, latest), 0);
                } else {
                    const held = holdNew(shownRef.current, latest);
                    showList(held.shown, held.pending);
                }
                setTotal(response.total);
                // A background poll re-reads the open request only when its own row changed.
                const after = open ? latest.find(r => r.request_id === open) : undefined;
                if (!silent || (before && after && !sameRow(before, after))) setDetailVersion(version => version + 1);
            } else if (!silent) {
                setDetailVersion(version => version + 1);
            }
        } catch (e: any) {
            if (sequence === listSequence.current) setError(e instanceof Error ? e.message : t('logViewer.loadFailed', { defaultValue: 'Failed to load requests' }));
        } finally {
            if (sequence === listSequence.current && !silent) setLoading(false);
        }
    };

    // Reload whenever the filters change (and on mount).
    useEffect(() => {
        loadRequests({ flush: true });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [scenario, limit]);

    useAutoRefresh(autoRefresh, () => loadRequests({ silent: true }), [scenario, limit]);

    const showHeld = useCallback((scrollToTop: boolean) => {
        showList(reconcile(shownRef.current, latestRef.current), 0);
        if (scrollToTop && tableContainerRef.current) tableContainerRef.current.scrollTop = 0;
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Closing the open request, or scrolling back to the top, ends the hold.
    useEffect(() => {
        if (pending > 0 && !isReading()) showHeld(false);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [expandedId]);
    const handleScroll = () => {
        if (pending > 0 && !isReading()) showHeld(false);
    };

    const toggleRow = useCallback((id: string) => {
        const next = expandedIdRef.current === id ? null : id;
        expandedIdRef.current = next;
        setExpandedId(next);
    }, []);

    // Client-side sort — re-sorts the already-fetched rows instead of
    // refetching from the network on every sort change.
    const sortedRequests = useMemo(() => {
        const query = search.trim().toLowerCase();
        return requests.filter(request => (!errorsOnly || request.has_error) &&
            (!query || JSON.stringify(request).toLowerCase().includes(query))).sort((a, b) => {
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
    }, [requests, sortField, sortOrder, search, errorsOnly]);

    // Only the open request owns detail state. Cleanup rejects stale responses.
    useEffect(() => {
        if (!expandedId) return;
        let cancelled = false;
        getRequestDetail(expandedId).then(detail => {
            // An unchanged journey keeps its object, so it is not re-rendered or re-fetched for traces.
            if (!cancelled) setDetailState(prev => prev?.id === expandedId && detail && prev.detail && JSON.stringify(prev.detail) === JSON.stringify(detail) ? prev
                : { id: expandedId, detail: detail ?? undefined,
                    error: detail ? undefined : t('logViewer.detailGone', { defaultValue: 'Request details are no longer available in retained history.' }) });
        }).catch((error: unknown) => {
            if (!cancelled) setDetailState({ id: expandedId, error: error instanceof Error ? error.message : t('logViewer.detailFailed', { defaultValue: 'Failed to load request detail' }) });
        });
        return () => { cancelled = true; };
    }, [expandedId, detailVersion, getRequestDetail, t]);

    const filtered = errorsOnly || search.trim() !== '';
    const columns: Array<{ field: SortField; label: string; align?: 'right' }> = [
        { field: 'time', label: t('logViewer.columns.time', { defaultValue: 'Time' }) },
        ...(scenario ? [] : [{ field: 'scenario' as const, label: t('logViewer.columns.agent', { defaultValue: 'Agent' }) }]),
        { field: 'model', label: t('logViewer.columns.model', { defaultValue: 'Model' }) },
        { field: 'provider', label: t('logViewer.columns.provider', { defaultValue: 'Provider' }) },
        { field: 'status', label: t('logViewer.columns.status', { defaultValue: 'Status' }) },
        { field: 'latency', label: t('logViewer.columns.latency', { defaultValue: 'Latency' }), align: 'right' },
    ];
    const columnCount = columns.length + 1;

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            {/* Filters on the left, liveness on the right: the same toolbar as the system log. */}
            <Stack direction="row" spacing={1} useFlexGap sx={{ alignItems: 'center', flexWrap: 'wrap', flexShrink: 0 }}>
                <TextField
                    size="small"
                    placeholder={t('logViewer.search', { defaultValue: 'Search requests' })}
                    value={search}
                    onChange={event => setSearch(event.target.value)}
                    slotProps={{ htmlInput: { 'aria-label': t('logViewer.search', { defaultValue: 'Search requests' }) } }}
                    sx={{ width: 240 }}
                />
                <Chip
                    label={t('logViewer.errorsOnly', { defaultValue: 'Errors only' })}
                    color={errorsOnly ? 'error' : 'default'}
                    variant={errorsOnly ? 'filled' : 'outlined'}
                    onClick={() => setErrorsOnly(value => !value)}
                />
                {!scenario && (
                    <TextField
                            select
                            size="small"
                            value={limit}
                            onChange={event => setLimit(Number(event.target.value))}
                            slotProps={{ htmlInput: { 'aria-label': t('logViewer.load', { defaultValue: 'Load' }) } }}
                        >
                            {[200, 500, 1000].map(value => <MenuItem key={value} value={value}>{t('logViewer.latest', { count: value, defaultValue: 'Latest {{count}}' })}</MenuItem>)}
                    </TextField>
                )}
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" sx={{ color: 'text.secondary', whiteSpace: 'nowrap' }}>
                    {loading
                        ? t('logViewer.refreshing', { defaultValue: 'Refreshing…' })
                        : filtered
                            ? t('logViewer.countFiltered', { shown: sortedRequests.length, total, defaultValue: '{{shown}} of {{total}} requests' })
                            : t('logViewer.count', { count: total, defaultValue: '{{count}} requests' })}
                </Typography>
                <FormControlLabel
                    control={<Switch size="small" checked={autoRefresh} onChange={(_, checked) => setAutoRefresh(checked)} />}
                    label={<Typography variant="body2">{t('logViewer.autoRefresh', { defaultValue: 'Auto-refresh' })}</Typography>}
                    sx={{ m: 0 }}
                />
                <Tooltip title={t('logViewer.refreshNow', { defaultValue: 'Refresh now' })} arrow>
                    <span>
                        <IconButton
                            size="small"
                            aria-label={t('logViewer.refreshRequests', { defaultValue: 'Refresh requests' })}
                            onClick={() => loadRequests()}
                            disabled={loading}
                        >
                            <RefreshIcon fontSize="small" />
                        </IconButton>
                    </span>
                </Tooltip>
            </Stack>
            {error && (
                <Alert severity="error" onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}
            <Box sx={{ position: 'relative', flex: 1, minHeight: 0, display: 'flex' }}>
            {pending > 0 && (
                <Fade in appear>
                    <Chip
                        clickable
                        color="primary"
                        size="small"
                        icon={<KeyboardArrowUpIcon />}
                        label={t('logViewer.newRequests', { count: pending, defaultValue: '{{count}} new requests' })}
                        onClick={() => showHeld(true)}
                        sx={{ position: 'absolute', top: 44, left: '50%', transform: 'translateX(-50%)', zIndex: 3, boxShadow: 2 }}
                    />
                </Fade>
            )}
            <Box
                ref={tableContainerRef}
                onScroll={handleScroll}
                sx={{ flex: 1, overflow: 'auto', minHeight: 0, backgroundColor: 'background.paper', borderRadius: radius.row, border: 1, borderColor: 'divider' }}
            >
                <TableContainer sx={{ maxHeight: 'none' }}>
                    <Table stickyHeader size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell padding="checkbox" />
                                {columns.map(({ field, label, align }) => (
                                    <TableCell key={field} align={align}>
                                        <TableSortLabel
                                            active={sortField === field}
                                            direction={sortField === field ? sortOrder : field === 'time' ? 'desc' : 'asc'}
                                            onClick={() => handleSort(field)}
                                        >
                                            {label}
                                        </TableSortLabel>
                                    </TableCell>
                                ))}
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {sortedRequests.length === 0 ? (
                                <TableRow>
                                    <TableCell colSpan={columnCount} align="center" sx={{ py: 4 }}>
                                        <Typography sx={{
                                            color: "text.secondary"
                                        }}>
                                            {loading
                                                ? t('logViewer.loading', { defaultValue: 'Loading…' })
                                                : requests.length > 0
                                                    ? t('logViewer.noMatch', { defaultValue: 'No requests match these filters.' })
                                                    : scenario
                                                        ? t('logViewer.emptyAgent', { defaultValue: 'No requests from this agent yet. Once it sends one, it shows up here.' })
                                                        : t('logViewer.empty', { defaultValue: 'No model requests in recent retained history.' })}
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                sortedRequests.map((req) => (
                                    <RequestRow
                                        key={req.request_id}
                                        req={req}
                                        expanded={expandedId === req.request_id}
                                        current={detailState?.id === req.request_id ? detailState : undefined}
                                        showScenario={!scenario}
                                        columnCount={columnCount}
                                        getTrace={getTrace}
                                        onToggle={toggleRow}
                                    />
                                ))
                            )}
                        </TableBody>
                    </Table>
                </TableContainer>
            </Box>
            </Box>
        </Stack>
    );
};

export default AILogViewer;
