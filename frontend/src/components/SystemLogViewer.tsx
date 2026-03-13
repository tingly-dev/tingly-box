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
    Menu,
    MenuItem,
} from '@mui/material';
import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import FilterListIcon from '@mui/icons-material/FilterList';
import ClearIcon from '@mui/icons-material/Clear';
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
    const { t } = useTranslation();
    const [logs, setLogs] = useState<SystemLogEntry[]>([]);
    const [allLogs, setAllLogs] = useState<SystemLogEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [filterLevel, setFilterLevel] = useState<string | null>(null);
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [autoRefresh, setAutoRefresh] = useState(false);
    const tableContainerRef = useRef<HTMLDivElement>(null);

    // Menu anchor elements
    const [levelMenuAnchor, setLevelMenuAnchor] = useState<null | HTMLElement>(null);

    const loadLogs = async () => {
        setLoading(true);
        try {
            const response = await getLogs({ limit: 200 });
            if (response && response.logs) {
                // Sort logs by time ascending (oldest first)
                const sortedLogs = [...response.logs].sort((a, b) =>
                    new Date(a.time).getTime() - new Date(b.time).getTime()
                );
                setAllLogs(sortedLogs);
                // Apply current filter to newly loaded logs
                if (filterLevel === null) {
                    setLogs(sortedLogs);
                } else {
                    setLogs(sortedLogs.filter(log => log.level?.toLowerCase() === filterLevel?.toLowerCase()));
                }
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

    const getLevelColor = (level: string): string => {
        switch (level.toLowerCase()) {
            case 'panic':
                return '#991b1b';
            case 'fatal':
                return '#dc2626';
            case 'error':
                return '#ef4444';
            case 'warning':
            case 'warn':
                return '#f59e0b';
            case 'info':
                return '#3b82f6';
            case 'debug':
                return '#6b7280';
            default:
                return '#10b981';
        }
    };

    const getStatusCodeColor = (statusCode?: number): string => {
        if (!statusCode) return '#6b7280'; // gray for missing
        if (statusCode >= 200 && statusCode < 300) return '#10b981'; // green for 2xx
        if (statusCode >= 300 && statusCode < 400) return '#3b82f6'; // blue for 3xx
        if (statusCode >= 400 && statusCode < 500) return '#f59e0b'; // orange for 4xx
        if (statusCode >= 500) return '#ef4444'; // red for 5xx
        return '#6b7280';
    };

    const formatTimestamp = (timestamp: string): string => {
        try {
            const date = new Date(timestamp);
            return date.toLocaleString();
        } catch {
            return timestamp;
        }
    };

    // Client-side filter when filterLevel changes
    useEffect(() => {
        let filtered = allLogs;
        if (filterLevel !== null) {
            filtered = filtered.filter(log => log.level?.toLowerCase() === filterLevel?.toLowerCase());
        }
        setLogs(filtered);
    }, [filterLevel, allLogs]);

    useEffect(() => {
        loadLogs();
    }, []); // Only load on mount

    useEffect(() => {
        if (autoRefresh) {
            const interval = setInterval(loadLogs, 5000);
            return () => clearInterval(interval);
        }
    }, [autoRefresh]);

    // Scroll to bottom when logs change (show newest)
    useEffect(() => {
        if (tableContainerRef.current && logs.length > 0) {
            tableContainerRef.current.scrollTop = tableContainerRef.current.scrollHeight;
        }
    }, [logs]);

    return (
        <Stack spacing={2}>
            {/* Header */}
            <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
                <Stack direction="row" spacing={2} alignItems="center">
                    <Button
                        variant={autoRefresh ? 'contained' : 'outlined'}
                        size="small"
                        onClick={() => setAutoRefresh(!autoRefresh)}
                    >
                        Auto Refresh
                    </Button>
                    <Button
                        variant="outlined"
                        size="small"
                        onClick={loadLogs}
                        disabled={loading}
                        startIcon={<RefreshIcon />}
                    >
                        Refresh
                    </Button>
                    {filterLevel !== null && (
                        <Button
                            variant="outlined"
                            size="small"
                            startIcon={<ClearIcon />}
                            onClick={() => {
                                setFilterLevel(null);
                            }}
                        >
                            Clear Filter
                        </Button>
                    )}
                </Stack>
                <Typography variant="body2" color="text.secondary">
                    Total: {logs.length}{allLogs.length !== logs.length && ` / ${allLogs.length}`}
                </Typography>
            </Stack>

            {/* Logs Table */}
            <TableContainer component={Paper} sx={{ height: 600 }} ref={tableContainerRef}>
                <Table stickyHeader size="small">
                    <TableHead>
                        <TableRow>
                            <TableCell padding="checkbox" />
                            <TableCell sx={{ width: 180 }}>Time</TableCell>
                            <TableCell sx={{ width: 100 }}>
                                <Stack direction="row" alignItems="center" spacing={0.5}>
                                    <span>Level</span>
                                    <IconButton
                                        size="small"
                                        onClick={(e) => setLevelMenuAnchor(e.currentTarget)}
                                        sx={{ padding: 0.5 }}
                                        color={filterLevel !== null ? 'primary' : 'default'}
                                    >
                                        <FilterListIcon fontSize="small" />
                                    </IconButton>
                                </Stack>
                            </TableCell>
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
                                        <TableCell
                                            colSpan={5}
                                            sx={{ pb: 0, pt: 0, border: 'none' }}
                                        >
                                            <Collapse
                                                in={expandedRows.has(index)}
                                                timeout="auto"
                                                unmountOnExit
                                            >
                                                <Box sx={{ p: 2, backgroundColor: 'rgba(0,0,0,0.03)' }}>
                                                    {log.fields && Object.keys(log.fields).length > 0 && (
                                                        <Stack spacing={1}>
                                                            <Typography variant="subtitle2" color="text.secondary">
                                                                Fields:
                                                            </Typography>
                                                            {Object.entries(log.fields).map(([key, value]) => (
                                                                <Typography
                                                                    key={key}
                                                                    variant="body2"
                                                                    sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}
                                                                >
                                                                    <strong>{key}:</strong> {String(value)}
                                                                </Typography>
                                                            ))}
                                                        </Stack>
                                                    )}
                                                    {!log.fields || Object.keys(log.fields).length === 0 && (
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

            {/* Level Filter Menu */}
            <Menu
                anchorEl={levelMenuAnchor}
                open={Boolean(levelMenuAnchor)}
                onClose={() => setLevelMenuAnchor(null)}
            >
                <MenuItem
                    selected={filterLevel === null}
                    onClick={() => {
                        setFilterLevel(null);
                        setLevelMenuAnchor(null);
                    }}
                >
                    All Levels
                </MenuItem>
                {LOG_LEVELS.map((level) => (
                    <MenuItem
                        key={level}
                        selected={filterLevel === level}
                        onClick={() => {
                            setFilterLevel(level);
                            setLevelMenuAnchor(null);
                        }}
                    >
                        <Box sx={{ width: 12, height: 12, borderRadius: '50%', backgroundColor: getLevelColor(level), mr: 1 }} />
                        {level.charAt(0).toUpperCase() + level.slice(1)}
                    </MenuItem>
                ))}
            </Menu>
        </Stack>
    );
};

export default SystemLogViewer;
