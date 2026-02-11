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
    project_path?: string;
}

interface ChatMessage {
    role: 'user' | 'assistant';
    content: string;
    summary?: string;
    timestamp: string;
}

const RemoteCoderPage: React.FC = () => {
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
    const [expandedMessages, setExpandedMessages] = useState<Set<number>>(new Set());
    const [projectPathNewSession, setProjectPathNewSession] = useState<string>('');
    const [sessionsLoaded, setSessionsLoaded] = useState(false);

    const isSessionThinking = !!selectedSession?.id
        && selectedSession.status === 'running'
        && (chatHistory.length === 0 || chatHistory[chatHistory.length - 1].role === 'user');
    const isChatBusy = sending || isSessionThinking;

    useEffect(() => {
        if (selectedSession?.id) {
            const stored = selectedSession.project_path || '';
            setProjectPath(stored);
            setProjectPathDialogOpen(!stored.trim());
        } else {
            const stored = projectPathNewSession || '';
            setProjectPath(stored);
            setProjectPathDialogOpen(!stored.trim());
        }
    }, [selectedSession?.id, selectedSession?.project_path, projectPathNewSession]);

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
                const sortedSessions = [...data.sessions].sort((a: Session, b: Session) => {
                    const aTime = new Date(a.last_activity).getTime();
                    const bTime = new Date(b.last_activity).getTime();
                    return bTime - aTime;
                });
                setSessions(sortedSessions);
                if (selectedSession?.id) {
                    const updated = sortedSessions.find((s: Session) => s.id === selectedSession.id);
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
            setSessionsLoaded(true);
        }
    };

    const loadSessionState = async (sessionId: string) => {
        try {
            const state = await api.getRemoteCCSessionState(sessionId);
            if (state?.success === false) return;
            if (Array.isArray(state?.expanded_messages)) {
                const next = new Set(state.expanded_messages.filter((v: any) => Number.isInteger(v)));
                setExpandedMessages(next);
            } else {
                setExpandedMessages(new Set());
            }
            if (typeof state?.project_path === 'string' && state.project_path.trim()) {
                setProjectPath(state.project_path);
                setProjectPathDialogOpen(false);
            }
        } catch (err) {
            console.error('Failed to load session state:', err);
        }
    };

    useEffect(() => {
        fetchSessions();
    }, []);

    useEffect(() => {
        if (!sessionsLoaded || selectedSession || !sessions.length) return;
        handleSessionSelect(sessions[0]);
    }, [sessionsLoaded, sessions, selectedSession]);

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    };

    useEffect(() => {
        scrollToBottom();
    }, [chatHistory]);

    useEffect(() => {
        if (!selectedSession?.id) return;
        const timer = window.setTimeout(() => {
            api.updateRemoteCCSessionState(selectedSession.id, {
                project_path: projectPath.trim(),
            }).catch((err: any) => {
                console.error('Failed to update project path:', err);
            });
        }, 400);
        return () => window.clearTimeout(timer);
    }, [projectPath, selectedSession?.id]);

    useEffect(() => {
        if (!selectedSession?.id) return;
        const timer = window.setTimeout(() => {
            api.updateRemoteCCSessionState(selectedSession.id, {
                expanded_messages: Array.from(expandedMessages),
            }).catch((err: any) => {
                console.error('Failed to update expanded messages:', err);
            });
        }, 250);
        return () => window.clearTimeout(timer);
    }, [expandedMessages, selectedSession?.id]);

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

        // If starting a new session, clear existing chat area before first message
        if (!selectedSession) {
            setChatHistory([]);
        }

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
                    setExpandedMessages(new Set());
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
        setExpandedMessages(new Set());
        const path = session.project_path || '';
        setProjectPath(path);
        setProjectPathDialogOpen(!path.trim());

        loadSessionState(session.id);

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
        setExpandedMessages(new Set());
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
                        Remote Coder Chat
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
                        to="/remote-coder/sessions"
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
                                    }
                                    setProjectPathDialogOpen(false);
                                }
                            }
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button
                        variant="text"
                        onClick={() => setProjectPathDialogOpen(false)}
                    >
                        Cancel
                    </Button>
                    <Button
                        variant="contained"
                        onClick={() => {
                            if (!selectedSession) {
                                setProjectPathNewSession(projectPath.trim());
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
                            if (!selectedSession?.id) {
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
                                                setExpandedMessages((prev) => {
                                                    const next = new Set(prev);
                                                    if (next.has(index)) {
                                                        next.delete(index);
                                                    } else {
                                                        next.add(index);
                                                    }
                                                    return next;
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

export default RemoteCoderPage;
