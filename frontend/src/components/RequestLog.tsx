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
    Select,
    MenuItem,
    FormControl,
    InputLabel,
    IconButton,
    Collapse,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';

interface LogEntry {
    time: string;
    level: string;
    message: string;
    data?: Record<string, any>;
    fields?: Record<string, any>;
}

interface LogsResponse {
    total: number;
    logs: LogEntry[];
}

interface RequestLogProps {
    // API methods will be implemented by user
    getLogs: (params?: { limit?: number; level?: string; since?: string }) => Promise<LogsResponse>;
    clearLogs: () => Promise<{ success: boolean; message?: string }>;
}

const RequestLog = ({ getLogs, clearLogs }: RequestLogProps) => {
    const { t } = useTranslation();
    const [logs, setLogs] = useState<LogEntry[]>([]);
    const [allLogs, setAllLogs] = useState<LogEntry[]>([]); // Store all logs
    const [loading, setLoading] = useState(false);
    const [filterLevel, setFilterLevel] = useState<string>('all');
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
    const [autoRefresh, setAutoRefresh] = useState(false);

    const loadLogs = async () => {
        setLoading(true);
        try {
            const response = await getLogs({ limit: 100 });
            if (response && response.logs) {
                setAllLogs(response.logs);
                // Apply current filter to newly loaded logs
                if (filterLevel === 'all') {
                    setLogs(response.logs);
                } else {
                    setLogs(response.logs.filter(log => log.level.toLowerCase() === filterLevel.toLowerCase()));
                }
            }
        } catch (error) {
            console.error('Failed to load logs:', error);
        } finally {
            setLoading(false);
        }
    };

    const handleClearLogs = async () => {
        if (confirm('Are you sure you want to clear all logs?')) {
            try {
                await clearLogs();
                setAllLogs([]);
                setLogs([]);
            } catch (error) {
                console.error('Failed to clear logs:', error);
            }
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
        if (filterLevel === 'all') {
            setLogs(allLogs);
        } else {
            setLogs(allLogs.filter(log => log.level.toLowerCase() === filterLevel.toLowerCase()));
        }
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

    return (
        <Stack spacing={2}>
            {/* Header */}
            <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
                <Stack direction="row" spacing={2} alignItems="center">
                    <FormControl size="small" sx={{ minWidth: 120 }}>
                        <InputLabel>Level</InputLabel>
                        <Select
                            value={filterLevel}
                            label="Level"
                            onChange={(e) => setFilterLevel(e.target.value)}
                        >
                            <MenuItem value="all">All</MenuItem>
                            <MenuItem value="error">Error</MenuItem>
                            <MenuItem value="warning">Warning</MenuItem>
                            <MenuItem value="info">Info</MenuItem>
                            <MenuItem value="debug">Debug</MenuItem>
                        </Select>
                    </FormControl>
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
                    >
                        Refresh
                    </Button>
                    <Button
                        variant="outlined"
                        size="small"
                        color="error"
                        onClick={handleClearLogs}
                    >
                        Clear
                    </Button>
                </Stack>
                <Typography variant="body2" color="text.secondary">
                    Total: {logs.length}
                </Typography>
            </Stack>

            {/* Logs Table */}
            <TableContainer component={Paper} sx={{ maxHeight: 600 }}>
                <Table stickyHeader size="small">
                    <TableHead>
                        <TableRow>
                            <TableCell padding="checkbox" />
                            <TableCell sx={{ width: 180 }}>Time</TableCell>
                            <TableCell sx={{ width: 100 }}>Level</TableCell>
                            <TableCell>Message</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {logs.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={4} align="center" sx={{ py: 4 }}>
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
                                                label={log.level}
                                                size="small"
                                                sx={{
                                                    backgroundColor: getLevelColor(log.level),
                                                    color: 'white',
                                                    fontSize: '0.7rem',
                                                    height: 20,
                                                }}
                                            />
                                        </TableCell>
                                        <TableCell sx={{ fontSize: '0.8rem' }}>
                                            {log.message}
                                        </TableCell>
                                    </TableRow>
                                    <TableRow>
                                        <TableCell
                                            colSpan={4}
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
                                                    {log.data && Object.keys(log.data).length > 0 && (
                                                        <Stack spacing={1} sx={{ mt: 2 }}>
                                                            <Typography variant="subtitle2" color="text.secondary">
                                                                Data:
                                                            </Typography>
                                                            {Object.entries(log.data).map(([key, value]) => (
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

export default RequestLog;
