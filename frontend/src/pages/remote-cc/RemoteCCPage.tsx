import React, { useState, useEffect, useRef } from 'react';
import {
    Box,
    Card,
    CardContent,
    Typography,
    TextField,
    Button,
    List,
    ListItem,
    ListItemText,
    ListItemIcon,
    Chip,
    Divider,
    CircularProgress,
    IconButton,
    Paper,
    Alert,
    Select,
    MenuItem,
    FormControl,
    InputLabel,
    Tab,
    Tabs,
} from '@mui/material';
import {
    Send as SendIcon,
    Chat as ChatIcon,
    History as HistoryIcon,
    CheckCircle as SuccessIcon,
    Error as ErrorIcon,
    Schedule as PendingIcon,
    Cancel as ClosedIcon,
    Refresh as RefreshIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import api from '@/services/api';

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

interface ChatMessage {
    role: 'user' | 'assistant';
    content: string;
    summary?: string;
    timestamp: string;
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

const RemoteCCPage: React.FC = () => {
    const { t } = useTranslation();
    const [activeTab, setActiveTab] = useState(0);
    const [sessions, setSessions] = useState<Session[]>([]);
    const [selectedSession, setSelectedSession] = useState<Session | null>(null);
    const [stats, setStats] = useState<Stats | null>(null);
    const [loading, setLoading] = useState(true);
    const [sending, setSending] = useState(false);
    const [message, setMessage] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [chatHistory, setChatHistory] = useState<ChatMessage[]>([]);
    const [statusFilter, setStatusFilter] = useState<string>('');
    const messagesEndRef = useRef<HTMLDivElement>(null);

    const fetchSessions = async () => {
        try {
            setLoading(true);
            const data = await api.getRemoteCCSessions({
                page: 1,
                limit: 100,
                status: statusFilter || undefined,
            });

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

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    };

    useEffect(() => {
        scrollToBottom();
    }, [chatHistory]);

    const handleSendMessage = async () => {
        if (!message.trim() || sending) return;

        const userMessage = message.trim();
        setMessage('');
        setSending(true);
        setError(null);

        // Add user message to chat
        setChatHistory(prev => [
            ...prev,
            {
                role: 'user',
                content: userMessage,
                timestamp: new Date().toISOString(),
            },
        ]);

        try {
            const data = await api.sendRemoteCCChat({
                session_id: selectedSession?.id || undefined,
                message: userMessage,
            });

            if (data.error) {
                throw new Error(data.error);
            }

            // Add assistant response to chat
            setChatHistory(prev => [
                ...prev,
                {
                    role: 'assistant',
                    content: data.full_response || '',
                    summary: data.summary,
                    timestamp: new Date().toISOString(),
                },
            ]);

            // Update selected session
            if (data.session_id && !selectedSession) {
                const sessionData = await api.getRemoteCCSession(data.session_id);
                if (sessionData.id) {
                    setSelectedSession(sessionData);
                }
            }

            // Refresh sessions list
            fetchSessions();
        } catch (err: any) {
            setError(err.message || 'Failed to send message to Claude Code');
            console.error(err);
        } finally {
            setSending(false);
        }
    };

    const handleSessionSelect = async (session: Session) => {
        setSelectedSession(session);
        setChatHistory([]);

        // Load chat history from session
        if (session.request || session.response) {
            setChatHistory([
                {
                    role: 'user',
                    content: session.request,
                    timestamp: session.created_at,
                },
                {
                    role: 'assistant',
                    content: session.response,
                    summary: session.response,
                    timestamp: session.last_activity,
                },
            ]);
        }
    };

    const handleNewChat = () => {
        setSelectedSession(null);
        setChatHistory([]);
        setActiveTab(1); // Switch to chat tab
    };

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
                        Chat with Claude Code sessions remotely
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
                        variant="contained"
                        startIcon={<ChatIcon />}
                        onClick={handleNewChat}
                    >
                        New Chat
                    </Button>
                </Box>
            </Box>

            {error && (
                <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

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

            {/* Tabs */}
            <Paper sx={{ mb: 3 }}>
                <Tabs value={activeTab} onChange={(_, v) => setActiveTab(v)}>
                    <Tab label="Sessions" />
                    <Tab label="Chat" disabled={!selectedSession && chatHistory.length === 0} />
                </Tabs>
            </Paper>

            {/* Sessions Tab */}
            {activeTab === 0 && (
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
                                                onClick={() => handleSessionSelect(session)}
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
                                                        <Typography variant="caption" color="text.secondary">
                                                            {new Date(session.created_at).toLocaleString()}
                                                        </Typography>
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
                                            Select a session to view details or start a new chat
                                        </Typography>
                                    </Box>
                                )}
                            </CardContent>
                        </Card>
                    </Box>
                </Box>
            )}

            {/* Chat Tab */}
            {activeTab === 1 && (
                <Card sx={{ height: 'calc(100vh - 400px)', minHeight: 400, display: 'flex', flexDirection: 'column' }}>
                    <CardContent sx={{ p: 2, flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                        {/* Chat Messages */}
                        <Box sx={{ flex: 1, overflow: 'auto', mb: 2 }}>
                            {chatHistory.map((msg, index) => (
                                <Box
                                    key={index}
                                    sx={{
                                        display: 'flex',
                                        flexDirection: 'column',
                                        alignItems: msg.role === 'user' ? 'flex-end' : 'flex-start',
                                        mb: 2,
                                    }}
                                >
                                    <Chip
                                        label={msg.role === 'user' ? 'You' : 'Claude Code'}
                                        size="small"
                                        sx={{
                                            mb: 0.5,
                                            bgcolor: msg.role === 'user' ? 'primary.main' : alpha('#10b981', 0.1),
                                            color: msg.role === 'user' ? 'white' : '#10b981',
                                        }}
                                    />
                                    <Paper
                                        variant="outlined"
                                        sx={{
                                            p: 2,
                                            maxWidth: '80%',
                                            bgcolor: msg.role === 'user' ? 'grey.50' : alpha('#10b981', 0.05),
                                        }}
                                    >
                                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: 13 }}>
                                            {msg.role === 'assistant' && msg.summary
                                                ? msg.summary
                                                : msg.content}
                                        </Typography>
                                        {msg.role === 'assistant' && msg.content && msg.content !== msg.summary && (
                                            <Typography
                                                variant="caption"
                                                color="text.secondary"
                                                sx={{ display: 'block', mt: 1, cursor: 'pointer', textDecoration: 'underline' }}
                                                onClick={() => alert(msg.content)}
                                            >
                                                Show full response ({msg.content.length} chars)
                                            </Typography>
                                        )}
                                    </Paper>
                                    <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                                        {new Date(msg.timestamp).toLocaleTimeString()}
                                    </Typography>
                                </Box>
                            ))}
                            {sending && (
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                    <CircularProgress size={16} />
                                    <Typography variant="body2" color="text.secondary">
                                        Claude Code is thinking...
                                    </Typography>
                                </Box>
                            )}
                            <div ref={messagesEndRef} />
                        </Box>

                        {/* Message Input */}
                        <Box sx={{ display: 'flex', gap: 1 }}>
                            <TextField
                                fullWidth
                                multiline
                                maxRows={4}
                                placeholder="Send a message to Claude Code..."
                                value={message}
                                onChange={(e) => setMessage(e.target.value)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter' && !e.shiftKey) {
                                        e.preventDefault();
                                        handleSendMessage();
                                    }
                                }}
                                size="small"
                            />
                            <IconButton
                                color="primary"
                                onClick={handleSendMessage}
                                disabled={!message.trim() || sending}
                                sx={{ alignSelf: 'flex-end' }}
                            >
                                {sending ? <CircularProgress size={24} /> : <SendIcon />}
                            </IconButton>
                        </Box>
                    </CardContent>
                </Card>
            )}
        </Box>
    );
};

export default RemoteCCPage;
