import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Chip, Paper, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { ProjectMessage, Thread } from '../../client';
import { Composer } from '../Composer';
import { Markdown } from '../Markdown';
import { DiffIcon, PlayArrow } from '../icons';
import { DiffDialog } from './DiffDialog';
import { StatusIcon, statusColor, ThreadBadges, ThreadSubtitle, toneColor } from './threadStatus';

const COLUMN = { maxWidth: 760, mx: 'auto', width: '100%', px: { xs: 2, md: 3 } } as const;

function parse<T>(payload?: string): T | null {
    if (!payload) return null;
    try {
        return JSON.parse(payload) as T;
    } catch {
        return null;
    }
}

/**
 * A thread as one line in the conversation: its status, title, what it last
 * did and its work. The line is the thread's live state; it changes in place
 * rather than the conversation adding a note. A thread with changes to
 * review offers "Review" right here.
 */
export function ThreadCard({ thread, onOpen }: { thread: Thread; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const [reviewing, setReviewing] = useState(false);
    const canReview = thread.status === 'ready' && !!thread.branch && thread.commits_ahead > 0 && !thread.merged_at;
    return (
        <>
            <Box
                role="button"
                tabIndex={0}
                onClick={() => onOpen(thread.id)}
                onKeyDown={(e) => e.key === 'Enter' && onOpen(thread.id)}
                sx={{
                    display: 'flex', alignItems: 'center', gap: 1, minHeight: 40, px: 1.25, py: 0.5, borderRadius: 1, cursor: 'pointer',
                    border: 1, borderColor: 'divider', bgcolor: 'background.paper', outline: 'none',
                    '&:hover': { borderColor: 'text.disabled' },
                    '&:focus-visible': { boxShadow: (th) => `inset 0 0 0 2px ${th.palette.primary.main}` },
                }}
            >
                <Box component="span" title={t(`thread.status.${thread.status}`)}
                    sx={{ display: 'inline-flex', flexShrink: 0, color: toneColor(statusColor(thread.status)) }}>
                    <StatusIcon status={thread.status} />
                </Box>
                <Typography noWrap sx={{ flex: { xs: 1, sm: '0 1 auto' }, minWidth: 0, maxWidth: { sm: '45%' }, fontSize: '0.875rem', fontWeight: 600, color: 'text.primary' }}>{thread.title}</Typography>
                {/* On a phone the line keeps the title; the thread holds the rest. */}
                <Typography noWrap component="div" sx={{ display: { xs: 'none', sm: 'block' }, flex: 1, minWidth: 0, fontSize: fontSizes.md, color: thread.status === 'waiting' ? toneColor('warning') : 'text.secondary' }}>
                    <ThreadSubtitle thread={thread} />
                </Typography>
                <ThreadBadges thread={thread} />
                {canReview && (
                    <Button size="small" variant="outlined" startIcon={<DiffIcon />} sx={{ flexShrink: 0, py: 0.125 }}
                        onClick={(e) => {
                            e.stopPropagation();
                            setReviewing(true);
                        }}>
                        {t('thread.review')}
                    </Button>
                )}
            </Box>
            {canReview && <DiffDialog thread={thread} open={reviewing} onClose={() => setReviewing(false)} />}
        </>
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
    const { t } = useTranslation('projects');
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
        <Paper variant="outlined" sx={{ borderRadius: 1.5, p: 1.5, my: 1.5 }}>
            <Typography variant="overline" color="text.secondary">{t('conversation.suggested')}</Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, mt: 0.5 }}>
                {suggestions.map((s, i) => {
                    const th = s.thread_id ? threads.get(s.thread_id) : undefined;
                    if (th) return <ThreadCard key={i} thread={th} onOpen={onOpen} />;
                    return (
                        <Box key={i} sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, p: 1.25, borderRadius: 1, bgcolor: 'action.hover' }}>
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography sx={{ fontWeight: 600, fontSize: '0.875rem', color: 'text.primary' }}>{s.title}</Typography>
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
    const { t } = useTranslation('projects');
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
                                    <Box sx={{ width: 22, height: 22, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', fontSize: fontSizes.sm, fontWeight: 700, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, mt: 0.25 }}>{i}</Box>
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
                                        <Box sx={{ maxWidth: '85%', bgcolor: 'action.hover', borderRadius: 1.5, px: 1.75, py: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: 'text.primary', fontSize: '0.875rem', lineHeight: 1.65 }}>{m.content}</Box>
                                    </Box>
                                ) : (
                                    <Box key={m.id} sx={{ my: 2 }}><Markdown text={m.content} /></Box>
                                );
                            case 'thread_started': {
                                const ids = parse<{ thread_ids: string[] }>(m.payload)?.thread_ids ?? [];
                                return (
                                    <Box key={m.id} sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, my: 1.5 }}>
                                        {ids.map((id) => {
                                            const th = threads.get(id);
                                            return th ? <ThreadCard key={id} thread={th} onOpen={onOpenThread} /> : null;
                                        })}
                                    </Box>
                                );
                            }
                            case 'suggestions':
                                return <SuggestionsCard key={m.id} message={m} threads={threads} onStart={onStartSuggestions} onOpen={onOpenThread} disabled={disabled} />;
                            // A thread's change shows on its line above, not as a note.
                            case 'thread_update':
                                return null;
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
