import React, { useState, useEffect, useRef } from 'react';
import {
    Box,
    Card,
    CardContent,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Typography,
    TextField,
    Button,
    Chip,
    CircularProgress,
    IconButton,
    Paper,
    Alert,
    Select,
    MenuItem,
    FormControl,
    InputLabel,
} from '@mui/material';
import {
    Send as SendIcon,
    Refresh as RefreshIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';
import { Link as RouterLink } from 'react-router-dom';
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

const RemoteCCPage: React.FC = () => {
    const [sessions, setSessions] = useState<Session[]>([]);
    const [selectedSession, setSelectedSession] = useState<Session | null>(null);
    const [loading, setLoading] = useState(true);
    const [sending, setSending] = useState(false);
    const [message, setMessage] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [chatHistory, setChatHistory] = useState<ChatMessage[]>([]);
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const [projectPath, setProjectPath] = useState('');
    const [projectPathDialogOpen, setProjectPathDialogOpen] = useState(true);
    const [expandedBySession, setExpandedBySession] = useState<Record<string, Set<number>>>({});
    const [projectPathBySession, setProjectPathBySession] = useState<Record<string, string>>({});
    const [projectPathNewSession, setProjectPathNewSession] = useState<string>('');
    const [lastSelectedSessionId, setLastSelectedSessionId] = useState<string>('');

    const sessionKey = selectedSession?.id || 'new';
    const expandedMessages = expandedBySession[sessionKey] || new Set<number>();
    const isSessionThinking = !!selectedSession?.id
        && selectedSession.status === 'running'
        && (chatHistory.length === 0 || chatHistory[chatHistory.length - 1].role === 'user');
    const isChatBusy = sending || isSessionThinking;

    useEffect(() => {
        const storedNewPath = localStorage.getItem('remotecc.projectPath.new') || '';
        const storedPathsRaw = localStorage.getItem('remotecc.projectPaths') || '';
        let storedPaths: Record<string, string> = {};
        if (storedPathsRaw) {
            try {
                storedPaths = JSON.parse(storedPathsRaw);
            } catch {
                storedPaths = {};
            }
        }
        const storedLastSessionId = localStorage.getItem('remotecc.lastSessionId') || '';
        if (storedNewPath) {
            setProjectPathNewSession(storedNewPath);
        }
        if (Object.keys(storedPaths).length > 0) {
            setProjectPathBySession(storedPaths);
        }
        if (storedLastSessionId) {
            setLastSelectedSessionId(storedLastSessionId);
        }
    }, []);

    useEffect(() => {
        localStorage.setItem('remotecc.projectPath.new', projectPathNewSession);
    }, [projectPathNewSession]);

    useEffect(() => {
        localStorage.setItem('remotecc.projectPaths', JSON.stringify(projectPathBySession));
    }, [projectPathBySession]);

    useEffect(() => {
        localStorage.setItem('remotecc.lastSessionId', lastSelectedSessionId);
    }, [lastSelectedSessionId]);

    useEffect(() => {
        const raw = localStorage.getItem('remotecc.expandedMessages') || '';
        if (!raw) return;
        try {
            const data = JSON.parse(raw) as Record<string, number[]>;
            const next: Record<string, Set<number>> = {};
            for (const [key, values] of Object.entries(data)) {
                if (Array.isArray(values)) {
                    next[key] = new Set(values.filter((v) => Number.isInteger(v)));
                }
            }
            setExpandedBySession(next);
        } catch {
            // ignore parse errors
        }
    }, []);

    useEffect(() => {
        const payload: Record<string, number[]> = {};
        for (const [key, value] of Object.entries(expandedBySession)) {
            payload[key] = Array.from(value);
        }
        localStorage.setItem('remotecc.expandedMessages', JSON.stringify(payload));
    }, [expandedBySession]);

    useEffect(() => {
        if (!sessionKey) return;
        const raw = localStorage.getItem(`remotecc.chatHistory.${sessionKey}`) || '';
        if (!raw) return;
        try {
            const parsed = JSON.parse(raw) as ChatMessage[];
            if (Array.isArray(parsed)) {
                setChatHistory(parsed);
            }
        } catch {
            // ignore parse errors
        }
    }, [sessionKey]);

    useEffect(() => {
        if (!sessionKey) return;
        localStorage.setItem(`remotecc.chatHistory.${sessionKey}`, JSON.stringify(chatHistory));
    }, [chatHistory, sessionKey]);

    useEffect(() => {
        if (selectedSession?.id) {
            const stored = projectPathBySession[selectedSession.id] || '';
            setProjectPath(stored);
            setProjectPathDialogOpen(!stored.trim());
        } else {
            const stored = projectPathNewSession || '';
            setProjectPath(stored);
            setProjectPathDialogOpen(!stored.trim());
        }
    }, [selectedSession?.id, projectPathBySession, projectPathNewSession]);

    const fetchSessions = async () => {
        try {
            setLoading(true);
            const data = await api.getRemoteCCSessions({
                page: 1,
                limit: 100,
            });

            if (data?.success === false) {
                setError(data.error || 'Failed to load sessions');
                return;
            }

            if (data.sessions) {
                setSessions(data.sessions);
                if (selectedSession?.id) {
                    const updated = data.sessions.find((s: Session) => s.id === selectedSession.id);
                    if (updated) {
                        setSelectedSession(updated);
                    }
                }
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
    }, []);

    useEffect(() => {
        if (!lastSelectedSessionId || selectedSession) return;
        api.getRemoteCCSession(lastSelectedSessionId)
            .then(async (sessionData) => {
                if (!sessionData?.id) return;
                setSelectedSession(sessionData);
                setSessions((prev) => {
                    if (prev.some((s) => s.id === sessionData.id)) return prev;
                    return [sessionData, ...prev];
                });
                const messages = await api.getRemoteCCSessionMessages(sessionData.id);
                if (messages?.messages && Array.isArray(messages.messages)) {
                    setChatHistory(messages.messages.map((m: any) => ({
                        role: m.role,
                        content: m.content || '',
                        summary: m.summary,
                        timestamp: m.timestamp || new Date().toISOString(),
                    })));
                    return;
                }

                if (sessionData.request || sessionData.response) {
                    setChatHistory([
                        {
                            role: 'user',
                            content: sessionData.request || '',
                            timestamp: sessionData.created_at,
                        },
                        {
                            role: 'assistant',
                            content: sessionData.response || '',
                            summary: sessionData.response || '',
                            timestamp: sessionData.last_activity,
                        },
                    ]);
                }
            })
            .catch((err) => {
                console.error('Failed to restore last session:', err);
            });
    }, [lastSelectedSessionId, selectedSession]);

    useEffect(() => {
        if (!sessions.length || selectedSession) return;
        const saved = lastSelectedSessionId
            ? sessions.find((s) => s.id === lastSelectedSessionId)
            : undefined;
        if (saved) {
            handleSessionSelect(saved);
        }
    }, [sessions, selectedSession, lastSelectedSessionId]);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    };

    useEffect(() => {
        scrollToBottom();
    }, [chatHistory]);

    const handleSendMessage = async () => {
        if (!message.trim() || isChatBusy) return;
        if (!projectPath.trim()) {
            setProjectPathDialogOpen(true);
            return;
        }

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
                context: { project_path: projectPath.trim() },
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
                    setProjectPathBySession((prev) => ({
                        ...prev,
                        [sessionData.id]: projectPath.trim(),
                    }));
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
        setProjectPath(projectPathBySession[session.id] || '');
        setLastSelectedSessionId(session.id);
        setProjectPathDialogOpen(!(projectPathBySession[session.id] || '').trim());

        const messages = await api.getRemoteCCSessionMessages(session.id);
        if (messages?.messages && Array.isArray(messages.messages)) {
            setChatHistory(messages.messages.map((m: any) => ({
                role: m.role,
                content: m.content || '',
                summary: m.summary,
                timestamp: m.timestamp || new Date().toISOString(),
            })));
            return;
        }

        // Fallback to session summary
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
        setLastSelectedSessionId('');
        if (projectPathNewSession.trim()) {
            setProjectPath(projectPathNewSession.trim());
            setProjectPathDialogOpen(false);
        } else {
            setProjectPath('');
            setProjectPathDialogOpen(true);
        }
    };

    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
                <Box>
                    <Typography variant="h4" fontWeight={700} gutterBottom>
                        Remote Claude Code Chat
                    </Typography>
                    <Typography variant="body1" color="text.secondary">
                        Chat with Claude Code sessions remotely
                    </Typography>
                    <Box sx={{ mt: 1, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                        <Chip
                            label={selectedSession?.id ? `Session: ${selectedSession.id}` : 'Session: New'}
                            size="small"
                            sx={{ fontFamily: 'monospace' }}
                        />
                    </Box>
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
                        onClick={handleNewChat}
                    >
                        New Chat
                    </Button>
                    <Button
                        component={RouterLink}
                        to="/remote-cc/sessions"
                        variant="outlined"
                    >
                        Manage Sessions
                    </Button>
                </Box>
            </Box>

            {error && (
                <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            <Dialog open={projectPathDialogOpen} onClose={() => {}}>
                <DialogTitle>Set Project Path</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Enter the project path to provide Claude Code context for this chat.
                    </Typography>
                    <TextField
                        autoFocus
                        fullWidth
                        label="Project Path"
                        placeholder="/path/to/project"
                        value={projectPath}
                        onChange={(e) => setProjectPath(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                                e.preventDefault();
                                if (projectPath.trim()) {
                                    if (!selectedSession) {
                                        setProjectPathNewSession(projectPath.trim());
                                    } else if (selectedSession?.id) {
                                        setProjectPathBySession((prev) => ({
                                            ...prev,
                                            [selectedSession.id]: projectPath.trim(),
                                        }));
                                    }
                                    setProjectPathDialogOpen(false);
                                }
                            }
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button
                        variant="contained"
                        onClick={() => {
                            if (!selectedSession) {
                                setProjectPathNewSession(projectPath.trim());
                            } else if (selectedSession?.id) {
                                setProjectPathBySession((prev) => ({
                                    ...prev,
                                    [selectedSession.id]: projectPath.trim(),
                                }));
                            }
                            setProjectPathDialogOpen(false);
                        }}
                        disabled={!projectPath.trim()}
                    >
                        Continue
                    </Button>
                </DialogActions>
            </Dialog>

            <Card sx={{ mb: 3 }}>
                <CardContent sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, alignItems: 'center' }}>
                    <TextField
                        label="Project Path"
                        value={projectPath}
                        onChange={(e) => {
                            const next = e.target.value;
                            setProjectPath(next);
                            if (selectedSession?.id) {
                                setProjectPathBySession((prev) => ({
                                    ...prev,
                                    [selectedSession.id]: next,
                                }));
                            } else {
                                setProjectPathNewSession(next);
                            }
                        }}
                        size="small"
                        sx={{ minWidth: 260 }}
                    />
                    <FormControl size="small" sx={{ minWidth: 240 }}>
                        <InputLabel>Session</InputLabel>
                        <Select
                            value={selectedSession?.id || ''}
                            label="Session"
                            onChange={(e) => {
                                const session = sessions.find((s) => s.id === e.target.value);
                                if (session) {
                                    handleSessionSelect(session);
                                } else {
                                    handleNewChat();
                                }
                            }}
                        >
                            <MenuItem value="">
                                New Session
                            </MenuItem>
                            {sessions.map((session) => (
                                <MenuItem key={session.id} value={session.id}>
                                    {session.request || 'New Session'}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    {loading && (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <CircularProgress size={16} />
                            <Typography variant="body2" color="text.secondary">
                                Loading sessions...
                            </Typography>
                        </Box>
                    )}
                </CardContent>
            </Card>

            <Card sx={{ height: 'calc(100vh - 320px)', minHeight: 400, display: 'flex', flexDirection: 'column' }}>
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
                                            ? (expandedMessages.has(index) ? msg.content : msg.summary)
                                            : msg.content}
                                    </Typography>
                                    {msg.role === 'assistant' && msg.content && msg.content !== msg.summary && (
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            sx={{ display: 'block', mt: 1, cursor: 'pointer', textDecoration: 'underline' }}
                                            onClick={() => {
                                                setExpandedBySession((prev) => {
                                                    const current = prev[sessionKey] || new Set<number>();
                                                    const next = new Set(current);
                                                    if (next.has(index)) {
                                                        next.delete(index);
                                                    } else {
                                                        next.add(index);
                                                    }
                                                    return { ...prev, [sessionKey]: next };
                                                });
                                            }}
                                        >
                                            {expandedMessages.has(index)
                                                ? 'Collapse response'
                                                : `Show full response (${msg.content.length} chars)`}
                                        </Typography>
                                    )}
                                </Paper>
                                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                                    {new Date(msg.timestamp).toLocaleTimeString()}
                                </Typography>
                            </Box>
                        ))}
                            {isChatBusy && (
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
                                disabled={!message.trim() || isChatBusy}
                                sx={{ alignSelf: 'flex-end' }}
                            >
                                {isChatBusy ? <CircularProgress size={24} /> : <SendIcon />}
                            </IconButton>
                    </Box>
                </CardContent>
            </Card>
        </Box>
    );
};

export default RemoteCCPage;
