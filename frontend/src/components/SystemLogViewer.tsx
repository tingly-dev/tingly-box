import {
    Box,
    Button,
    Chip,
    Paper,
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
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import RefreshIcon from '@mui/icons-material/Refresh';

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

const LOG_LEVELS = ['debug', 'info', 'warn', 'error', 'fatal', 'panic'];

const SystemLogViewer = ({ getLogs }: SystemLogViewerProps) => {
    const [logs, setLogs] = useState<SystemLogEntry[]>([]);
    const [allLogs, setAllLogs] = useState<SystemLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    // Multi-select level filter: empty set = show all
    const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [autoRefresh, setAutoRefresh] = useState(false);
    const tableContainerRef = useRef<HTMLDivElement>(null);

    const loadLogs = async () => {
        setLoading(true);
        try {
            const response = await getLogs({ limit: 200 });
            if (response && response.logs) {
                const sortedLogs = [...response.logs].sort((a, b) =>
                    new Date(a.time).getTime() - new Date(b.time).getTime()
                );
                setAllLogs(sortedLogs);
            }
        } catch (error) {
            console.error('Failed to load system logs:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleRow = (index: number) => {
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

    const getLevelColor = (level: string): string => {
        switch (level.toLowerCase()) {
            case 'panic':   return '#991b1b';
            case 'fatal':   return '#dc2626';
            case 'error':   return '#ef4444';
            case 'warning':
            case 'warn':    return '#f59e0b';
            case 'info':    return '#3b82f6';
            case 'debug':   return '#6b7280';
            default:        return '#10b981';
        }
    };

    const getStatusCodeColor = (statusCode?: number): string => {
        if (!statusCode) return '#6b7280';
        if (statusCode >= 200 && statusCode < 300) return '#10b981';
        if (statusCode >= 300 && statusCode < 400) return '#3b82f6';
        if (statusCode >= 400 && statusCode < 500) return '#f59e0b';
        if (statusCode >= 500) return '#ef4444';
        return '#6b7280';
    };

    const formatTimestamp = (timestamp: string): string => {
        try {
            return new Date(timestamp).toLocaleString();
        } catch {
            return timestamp;
        }
    };

    // Client-side filter
    useEffect(() => {
        if (selectedLevels.size === 0) {
            setLogs(allLogs);
        } else {
            setLogs(allLogs.filter(log => {
                const l = log.level?.toLowerCase() ?? '';
                // match "warn" tag against "warn" or "warning"
                return [...selectedLevels].some(sel =>
                    l === sel || (sel === 'warn' && l === 'warning')
                );
            }));
        }
    }, [selectedLevels, allLogs]);

    useEffect(() => {
        loadLogs();
    }, []);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(loadLogs, 5000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh]);

    // Scroll to bottom when logs update
    useEffect(() => {
        if (tableContainerRef.current && logs.length > 0) {
            tableContainerRef.current.scrollTop = tableContainerRef.current.scrollHeight;
        }
    }, [logs]);

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            {/* Toolbar */}
            <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap>
                {/* Actions */}
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
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                        {logs.length}{allLogs.length !== logs.length ? ` / ${allLogs.length}` : ''}
                    </Typography>
                </Stack>

                <Box sx={{ flex: 1 }} />

                {/* Level filter tags */}
                <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
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
                                    fontSize: '0.7rem',
                                    height: 24,
                                    '&:hover': {
                                        backgroundColor: active
                                            ? getLevelColor(level)
                                            : `${getLevelColor(level)}22`,
                                        borderColor: getLevelColor(level),
                                        color: active ? 'white' : getLevelColor(level),
                                    },
                                }}
                            />
                        );
                    })}
                    {selectedLevels.size > 0 && (
                        <Chip
                            label="Clear"
                            size="small"
                            clickable
                            onClick={() => setSelectedLevels(new Set())}
                            sx={{ fontSize: '0.7rem', height: 24 }}
                        />
                    )}
                </Stack>
            </Stack>

            {/* Logs Table — fills remaining space */}
            <TableContainer
                component={Paper}
                ref={tableContainerRef}
                sx={{ flex: 1, overflow: 'auto', minHeight: 0 }}
            >
                <Table stickyHeader size="small">
                    <TableHead>
                        <TableRow>
                            <TableCell padding="checkbox" />
                            <TableCell sx={{ width: 180 }}>Time</TableCell>
                            <TableCell sx={{ width: 90 }}>Level</TableCell>
                            <TableCell sx={{ width: 80 }}>Status</TableCell>
                            <TableCell>Message</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {logs.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={5} align="center" sx={{ py: 4 }}>
                                    <Typography color="text.secondary">
                                        {loading ? 'Loading...' : 'No logs available'}
                                    </Typography>
                                </TableCell>
                            </TableRow>
                        ) : (
                            logs.map((log, index) => (
                                <>
                                    <TableRow
                                        key={index}
                                        hover
                                        sx={{ cursor: 'pointer' }}
                                        onClick={() => toggleRow(index)}
                                    >
                                        <TableCell padding="checkbox">
                                            <IconButton size="small">
                                                {expandedRows.has(index) ? (
                                                    <KeyboardArrowUpIcon />
                                                ) : (
                                                    <KeyboardArrowDownIcon />
                                                )}
                                            </IconButton>
                                        </TableCell>
                                        <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                                            {formatTimestamp(log.time)}
                                        </TableCell>
                                        <TableCell>
                                            <Chip
                                                label={log.level.toUpperCase()}
                                                size="small"
                                                sx={{
                                                    backgroundColor: getLevelColor(log.level),
                                                    color: 'white',
                                                    fontSize: '0.7rem',
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
                                                        fontSize: '0.7rem',
                                                        height: 20,
                                                        fontWeight: 'bold',
                                                    }}
                                                />
                                            ) : (
                                                <Typography sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>-</Typography>
                                            )}
                                        </TableCell>
                                        <TableCell sx={{ fontSize: '0.8rem' }}>
                                            {log.message}
                                        </TableCell>
                                    </TableRow>
                                    <TableRow key={`${index}-expanded`}>
                                        <TableCell colSpan={5} sx={{ pb: 0, pt: 0, border: 'none' }}>
                                            <Collapse in={expandedRows.has(index)} timeout="auto" unmountOnExit>
                                                <Box sx={{ p: 2, backgroundColor: 'rgba(0,0,0,0.03)' }}>
                                                    {log.fields && Object.keys(log.fields).length > 0 ? (
                                                        <Stack spacing={1}>
                                                            {log.fields.error && (
                                                                <Box sx={{ p: 1, backgroundColor: 'error.dark', borderRadius: 1 }}>
                                                                    <Typography
                                                                        variant="body2"
                                                                        sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'error.contrastText', fontWeight: 'bold', wordBreak: 'break-all' }}
                                                                    >
                                                                        ERROR: {typeof log.fields.error === 'object' && log.fields.error !== null ? JSON.stringify(log.fields.error) : String(log.fields.error)}
                                                                    </Typography>
                                                                    {log.fields.error_type && (
                                                                        <Typography
                                                                            variant="caption"
                                                                            sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'error.contrastText', opacity: 0.8 }}
                                                                        >
                                                                            Type: {typeof log.fields.error_type === 'object' && log.fields.error_type !== null ? JSON.stringify(log.fields.error_type) : String(log.fields.error_type)}
                                                                        </Typography>
                                                                    )}
                                                                </Box>
                                                            )}
                                                            {Object.entries(log.fields)
                                                                .filter(([key]) => key !== 'error' && key !== 'error_type')
                                                                .map(([key, value]) => (
                                                                    <Typography key={key} variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                                                        <strong>{key}:</strong> {typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)}
                                                                    </Typography>
                                                                ))}
                                                        </Stack>
                                                    ) : (
                                                        <Typography variant="body2" color="text.secondary">
                                                            No additional fields
                                                        </Typography>
                                                    )}
                                                </Box>
                                            </Collapse>
                                        </TableCell>
                                    </TableRow>
                                </>
                            ))
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
        </Stack>
    );
};

export default SystemLogViewer;
