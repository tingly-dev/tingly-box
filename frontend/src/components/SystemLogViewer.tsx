import {
    Alert,
    Box,
    Chip,
    FormControlLabel,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    IconButton,
    Collapse,
    TableSortLabel,
    Tooltip,
    TextField,
    MenuItem,
} from '@mui/material';
import { Fragment, useMemo, useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardArrowDown as KeyboardArrowDownIcon } from '@/components/icons';
import { KeyboardArrowUp as KeyboardArrowUpIcon } from '@/components/icons';
import { Refresh as RefreshIcon } from '@/components/icons';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { useTableSort } from '@/hooks/useTableSort';
import { formatLogTime } from '@/utils/datetime';
import { fontMono, fontSizes } from '@/theme/fonts';
import { alpha, useTheme } from '@mui/material/styles';
import { getStatusColor, logLevelTone, httpStatusTone } from '@/theme/status';

export interface SystemLogEntry {
    time: string;
    level: string;
    message: string;
    fields?: Record<string, any>;
}

export interface SystemLogsResponse {
    total: number;
    logs: SystemLogEntry[];
}

interface SystemLogViewerProps {
    getLogs: (params?: { limit?: number; level?: string; since?: string }) => Promise<SystemLogsResponse>;
}

type SortField = 'time' | 'level' | 'status' | 'message';

const LOG_LEVELS = ['debug', 'info', 'warn', 'error', 'fatal', 'panic'];

