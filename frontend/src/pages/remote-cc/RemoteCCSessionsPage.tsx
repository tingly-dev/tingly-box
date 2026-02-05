import React, { useEffect, useState } from 'react';
import {
    Box,
    Card,
    CardContent,
    Typography,
    List,
    ListItem,
    ListItemText,
    ListItemIcon,
    Chip,
    Divider,
    CircularProgress,
    Paper,
    Alert,
    Select,
    MenuItem,
    FormControl,
    InputLabel,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
} from '@mui/material';
import {
    History as HistoryIcon,
    CheckCircle as SuccessIcon,
    Error as ErrorIcon,
    Schedule as PendingIcon,
    Cancel as ClosedIcon,
    Refresh as RefreshIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';
import api from '@/services/api';
import { Link as RouterLink } from 'react-router-dom';

interface Session {
    id: string;
    status: string;
    request: string;
    response: string;
    error: string;
    created_at: string;
    last_activity: string;
    expires_at: string;
}

interface Stats {
    total: number;
    active: number;
    completed: number;
    failed: number;
    closed: number;
    uptime: string;
}

const statusColors: Record<string, string> = {
    running: '#0891b2',
    completed: '#10b981',
    failed: '#ef4444',
    pending: '#f59e0b',
    closed: '#6b7280',
    expired: '#9ca3af',
};

const statusIcons: Record<string, React.ReactNode> = {
    running: <PendingIcon fontSize="small" />,
    completed: <SuccessIcon fontSize="small" />,
    failed: <ErrorIcon fontSize="small" />,
    pending: <PendingIcon fontSize="small" />,
    closed: <ClosedIcon fontSize="small" />,
};

const RemoteCCSessionsPage: React.FC = () => {
    const [sessions, setSessions] = useState<Session[]>([]);
    const [selectedSession, setSelectedSession] = useState<Session | null>(null);
    const [stats, setStats] = useState<Stats | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [statusFilter, setStatusFilter] = useState<string>('');
    const [clearDialogOpen, setClearDialogOpen] = useState(false);

    const fetchSessions = async () => {
        try {
            setLoading(true);
            const data = await api.getRemoteCCSessions({
                page: 1,
                limit: 100,
                status: statusFilter || undefined,
            });

            if (data?.success === false) {
                setError(data.error || 'Failed to load sessions');
                return;
            }

            if (data.sessions) {
                setSessions(data.sessions);
            }
            if (data.stats) {
                setStats({
                    total: data.stats.total || 0,
                    active: data.stats.active || 0,
                    completed: data.stats.completed || 0,
                    failed: data.stats.failed || 0,
                    closed: data.stats.closed || 0,
                    uptime: typeof data.stats.uptime === 'string' ? data.stats.uptime : '0s',
                });
            }
        } catch (err) {
            setError('Failed to load sessions');
            console.error(err);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        fetchSessions();
    }, [statusFilter]);

    const formatDuration = (str: string): string => {
        const match = str.match(/(?:(\d+)h)?(\d+)m(\d+)s/);
        if (match) {
            const [, hours, minutes, seconds] = match;
            const parts: string[] = [];
            if (hours) parts.push(`${hours}h`);
            if (minutes) parts.push(`${minutes}m`);
            parts.push(`${seconds}s`);
            return parts.join(' ');
        }
        return str;
    };

    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
                <Box>
                    <Typography variant="h4" fontWeight={700} gutterBottom>
                        Remote Claude Code
                    </Typography>
                    <Typography variant="body1" color="text.secondary">
                        Manage remote Claude Code sessions
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', gap: 1 }}>
                    <Button
                        variant="outlined"
                        startIcon={<RefreshIcon />}
                        onClick={fetchSessions}
                        disabled={loading}
                    >
                        Refresh
                    </Button>
                    <Button
                        variant="outlined"
                        color="error"
                        onClick={() => setClearDialogOpen(true)}
                    >
                        Clear All
                    </Button>
                    <Button
                        component={RouterLink}
                        to="/remote-cc/chat"
                        variant="contained"
                    >
                        Open Chat
                    </Button>
                </Box>
            </Box>

            {error && (
                <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            <Dialog open={clearDialogOpen} onClose={() => setClearDialogOpen(false)}>
                <DialogTitle>Clear All Sessions</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        This will clear all stored Remote CC sessions from the local UI cache.
                        It does not delete sessions on the server.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setClearDialogOpen(false)}>
                        Cancel
                    </Button>
                    <Button
                        color="error"
                        variant="contained"
                        onClick={() => {
                            api.clearRemoteCCSessions()
                                .then((result) => {
                                    if (result?.success) {
                                        setSessions([]);
                                        setSelectedSession(null);
                                        setStats(null);
                                    } else {
                                        setError(result?.error || 'Failed to clear sessions');
                                    }
                                })
                                .catch((err) => {
                                    console.error(err);
                                    setError('Failed to clear sessions');
                                })
                                .finally(() => {
                                    setClearDialogOpen(false);
                                });
                        }}
                    >
                        Clear
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Stats Cards */}
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, mb: 3 }}>
                <Box sx={{ width: { xs: 'calc(50% - 8px)', sm: 'calc(20% - 12px)' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Typography variant="body2" color="text.secondary">Total</Typography>
                            <Typography variant="h5" fontWeight={700}>{stats?.total || 0}</Typography>
                        </CardContent>
                    </Card>
                </Box>
                <Box sx={{ width: { xs: 'calc(50% - 8px)', sm: 'calc(20% - 12px)' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Typography variant="body2" color="text.secondary">Active</Typography>
                            <Typography variant="h5" fontWeight={700} sx={{ color: '#0891b2' }}>{stats?.active || 0}</Typography>
                        </CardContent>
                    </Card>
                </Box>
                <Box sx={{ width: { xs: 'calc(50% - 8px)', sm: 'calc(20% - 12px)' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Typography variant="body2" color="text.secondary">Completed</Typography>
                            <Typography variant="h5" fontWeight={700} sx={{ color: '#10b981' }}>{stats?.completed || 0}</Typography>
                        </CardContent>
                    </Card>
                </Box>
                <Box sx={{ width: { xs: 'calc(50% - 8px)', sm: 'calc(20% - 12px)' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Typography variant="body2" color="text.secondary">Failed</Typography>
                            <Typography variant="h5" fontWeight={700} sx={{ color: '#ef4444' }}>{stats?.failed || 0}</Typography>
                        </CardContent>
                    </Card>
                </Box>
                <Box sx={{ width: { xs: 'calc(100% - 16px)', sm: 'calc(20% - 12px)' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Typography variant="body2" color="text.secondary">Uptime</Typography>
                            <Typography variant="h5" fontWeight={700}>{formatDuration(stats?.uptime || '0s')}</Typography>
                        </CardContent>
                    </Card>
                </Box>
            </Box>

            {/* Sessions */}
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}>
                {/* Session List */}
                <Box sx={{ width: { xs: '100%', md: '40%' } }}>
                    <Card>
                        <CardContent sx={{ p: 2 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                                <Typography variant="h6" fontWeight={600}>
                                    Sessions
                                </Typography>
                                <FormControl size="small" sx={{ minWidth: 120 }}>
                                    <InputLabel>Status</InputLabel>
                                    <Select
                                        value={statusFilter}
                                        label="Status"
                                        onChange={(e) => setStatusFilter(e.target.value)}
                                    >
                                        <MenuItem value="">All</MenuItem>
                                        <MenuItem value="running">Running</MenuItem>
                                        <MenuItem value="completed">Completed</MenuItem>
                                        <MenuItem value="failed">Failed</MenuItem>
                                        <MenuItem value="closed">Closed</MenuItem>
                                    </Select>
                                </FormControl>
                            </Box>

                            {loading ? (
                                <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
                                    <CircularProgress />
                                </Box>
                            ) : (
                                <List dense>
                                    {sessions.map((session) => (
                                        <ListItem
                                            key={session.id}
                                            button
                                            selected={selectedSession?.id === session.id}
                                            onClick={() => setSelectedSession(session)}
                                            sx={{
                                                borderRadius: 1,
                                                mb: 0.5,
                                                bgcolor: selectedSession?.id === session.id
                                                    ? alpha('#2563eb', 0.1)
                                                    : 'transparent',
                                            }}
                                        >
                                            <ListItemIcon sx={{ minWidth: 32 }}>
                                                <Box sx={{ color: statusColors[session.status] || '#6b7280' }}>
                                                    {statusIcons[session.status] || <HistoryIcon />}
                                                </Box>
                                            </ListItemIcon>
                                            <ListItemText
                                                primary={
                                                    <Typography variant="body2" noWrap>
                                                        {session.request || 'New Session'}
                                                    </Typography>
                                                }
                                                secondary={
                                                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25 }}>
                                                        <Typography variant="caption" color="text.secondary">
                                                            {new Date(session.created_at).toLocaleString()}
                                                        </Typography>
                                                        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                                                            {session.id}
                                                        </Typography>
                                                    </Box>
                                                }
                                            />
                                            <Chip
                                                label={session.status}
                                                size="small"
                                                sx={{
                                                    bgcolor: alpha(statusColors[session.status] || '#6b7280', 0.1),
                                                    color: statusColors[session.status] || '#6b7280',
                                                    fontSize: '0.7rem',
                                                }}
                                            />
                                        </ListItem>
                                    ))}
                                    {sessions.length === 0 && (
                                        <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                                            No sessions found
                                        </Typography>
                                    )}
                                </List>
                            )}
                        </CardContent>
                    </Card>
                </Box>

                {/* Session Details */}
                <Box sx={{ width: { xs: '100%', md: '60%' } }}>
                    <Card sx={{ height: 'calc(100vh - 420px)', minHeight: 400 }}>
                        <CardContent sx={{ p: 2, height: '100%', display: 'flex', flexDirection: 'column' }}>
                            {selectedSession ? (
                                <>
                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                                        <Typography variant="h6" fontWeight={600}>
                                            Session Details
                                        </Typography>
                                        <Chip
                                            label={selectedSession.status}
                                            sx={{
                                                bgcolor: alpha(statusColors[selectedSession.status] || '#6b7280', 0.1),
                                                color: statusColors[selectedSession.status] || '#6b7280',
                                            }}
                                        />
                                    </Box>

                                    <Divider sx={{ mb: 2 }} />

                                    <Box sx={{ flex: 1, overflow: 'auto' }}>
                                        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                            Request
                                        </Typography>
                                        <Paper
                                            variant="outlined"
                                            sx={{ p: 2, mb: 2, bgcolor: 'grey.50', fontFamily: 'monospace', fontSize: 13 }}
                                        >
                                            {selectedSession.request || '-'}
                                        </Paper>

                                        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                                            Response (Summary)
                                        </Typography>
                                        <Paper
                                            variant="outlined"
                                            sx={{ p: 2, bgcolor: alpha('#10b981', 0.05), fontFamily: 'monospace', fontSize: 13 }}
                                        >
                                            {selectedSession.response ? (
                                                <>
                                                    <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                                                        {selectedSession.response.substring(0, 1000)}
                                                        {selectedSession.response.length > 1000 && '...'}
                                                    </Typography>
                                                    {selectedSession.response.length > 1000 && (
                                                        <Typography variant="caption" color="text.secondary">
                                                            (Response truncated. View full response in chat.)
                                                        </Typography>
                                                    )}
                                                </>
                                            ) : (
                                                '-'
                                            )}
                                        </Paper>

                                        {selectedSession.error && (
                                            <>
                                                <Typography variant="subtitle2" color="error" gutterBottom sx={{ mt: 2 }}>
                                                    Error
                                                </Typography>
                                                <Paper
                                                    variant="outlined"
                                                    sx={{ p: 2, bgcolor: alpha('#ef4444', 0.05), fontFamily: 'monospace', fontSize: 13 }}
                                                >
                                                    <Typography variant="body2" color="error">
                                                        {selectedSession.error}
                                                    </Typography>
                                                </Paper>
                                            </>
                                        )}
                                    </Box>
                                </>
                            ) : (
                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
                                    <Typography variant="body2" color="text.secondary">
                                        Select a session to view details
                                    </Typography>
                                </Box>
                            )}
                        </CardContent>
                    </Card>
                </Box>
            </Box>
        </Box>
    );
};

export default RemoteCCSessionsPage;
