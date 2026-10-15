import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Chip, Paper, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { ProjectMessage, Thread } from '../../client';
import { Composer } from '../Composer';
import { Markdown } from '../Markdown';
import { PlayArrow } from '../icons';
import { ThreadStatusChip, ThreadSubtitle } from './threadStatus';

const COLUMN = { maxWidth: 760, mx: 'auto', width: '100%', px: { xs: 2, md: 3 } } as const;

function parse<T>(payload?: string): T | null {
    if (!payload) return null;
    try {
        return JSON.parse(payload) as T;
    } catch {
        return null;
    }
}

/** A thread as a card: title + live status, click to open. */
export function ThreadCard({ thread, onOpen }: { thread: Thread; onOpen: (id: string) => void }) {
    return (
        <Paper
            variant="outlined"
            role="button"
            tabIndex={0}
            onClick={() => onOpen(thread.id)}
            onKeyDown={(e) => e.key === 'Enter' && onOpen(thread.id)}
            sx={{ p: 1.25, px: 1.5, borderRadius: 2.5, display: 'flex', alignItems: 'center', gap: 1.25, '&:hover': { borderColor: 'text.disabled', bgcolor: 'action.hover' } }}
        >
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography noWrap sx={{ fontWeight: 600, fontSize: 14, color: 'text.primary' }}>{thread.title}</Typography>
                <Typography variant="caption" color="text.secondary" noWrap component="div"><ThreadSubtitle thread={thread} /></Typography>
            </Box>
            <ThreadStatusChip thread={thread} />
        </Paper>
    );
}

interface Suggestion {
    title: string;
    task: string;
    repo?: string;
    thread_id?: string;
}

function SuggestionsCard({ message, threads, onStart, onOpen, disabled }: {
    message: ProjectMessage;
    threads: Map<string, Thread>;
    onStart: (messageId: string, indexes: number[]) => Promise<void>;
    onOpen: (id: string) => void;
    disabled: boolean;
}) {
    const { t } = useTranslation();
    const [busy, setBusy] = useState(false);
    const suggestions = parse<{ suggestions: Suggestion[] }>(message.payload)?.suggestions ?? [];
    const remaining = suggestions.filter((s) => !s.thread_id).length;
    const start = async (indexes: number[]) => {
        setBusy(true);
        try {
            await onStart(message.id, indexes);
        } finally {
            setBusy(false);
        }
    };
    return (
        <Paper variant="outlined" sx={{ borderRadius: 3, p: 1.5, my: 1.5 }}>
            <Typography variant="overline" color="text.secondary">{t('conversation.suggested')}</Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, mt: 0.5 }}>
                {suggestions.map((s, i) => {
                    const th = s.thread_id ? threads.get(s.thread_id) : undefined;
                    if (th) return <ThreadCard key={i} thread={th} onOpen={onOpen} />;
                    return (
                        <Box key={i} sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, p: 1.25, borderRadius: 2, bgcolor: 'action.hover' }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography sx={{ fontWeight: 600, fontSize: 14, color: 'text.primary' }}>{s.title}</Typography>
                                <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{s.task}</Typography>
                            </Box>
                            <Button size="small" startIcon={<PlayArrow />} disabled={busy || disabled} onClick={() => void start([i])}>{t('conversation.start')}</Button>
                        </Box>
                    );
                })}
            </Box>
            {remaining > 1 && (
                <Button variant="contained" size="small" sx={{ mt: 1.25 }} disabled={busy || disabled} onClick={() => void start([])}>
                    {t('conversation.startAll', { count: remaining })}
                </Button>
            )}
        </Paper>
    );
}

function UpdateLine({ message, threads, onOpen }: { message: ProjectMessage; threads: Map<string, Thread>; onOpen: (id: string) => void }) {
    const { t } = useTranslation();
    const pl = parse<{ thread_id: string; title: string; kind: string }>(message.payload);
    if (!pl) return null;
    const th = threads.get(pl.thread_id);
    return (
        <Box sx={{ display: 'flex', justifyContent: 'center', my: 1 }}>
            <Chip
                size="small"
                variant="outlined"
                onClick={() => onOpen(pl.thread_id)}
                label={t(`conversation.update.${pl.kind}`, { title: th?.title ?? pl.title })}
                sx={{ color: 'text.secondary', maxWidth: '100%' }}
            />
        </Box>
    );
}

const EXAMPLES = ['conversation.example1', 'conversation.example2', 'conversation.example3'];

interface Props {
    messages: ProjectMessage[];
    threads: Map<string, Thread>;
    busy: boolean;
    disabled: boolean;
    disabledReason?: React.ReactNode;
    onSend: (text: string) => Promise<boolean>;
    onOpenThread: (id: string) => void;
    onStartSuggestions: (messageId: string, indexes: number[]) => Promise<void>;
}