const SystemLogViewer = ({ getLogs }: SystemLogViewerProps) => {
    const { t } = useTranslation();
    const [allLogs, setAllLogs] = useState<SystemLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    // Multi-select level filter: empty set = show all
    const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
    const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
    const [search, setSearch] = useState('');
    const [limit, setLimit] = useState(200);
    // Off by default: a log only refetches when asked (switch or refresh button).
    const [autoRefresh, setAutoRefresh] = useState(false);
    const tableContainerRef = useRef<HTMLDivElement>(null);
    const listSequence = useRef(0);
    // Sorting is client-side only — never triggers a refetch.
    const { sortField, sortOrder, handleSort } = useTableSort<SortField>('time');

    const loadLogs = async () => {
        const sequence = ++listSequence.current;
        setLoading(true);
        setError(null);
        try {
            const response = await getLogs({ limit });
            if (sequence !== listSequence.current) return;
            if (response && response.logs) {
                setAllLogs(response.logs);
            }
        } catch (error) {
            if (sequence === listSequence.current) setError(error instanceof Error ? error.message : 'Failed to load system logs');
        } finally {
            if (sequence === listSequence.current) setLoading(false);
        }
    };

    const toggleRow = (index: string) => {
        const newExpanded = new Set(expandedRows);
        if (newExpanded.has(index)) {
            newExpanded.delete(index);
        } else {
            newExpanded.add(index);
        }
        setExpandedRows(newExpanded);
    };

    const toggleLevel = (level: string) => {
        const next = new Set(selectedLevels);
        if (next.has(level)) {
            next.delete(level);
        } else {
            next.add(level);
        }
        setSelectedLevels(next);
    };

    const theme = useTheme();
    const getLevelColor = (level: string): string => getStatusColor(theme, logLevelTone(level));
    const getStatusCodeColor = (statusCode?: number): string => getStatusColor(theme, httpStatusTone(statusCode));

    const logs = useMemo(() => {
        const query = search.trim().toLowerCase();
        return allLogs.filter(log => {
            const level = log.level?.toLowerCase() ?? '';
            return (!query || JSON.stringify(log).toLowerCase().includes(query)) &&
                (selectedLevels.size === 0 || selectedLevels.has(level === 'warning' ? 'warn' : level));
        });
    }, [allLogs, selectedLevels, search]);

    // Fetch only on mount / refresh — sorting is derived client-side below.
    useEffect(() => {
        loadLogs();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [limit]);

    useAutoRefresh(autoRefresh, loadLogs, [limit]);

    // Client-side sort — re-sorts the already-fetched rows instead of
    // refetching from the network on every sort change.
    const sortedLogs = useMemo(() => {
        return [...logs].sort((a, b) => {
            let comparison = 0;
            switch (sortField) {
                case 'time':
                    comparison = new Date(a.time).getTime() - new Date(b.time).getTime();
                    break;
                case 'level':
                    comparison = a.level.localeCompare(b.level);
                    break;
                case 'message':
                    comparison = a.message.localeCompare(b.message);
                    break;
                case 'status':
                    const statusA = (a.fields?.status as number) ?? 0;
                    const statusB = (b.fields?.status as number) ?? 0;
                    comparison = statusA - statusB;
                    break;
            }
            return sortOrder === 'asc' ? comparison : -comparison;
        });
    }, [logs, sortField, sortOrder]);

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            {/* Filters on the left, liveness on the right: the same toolbar as the requests log. */}
            <Stack direction="row" spacing={1} useFlexGap sx={{ alignItems: 'center', flexWrap: 'wrap', flexShrink: 0 }}>
                <TextField
                    size="small"
                    placeholder={t('logViewer.searchLogs', { defaultValue: 'Search logs' })}
                    value={search}
                    onChange={event => setSearch(event.target.value)}
                    slotProps={{ htmlInput: { 'aria-label': t('logViewer.searchLogs', { defaultValue: 'Search logs' }) } }}
                    sx={{ width: 240 }}
                />
                <TextField
                        select
                        size="small"
                        value={limit}
                        onChange={event => setLimit(Number(event.target.value))}
                        slotProps={{ htmlInput: { 'aria-label': t('logViewer.load', { defaultValue: 'Load' }) } }}
                    >
                        {[200, 500, 1000].map(value => <MenuItem key={value} value={value}>{t('logViewer.latest', { count: value, defaultValue: 'Latest {{count}}' })}</MenuItem>)}
                </TextField>
                <Stack
                    direction="row"
                    spacing={0.5}
                    useFlexGap
                    sx={{
                        alignItems: 'center',
                        flexWrap: 'wrap',
                        minHeight: 30,
                        alignContent: 'center',
                    }}
                >
                    <Chip
                        label={t('logViewer.allLevels', { defaultValue: 'All' })}
                        size="small"
                        clickable
                        color={selectedLevels.size === 0 ? 'primary' : 'default'}
                        variant={selectedLevels.size === 0 ? 'filled' : 'outlined'}
                        onClick={() => setSelectedLevels(new Set())}
                        sx={{
                            fontWeight: 600,
                            fontSize: fontSizes.xs,
                            height: 24,
                        }}
                    />
                    {LOG_LEVELS.map((level) => {
                        const active = selectedLevels.has(level);
                        return (
                            <Chip
                                key={level}
                                label={level.toUpperCase()}
                                size="small"
                                clickable
                                onClick={() => toggleLevel(level)}
                                sx={{
                                    backgroundColor: active ? getLevelColor(level) : 'transparent',
                                    color: active ? 'white' : 'text.secondary',
                                    border: active ? `1px solid ${getLevelColor(level)}` : '1px solid',
                                    borderColor: active ? getLevelColor(level) : 'divider',
                                    fontWeight: 'bold',
                                    fontSize: fontSizes.xs,
                                    height: 24,
                                    '&:hover': {
                                        backgroundColor: active
                                            ? getLevelColor(level)
                                            : alpha(getLevelColor(level), 0.13),
                                        borderColor: getLevelColor(level),
                                        color: active ? 'white' : getLevelColor(level),
                                    },
                                }}
                            />
                        );
                    })}
                </Stack>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" sx={{ color: 'text.secondary', whiteSpace: 'nowrap' }}>
                    {loading
                        ? t('logViewer.refreshing', { defaultValue: 'Refreshing…' })
                        : selectedLevels.size > 0 || search.trim()
                            ? t('logViewer.entriesFiltered', { shown: logs.length, total: allLogs.length, defaultValue: '{{shown}} of {{total}} entries' })
                            : t('logViewer.entries', { count: allLogs.length, defaultValue: '{{count}} entries' })}
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
                            aria-label={t('logViewer.refreshLogs', { defaultValue: 'Refresh logs' })}
                            onClick={loadLogs}
                            disabled={loading}
                        >
                            <RefreshIcon fontSize="small" />
                        </IconButton>
                    </span>
                </Tooltip>
            </Stack>
            {/* Logs Table — fills remaining space */}
            {error && <Alert severity="error">{error}</Alert>}
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
                                <TableCell sx={{ width: 180 }}>
                                    <TableSortLabel
                                        active={sortField === 'time'}
                                        direction={sortField === 'time' ? sortOrder : 'desc'}
                                        onClick={() => handleSort('time')}
                                    >
                                        Time
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell sx={{ width: 90 }}>
                                    <TableSortLabel
                                        active={sortField === 'level'}
                                        direction={sortField === 'level' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('level')}
                                    >
                                        Level
                                    </TableSortLabel>
                                </TableCell>
                                <TableCell sx={{ width: 80 }}>
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
                                        active={sortField === 'message'}
                                        direction={sortField === 'message' ? sortOrder : 'asc'}
                                        onClick={() => handleSort('message')}
                                    >
                                        Message
                                    </TableSortLabel>
                                </TableCell>
                            </TableRow>
                        </TableHead>
                    <TableBody>
                        {sortedLogs.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                                    <Typography sx={{
                                        color: "text.secondary"
                                    }}>
                                        {loading ? 'Loading...' : 'No logs available'}
                                    </Typography>
                                </TableCell>
                            </TableRow>
                        ) : (
                            sortedLogs.map((log) => {
                                const key = JSON.stringify(log);
                                return (
                                <Fragment key={key}>
                                    <TableRow
                                        hover
                                        sx={{ cursor: 'pointer' }}
                                        onClick={() => toggleRow(key)}
                                    >
                                        <TableCell padding="checkbox">
                                            <IconButton size="small">
                                                {expandedRows.has(key) ? (
                                                    <KeyboardArrowUpIcon />
                                                ) : (
                                                    <KeyboardArrowDownIcon />
                                                )}
                                            </IconButton>
                                        </TableCell>
                                        <TableCell sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>
                                            {formatLogTime(log.time)}
                                        </TableCell>
                                        <TableCell>
                                            <Chip
                                                label={log.level.toUpperCase()}
                                                size="small"
                                                sx={{
                                                    backgroundColor: getLevelColor(log.level),
                                                    color: 'white',
                                                    fontSize: fontSizes.xs,
                                                    height: 20,
                                                    fontWeight: 'bold',
                                                }}
                                            />
                                        </TableCell>
                                        <TableCell>
                                            {log.fields?.status !== undefined ? (
                                                <Chip
                                                    label={log.fields.status as number}
                                                    size="small"
                                                    sx={{
                                                        backgroundColor: getStatusCodeColor(log.fields.status as number),
                                                        color: 'white',
                                                        fontSize: fontSizes.xs,
                                                        height: 20,
                                                        fontWeight: 'bold',
                                                    }}
                                                />
                                            ) : (
                                                <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>-</Typography>
                                            )}
                                        </TableCell>
                                        <TableCell sx={{ fontSize: fontSizes.md }}>
                                            {log.message}
                                        </TableCell>
                                    </TableRow>
                                    <TableRow>
                                        <TableCell colSpan={5} sx={{ pb: 0, pt: 0, border: 'none' }}>
                                            <Collapse in={expandedRows.has(key)} timeout="auto" unmountOnExit>
                                                <Box sx={{ p: 2, backgroundColor: 'rgba(0,0,0,0.03)' }}>
                                                    <Typography component="pre" sx={{ m: 0, mb: 1, fontFamily: fontMono, fontSize: fontSizes.sm, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{log.message}</Typography>
                                                    {log.fields && Object.keys(log.fields).length > 0 ? (
                                                        <Stack spacing={1}>
                                                            {log.fields.error && (
                                                                <Box sx={{ p: 1, backgroundColor: 'error.dark', borderRadius: 1 }}>
                                                                    <Typography
                                                                        variant="body2"
                                                                        sx={{ fontFamily: fontMono, fontSize: fontSizes.sm, color: 'error.contrastText', fontWeight: 'bold', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                                                                    >
                                                                        ERROR: {typeof log.fields.error === 'object' && log.fields.error !== null ? JSON.stringify(log.fields.error) : String(log.fields.error)}
                                                                    </Typography>
                                                                    {log.fields.error_type && (
                                                                        <Typography
                                                                            variant="caption"
                                                                            sx={{ fontFamily: fontMono, fontSize: fontSizes.xs, color: 'error.contrastText', opacity: 0.8 }}
                                                                        >
                                                                            Type: {typeof log.fields.error_type === 'object' && log.fields.error_type !== null ? JSON.stringify(log.fields.error_type) : String(log.fields.error_type)}
                                                                        </Typography>
                                                                    )}
                                                                </Box>
                                                            )}
                                                            {Object.entries(log.fields)
                                                                .filter(([key]) => key !== 'error' && key !== 'error_type')
                                                                .map(([key, value]) => (
                                                                    <Box key={key}>
                                                                        <Typography variant="body2" sx={{ fontFamily: fontMono, fontSize: fontSizes.sm, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                                                                            <strong>{key}:</strong> {typeof value === 'object' && value !== null ? JSON.stringify(value, null, 2) : String(value)}
                                                                        </Typography>
                                                                    </Box>
                                                                ))}
                                                        </Stack>
                                                    ) : (
                                                        <Typography variant="body2" sx={{
                                                            color: "text.secondary"
                                                        }}>
                                                            No additional fields
                                                        </Typography>
                                                    )}
                                                </Box>
                                            </Collapse>
                                        </TableCell>
                                    </TableRow>
                                </Fragment>
                            ); })
                        )}
                    </TableBody>
                </Table>
                </TableContainer>
            </Box>
        </Stack>
    );
};

export default SystemLogViewer;
