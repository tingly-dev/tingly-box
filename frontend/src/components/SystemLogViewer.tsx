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
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    TextField,
    Alert,
    TableSortLabel,
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import { KeyboardArrowDown as KeyboardArrowDownIcon } from '@/components/icons';
import { KeyboardArrowUp as KeyboardArrowUpIcon } from '@/components/icons';
import { Refresh as RefreshIcon } from '@/components/icons';
import { Close as CloseIcon } from '@/components/icons';

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
    getRequestBody?: (bodyRef: string) => Promise<{ id: string; method: string; path: string; body: string; truncated: boolean } | null>;
}

type SortField = 'time' | 'level' | 'status' | 'message';
type SortOrder = 'asc' | 'desc';

const LOG_LEVELS = ['debug', 'info', 'warn', 'error', 'fatal', 'panic'];

const SystemLogViewer = ({ getLogs, getRequestBody }: SystemLogViewerProps) => {
    const [logs, setLogs] = useState<SystemLogEntry[]>([]);
    const [allLogs, setAllLogs] = useState<SystemLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    // Multi-select level filter: empty set = show all
    const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [autoRefresh, setAutoRefresh] = useState(false);
    const tableContainerRef = useRef<HTMLDivElement>(null);
    // Sorting state
    const [sortField, setSortField] = useState<SortField>('time');
    const [sortOrder, setSortOrder] = useState<SortOrder>('desc');

    // Request body dialog state
    const [bodyDialogOpen, setBodyDialogOpen] = useState(false);
    const [selectedBodyRef, setSelectedBodyRef] = useState<string | null>(null);
    const [requestBody, setRequestBody] = useState<{ id: string; method: string; path: string; body: string; truncated: boolean } | null>(null);
    const [loadingBody, setLoadingBody] = useState(false);
    const [bodyError, setBodyError] = useState<string | null>(null);

    const loadLogs = async () => {
        setLoading(true);
        try {
            const response = await getLogs({ limit: 200 });
            if (response && response.logs) {
                const sortedLogs = [...response.logs].sort((a, b) => {
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

    const openRequestBodyDialog = async (bodyRef: string) => {
        setSelectedBodyRef(bodyRef);
        setBodyDialogOpen(true);
        setRequestBody(null);
        setBodyError(null);
        setLoadingBody(true);

        if (!getRequestBody) {
            setBodyError('Request body viewing not available');
            setLoadingBody(false);
            return;
        }

        try {
            const result = await getRequestBody(bodyRef);
            if (result) {
                setRequestBody(result);
            } else {
                setBodyError('Request body not found (may have been evicted from memory)');
            }
        } catch (error: any) {
            setBodyError(error instanceof Error ? error.message : 'Failed to load request body');
        } finally {
            setLoadingBody(false);
        }
    };

    const closeRequestBodyDialog = () => {
        setBodyDialogOpen(false);
        setSelectedBodyRef(null);
        setRequestBody(null);
        setBodyError(null);
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

    // Client-side filter by level
    useEffect(() => {
        let next = allLogs;
        if (selectedLevels.size > 0) {
            next = next.filter(log => {
                const l = log.level?.toLowerCase() ?? '';
                // match "warn" tag against "warn" or "warning"
                return [...selectedLevels].some(sel =>
                    l === sel || (sel === 'warn' && l === 'warning')
                );
            });
        }
        setLogs(next);
    }, [selectedLevels, allLogs]);

    useEffect(() => {
        loadLogs();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [sortField, sortOrder]);

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(loadLogs, 5000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh]);

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

    const formatRequestBody = (body: string): string => {
        try {
            const parsed = JSON.parse(body);
            return JSON.stringify(parsed, null, 2);
        } catch {
            return body;
        }
    };

    return (
        <Stack spacing={1.5} sx={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            {/* Toolbar */}
            <Stack
                direction="row"
                spacing={1.5}
                alignItems="center"
                flexWrap="wrap"
                useFlexGap
                sx={{
                    flexShrink: 0,
                    minHeight: 40,
                    py: 0.75,
                    alignContent: 'center',
                }}
            >
                {/* Actions */}
                <Stack direction="row" spacing={1} alignItems="center" sx={{ minHeight: 30 }}>
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
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'nowrap', lineHeight: 1.4 }}>
                        {logs.length}{allLogs.length !== logs.length ? ` / ${allLogs.length}` : ''}
                    </Typography>
                </Stack>

                <Box sx={{ flex: 1 }} />

                {/* Level filter tags */}
                <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap sx={{ minHeight: 30, alignContent: 'center' }}>
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
                                                                    <Box key={key}>
                                                                        {key === 'body_ref' ? (
                                                                            <Stack direction="row" spacing={1} alignItems="center">
                                                                                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                                                                    <strong>body_ref:</strong> {String(value)}
                                                                                </Typography>
                                                                                <Button
                                                                                    size="small"
                                                                                    variant="outlined"
                                                                                    onClick={(e) => {
                                                                                        e.stopPropagation();
                                                                                        openRequestBodyDialog(String(value));
                                                                                    }}
                                                                                    sx={{ fontSize: '0.7rem', py: 0.25, px: 1 }}
                                                                                >
                                                                                    View Request Body
                                                                                </Button>
                                                                            </Stack>
                                                                        ) : (
                                                                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                                                                <strong>{key}:</strong> {typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)}
                                                                            </Typography>
                                                                        )}
                                                                    </Box>
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
            </Box>

            {/* Request Body Dialog */}
            <Dialog
                open={bodyDialogOpen}
                onClose={closeRequestBodyDialog}
                maxWidth="md"
                fullWidth
            >
                <DialogTitle>
                    <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between">
                        <Stack direction="row" spacing={1} alignItems="center">
                            <Typography variant="h6">Request Body</Typography>
                            {requestBody?.truncated && (
                                <Chip
                                    label="Truncated"
                                    size="small"
                                    color="warning"
                                    sx={{ fontSize: '0.7rem', height: 20 }}
                                />
                            )}
                        </Stack>
                        <IconButton onClick={closeRequestBodyDialog} size="small">
                            <CloseIcon />
                        </IconButton>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    {loadingBody && (
                        <Typography color="text.secondary">Loading...</Typography>
                    )}
                    {bodyError && (
                        <Typography color="error">{bodyError}</Typography>
                    )}
                    {requestBody && (
                        <Stack spacing={2}>
                            <Stack direction="row" spacing={2}>
                                <Box>
                                    <Typography variant="caption" color="text.secondary">Method</Typography>
                                    <Typography variant="body2">{requestBody.method}</Typography>
                                </Box>
                                <Box>
                                    <Typography variant="caption" color="text.secondary">Path</Typography>
                                    <Typography variant="body2">{requestBody.path}</Typography>
                                </Box>
                            </Stack>
                            {requestBody.truncated && (
                                <Alert severity="warning" sx={{ py: 0.5 }}>
                                    <Typography variant="caption">
                                        Request body was truncated to 1MB. Original size was larger.
                                    </Typography>
                                </Alert>
                            )}
                            <TextField
                                fullWidth
                                multiline
                                rows={15}
                                value={formatRequestBody(requestBody.body)}
                                InputProps={{
                                    readOnly: true,
                                    sx: {
                                        fontFamily: 'monospace',
                                        fontSize: '0.75rem',
                                    },
                                }}
                                sx={{
                                    '& .MuiInputBase-input': {
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                    },
                                }}
                            />
                        </Stack>
                    )}
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeRequestBodyDialog}>Close</Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
};

export default SystemLogViewer;