export function ConversationPane({ messages, threads, busy, disabled, disabledReason, onSend, onOpenThread, onStartSuggestions }: Props) {
    const { t } = useTranslation();
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);
    const [seed, setSeed] = useState('');

    useEffect(() => {
        const el = scrollRef.current;
        if (el && stick.current) el.scrollTop = el.scrollHeight;
    }, [messages, busy]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, minWidth: 0 }}>
            <Box
                ref={scrollRef}
                onScroll={() => {
                    const el = scrollRef.current;
                    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                }}
                sx={{ flex: 1, overflowY: 'auto', overscrollBehavior: 'contain' }}
            >
                <Box sx={{ ...COLUMN, pt: 2, pb: 3 }}>
                    {messages.length === 0 && (
                        <Box sx={{ py: { xs: 3, md: 6 } }}>
                            <Typography variant="h3" sx={{ mb: 2 }}>{t('conversation.emptyTitle')}</Typography>
                            {[1, 2, 3].map((i) => (
                                <Box key={i} sx={{ display: 'flex', gap: 1.5, mb: 1.5 }}>
                                    <Box sx={{ width: 22, height: 22, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', fontSize: 12, fontWeight: 700, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, mt: 0.25 }}>{i}</Box>
                                    <Typography color="text.secondary">{t(`conversation.step${i}`)}</Typography>
                                </Box>
                            ))}
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 3, mb: 1 }}>{t('conversation.tryExample')}</Typography>
                            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                                {EXAMPLES.map((k) => (
                                    <Chip key={k} label={t(k)} variant="outlined" onClick={() => setSeed(t(k))} sx={{ height: 'auto', py: 0.75, '& .MuiChip-label': { whiteSpace: 'normal' } }} />
                                ))}
                            </Box>
                        </Box>
                    )}
                    {messages.map((m) => {
                        switch (m.kind) {
                            case 'text':
                                return m.role === 'user' ? (
                                    <Box key={m.id} sx={{ display: 'flex', justifyContent: 'flex-end', my: 2 }}>
                                        <Box sx={{ maxWidth: '85%', bgcolor: 'action.hover', borderRadius: 3, px: 2, py: 1.25, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: 'text.primary', fontSize: 15, lineHeight: 1.6 }}>{m.content}</Box>
                                    </Box>
                                ) : (
                                    <Box key={m.id} sx={{ my: 2 }}><Markdown text={m.content} /></Box>
                                );
                            case 'thread_started': {
                                const ids = parse<{ thread_ids: string[] }>(m.payload)?.thread_ids ?? [];
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', flexDirection: 'column', gap: 1, my: 1.5 }}>
                                        {ids.map((id) => {
                                            const th = threads.get(id);
                                            return th ? <ThreadCard key={id} thread={th} onOpen={onOpenThread} /> : null;
                                        })}
                                    </Box>
                                );
                            }
                            case 'suggestions':
                                return <SuggestionsCard key={m.id} message={m} threads={threads} onStart={onStartSuggestions} onOpen={onOpenThread} disabled={disabled} />;
                            case 'thread_update':
                                return <UpdateLine key={m.id} message={m} threads={threads} onOpen={onOpenThread} />;
                            case 'error':
                                return (
                                    <Alert key={m.id} severity="error" sx={{ my: 1.5, '& .MuiAlert-message': { minWidth: 0, wordBreak: 'break-word' } }}
                                        action={<Button color="inherit" size="small" component={RouterLink} to="/settings">{t('connection.open')}</Button>}>
                                        {m.content}
                                    </Alert>
                                );
                            default:
                                return null;
                        }
                    })}
                    {busy && (
                        <Box sx={{ display: 'flex', gap: 0.75, py: 1.5 }} aria-label={t('chat.thinking')}>
                            {[0, 1, 2].map((i) => (
                                <Box key={i} sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: 'primary.main', animation: 'pulse 1.2s infinite', animationDelay: `${i * 0.15}s`, '@keyframes pulse': { '0%, 80%, 100%': { opacity: 0.25 }, '40%': { opacity: 1 } } }} />
                            ))}
                        </Box>
                    )}
                </Box>
            </Box>
            <Box sx={{ ...COLUMN, pb: 2 }}>
                {disabledReason}
                <Composer
                    autoFocus
                    minRows={2}
                    disabled={disabled}
                    seed={seed}
                    placeholder={t('conversation.placeholder')}
                    onSend={async (text) => {
                        stick.current = true;
                        return onSend(text);
                    }}
                />
            </Box>
        </Box>
    );
}
